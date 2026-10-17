package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"tradepulse/examples/market"
	"tradepulse/examples/status"
	"tradepulse/internal/access"
	"tradepulse/internal/api"
	"tradepulse/internal/auth"
	"tradepulse/internal/config"
	"tradepulse/internal/connector"
	"tradepulse/internal/delivery"
	"tradepulse/internal/domain/audit"
	"tradepulse/internal/domain/feed"
	"tradepulse/internal/domain/invite"
	"tradepulse/internal/domain/sourcedef"
	"tradepulse/internal/domain/user"
	"tradepulse/internal/domain/event"
	"tradepulse/internal/infrastructure/eventbus"
	"tradepulse/internal/ingestion"
	"tradepulse/internal/personalization"
	"tradepulse/internal/processing"
	"tradepulse/internal/safefetch"
	"tradepulse/internal/source"
	"tradepulse/internal/storage/memory"
	"tradepulse/internal/storage/postgres"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" { // used by Docker HEALTHCHECK (distroless has no curl)
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	if cfg.Production() && cfg.DocsEnabled {
		log.Warn("API docs are enabled in production; set DOCS_ENABLED=false")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Infrastructure. Storage comes from DATABASE_URL (any managed Postgres); without it
	// (development only) data lives in memory and is lost on restart.
	bus := eventbus.NewMemory(1024, log)
	var (
		users   user.Repository
		feeds   feed.Repository
		invites invite.Repository
		audits  audit.Repository
		sources sourcedef.Repository
		runs    sourcedef.RunRepository
		items   sourcedef.ItemStore
		ready   func(context.Context) error
	)
	if cfg.DatabaseURL != "" {
		mig, err := postgres.Open(ctx, cfg.DatabaseMigrationURL, 2, cfg.DBSimpleProtocol)
		if err != nil {
			return fmt.Errorf("database (migrations): %w", err)
		}
		err = postgres.Migrate(ctx, mig, log)
		mig.Close()
		if err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
		pool, err := postgres.Open(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBSimpleProtocol)
		if err != nil {
			return fmt.Errorf("database: %w", err)
		}
		defer pool.Close()
		users, feeds = postgres.NewUserRepo(pool), postgres.NewFeedRepo(pool)
		invites, audits = postgres.NewInviteRepo(pool), postgres.NewAuditRepo(pool)
		sources, runs, items = postgres.NewSourceRepo(pool), postgres.NewRunRepo(pool), postgres.NewItemStore(pool)
		ready = pool.Ping
		go postgres.RunRetention(ctx, pool, log, 1000, 365*24*time.Hour)
		log.Info("storage: postgres")
	} else {
		users, feeds = memory.NewUserRepo(), memory.NewFeedRepo(200)
		invites, audits = memory.NewInviteRepo(), memory.NewAuditRepo(10000)
		sources, runs, items = memory.NewSourceRepo(), memory.NewRunRepo(), memory.NewItemStore()
		log.Warn("storage: in-memory (development only; data is lost on restart)")
	}
	hub := delivery.NewHub(log, cfg.AllowedOrigins, cfg.MaxWSPerUser)

	if cfg.BootstrapAdminEmail != "" {
		created, err := access.EnsureAdmin(ctx, users, cfg.BootstrapAdminEmail, cfg.BootstrapAdminPassword)
		if err != nil {
			return fmt.Errorf("bootstrap admin: %w", err)
		}
		log.Info("bootstrap admin checked", "created", created) // never log credentials
	}

	// Application: Events -> Matching -> Delivery.
	reg := event.NewRegistry()
	engine := personalization.NewEngine(log, feeds, hub, reg)
	if err := engine.Load(ctx, users); err != nil {
		return err
	}
	bus.Subscribe("personalization", engine.Handle)

	// Sources: an example domain chosen by SIM_DEMO (none in production).
	var listeners []source.Listener
	procs := map[string]processing.Processor{}
	switch cfg.SimDemo {
	case "market":
		if err := market.RegisterTypes(reg); err != nil {
			return err
		}
		listeners = append(listeners, market.NewSimulator(cfg.SimSymbols, cfg.SimInterval))
		procs[market.SourceName] = market.NewNSETick(market.SourceName)
		detector := market.NewUnusualActivityDetector(bus, 5*time.Minute, 10*time.Minute)
		bus.Subscribe("signals", detector.Handle)
	case "status":
		if err := status.RegisterTypes(reg); err != nil {
			return err
		}
		listeners = append(listeners, status.NewSimulator(cfg.SimInterval))
		procs[status.SourceName] = status.NewProcessor()
	}

	// Outbound fetching: the ONLY network path for sources is the SSRF-safe fetcher.
	fetcher := safefetch.New(safefetch.Options{UserAgent: cfg.FetchUserAgent})
	conns := connector.NewRegistry(connector.NewRSS(fetcher), connector.NewWebMonitor(fetcher, connector.NewRobots(fetcher, time.Hour, nil)))

	jwt, err := auth.NewJWT(cfg.JWTSecret, "tradepulse", cfg.JWTTTL)
	if err != nil {
		return err
	}
	tickets := auth.NewTicketStore(30 * time.Second)
	go tickets.Run(ctx)

	apiSrv, err := api.New(api.Deps{
		Cfg: cfg, Log: log, Users: users, Feeds: feeds, Invites: invites, Audit: audits, JWT: jwt,
		Tickets: tickets, Personalizer: engine, Hub: hub, Registry: reg, Ready: ready, Sources: sources, Runs: runs, Bus: bus, Connectors: conns,
	})
	if err != nil {
		return err
	}
	go apiSrv.Run(ctx)

	var wg sync.WaitGroup
	sched := ingestion.NewScheduler(log, sources, runs, items, conns, bus, reg, ingestion.SchedulerConfig{
		Bounds: sourcedef.Bounds{Min: cfg.SourceMinInterval, Max: cfg.SourceMaxInterval},
	})
	wg.Add(1)
	go func() {
		defer wg.Done()
		sched.Run(ctx)
	}()
	if len(listeners) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ingestion.Run(ctx, log, bus, reg, listeners, procs)
		}()
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           apiSrv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr, "env", cfg.Env, "demo", cfg.SimDemo)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errCh:
		return err
	}

	sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(sctx)
	hub.Shutdown() // Shutdown does not track hijacked WebSocket connections
	wg.Wait()
	bus.Close()
	return nil
}

func healthcheck() int {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Get("http://" + addr + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
