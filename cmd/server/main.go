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
	"tradepulse/internal/delivery"
	"tradepulse/internal/domain/event"
	"tradepulse/internal/infrastructure/eventbus"
	"tradepulse/internal/ingestion"
	"tradepulse/internal/personalization"
	"tradepulse/internal/processing"
	"tradepulse/internal/source"
	"tradepulse/internal/storage/memory"
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

	// Infrastructure (swap for Postgres/Redis/NATS implementations later).
	bus := eventbus.NewMemory(1024, log)
	users := memory.NewUserRepo()
	feeds := memory.NewFeedRepo(200)
	invites := memory.NewInviteRepo()
	audits := memory.NewAuditRepo(10000)
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

	jwt, err := auth.NewJWT(cfg.JWTSecret, "tradepulse", cfg.JWTTTL)
	if err != nil {
		return err
	}
	tickets := auth.NewTicketStore(30 * time.Second)
	go tickets.Run(ctx)

	apiSrv, err := api.New(api.Deps{
		Cfg: cfg, Log: log, Users: users, Feeds: feeds, Invites: invites, Audit: audits, JWT: jwt,
		Tickets: tickets, Personalizer: engine, Hub: hub, Registry: reg,
	})
	if err != nil {
		return err
	}
	go apiSrv.Run(ctx)

	var wg sync.WaitGroup
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
