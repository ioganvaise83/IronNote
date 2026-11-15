// Package api wires HTTP to the service layer.
//
// Handlers decode, validate shape, call one service method, and encode. They do
// not run SQL and do not compute statistics: that keeps HTTP concerns (status
// codes, JSON casing) from leaking into the rules.
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ioganvaise83/ironnote/internal/apperror"
	"github.com/ioganvaise83/ironnote/internal/auth"
	"github.com/ioganvaise83/ironnote/internal/database"
	"github.com/ioganvaise83/ironnote/internal/service"
)

type Server struct {
	svc     *service.Service
	auth    *auth.Service
	log     *slog.Logger
	pool    *pgxpool.Pool
	version string
	// secureCookies marks the session cookie Secure; enabled when TLS is in front.
	secureCookies bool
	// publicOrigin is the canonical https:// origin, used to validate Origin on
	// state-changing requests.
	publicOrigin   string
	trustedProxies []*net.IPNet
	startedAt      time.Time
	corsOrigins    []string
	// Rate limiters. Login is deliberately much stricter than general traffic.
	loginLimiter  *rateLimiter
	globalLimiter *rateLimiter
}

type Options struct {
	Service        *service.Service
	Auth           *auth.Service
	Logger         *slog.Logger
	Pool           *pgxpool.Pool
	Version        string
	CORSOrigins    []string
	SecureCookies  bool
	PublicOrigin   string
	TrustedProxies []string
}

func NewServer(opts Options) *Server {
	return &Server{
		svc:            opts.Service,
		auth:           opts.Auth,
		log:            opts.Logger,
		pool:           opts.Pool,
		version:        opts.Version,
		startedAt:      time.Now(),
		corsOrigins:    opts.CORSOrigins,
		secureCookies:  opts.SecureCookies,
		publicOrigin:   strings.TrimRight(opts.PublicOrigin, "/"),
		trustedProxies: parseCIDRs(opts.TrustedProxies),
		// Login allows a burst of 20 then 10/min. Deliberately larger than the
		// per-account lockout threshold (5): a family shares one public IP, so a
		// tight per-IP budget would lock the household out before the real defence
		// — the per-account lockout — ever engaged.
		loginLimiter:  newRateLimiter(10.0/60.0, 20),
		globalLimiter: newRateLimiter(2.0, 120),
	}
}

// parseCIDRs accepts plain IPs too ("127.0.0.1" means 127.0.0.1/32), because an
// operator will write the former.
func parseCIDRs(values []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(values))
	for _, raw := range values {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if !strings.Contains(v, "/") {
			if ip := net.ParseIP(v); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				_, network, err := net.ParseCIDR(fmt.Sprintf("%s/%d", ip.String(), bits))
				if err == nil {
					out = append(out, network)
				}
			}
			continue
		}
		if _, network, err := net.ParseCIDR(v); err == nil {
			out = append(out, network)
		}
	}
	return out
}

// Router builds the full API tree with its middleware stack.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// RealIP is deliberately NOT used: it trusts X-Forwarded-For from anyone, which
	// would make per-IP rate limiting trivially bypassable. clientIP does it
	// correctly, only for configured proxies.
	r.Use(middleware.RequestID)
	r.Use(s.securityHeaders)
	r.Use(s.logRequests)
	r.Use(middleware.Recoverer)
	r.Use(withTimeout)
	r.Use(s.authenticate)

	if len(s.corsOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   s.corsOrigins,
			AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete, http.MethodOptions},
			AllowedHeaders:   []string{"Content-Type", "Accept"},
			AllowCredentials: false,
			MaxAge:           300,
		}))
	}

	r.Get("/healthz", s.handleHealth)
	r.Get("/readyz", s.handleReady)

	r.Route("/api", func(r chi.Router) {
		// Public: reachable without a session, heavily throttled.
		r.Group(func(r chi.Router) {
			r.Use(s.rateLimit(s.loginLimiter, "Слишком много попыток входа. Попробуйте через минуту."))
			r.Post("/auth/login", s.handleLogin)
		})

		// Everything below needs a session.
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)

			r.Get("/meta", s.handleMeta)
			r.Get("/auth/me", s.handleMe)

			// Session-mutating endpoints are CSRF-protected like every other write:
			// a cross-site form must not be able to log the user out or push a
			// password change, even though both require the session cookie.
			r.Group(func(r chi.Router) {
				r.Use(s.requireCSRF)
				r.Post("/auth/logout", s.handleLogout)
				r.Post("/auth/password", s.handleChangePassword)
			})

			// Writes additionally need the CSRF token.
			r.Group(func(r chi.Router) {
				r.Use(s.requireCSRF)
				r.Use(s.rateLimit(s.globalLimiter, "Слишком много запросов"))
				s.adminAndTrainingRoutes(r)
			})
		})

		// Admin-only surface. requireAdmin is the route-level guard; the service
		// layer checks the role again.
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Use(s.requireAdmin)
			r.Use(s.requireCSRF)
			r.Use(s.rateLimit(s.globalLimiter, "Слишком много запросов"))

			r.Route("/users", func(r chi.Router) {
				r.Get("/", s.handleListUsers)
				r.Post("/", s.handleCreateUser)
				r.Patch("/{id}", s.handleUpdateUser)
				r.Delete("/{id}", s.handleDeleteUser)
				r.Post("/{id}/password", s.handleResetUserPassword)
			})

			// Catalogue writes.
			r.Post("/muscle-groups", s.handleCreateMuscleGroup)
			r.Patch("/muscle-groups/{id}", s.handleUpdateMuscleGroup)
			r.Delete("/muscle-groups/{id}", s.handleDeleteMuscleGroup)
			r.Post("/exercises", s.handleCreateExercise)
			r.Patch("/exercises/{id}", s.handleUpdateExercise)
			r.Delete("/exercises/{id}", s.handleDeleteExercise)
			r.Post("/exercises/{id}/archive", s.handleArchiveExercise)
			r.Post("/exercises/{id}/unarchive", s.handleUnarchiveExercise)
		})

		// Catalogue reads stay open to every signed-in user.
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/muscle-groups", s.handleListMuscleGroups)
			r.Get("/muscle-groups/{id}", s.handleGetMuscleGroup)
			r.Get("/exercises", s.handleListExercises)
			r.Get("/exercises/{id}", s.handleGetExercise)
		})

		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, r, s.log, apperror.New(apperror.CodeNotFound, "Маршрут не найден"))
		})
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, s.log, apperror.New(apperror.CodeNotFound, "Маршрут не найден"))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, s.log, apperror.New(apperror.CodeValidation, "Метод не поддерживается"))
	})

	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := database.Health(r.Context(), s.pool); err != nil {
		s.log.Error("readiness check failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":   s.version,
		"startedAt": s.startedAt,
		"timezone":  s.svc.Timezone().String(),
	})
}

// --- request helpers -------------------------------------------------------

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return apperror.Validation("Некорректный JSON в запросе", map[string]string{"body": err.Error()})
	}
	return nil
}

func pathID(r *http.Request, name string) (int64, error) {
	raw := chi.URLParam(r, name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, apperror.Validation("Некорректный идентификатор", map[string]string{name: raw})
	}
	return id, nil
}
