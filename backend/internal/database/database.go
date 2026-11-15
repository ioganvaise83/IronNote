// Package database owns the connection pool and the migration runner.
package database

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ioganvaise83/ironnote/migrations"
)

// Connect opens a pool and verifies it with a ping, so a bad DATABASE_URL fails
// at startup instead of on the first user request.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	// A single-user app: a small pool is plenty and leaves connections for psql.
	cfg.MaxConns = 8
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 15 * time.Minute
	cfg.ConnConfig.RuntimeParams["application_name"] = "ironnote-api"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrator hooks let the caller prepare data that a later migration depends on.
//
// The ownership migration attributes existing history to an admin account, so the
// admin must exist first. Expressing that inside SQL would mean duplicating the
// Argon2 hashing in PL/pgSQL; a typed hook keeps one implementation of it.
type Migrator struct {
	Logger *slog.Logger
	// AfterMigration runs inside the same run, after the named migration is
	// committed. Errors abort the whole run and leave later migrations unapplied.
	AfterMigration map[string]func(context.Context, *pgxpool.Pool) error
}

// Migrate applies every embedded migration exactly once, inside a transaction per
// file, and records the version in schema_migrations. It is safe to run on every
// boot and concurrent starts are serialised with an advisory lock.
func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	return MigrateWithHooks(ctx, pool, Migrator{Logger: logger})
}

func MigrateWithHooks(ctx context.Context, pool *pgxpool.Pool, m Migrator) error {
	logger := m.Logger
	if logger == nil {
		logger = slog.Default()
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	const lockKey = 7_169_001 // arbitrary, stable: "ironnote migrations"
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, `SELECT pg_advisory_unlock($1)`, lockKey)
	}()

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     TEXT PRIMARY KEY,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[string]bool{}
	rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("read applied migrations: %w", err)
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return fmt.Errorf("scan applied migration: %w", err)
		}
		applied[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate applied migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		version := strings.TrimSuffix(name, ".sql")
		if applied[version] {
			continue
		}
		body, err := migrations.FS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
		logger.Info("migration applied", "version", version)

		if hook, ok := m.AfterMigration[version]; ok {
			if err := hook(ctx, pool); err != nil {
				return fmt.Errorf("post-migration hook for %s: %w", version, err)
			}
			logger.Info("post-migration hook completed", "version", version)
		}
	}
	return nil
}

// Health runs a cheap query to prove the API can reach its database.
func Health(ctx context.Context, pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var one int
	return pool.QueryRow(ctx, `SELECT 1`).Scan(&one)
}
