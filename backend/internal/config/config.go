// Package config reads the service configuration from the environment.
//
// Only two variables are required and neither has a secret default: if the
// database URL is missing the service refuses to start rather than guessing a
// connection that might point at the wrong database.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// HTTPAddr is the listen address, e.g. ":8080".
	HTTPAddr string
	// DatabaseURL is a libpq/pgx connection string.
	DatabaseURL string
	// LogLevel is one of debug, info, warn, error.
	LogLevel string
	// Timezone is the IANA zone used to bucket sessions by month. History and the
	// API always store UTC instants; this only affects reporting boundaries.
	Timezone *time.Location
	// ShutdownTimeout bounds graceful shutdown.
	ShutdownTimeout time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        env("HTTP_ADDR", ":8080"),
		DatabaseURL:     strings.TrimSpace(os.Getenv("DATABASE_URL")),
		LogLevel:        strings.ToLower(env("LOG_LEVEL", "info")),
		ShutdownTimeout: 10 * time.Second,
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required (see .env.example)")
	}

	tzName := env("APP_TIMEZONE", "UTC")
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return Config{}, fmt.Errorf("APP_TIMEZONE %q is not a valid IANA timezone: %w", tzName, err)
	}
	cfg.Timezone = loc

	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("LOG_LEVEL %q must be one of debug, info, warn, error", cfg.LogLevel)
	}

	if d := os.Getenv("SHUTDOWN_TIMEOUT_SECONDS"); d != "" {
		secs, err := strconv.Atoi(d)
		if err != nil || secs <= 0 {
			return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT_SECONDS must be a positive integer")
		}
		cfg.ShutdownTimeout = time.Duration(secs) * time.Second
	}

	return cfg, nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
