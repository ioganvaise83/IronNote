package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// logRequests emits one structured line per request with status and duration.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.log.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration_ms", float64(time.Since(start).Microseconds())/1000.0,
			"request_id", requestID(r),
		)
	})
}

func requestID(r *http.Request) string { return middleware.GetReqID(r.Context()) }
