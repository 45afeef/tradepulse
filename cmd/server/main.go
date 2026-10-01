package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"tradepulse/internal/api"
	"tradepulse/internal/auth"
	"tradepulse/internal/config"
	"tradepulse/internal/delivery"
	"tradepulse/internal/domain/rule"
	"tradepulse/internal/infrastructure/eventbus"
	"tradepulse/internal/ingestion"
	"tradepulse/internal/personalization"
	"tradepulse/internal/processing"
	"tradepulse/internal/source"
	"tradepulse/internal/source/simulated"
	"tradepulse/internal/storage/memory"
)

func main() {
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Infrastructure (swap these for Postgres/Redis/NATS implementations later).
	bus := eventbus.NewMemory(1024, log)
	users := memory.NewUserRepo()
	feeds := memory.NewFeedRepo(200)
	hub := delivery.NewHub(log, cfg.AllowedOrigins, cfg.MaxWSPerUser)

	// Application: Events -> Signals -> Personalization -> Delivery.
	engine := personalization.NewEngine(log, feeds, hub, rule.Default())
	if err := engine.Load(ctx, users); err != nil {
		return err
	}
	bus.Subscribe("personalization", engine.Handle)
	detector := processing.NewUnusualActivityDetector(bus, 5*time.Minute, 10*time.Minute)
	bus.Subscribe("signals", detector.Handle)

	jwt, err := auth.NewJWT(cfg.JWTSecret, "tradepulse", cfg.JWTTTL)
	if err != nil {
		return err
	}
	tickets := auth.NewTicketStore(30 * time.Second)
	go tickets.Run(ctx)

	apiSrv, err := api.New(api.Deps{
		Cfg: cfg, Log: log, Users: users, Feeds: feeds, JWT: jwt,
		Tickets: tickets, Personalizer: engine, Hub: hub,
	})
	if err != nil {
		return err
	}
	go apiSrv.Run(ctx)

	var wg sync.WaitGroup
	if cfg.SimEnabled {
		listeners := []source.Listener{simulated.New(cfg.SimSymbols, cfg.SimInterval)}
		procs := map[string]processing.Processor{simulated.Name: processing.NewNSETick(simulated.Name)}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ingestion.Run(ctx, log, bus, listeners, procs)
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
		log.Info("listening", "addr", cfg.Addr, "env", cfg.Env)
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
