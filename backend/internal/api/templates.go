package api

import (
	"net/http"

	"github.com/ioganvaise83/ironnote/internal/service"
)

type templateExerciseResponse struct {
	ID                     int64    `json:"id"`
	ExerciseID             int64    `json:"exerciseId"`
	ExerciseName           string   `json:"exerciseName"`
	ExerciseArchived       bool     `json:"exerciseArchived"`
	SortOrder              int32    `json:"sortOrder"`
	PlannedSets            *int32   `json:"plannedSets"`
	PlannedReps            *int32   `json:"plannedReps"`
	PlannedWeightKg        *float64 `json:"plannedWeightKg"`
	PlannedDurationSeconds *int32   `json:"plannedDurationSeconds"`
}

type templateResponse struct {
	ID          int64                      `json:"id"`
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	Exercises   []templateExerciseResponse `json:"exercises"`
	CreatedAt   string                     `json:"createdAt"`
	UpdatedAt   string                     `json:"updatedAt"`
}

type templateSummaryResponse struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	ExerciseCount int64  `json:"exerciseCount"`
	PlannedSets   int64  `json:"plannedSets"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

type templateRequest struct {
	Name        string                          `json:"name"`
	Description string                          `json:"description"`
	Exercises   []service.TemplateExerciseInput `json:"exercises"`
}

func (r templateRequest) toInput() service.TemplateInput {
	exercises := r.Exercises
	if exercises == nil {
		exercises = []service.TemplateExerciseInput{}
	}
	return service.TemplateInput{Name: r.Name, Description: r.Description, Exercises: exercises}
}

func templateJSON(t service.Template) templateResponse {
	exercises := make([]templateExerciseResponse, 0, len(t.Exercises))
	for _, e := range t.Exercises {
		exercises = append(exercises, templateExerciseResponse{
			ID: e.ID, ExerciseID: e.ExerciseID, ExerciseName: e.ExerciseName,
			ExerciseArchived: e.ExerciseArchived, SortOrder: e.SortOrder,
			PlannedSets: e.PlannedSets, PlannedReps: e.PlannedReps,
			PlannedWeightKg:        e.PlannedWeightKg.Ptr(),
			PlannedDurationSeconds: e.PlannedDurationSeconds,
		})
	}
	return templateResponse{
		ID: t.ID, Name: t.Name, Description: t.Description, Exercises: exercises,
		CreatedAt: timeJSON(t.CreatedAt), UpdatedAt: timeJSON(t.UpdatedAt),
	}
}

func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	items, err := s.svc.ListTemplates(r.Context(), mustActor(r))
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	out := make([]templateSummaryResponse, 0, len(items))
	for _, t := range items {
		out = append(out, templateSummaryResponse{
			ID: t.ID, Name: t.Name, Description: t.Description,
			ExerciseCount: t.ExerciseCount, PlannedSets: t.PlannedSets,
			CreatedAt: timeJSON(t.CreatedAt), UpdatedAt: timeJSON(t.UpdatedAt),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleGetTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	t, err := s.svc.GetTemplate(r.Context(), mustActor(r), id)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, templateJSON(t))
}

func (s *Server) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	var req templateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	t, err := s.svc.CreateTemplate(r.Context(), mustActor(r), req.toInput())
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, templateJSON(t))
}

func (s *Server) handleUpdateTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	var req templateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	t, err := s.svc.UpdateTemplate(r.Context(), mustActor(r), id, req.toInput())
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, templateJSON(t))
}

func (s *Server) handleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	if err := s.svc.DeleteTemplate(r.Context(), mustActor(r), id); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleStartFromTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	session, err := s.svc.StartSession(r.Context(), mustActor(r), service.StartSessionInput{TemplateID: &id})
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, sessionJSON(session))
}
