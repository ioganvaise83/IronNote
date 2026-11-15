package api

import (
	"github.com/go-chi/chi/v5"
)

// adminAndTrainingRoutes registers the authenticated training surface.
//
// Kept in its own function so the route table in Router reads as a policy
// statement ("public", "authenticated", "admin") rather than an unrolled list.
func (s *Server) adminAndTrainingRoutes(r chi.Router) {
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
}
