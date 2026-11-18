package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/httpapi"
	"github.com/scrothers/aim/internal/wire"
)

func TestApprovalSettings(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	got := decode[obj](t, e.do(t, http.MethodGet, "/api/settings/approvals", nil), http.StatusOK)
	if got["enabled"] != false || got["timeout_seconds"] != float64(300) {
		t.Fatalf("defaults = %v", got)
	}
	got = decode[obj](t, e.do(t, http.MethodPut, "/api/settings/approvals", obj{"enabled": true, "timeout_seconds": 60}), http.StatusOK)
	if got["enabled"] != true || got["timeout_seconds"] != float64(60) {
		t.Fatalf("update = %v", got)
	}
	got = decode[obj](t, e.do(t, http.MethodGet, "/api/settings/approvals", nil), http.StatusOK)
	if got["enabled"] != true || got["timeout_seconds"] != float64(60) {
		t.Fatalf("stored = %v", got)
	}
	for _, secs := range []int{5, 3601} {
		if rec := e.do(t, http.MethodPut, "/api/settings/approvals", obj{"enabled": true, "timeout_seconds": secs}); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("timeout %d status = %d: %s", secs, rec.Code, rec.Body.String())
		}
	}
	got = decode[obj](t, e.do(t, http.MethodPut, "/api/settings/approvals", obj{"enabled": false}), http.StatusOK)
	if got["enabled"] != false || got["timeout_seconds"] != float64(300) {
		t.Fatalf("omitted timeout = %v", got)
	}
}

type approvalItem struct {
	ID       string          `json:"id"`
	Identity string          `json:"identity"`
	Tool     string          `json:"tool"`
	Summary  string          `json:"summary"`
	Input    json.RawMessage `json:"input"`
	Machine  string          `json:"machine"`
}

func (e *env) pendingApprovals(t *testing.T, n int) []approvalItem {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := decode[struct {
			Items []approvalItem `json:"items"`
		}](t, e.do(t, http.MethodGet, "/api/approvals", nil), http.StatusOK)
		if len(got.Items) == n {
			return got.Items
		}
		if time.Now().After(deadline) {
			t.Fatalf("pending = %d, want %d", len(got.Items), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestApprovalsListAndDecide(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	if rec := e.do(t, http.MethodGet, "/api/approvals", nil); rec.Code != http.StatusOK || rec.Body.String() == "" ||
		decode[obj](t, rec, http.StatusOK)["items"] == nil {
		t.Fatalf("empty list = %d %s", rec.Code, rec.Body.String())
	}
	decode[obj](t, e.do(t, http.MethodPut, "/api/settings/approvals", obj{"enabled": true}), http.StatusOK)

	replies := make(chan wire.HookReply, 1)
	go func() {
		r, _ := e.hooks.Approve(context.Background(), wire.Preamble{Machine: "box", SessionID: "s1", Kind: wire.KindHook},
			wire.HookRequest{Op: wire.HookOpApprove, ToolName: "Bash", Detail: "Bash: rm -rf build", ToolInput: json.RawMessage(`{"command":"rm -rf build"}`)})
		replies <- r
	}()
	items := e.pendingApprovals(t, 1)
	a := items[0]
	if a.Tool != "Bash" || a.Summary != "Bash: rm -rf build" || string(a.Input) != `{"command":"rm -rf build"}` || a.Machine != "box" {
		t.Fatalf("approval = %+v", a)
	}

	if rec := e.do(t, http.MethodPost, "/api/approvals/"+a.ID, obj{"decision": "maybe"}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad decision status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(t, http.MethodPost, "/api/approvals/appr-unknown", obj{"decision": "allow"}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown id status = %d: %s", rec.Code, rec.Body.String())
	}
	got := decode[obj](t, e.do(t, http.MethodPost, "/api/approvals/"+a.ID, obj{"decision": "deny", "reason": "not now"}), http.StatusOK)
	if got["id"] != a.ID || got["decision"] != "deny" {
		t.Fatalf("decide = %v", got)
	}
	select {
	case r := <-replies:
		if r.Decision != wire.DecisionDeny || r.Reason != "not now" {
			t.Fatalf("hook reply = %+v", r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("hook not released")
	}
	if rec := e.do(t, http.MethodPost, "/api/approvals/"+a.ID, obj{"decision": "allow"}); rec.Code != http.StatusNotFound {
		t.Errorf("second decision status = %d", rec.Code)
	}
	e.pendingApprovals(t, 0)
}

func TestApprovalsUnavailable(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.h = httpapi.New(httpapi.Deps{Pinger: fakePinger{}, Mail: e.svc, Presence: e.reg, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if rec := e.do(t, http.MethodGet, "/api/approvals", nil); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("list status = %d", rec.Code)
	}
	if rec := e.do(t, http.MethodPost, "/api/approvals/appr-x", obj{"decision": "allow"}); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("decide status = %d", rec.Code)
	}
}
