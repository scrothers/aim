package httpapi

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/presence"
	"github.com/scrothers/aim/internal/render"
	"github.com/scrothers/aim/internal/tasks"
)

// Pinger reports whether a backing dependency is healthy.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps are the collaborators of the HTTP API.
type Deps struct {
	Pinger   Pinger
	Mail     *mail.Service
	Presence *presence.Registry
	// Maint runs backups and pruning; nil makes those operations return 503.
	Maint Maintenance
	// Notify manages webhooks; nil makes those operations return 503.
	Notify Notifier
	// Approvals holds agent permission prompts; nil makes the approval list
	// and decisions return 503.
	Approvals Approver
	// Tasks is the task board; nil makes the task operations return 503.
	Tasks *tasks.Service
	// Static serves the web UI; nil disables static serving.
	Static  fs.FS
	Version string
	Logger  *slog.Logger
	// AllowedHosts extends the default Host header allowlist.
	AllowedHosts []string
}

// HealthOutput is the body of a successful health check.
type HealthOutput struct {
	Body struct {
		Status string `json:"status" example:"ok" doc:"Always ok when healthy."`
	}
}

// New returns the HTTP handler for the aim API and web UI.
func New(d Deps) http.Handler {
	logger := d.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	router := chi.NewRouter()
	router.Use(middleware.RequestID, middleware.Recoverer, requestLogger(logger), guards(d.AllowedHosts))

	api := humachi.New(router, config(d.Version))
	register(api, d)
	mountStatic(router, d.Static)
	return router
}

func config(version string) huma.Config {
	return huma.DefaultConfig("AIM", version)
}

// register adds every operation to api. Handlers only touch d when invoked,
// so a zero Deps is enough to describe the API.
func register(api huma.API, d Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "healthz",
		Method:      http.MethodGet,
		Path:        "/healthz",
		Summary:     "Health check",
	}, func(ctx context.Context, _ *struct{}) (*HealthOutput, error) {
		if err := d.Pinger.Ping(ctx); err != nil {
			return nil, huma.Error503ServiceUnavailable("unhealthy", err)
		}
		out := &HealthOutput{}
		out.Body.Status = "ok"
		return out, nil
	})
	registerOperations(api, newHandlers(d))
}

func newHandlers(d Deps) *handlers {
	return &handlers{
		mail:      d.Mail,
		presence:  d.Presence,
		render:    render.New(),
		version:   d.Version,
		caller:    operatorCaller(),
		maint:     d.Maint,
		notify:    d.Notify,
		approvals: d.Approvals,
		tasks:     d.Tasks,
	}
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
