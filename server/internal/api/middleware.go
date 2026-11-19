package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(payload []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	written, err := r.ResponseWriter.Write(payload)
	r.bytes += written
	return written, err
}

func (r *responseRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// requestLogger adds a stable request id to responses and emits one structured
// line per API request. Health probes are skipped to keep the log useful.
func (a *App) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &responseRecorder{ResponseWriter: w}
		requestID := middleware.GetReqID(r.Context())
		if requestID != "" {
			recorder.Header().Set("X-Request-Id", requestID)
		}

		next.ServeHTTP(recorder, r)

		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			return
		}
		route := ""
		if routeContext := chi.RouteContext(r.Context()); routeContext != nil {
			route = routeContext.RoutePattern()
		}
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		a.logger.Info("http request",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"route", route,
			"status", status,
			"bytes", recorder.bytes,
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", clientAddress(r.RemoteAddr),
		)
	})
}
