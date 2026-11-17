package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/httpapi"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/wire"
)

const secretToken = "SECRETTOKEN-abc123"

// noLeak fails when a response body contains a full webhook URL or its token.
func noLeak(t *testing.T, _ *env, rec *httptest.ResponseRecorder) {
	t.Helper()
	b := rec.Body.String()
	if strings.Contains(b, secretToken) {
		t.Errorf("response leaks the webhook url: %s", b)
	}
}

func (e *env) hookURL() string { return e.hookBase + "/api/webhooks/123/" + secretToken }

func TestSettingsDefaultsAndRetention(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	s := decode[obj](t, e.do(t, http.MethodGet, "/api/settings", nil), http.StatusOK)
	if s["retention"].(obj)["prune_archived_after_days"] != float64(0) {
		t.Errorf("retention = %v", s["retention"])
	}
	b := s["backups"].(obj)
	if b["every_hours"] != float64(24) || b["keep"] != float64(7) || b["effective_dir"] != e.backupDir {
		t.Errorf("backups = %v", b)
	}
	if n := s["notify"].(obj); len(n["webhooks"].([]any)) != 0 {
		t.Errorf("notify = %v", n)
	}

	r := decode[obj](t, e.do(t, http.MethodPut, "/api/settings/retention", obj{"prune_archived_after_days": 30}), http.StatusOK)
	if r["prune_archived_after_days"] != float64(30) {
		t.Errorf("updateRetention = %v", r)
	}
	s = decode[obj](t, e.do(t, http.MethodGet, "/api/settings", nil), http.StatusOK)
	if s["retention"].(obj)["prune_archived_after_days"] != float64(30) {
		t.Errorf("retention not stored: %v", s["retention"])
	}
	if rec := e.do(t, http.MethodPut, "/api/settings/retention", obj{"prune_archived_after_days": -5}); rec.Code != http.StatusBadRequest {
		t.Errorf("negative retention status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateBackups(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	dir := filepath.Join(t.TempDir(), "mine")
	b := decode[obj](t, e.do(t, http.MethodPut, "/api/settings/backups", obj{"every_hours": 6, "keep": 3, "dir": dir}), http.StatusOK)
	if b["every_hours"] != float64(6) || b["keep"] != float64(3) || b["dir"] != dir || b["effective_dir"] != dir {
		t.Errorf("updateBackups = %v", b)
	}
	if rec := e.do(t, http.MethodPut, "/api/settings/backups", obj{"every_hours": 6, "keep": 3, "dir": "relative/path"}); rec.Code != http.StatusBadRequest {
		t.Errorf("relative dir status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(t, http.MethodPut, "/api/settings/backups", obj{"every_hours": 100000, "keep": 3}); rec.Code != http.StatusBadRequest {
		t.Errorf("huge every_hours status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBackupNowAndPrune(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	st := decode[obj](t, e.do(t, http.MethodPost, "/api/settings/backup", nil), http.StatusOK)
	path, _ := st["last_path"].(string)
	if path == "" || filepath.Dir(path) != e.backupDir || st["last_error"] != nil {
		t.Fatalf("backupNow = %v", st)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("backup file: %v", err)
	}
	s := decode[obj](t, e.do(t, http.MethodGet, "/api/settings", nil), http.StatusOK)
	if s["backup_status"].(obj)["last_path"] != path {
		t.Errorf("backup_status = %v", s["backup_status"])
	}

	p := decode[obj](t, e.do(t, http.MethodPost, "/api/settings/prune?dry_run=true", nil), http.StatusOK)
	if p["enabled"] != false || p["dry_run"] != true {
		t.Errorf("prune with retention off = %v", p)
	}
	decode[obj](t, e.do(t, http.MethodPut, "/api/settings/retention", obj{"prune_archived_after_days": 1}), http.StatusOK)
	p = decode[obj](t, e.do(t, http.MethodPost, "/api/settings/prune?dry_run=true", nil), http.StatusOK)
	if p["enabled"] != true || p["dry_run"] != true || p["messages"] != float64(0) {
		t.Errorf("dry prune = %v", p)
	}
	p = decode[obj](t, e.do(t, http.MethodPost, "/api/settings/prune", nil), http.StatusOK)
	if p["dry_run"] != false {
		t.Errorf("prune = %v", p)
	}
}

func TestBackupNowFailureIsReportedInBody(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	// A file where the backup directory should be makes the backup fail.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	decode[obj](t, e.do(t, http.MethodPut, "/api/settings/backups", obj{"dir": filepath.Join(blocker, "sub")}), http.StatusOK)
	st := decode[obj](t, e.do(t, http.MethodPost, "/api/settings/backup", nil), http.StatusOK)
	if msg, _ := st["last_error"].(string); msg == "" {
		t.Errorf("backupNow = %v, want last_error", st)
	}
}

func TestNotifySettingsMaskURLs(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	rec := e.do(t, http.MethodPut, "/api/settings/notify", obj{
		"public_url": "http://aim.lan:7172/",
		"webhooks":   []obj{{"name": "ops", "url": e.hookURL(), "enabled": true}},
	})
	noLeak(t, e, rec)
	v := decode[obj](t, rec, http.StatusOK)
	whs := v["webhooks"].([]any)
	if len(whs) != 1 || v["public_url"] != "http://aim.lan:7172" {
		t.Fatalf("updateNotify = %v", v)
	}
	wh := whs[0].(obj)
	id, _ := wh["id"].(string)
	if id == "" || wh["enabled"] != true || !strings.HasSuffix(wh["url"].(string), "/****") {
		t.Errorf("webhook = %v", wh)
	}

	rec = e.do(t, http.MethodGet, "/api/settings", nil)
	noLeak(t, e, rec)
	if got := decode[obj](t, rec, http.StatusOK)["notify"].(obj)["webhooks"].([]any); len(got) != 1 {
		t.Errorf("settings webhooks = %v", got)
	}

	// An empty url keeps the stored one: rename and disable, then test-send.
	rec = e.do(t, http.MethodPut, "/api/settings/notify", obj{
		"webhooks": []obj{{"id": id, "name": "renamed", "enabled": false}},
	})
	noLeak(t, e, rec)
	wh = decode[obj](t, rec, http.StatusOK)["webhooks"].([]any)[0].(obj)
	if wh["name"] != "renamed" || wh["enabled"] != false || wh["id"] != id {
		t.Errorf("renamed webhook = %v", wh)
	}
	rec = e.do(t, http.MethodPost, "/api/settings/webhooks/test?id="+id, nil)
	noLeak(t, e, rec)
	res := decode[obj](t, rec, http.StatusOK)
	if res["status"] != float64(http.StatusNoContent) || res["error"] != nil || e.hookHits.Load() != 1 {
		t.Errorf("testWebhook = %v, hits = %d", res, e.hookHits.Load())
	}
	rec = e.do(t, http.MethodGet, "/api/settings", nil)
	noLeak(t, e, rec)
	wh = decode[obj](t, rec, http.StatusOK)["notify"].(obj)["webhooks"].([]any)[0].(obj)
	if wh["last_status"] != float64(http.StatusNoContent) || wh["last_sent_at"] == nil {
		t.Errorf("delivery status not recorded: %v", wh)
	}

	// Omitting a webhook deletes it.
	v = decode[obj](t, e.do(t, http.MethodPut, "/api/settings/notify", obj{"webhooks": []obj{}}), http.StatusOK)
	if len(v["webhooks"].([]any)) != 0 {
		t.Errorf("webhooks after delete = %v", v)
	}
}

func TestNotifyErrors(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	for _, tt := range []struct {
		name string
		body obj
	}{
		{"disallowed host", obj{"webhooks": []obj{{"name": "x", "url": "https://evil.example/hook/" + secretToken, "enabled": true}}}},
		{"new webhook without url", obj{"webhooks": []obj{{"name": "x", "enabled": true}}}},
		{"unknown id", obj{"webhooks": []obj{{"id": "whk-nope", "name": "x", "enabled": true}}}},
		{"bad public url", obj{"public_url": "ftp://x", "webhooks": []obj{}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := e.do(t, http.MethodPut, "/api/settings/notify", tt.body)
			noLeak(t, e, rec)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
	if rec := e.do(t, http.MethodPost, "/api/settings/webhooks/test?id=whk-nope", nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown webhook test status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(t, http.MethodPost, "/api/settings/webhooks/test", nil); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("missing id status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMuteThread(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	e.register(t, alice, "dev")
	sent, err := e.svc.Send(ctx, e.agentCaller(t, alice), mail.SendInput{To: []string{"operator"}, Subject: "hello", Body: "body"})
	if err != nil {
		t.Fatal(err)
	}
	list := func(q string) []any {
		return decode[obj](t, e.do(t, http.MethodGet, "/api/messages?address=operator&state=inbox"+q, nil), http.StatusOK)["items"].([]any)
	}
	if items := list(""); len(items) != 1 || items[0].(obj)["muted"] != false {
		t.Fatalf("before mute = %v", items)
	}

	m := decode[obj](t, e.do(t, http.MethodPost, "/api/threads/mute?thread_id="+sent.ThreadID, nil), http.StatusOK)
	if m["thread_id"] != sent.ThreadID || m["muted"] != true {
		t.Errorf("muteThread = %v", m)
	}
	if items := list(""); len(items) != 0 {
		t.Errorf("muted thread still listed: %v", items)
	}
	items := list("&include_muted=true")
	if len(items) != 1 || items[0].(obj)["muted"] != true {
		t.Errorf("include_muted = %v", items)
	}
	if msg := decode[obj](t, e.do(t, http.MethodGet, "/api/messages/"+sent.ID, nil), http.StatusOK); msg["muted"] != true {
		t.Errorf("message muted = %v", msg["muted"])
	}
	muted := decode[obj](t, e.do(t, http.MethodGet, "/api/muted-threads", nil), http.StatusOK)["items"].([]any)
	if len(muted) != 1 || muted[0].(obj)["thread_id"] != sent.ThreadID {
		t.Errorf("listMutedThreads = %v", muted)
	}

	m = decode[obj](t, e.do(t, http.MethodPost, "/api/threads/unmute?thread_id="+sent.ThreadID, nil), http.StatusOK)
	if m["muted"] != false || len(list("")) != 1 {
		t.Errorf("unmuteThread = %v", m)
	}
	if got := decode[obj](t, e.do(t, http.MethodGet, "/api/muted-threads", nil), http.StatusOK)["items"].([]any); len(got) != 0 {
		t.Errorf("muted after unmute = %v", got)
	}

	if rec := e.do(t, http.MethodPost, "/api/threads/mute?thread_id=msg-nope", nil); rec.Code != http.StatusNotFound {
		t.Errorf("mute unknown thread status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(t, http.MethodPost, "/api/threads/mute", nil); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("mute without thread_id status = %d", rec.Code)
	}
}

func TestSavedFilters(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	if items := decode[obj](t, e.do(t, http.MethodGet, "/api/filters", nil), http.StatusOK)["items"].([]any); len(items) != 0 {
		t.Fatalf("initial filters = %v", items)
	}
	f := decode[obj](t, e.do(t, http.MethodPost, "/api/filters", obj{"name": "Urgent", "params": obj{"q": "urgent", "state": "unread"}}), http.StatusCreated)
	id, _ := f["id"].(string)
	if id == "" || f["name"] != "Urgent" || f["params"].(obj)["q"] != "urgent" {
		t.Fatalf("createSavedFilter = %v", f)
	}
	f = decode[obj](t, e.do(t, http.MethodPatch, "/api/filters?id="+id, obj{"name": "Renamed"}), http.StatusOK)
	if f["name"] != "Renamed" || f["params"].(obj)["state"] != "unread" {
		t.Errorf("rename = %v", f)
	}
	f = decode[obj](t, e.do(t, http.MethodPatch, "/api/filters?id="+id, obj{"params": obj{"q": "other"}}), http.StatusOK)
	if f["name"] != "Renamed" || len(f["params"].(obj)) != 1 {
		t.Errorf("replace params = %v", f)
	}
	if items := decode[obj](t, e.do(t, http.MethodGet, "/api/filters", nil), http.StatusOK)["items"].([]any); len(items) != 1 {
		t.Errorf("filters = %v", items)
	}

	if rec := e.do(t, http.MethodPost, "/api/filters", obj{"name": "", "params": obj{"q": "x"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("empty name status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(t, http.MethodPatch, "/api/filters?id=flt-nope", obj{"name": "x"}); rec.Code != http.StatusNotFound {
		t.Errorf("update unknown status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(t, http.MethodDelete, "/api/filters?id="+id, nil); rec.Code != http.StatusNoContent {
		t.Errorf("delete status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(t, http.MethodDelete, "/api/filters?id="+id, nil); rec.Code != http.StatusNotFound {
		t.Errorf("delete again status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSearchSegmentsAndFilters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	e.register(t, alice, "dev")
	if _, err := e.svc.Send(ctx, e.agentCaller(t, alice), mail.SendInput{
		To: []string{"operator"}, Subject: "plan", Body: "the <b>rollout</b> starts tomorrow",
	}); err != nil {
		t.Fatal(err)
	}
	rec := e.do(t, http.MethodGet, "/api/search?q=rollout", nil)
	if strings.ContainsAny(rec.Body.String(), "\x02\x03") || strings.Contains(rec.Body.String(), `\u0002`) {
		t.Errorf("markers leaked into the response: %s", rec.Body.String())
	}
	hits := decode[obj](t, rec, http.StatusOK)["hits"].([]any)
	if len(hits) != 1 {
		t.Fatalf("hits = %v", hits)
	}
	hit := hits[0].(obj)
	var plain strings.Builder
	matched := ""
	for _, s := range hit["segments"].([]any) {
		seg := s.(obj)
		plain.WriteString(seg["text"].(string))
		if seg["match"] == true {
			matched += seg["text"].(string)
		}
	}
	if matched != "rollout" || !strings.Contains(plain.String(), "<b>") || !strings.Contains(hit["snippet"].(string), "[rollout]") {
		t.Errorf("segments = %v, snippet = %v", hit["segments"], hit["snippet"])
	}

	count := func(q string) int {
		return len(decode[obj](t, e.do(t, http.MethodGet, "/api/search?q=rollout"+q, nil), http.StatusOK)["hits"].([]any))
	}
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	for _, tt := range []struct {
		name, q string
		want    int
	}{
		{"from sender", "&from=" + alice, 1},
		{"from other", "&from=bob@github.com/acme/web", 0},
		{"since past", "&since=" + past, 1},
		{"since future", "&since=" + future, 0},
		{"until past", "&until=" + past, 0},
		{"until future", "&until=" + future, 1},
		{"state unread", "&state=unread&address=operator", 1},
		{"state read", "&state=read", 0},
		{"state archived", "&state=archived", 0},
		{"state inbox", "&state=inbox", 1},
		{"state all", "&state=all", 1},
	} {
		if got := count(tt.q); got != tt.want {
			t.Errorf("%s: hits = %d, want %d", tt.name, got, tt.want)
		}
	}
	if rec := e.do(t, http.MethodGet, "/api/search?q=rollout&state=bogus", nil); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad state status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(t, http.MethodGet, "/api/search?q=rollout&since="+future+"&until="+past, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("inverted range status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestNewMutatingOperationsRequireGuards(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	for _, tt := range []struct{ method, path string }{
		{http.MethodPut, "/api/settings/retention"},
		{http.MethodPut, "/api/settings/backups"},
		{http.MethodPut, "/api/settings/notify"},
		{http.MethodPost, "/api/settings/webhooks/test?id=x"},
		{http.MethodPost, "/api/settings/backup"},
		{http.MethodPost, "/api/settings/prune?dry_run=true"},
		{http.MethodPost, "/api/threads/mute?thread_id=x"},
		{http.MethodPost, "/api/threads/unmute?thread_id=x"},
		{http.MethodPost, "/api/filters"},
		{http.MethodPatch, "/api/filters?id=x"},
		{http.MethodDelete, "/api/filters?id=x"},
	} {
		for name, hdr := range map[string]http.Header{
			"no headers":       {},
			"no custom header": {"Content-Type": {"application/json"}},
			"wrong type":       {"Content-Type": {"text/plain"}, "X-AIM-Request": {"1"}},
		} {
			req := httptest.NewRequestWithContext(context.Background(), tt.method, tt.path, strings.NewReader("{}"))
			req.Host = "localhost:7172"
			req.Header = hdr
			rec := httptest.NewRecorder()
			e.h.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s %s (%s): status = %d, want 403", tt.method, tt.path, name, rec.Code)
			}
		}
	}
}

func TestPresenceStateAndIdleCount(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, alice, "dev")
	e.register(t, "bob@github.com/acme/web", "dev")
	e.signOn(t, alice, "sess-active", "devbox", "claude-code")
	old := time.Now().Add(-time.Hour)
	stale := e.reg.Add("sess-stale", wire.Preamble{SessionID: "sess-stale", Machine: "box"}, old)
	if err := stale.SignOn(address.MustParse("bob@github.com/acme/web"), "", old); err != nil {
		t.Fatal(err)
	}

	p := decode[obj](t, e.do(t, http.MethodGet, "/api/presence", nil), http.StatusOK)
	g := p["groups"].([]any)[0].(obj)
	if g["online"] != float64(2) || g["idle"] != float64(1) || g["total"] != float64(2) {
		t.Fatalf("group = %v", g)
	}
	states := map[string]string{}
	for _, b := range g["buddies"].([]any) {
		bd := b.(obj)
		if bd["last_active"] == nil {
			t.Errorf("buddy %v has no last_active", bd["address"])
		}
		states[bd["address"].(string)], _ = bd["state"].(string)
	}
	if states[alice] != "online" || states["bob@github.com/acme/web"] != "idle" {
		t.Errorf("states = %v", states)
	}
}

// newBare returns an env whose API has no maintenance runner or notifier.
func newBare(t *testing.T, base *env) *env {
	t.Helper()
	return &env{h: httpapi.New(httpapi.Deps{Pinger: fakePinger{}, Mail: base.svc, Presence: base.reg, Version: "test"}), svc: base.svc, reg: base.reg}
}

func TestSettingsUnavailableWithoutCollaborators(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	bare := newBare(t, e)
	for _, tt := range []struct{ method, path string }{
		{http.MethodGet, "/api/settings"},
		{http.MethodPut, "/api/settings/notify"},
		{http.MethodPost, "/api/settings/webhooks/test?id=x"},
		{http.MethodPost, "/api/settings/backup"},
		{http.MethodPost, "/api/settings/prune"},
	} {
		if rec := bare.do(t, tt.method, tt.path, obj{"webhooks": []obj{}}); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s: status = %d, want 503: %s", tt.method, tt.path, rec.Code, rec.Body.String())
		}
	}
}
