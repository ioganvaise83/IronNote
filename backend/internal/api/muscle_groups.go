package api

import (
	"net/http"

	"github.com/ioganvaise83/ironnote/internal/service"
)

type muscleGroupResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

func muscleGroupJSON(g service.MuscleGroup) muscleGroupResponse {
	return muscleGroupResponse{
		ID: g.ID, Name: g.Name, Description: g.Description,
		CreatedAt: timeJSON(g.CreatedAt), UpdatedAt: timeJSON(g.UpdatedAt),
	}
}

type muscleGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Server) handleListMuscleGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.svc.ListMuscleGroups(r.Context())
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	out := make([]muscleGroupResponse, 0, len(groups))
	for _, g := range groups {
		out = append(out, muscleGroupJSON(g))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleGetMuscleGroup(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	g, err := s.svc.GetMuscleGroup(r.Context(), id)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, muscleGroupJSON(g))
}

func (s *Server) handleCreateMuscleGroup(w http.ResponseWriter, r *http.Request) {
	var req muscleGroupRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	g, err := s.svc.CreateMuscleGroup(r.Context(), mustActor(r), service.MuscleGroupInput{
		Name: req.Name, Description: req.Description,
	})
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, muscleGroupJSON(g))
}

func (s *Server) handleUpdateMuscleGroup(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	var req muscleGroupRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	g, err := s.svc.UpdateMuscleGroup(r.Context(), mustActor(r), id, service.MuscleGroupInput{
		Name: req.Name, Description: req.Description,
	})
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, muscleGroupJSON(g))
}

func (s *Server) handleDeleteMuscleGroup(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	if err := s.svc.DeleteMuscleGroup(r.Context(), mustActor(r), id); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
