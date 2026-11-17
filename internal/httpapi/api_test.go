package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/httpapi"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver"
	"github.com/scrothers/aim/internal/presence"
	"github.com/scrothers/aim/internal/store"
	"github.com/scrothers/aim/internal/wire"
)

const alice = "alice@github.com/acme/web"

type env struct {
	h   http.Handler
	svc *mail.Service
	reg *presence.Registry
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "aim.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	reg := presence.New()
	svc := mail.New(st, mcpserver.NewPresence(reg))
	if err := svc.EnsureOperator(ctx); err != nil {
		t.Fatal(err)
	}
	h := httpapi.New(httpapi.Deps{
		Pinger: fakePinger{}, Mail: svc, Presence: reg, Version: "test",
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return &env{h: h, svc: svc, reg: reg}
}

func (e *env) register(t *testing.T, addr, role string) {
	t.Helper()
	if _, err := e.svc.Register(context.Background(), mail.Caller{}, mail.RegisterInput{Address: addr, Role: role}); err != nil {
		t.Fatal(err)
	}
}

func (e *env) signOn(t *testing.T, addr, session, machine, client string) {
	t.Helper()
	ent := e.reg.Add(session, wire.Preamble{SessionID: session, Machine: machine}, time.Now())
	if err := ent.SignOn(address.MustParse(addr), "brb", time.Now()); err != nil {
		t.Fatal(err)
	}
	ent.SetClient(client, "1.2.3")
}

func (e *env) agentCaller(t *testing.T, addr string) mail.Caller {
	t.Helper()
	return mail.Caller{SessionID: "s-" + addr, Identity: address.MustParse(addr), SignedOn: true}
}

// do performs a request as the browser UI would, with the guard headers.
func (e *env) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rd = strings.NewReader(string(b))
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, rd)
	req.Host = "localhost:7172"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AIM-Request", "1")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder, want int) T {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d: %s", rec.Code, want, rec.Body.String())
	}
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode: %v: %s", err, rec.Body.String())
	}
	return v
}

type obj = map[string]any

func TestEmptyStateShapes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	for _, path := range []string{
		"/api/views", "/api/tree", "/api/presence", "/api/stats", "/api/messages",
		"/api/roster", "/api/orgchart", "/api/search?q=hello",
	} {
		rec := e.do(t, http.MethodGet, path, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d: %s", path, rec.Code, rec.Body.String())
			continue
		}
		if strings.Contains(rec.Body.String(), "null") {
			t.Errorf("%s: body contains null: %s", path, rec.Body.String())
		}
	}
	pres := decode[obj](t, e.do(t, http.MethodGet, "/api/presence", nil), http.StatusOK)
	groups, _ := pres["groups"].([]any)
	if len(groups) != 1 || groups[0].(obj)["name"] != "Offline" {
		t.Errorf("groups = %v", pres["groups"])
	}
}

func TestMailFlow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	e.register(t, alice, "dev")
	e.signOn(t, alice, "sess-1", "devbox", "claude-code")
	sent, err := e.svc.Send(ctx, e.agentCaller(t, alice), mail.SendInput{
		To: []string{"operator"}, Subject: "need approval", Body: "**please** <script>alert(1)</script>",
	})
	if err != nil {
		t.Fatal(err)
	}

	views := decode[obj](t, e.do(t, http.MethodGet, "/api/views?today_since="+time.Now().Add(-time.Hour).UTC().Format(time.RFC3339), nil), http.StatusOK)
	if views["operator_unread"] != float64(1) || views["today"] != float64(1) {
		t.Errorf("views = %v", views)
	}
	decode[obj](t, e.do(t, http.MethodGet, "/api/views", nil), http.StatusOK)

	list := decode[obj](t, e.do(t, http.MethodGet, "/api/messages?address=operator&state=unread&limit=10", nil), http.StatusOK)
	if items := list["items"].([]any); len(items) != 1 {
		t.Fatalf("list = %v", list)
	}
	since := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	list = decode[obj](t, e.do(t, http.MethodGet, "/api/messages?address=operator&since="+since+"&until="+since, nil), http.StatusOK)
	if len(list["items"].([]any)) != 0 {
		t.Errorf("since filter = %v", list)
	}

	msg := decode[obj](t, e.do(t, http.MethodGet, "/api/messages/"+sent.ID, nil), http.StatusOK)
	html := msg["body_html"].(string)
	if !strings.Contains(html, "<strong>please</strong>") || strings.Contains(html, "<script") {
		t.Errorf("body_html = %q", html)
	}
	if msg["body"] != sent.Body || msg["from"] != alice {
		t.Errorf("message = %v", msg)
	}
	if got := decode[obj](t, e.do(t, http.MethodGet, "/api/views", nil), http.StatusOK); got["operator_unread"] != float64(1) {
		t.Errorf("peek marked read: %v", got)
	}

	decode[obj](t, e.do(t, http.MethodPost, "/api/messages/"+sent.ID+"/read", nil), http.StatusOK)
	if got := decode[obj](t, e.do(t, http.MethodGet, "/api/views", nil), http.StatusOK); got["operator_unread"] != float64(0) {
		t.Errorf("read did not mark: %v", got)
	}
	marks := decode[obj](t, e.do(t, http.MethodPost, "/api/marks", obj{"message_ids": []string{sent.ID}, "state": "unread"}), http.StatusOK)
	if marks["updated"] != float64(1) {
		t.Errorf("marks = %v", marks)
	}

	reply := decode[obj](t, e.do(t, http.MethodPost, "/api/messages", obj{
		"in_reply_to": sent.ID, "reply_all": true, "subject": "Re: need approval", "body": "approved",
	}), http.StatusCreated)
	if reply["from"] != "operator" || reply["thread_id"] != sent.ThreadID || reply["body_html"] == "" {
		t.Errorf("reply = %v", reply)
	}
	inbox, err := e.svc.List(ctx, mail.ListInput{Address: alice})
	if err != nil || len(inbox.Items) != 1 {
		t.Errorf("alice inbox = %+v, %v", inbox, err)
	}

	th := decode[obj](t, e.do(t, http.MethodGet, "/api/threads/"+sent.ThreadID, nil), http.StatusOK)
	if len(th["messages"].([]any)) != 2 {
		t.Errorf("thread = %v", th)
	}

	hits := decode[obj](t, e.do(t, http.MethodGet, "/api/search?q=approved", nil), http.StatusOK)
	if len(hits["hits"].([]any)) != 1 {
		t.Errorf("hits = %v", hits)
	}

	tree := decode[obj](t, e.do(t, http.MethodGet, "/api/tree", nil), http.StatusOK)
	if len(tree["orgs"].([]any)) != 1 {
		t.Errorf("tree = %v", tree)
	}
	stats := decode[obj](t, e.do(t, http.MethodGet, "/api/stats", nil), http.StatusOK)
	if stats["messages"] != float64(2) || stats["version"] != "test" || stats["sessions"] != float64(1) {
		t.Errorf("stats = %v", stats)
	}
	chart := decode[obj](t, e.do(t, http.MethodGet, "/api/orgchart", nil), http.StatusOK)
	if len(chart["nodes"].([]any)) < 2 {
		t.Errorf("orgchart = %v", chart)
	}

	r := decode[obj](t, e.do(t, http.MethodPost, "/api/render", obj{"markdown": "# hi"}), http.StatusOK)
	if !strings.Contains(r["html"].(string), "<h1") {
		t.Errorf("render = %v", r)
	}
}

func TestPresenceGroups(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, alice, "dev")
	e.register(t, "bob@github.com/acme/web", "qa")
	e.register(t, "carol@github.com/acme", "lead")
	e.signOn(t, alice, "sess-1", "devbox", "claude-code")

	body := decode[httpapi.PresenceBody](t, e.do(t, http.MethodGet, "/api/presence", nil), http.StatusOK)
	if len(body.Groups) != 2 {
		t.Fatalf("groups = %+v", body.Groups)
	}
	g := body.Groups[0]
	if g.Name != "github.com/acme/web" || g.Online != 1 || g.Total != 2 || len(g.Buddies) != 1 {
		t.Fatalf("group = %+v", g)
	}
	b := g.Buddies[0]
	if b.Name != "alice" || b.Client != "claude-code" || b.ClientVersion != "1.2.3" || b.Machine != "devbox" || b.Away != "brb" || b.SignedOnAt.IsZero() || !b.Online {
		t.Errorf("buddy = %+v", b)
	}
	off := body.Groups[1]
	if !off.Offline || off.Total != 2 || len(off.Buddies) != 2 {
		t.Errorf("offline = %+v", off)
	}
	if len(body.Events) == 0 || body.Events[0].Kind != "signed_on" {
		t.Errorf("events = %+v", body.Events)
	}
}

func TestRosterAndIdentities(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, alice, "dev")

	ros := decode[httpapi.RosterBody](t, e.do(t, http.MethodGet, "/api/roster", nil), http.StatusOK)
	if len(ros.Items) != 1 || ros.Items[0].Address != alice {
		t.Fatalf("roster = %+v", ros)
	}
	upd := decode[obj](t, e.do(t, http.MethodPatch, "/api/identities?address="+alice, obj{"role": "lead", "description": "d"}), http.StatusOK)
	if upd["role"] != "lead" || upd["description"] != "d" {
		t.Errorf("update = %v", upd)
	}
	ret := decode[obj](t, e.do(t, http.MethodPost, "/api/identities/retire?address="+alice, nil), http.StatusOK)
	if ret["retired_at"] == nil {
		t.Errorf("retire = %v", ret)
	}
	ros = decode[httpapi.RosterBody](t, e.do(t, http.MethodGet, "/api/roster", nil), http.StatusOK)
	if len(ros.Items) != 0 {
		t.Errorf("retired shown: %+v", ros)
	}
	ros = decode[httpapi.RosterBody](t, e.do(t, http.MethodGet, "/api/roster?include_retired=true", nil), http.StatusOK)
	if len(ros.Items) != 1 {
		t.Errorf("include_retired: %+v", ros)
	}
	un := decode[obj](t, e.do(t, http.MethodPost, "/api/identities/unretire?address="+alice, nil), http.StatusOK)
	if un["retired_at"] != nil {
		t.Errorf("unretire = %v", un)
	}
}

func TestErrorMapping(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, alice, "dev")
	e.register(t, "bob@github.com/acme/web", "qa")
	tests := []struct {
		name, method, path string
		body               any
		want               int
		contains           string
	}{
		{"bad query", http.MethodGet, "/api/search?q=%22", nil, http.StatusBadRequest, ""},
		{"bad address", http.MethodGet, "/api/messages?address=%25%25", nil, http.StatusBadRequest, ""},
		{"empty send", http.MethodPost, "/api/messages", obj{"subject": "s", "body": "b"}, http.StatusBadRequest, ""},
		{"unknown mailbox", http.MethodPost, "/api/messages", obj{"to": []string{"alic@github.com/acme/web"}, "subject": "s", "body": "b"}, http.StatusNotFound, "alice@github.com/acme/web"},
		{"unknown message", http.MethodGet, "/api/messages/msg-nope", nil, http.StatusNotFound, ""},
		{"read unknown", http.MethodPost, "/api/messages/msg-nope/read", nil, http.StatusNotFound, ""},
		{"unknown thread", http.MethodGet, "/api/threads/nope", nil, http.StatusNotFound, ""},
		{"update unknown", http.MethodPatch, "/api/identities?address=zed@github.com/acme/web", obj{"role": "x"}, http.StatusNotFound, ""},
		{"reserved", http.MethodPost, "/api/identities/retire?address=operator", nil, http.StatusUnprocessableEntity, ""},
		{"self cycle", http.MethodPatch, "/api/identities?address=" + alice, obj{"reports_to": alice}, http.StatusUnprocessableEntity, ""},
		{"bad mark state", http.MethodPost, "/api/marks", obj{"message_ids": []string{"x"}, "state": "bogus"}, http.StatusUnprocessableEntity, ""},
		{"missing address", http.MethodPost, "/api/identities/retire", nil, http.StatusUnprocessableEntity, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := e.do(t, tt.method, tt.path, tt.body)
			if rec.Code != tt.want || !strings.Contains(rec.Body.String(), tt.contains) {
				t.Errorf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestGuards(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	raw := func(method, host, ctype, hdr string) int {
		req := httptest.NewRequestWithContext(context.Background(), method, "/api/render", strings.NewReader(`{"markdown":"x"}`))
		req.Host = host
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		if hdr != "" {
			req.Header.Set("X-AIM-Request", hdr)
		}
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		return rec.Code
	}
	tests := []struct {
		name                    string
		method, host, ctype, hd string
		want                    int
	}{
		{"ok", http.MethodPost, "localhost:7172", "application/json", "1", http.StatusOK},
		{"ok charset", http.MethodPost, "127.0.0.1", "application/json; charset=utf-8", "1", http.StatusOK},
		{"ok ipv6", http.MethodPost, "[::1]:7172", "application/json", "1", http.StatusOK},
		{"bad host", http.MethodPost, "evil.example:7172", "application/json", "1", http.StatusMisdirectedRequest},
		{"no header", http.MethodPost, "localhost", "application/json", "", http.StatusForbidden},
		{"wrong header", http.MethodPost, "localhost", "application/json", "2", http.StatusForbidden},
		{"form content type", http.MethodPost, "localhost", "text/plain", "1", http.StatusForbidden},
		{"no content type", http.MethodPost, "localhost", "", "1", http.StatusForbidden},
		{"get needs no headers", http.MethodGet, "localhost", "", "", http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := raw(tt.method, tt.host, tt.ctype, tt.hd); got != tt.want {
				t.Errorf("status = %d, want %d", got, tt.want)
			}
		})
	}

	h := httpapi.New(httpapi.Deps{Pinger: fakePinger{}, AllowedHosts: []string{"aim.lan:9000", "other.lan"}})
	for host, want := range map[string]int{"aim.lan": http.StatusOK, "other.lan:1": http.StatusOK, "nope.lan": http.StatusMisdirectedRequest} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("host %s: status = %d, want %d", host, rec.Code, want)
		}
	}
}

func TestOpenAPIDocument(t *testing.T) {
	t.Parallel()
	b, err := httpapi.OpenAPI("9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		OpenAPI string `json:"openapi"`
		Paths   map[string]map[string]struct {
			ID string `json:"operationId"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(doc.OpenAPI, "3.1") {
		t.Errorf("openapi = %q", doc.OpenAPI)
	}
	ids := map[string]bool{}
	for _, ops := range doc.Paths {
		for _, op := range ops {
			ids[op.ID] = true
		}
	}
	for _, want := range []string{"healthz", "getViews", "getTree", "getPresence", "getStats", "listMessages", "getMessage",
		"readMessage", "getThread", "sendMessage", "markMessages", "searchMessages", "getRoster", "updateIdentity",
		"retireIdentity", "unretireIdentity", "getOrgChart", "renderMarkdown"} {
		if !ids[want] {
			t.Errorf("missing operation %s", want)
		}
	}
}
