package httpapi_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/httpapi"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
	req.Host = "localhost"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthzOK(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	h := httpapi.New(httpapi.Deps{Pinger: fakePinger{}, Version: "test", Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	rec := get(t, h, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("body = %s", rec.Body.String())
	}
	for _, want := range []string{"method=GET", "path=/healthz", "status=200", "request_id="} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log missing %q: %s", want, logs.String())
		}
	}
}

func TestHealthzUnhealthy(t *testing.T) {
	t.Parallel()
	h := httpapi.New(httpapi.Deps{Pinger: fakePinger{err: errors.New("db down")}, Version: "test", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	rec := get(t, h, "/healthz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOpenAPIAndDocs(t *testing.T) {
	t.Parallel()
	h := httpapi.New(httpapi.Deps{Pinger: fakePinger{}, Version: "1.2.3"})
	rec := get(t, h, "/openapi.json")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"operationId":"healthz"`) {
		t.Errorf("openapi: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(t, h, "/docs"); rec.Code != http.StatusOK {
		t.Errorf("docs status = %d", rec.Code)
	}
}
