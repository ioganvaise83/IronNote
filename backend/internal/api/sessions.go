package api

import (
	"net/http"
	"time"

	"github.com/ioganvaise83/ironnote/internal/apperror"
	"github.com/ioganvaise83/ironnote/internal/service"
)

type setResponse struct {
	ID          int64    `json:"id"`
	SetNumber   int32    `json:"setNumber"`
	WeightKg    *float64 `json:"weightKg"`
	Repetitions int16    `json:"repetitions"`
	IsWarmup    bool     `json:"isWarmup"`
	IsCompleted bool     `json:"isCompleted"`
}

type sessionExerciseResponse struct {
	ID               int64         `json:"id"`
	ExerciseID       int64         `json:"exerciseId"`
	ExerciseName     string        `json:"exerciseName"`
	Equipment        string        `json:"equipment"`
	ExerciseArchived bool          `json:"exerciseArchived"`
	SortOrder        int32         `json:"sortOrder"`
	Notes            string        `json:"notes"`
	Sets             []setResponse `json:"sets"`
	PreviousSets     []setResponse `json:"previousSets"`
	PreviousAt       *string       `json:"previousAt"`
}

type sessionResponse struct {
	ID              int64                     `json:"id"`
	TemplateID      *int64                    `json:"templateId"`
	Name            string                    `json:"name"`
	StartedAt       string                    `json:"startedAt"`
	FinishedAt      *string                   `json:"finishedAt"`
	Status          string                    `json:"status"`
	Notes           string                    `json:"notes"`
	Exercises       []sessionExerciseResponse `json:"exercises"`
	ExerciseCount   int                       `json:"exerciseCount"`
	CompletedSets   int                       `json:"completedSetCount"`
	VolumeKg        float64                   `json:"volumeKg"`
	DurationMinutes *float64                  `json:"durationMinutes"`
}

type sessionSummaryResponse struct {
	ID                int64    `json:"id"`
	TemplateID        *int64   `json:"templateId"`
	Name              string   `json:"name"`
	StartedAt         string   `json:"startedAt"`
	FinishedAt        *string  `json:"finishedAt"`
	Status            string   `json:"status"`
	Notes             string   `json:"notes"`
	ExerciseCount     int      `json:"exerciseCount"`
	CompletedSetCount int      `json:"completedSetCount"`
	VolumeKg          float64  `json:"volumeKg"`
	DurationMinutes   *float64 `json:"durationMinutes"`
}

func setJSON(s service.Set) setResponse {
	return setResponse{
		ID: s.ID, SetNumber: s.SetNumber, WeightKg: s.WeightKg.Ptr(),
		Repetitions: s.Repetitions, IsWarmup: s.IsWarmup, IsCompleted: s.IsCompleted,
	}
}

func sessionJSON(s service.Session) sessionResponse {
	summary := service.Summarize(s)
	exercises := make([]sessionExerciseResponse, 0, len(s.Exercises))
	for _, e := range s.Exercises {
		sets := make([]setResponse, 0, len(e.Sets))
		for _, set := range e.Sets {
			sets = append(sets, setJSON(set))
		}
		previous := make([]setResponse, 0, len(e.PreviousSets))
		for _, set := range e.PreviousSets {
			previous = append(previous, setJSON(set))
		}
		exercises = append(exercises, sessionExerciseResponse{
			ID: e.ID, ExerciseID: e.ExerciseID, ExerciseName: e.ExerciseName,
			Equipment: e.Equipment, ExerciseArchived: e.ExerciseArchived,
			SortOrder: e.SortOrder, Notes: e.Notes, Sets: sets,
			PreviousSets: previous, PreviousAt: timeJSONPtr(e.PreviousAt),
		})
	}
	return sessionResponse{
		ID: s.ID, TemplateID: s.TemplateID, Name: s.Name,
		StartedAt: timeJSON(s.StartedAt), FinishedAt: timeJSONPtr(s.FinishedAt),
		Status: s.Status, Notes: s.Notes, Exercises: exercises,
		ExerciseCount: summary.ExerciseCount, CompletedSets: summary.CompletedSetCount,
		VolumeKg: summary.VolumeKg, DurationMinutes: summary.DurationMinutes,
	}
}

func (s *Server) handleActiveSession(w http.ResponseWriter, r *http.Request) {
	session, err := s.svc.GetActiveSession(r.Context())
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	// 200 with an explicit null is friendlier for polling than a 404.
	writeJSON(w, http.StatusOK, map[string]any{"session": nullableSession(session)})
}

func nullableSession(session *service.Session) any {
	if session == nil {
		return nil
	}
	return sessionJSON(*session)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	filter := service.SessionFilter{Status: r.URL.Query().Get("status")}
	query := r.URL.Query()
	from, err := parseDate(query.Get("from"))
	if err != nil {
		writeError(w, r, s.log, apperror.Validation("Некорректная дата в from", map[string]string{"from": query.Get("from")}))
		return
	}
	to, err := parseDate(query.Get("to"))
	if err != nil {
		writeError(w, r, s.log, apperror.Validation("Некорректная дата в to", map[string]string{"to": query.Get("to")}))
		return
	}
	filter.From = from
	filter.To = to

	items, err := s.svc.ListSessions(r.Context(), filter)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	out := make([]sessionSummaryResponse, 0, len(items))
	for _, item := range items {
		out = append(out, sessionSummaryResponse{
			ID: item.ID, TemplateID: item.TemplateID, Name: item.Name,
			StartedAt: timeJSON(item.StartedAt), FinishedAt: timeJSONPtr(item.FinishedAt),
			Status: item.Status, Notes: item.Notes, ExerciseCount: item.ExerciseCount,
			CompletedSetCount: item.CompletedSetCount, VolumeKg: item.VolumeKg,
			DurationMinutes: item.DurationMinutes,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	session, err := s.svc.GetSession(r.Context(), id)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, sessionJSON(session))
}

type startSessionRequest struct {
	Name       string `json:"name"`
	TemplateID *int64 `json:"templateId"`
	Notes      string `json:"notes"`
}

func (s *Server) handleStartSession(w http.ResponseWriter, r *http.Request) {
	var req startSessionRequest
	if r.ContentLength != 0 {
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, r, s.log, err)
			return
		}
	}
	session, err := s.svc.StartSession(r.Context(), service.StartSessionInput{
		Name: req.Name, TemplateID: req.TemplateID, Notes: req.Notes,
	})
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, sessionJSON(session))
}

type updateSessionRequest struct {
	Name      *string    `json:"name"`
	Notes     *string    `json:"notes"`
	StartedAt *time.Time `json:"startedAt"`
}

func (s *Server) handleUpdateSession(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	var req updateSessionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	session, err := s.svc.UpdateSession(r.Context(), id, service.UpdateSessionInput{
		Name: req.Name, Notes: req.Notes, StartedAt: req.StartedAt,
	})
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, sessionJSON(session))
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	if err := s.svc.DeleteSession(r.Context(), id); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleFinishSession(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	session, err := s.svc.FinishSession(r.Context(), id)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, sessionJSON(session))
}

func (s *Server) handleReopenSession(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	session, err := s.svc.ReopenSession(r.Context(), id)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, sessionJSON(session))
}

type addExerciseRequest struct {
	ExerciseID int64  `json:"exerciseId"`
	Notes      string `json:"notes"`
}

func (s *Server) handleAddSessionExercise(w http.ResponseWriter, r *http.Request) {
	sessionID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	var req addExerciseRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	exercise, err := s.svc.AddSessionExercise(r.Context(), sessionID, req.ExerciseID, req.Notes)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"exercise": exerciseResponseForSession(exercise)})
}

func exerciseResponseForSession(e service.SessionExercise) sessionExerciseResponse {
	return sessionExerciseResponse{
		ID: e.ID, ExerciseID: e.ExerciseID, ExerciseName: e.ExerciseName,
		Equipment: e.Equipment, SortOrder: e.SortOrder, Notes: e.Notes,
		Sets: []setResponse{}, PreviousSets: []setResponse{},
	}
}

type moveExerciseRequest struct {
	Direction int `json:"direction"`
}

func (s *Server) handleMoveSessionExercise(w http.ResponseWriter, r *http.Request) {
	sessionID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	exerciseID, err := pathID(r, "exerciseId")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	var req moveExerciseRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	if err := s.svc.MoveSessionExercise(r.Context(), sessionID, exerciseID, req.Direction); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	session, err := s.svc.GetSession(r.Context(), sessionID)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, sessionJSON(session))
}

func (s *Server) handleDeleteSessionExercise(w http.ResponseWriter, r *http.Request) {
	sessionID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	exerciseID, err := pathID(r, "exerciseId")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	if err := s.svc.DeleteSessionExercise(r.Context(), sessionID, exerciseID); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type notesRequest struct {
	Notes string `json:"notes"`
}

func (s *Server) handleSessionExerciseNotes(w http.ResponseWriter, r *http.Request) {
	sessionID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	exerciseID, err := pathID(r, "exerciseId")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	var req notesRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	if err := s.svc.UpdateSessionExerciseNotes(r.Context(), sessionID, exerciseID, req.Notes); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddSet(w http.ResponseWriter, r *http.Request) {
	sessionID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	exerciseID, err := pathID(r, "exerciseId")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	var req service.SetInput
	if r.ContentLength != 0 {
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, r, s.log, err)
			return
		}
	}
	set, err := s.svc.AddSet(r.Context(), sessionID, exerciseID, req)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, setJSON(set))
}

func (s *Server) handleUpdateSet(w http.ResponseWriter, r *http.Request) {
	sessionID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	exerciseID, err := pathID(r, "exerciseId")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	setID, err := pathID(r, "setId")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	var req service.SetInput
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	set, err := s.svc.UpdateSet(r.Context(), sessionID, exerciseID, setID, req)
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, setJSON(set))
}

func (s *Server) handleDeleteSet(w http.ResponseWriter, r *http.Request) {
	sessionID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	exerciseID, err := pathID(r, "exerciseId")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	setID, err := pathID(r, "setId")
	if err != nil {
		writeError(w, r, s.log, err)
		return
	}
	if err := s.svc.DeleteSet(r.Context(), sessionID, exerciseID, setID); err != nil {
		writeError(w, r, s.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parseDate accepts a date (YYYY-MM-DD) or a full RFC 3339 instant. For a date in
// "to" the following midnight is used, so the range is half-open and inclusive
// of the whole day.
func parseDate(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
