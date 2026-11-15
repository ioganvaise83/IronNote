package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ioganvaise83/ironnote/internal/apperror"

	"github.com/ioganvaise83/ironnote/internal/service"
)

type exerciseGroupResponse struct {
	MuscleGroupID   int64  `json:"muscleGroupId"`
	MuscleGroupName string `json:"muscleGroupName"`
	IsPrimary       bool   `json:"isPrimary"`
}

type exerciseResponse struct {
	ID           int64                   `json:"id"`
	Name         string                  `json:"name"`
	Description  string                  `json:"description"`
	Equipment    string                  `json:"equipment"`
	IsArchived   bool                    `json:"isArchived"`
	MuscleGroups []exerciseGroupResponse `json:"muscleGroups"`
	CreatedAt    string                  `json:"createdAt"`
	UpdatedAt    string                  `json:"updatedAt"`
}

func exerciseJSON(e service.Exercise) exerciseResponse {
	groups := make([]exerciseGroupResponse, 0, len(e.MuscleGroups))
	for _, g := range e.MuscleGroups {
		groups = append(groups, exerciseGroupResponse{
			MuscleGroupID: g.MuscleGroupID, MuscleGroupName: g.MuscleGroupName, IsPrimary: g.IsPrimary,
		})
	}
	return exerciseResponse{
		ID: e.ID, Name: e.Name, Description: e.Description, Equipment: e.Equipment,
		IsArchived: e.IsArchived, MuscleGroups: groups,
		CreatedAt: timeJSON(e.CreatedAt), UpdatedAt: timeJSON(e.UpdatedAt),
	}
}

type exerciseRequest struct {
	Name         string                     `json:"name"`
	Description  string                     `json:"description"`
	Equipment    string                     `json:"equipment"`
	MuscleGroups []service.ExerciseGroupRef `json:"muscleGroups"`
}

func (r exerciseRequest) toInput() service.ExerciseInput {
	groups := r.MuscleGroups
	if groups == nil {
		groups = []service.ExerciseGroupRef{}
	}
	return service.ExerciseInput{
		Name: r.Name, Description: r.Description, Equipment: r.Equipment, MuscleGroups: groups,
	}
}

func (s *Server) handleListExercises(w http.ResponseWriter, r *http.Request) {
	filter := service.ExerciseFilter{
		Search:          r.URL.Query().Get("search"),
		IncludeArchived: r.URL.Query().Get("includeArchived") == "true",
	}
	if raw := r.URL.Query().Get("muscleGroupId"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, r, s.log, validationErr("muscleGroupId"))
			return
		}
		filter.MuscleGroupID = &id
	}
	items, err := s.svc.ListExercises(r.Context(), filter)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	out := make([]exerciseResponse, 0, len(items))
	for _, e := range items {
		out = append(out, exerciseJSON(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleGetExercise(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	e, err := s.svc.GetExercise(r.Context(), id)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, exerciseJSON(e))
}

func (s *Server) handleCreateExercise(w http.ResponseWriter, r *http.Request) {
	var req exerciseRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	e, err := s.svc.CreateExercise(r.Context(), req.toInput())
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, exerciseJSON(e))
}

func (s *Server) handleUpdateExercise(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	var req exerciseRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	e, err := s.svc.UpdateExercise(r.Context(), id, req.toInput())
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, exerciseJSON(e))
}

func (s *Server) handleDeleteExercise(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	if err := s.svc.DeleteExercise(r.Context(), id); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleArchiveExercise(w http.ResponseWriter, r *http.Request) {
	s.setArchived(w, r, true)
}

func (s *Server) handleUnarchiveExercise(w http.ResponseWriter, r *http.Request) {
	s.setArchived(w, r, false)
}

func (s *Server) setArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	e, err := s.svc.SetExerciseArchived(r.Context(), id, archived)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, exerciseJSON(e))
}

func validationErr(field string) error {
	return apperror.Validation("Некорректное значение параметра запроса",
		map[string]string{strings.TrimSpace(field): "некорректно"})
}
