package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/ioganvaise83/ironnote/internal/apperror"
)

// errorBody is the single error shape of the API.
type errorBody struct {
	Error errorPayload `json:"error"`
}

type errorPayload struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// The status line is already sent; nothing safe left to do but log.
		slog.Default().Error("encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	appErr, ok := apperror.As(err)
	if !ok {
		appErr = apperror.Internal(err)
	}
	status := appErr.Status()

	if status >= http.StatusInternalServerError {
		log.Error("request failed",
			"error", err,
			"method", r.Method,
			"path", r.URL.Path,
			"request_id", requestID(r),
		)
	} else {
		log.Info("request rejected",
			"code", appErr.Code,
			"message", appErr.Message,
			"method", r.Method,
			"path", r.URL.Path,
			"request_id", requestID(r),
		)
	}

	writeJSON(w, status, errorBody{Error: errorPayload{
		Code:    string(appErr.Code),
		Message: appErr.Message,
		Details: appErr.Details,
	}})
}

// timeJSON renders instants as RFC 3339 in UTC. Clients format for display.
func timeJSON(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func timeJSONPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := timeJSON(*t)
	return &s
}
