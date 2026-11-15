// Command api is the IronNote HTTP backend.
//
// Startup order matters and is deliberate:
//  1. load config (fail loudly on a missing DATABASE_URL),
//  2. connect and ping the database (fail loudly, not on first request),
//  3. apply embedded migrations (idempotent, advisory-locked),
//  4. serve, and shut down gracefully on SIGINT/SIGTERM.
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
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ioganvaise83/ironnote/internal/api"
	"github.com/ioganvaise83/ironnote/internal/auth"
	"github.com/ioganvaise83/ironnote/internal/config"
	"github.com/ioganvaise83/ironnote/internal/database"
	"github.com/ioganvaise83/ironnote/internal/service"
)

// version is overridden at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	// `api -healthcheck` lets a container health check probe the running server
	// without curl or wget in the runtime image.
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		os.Exit(healthcheck())
	}

	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)
	logger.Info("starting ironnote api", "version", version, "addr", cfg.HTTPAddr)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Refuse an unsafe bootstrap password before touching the database, so a
	// misconfigured public deployment never comes up at all.
	if cfg.RequireStrongAdminPassword && auth.IsWeakBootstrapPassword(cfg.AdminPassword) {
		return fmt.Errorf(
			"IRONNOTE_ADMIN_PASSWORD is too weak for a public deployment: use at least %d characters "+
				"with letters and digits, and not a placeholder. Set "+
				"IRONNOTE_REQUIRE_STRONG_ADMIN_PASSWORD=false only on a trusted local machine",
			auth.MinPasswordLength)
	}

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	authSvc := auth.NewService(pool, logger, auth.DefaultPolicy())

	// The ownership migration backfills existing history to the bootstrap admin,
	// so that account must exist between 0003 and 0004.
	migrator := database.Migrator{
		Logger: logger,
		AfterMigration: map[string]func(context.Context, *pgxpool.Pool) error{
			"0003_auth": func(hookCtx context.Context, hookPool *pgxpool.Pool) error {
				created, err := auth.NewService(hookPool, logger, auth.DefaultPolicy()).
					BootstrapAdmin(hookCtx, cfg.AdminUsername, cfg.AdminPassword)
				if err != nil {
					return err
				}
				if created {
					logger.Warn("bootstrap administrator created; change this password after first login",
						"username", cfg.AdminUsername)
				}
				return nil
			},
		},
	}
	if err := database.MigrateWithHooks(ctx, pool, migrator); err != nil {
		return err
	}
	// An earlier run may have applied 0003 already, so the hook would not fire;
	// ensure the account exists either way (the call is idempotent).
	if created, err := authSvc.BootstrapAdmin(ctx, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		return fmt.Errorf("bootstrap admin: %w", err)
	} else if created {
		logger.Warn("bootstrap administrator created; change this password after first login",
			"username", cfg.AdminUsername)
	}

	svc := service.New(pool, logger, cfg.Timezone)
	server := api.NewServer(api.Options{
		Service:        svc,
		Auth:           authSvc,
		Logger:         logger,
		Pool:           pool,
		Version:        version,
		CORSOrigins:    splitCORS(os.Getenv("CORS_ALLOWED_ORIGINS")),
		SecureCookies:  cfg.SecureCookies,
		PublicOrigin:   cfg.PublicOrigin,
		TrustedProxies: cfg.TrustedProxies,
	})

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server.Router(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("stopped cleanly")
	return nil
}

// healthcheck asks the local server whether it is alive. Exit 0 means healthy.
func healthcheck() int {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + addr + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// newLogger emits human-readable text in development and JSON elsewhere, so
// `docker compose logs` stays consumable while a terminal stays readable.
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	if os.Getenv("LOG_FORMAT") == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

func splitCORS(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
