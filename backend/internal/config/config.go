// Package config reads the service configuration from the environment.
//
// Required values have no secret default: a missing database URL or bootstrap
// password stops the service instead of guessing a connection or starting with a
// credential nobody chose. Security-relevant settings (TLS, trusted proxies) are
// validated for coherence here rather than failing open later.
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
	// AdminUsername and AdminPassword seed the first administrator. The password
	// is only used when the users table is empty; it is never reapplied.
	AdminUsername string
	AdminPassword string
	// RequireStrongAdminPassword refuses to start on a placeholder bootstrap
	// password. Default true: a public host must not come up with "change-me".
	RequireStrongAdminPassword bool
	// SecureCookies marks session cookies Secure; required once TLS is terminated
	// in front of the API.
	SecureCookies bool
	// PublicOrigin is the canonical external origin (https://host[:port]) used to
	// validate the Origin header on state-changing requests.
	PublicOrigin string
	// TrustedProxies are CIDRs whose X-Forwarded-For may be believed. Empty means
	// "trust nobody", which is correct when the API is not behind a proxy.
	TrustedProxies []string
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

	cfg.AdminUsername = env("IRONNOTE_ADMIN_USERNAME", "admin")
	cfg.AdminPassword = os.Getenv("IRONNOTE_ADMIN_PASSWORD")
	if cfg.AdminPassword == "" {
		return Config{}, fmt.Errorf("IRONNOTE_ADMIN_PASSWORD is required (see .env.example)")
	}
	cfg.RequireStrongAdminPassword = envBool("IRONNOTE_REQUIRE_STRONG_ADMIN_PASSWORD", true)
	cfg.SecureCookies = envBool("IRONNOTE_COOKIE_SECURE", false)
	cfg.PublicOrigin = strings.TrimSpace(os.Getenv("IRONNOTE_PUBLIC_ORIGIN"))
	cfg.TrustedProxies = splitList(os.Getenv("IRONNOTE_TRUSTED_PROXIES"))

	// Refuse an incoherent transport configuration rather than silently issuing
	// cookies that will not be sent, or trusting headers from anyone.
	if cfg.SecureCookies && strings.HasPrefix(cfg.PublicOrigin, "http://") {
		return Config{}, fmt.Errorf("IRONNOTE_COOKIE_SECURE=true requires an https IRONNOTE_PUBLIC_ORIGIN")
	}
	if cfg.PublicOrigin != "" && !strings.HasPrefix(cfg.PublicOrigin, "https://") &&
		!strings.HasPrefix(cfg.PublicOrigin, "http://") {
		return Config{}, fmt.Errorf("IRONNOTE_PUBLIC_ORIGIN must start with http:// or https://")
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

func envBool(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch raw {
	case "":
		return fallback
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func splitList(raw string) []string {
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

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
