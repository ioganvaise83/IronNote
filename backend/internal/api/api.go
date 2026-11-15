// Package api wires HTTP to the service layer.
//
// Handlers decode, validate shape, call one service method, and encode. They do
// not run SQL and do not compute statistics: that keeps HTTP concerns (status
// codes, JSON casing) from leaking into the rules.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ioganvaise83/ironnote/internal/apperror"
	"github.com/ioganvaise83/ironnote/internal/database"
	"github.com/ioganvaise83/ironnote/internal/service"
)

type Server struct {
	svc         *service.Service
	log         *slog.Logger
	pool        *pgxpool.Pool
	version     string
	startedAt   time.Time
	corsOrigins []string
}

type Options struct {
	Service     *service.Service
	Logger      *slog.Logger
	Pool        *pgxpool.Pool
	Version     string
	CORSOrigins []string
}

func NewServer(opts Options) *Server {
	return &Server{
		svc:         opts.Service,
		log:         opts.Logger,
		pool:        opts.Pool,
		version:     opts.Version,
		startedAt:   time.Now(),
		corsOrigins: opts.CORSOrigins,
	}
}

// Router builds the full API tree with its middleware stack.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(s.logRequests)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

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
		r.Get("/meta", s.handleMeta)

		r.Route("/muscle-groups", func(r chi.Router) {
			r.Get("/", s.handleListMuscleGroups)
			r.Post("/", s.handleCreateMuscleGroup)
			r.Get("/{id}", s.handleGetMuscleGroup)
			r.Patch("/{id}", s.handleUpdateMuscleGroup)
			r.Delete("/{id}", s.handleDeleteMuscleGroup)
		})

		r.Route("/exercises", func(r chi.Router) {
			r.Get("/", s.handleListExercises)
			r.Post("/", s.handleCreateExercise)
			r.Get("/{id}", s.handleGetExercise)
			r.Patch("/{id}", s.handleUpdateExercise)
			r.Delete("/{id}", s.handleDeleteExercise)
			r.Post("/{id}/archive", s.handleArchiveExercise)
			r.Post("/{id}/unarchive", s.handleUnarchiveExercise)
		})

		r.Route("/templates", func(r chi.Router) {
			r.Get("/", s.handleListTemplates)
			r.Post("/", s.handleCreateTemplate)
			r.Get("/{id}", s.handleGetTemplate)
			r.Put("/{id}", s.handleUpdateTemplate)
			r.Delete("/{id}", s.handleDeleteTemplate)
			r.Post("/{id}/start", s.handleStartFromTemplate)
		})

		r.Route("/sessions", func(r chi.Router) {
			r.Get("/", s.handleListSessions)
			r.Post("/", s.handleStartSession)
			r.Get("/active", s.handleActiveSession)
			r.Get("/{id}", s.handleGetSession)
			r.Patch("/{id}", s.handleUpdateSession)
			r.Delete("/{id}", s.handleDeleteSession)
			r.Post("/{id}/finish", s.handleFinishSession)
			r.Post("/{id}/reopen", s.handleReopenSession)
			r.Post("/{id}/exercises", s.handleAddSessionExercise)
			r.Patch("/{id}/exercises/{exerciseId}", s.handleMoveSessionExercise)
			r.Delete("/{id}/exercises/{exerciseId}", s.handleDeleteSessionExercise)
			r.Patch("/{id}/exercises/{exerciseId}/notes", s.handleSessionExerciseNotes)
			r.Post("/{id}/exercises/{exerciseId}/sets", s.handleAddSet)
			r.Patch("/{id}/exercises/{exerciseId}/sets/{setId}", s.handleUpdateSet)
			r.Delete("/{id}/exercises/{exerciseId}/sets/{setId}", s.handleDeleteSet)
		})

		r.Route("/stats", func(r chi.Router) {
			r.Get("/summary", s.handleStatsSummary)
			r.Get("/exercises", s.handleExerciseBests)
			r.Get("/exercises/{id}", s.handleExerciseStats)
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
