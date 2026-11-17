package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Pinger reports whether a backing dependency is healthy.
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthOutput is the body of a successful health check.
type HealthOutput struct {
	Body struct {
		Status string `json:"status" example:"ok" doc:"Always ok when healthy."`
	}
}

// New returns the HTTP handler for the aim API.
func New(p Pinger, version string, logger *slog.Logger) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID, middleware.Recoverer, requestLogger(logger))

	api := humachi.New(router, huma.DefaultConfig("AIM", version))
	huma.Register(api, huma.Operation{
		OperationID: "healthz",
		Method:      http.MethodGet,
		Path:        "/healthz",
		Summary:     "Health check",
	}, func(ctx context.Context, _ *struct{}) (*HealthOutput, error) {
		if err := p.Ping(ctx); err != nil {
			return nil, huma.Error503ServiceUnavailable("unhealthy", err)
		}
		out := &HealthOutput{}
		out.Body.Status = "ok"
		return out, nil
	})
	return router
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			logger.LogAttrs(r.Context(), slog.LevelInfo, "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", middleware.GetReqID(r.Context())),
			)
		})
	}
}
