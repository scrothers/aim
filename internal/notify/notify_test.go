package notify_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/notify"
	"github.com/scrothers/aim/internal/store"
)

const (
	alice  = "alice@github.com/acme/web"
	secret = "s3cr3t-token-value"
)

// testCheck routes by path so httptest servers can pose as either service.
func testCheck(u *url.URL) (notify.Kind, error) {
	switch {
	case strings.HasPrefix(u.Path, "/discord/"):
		return notify.KindDiscord, nil
	case strings.HasPrefix(u.Path, "/slack/"):
		return notify.KindSlack, nil
	}
	return "", errors.New("unsupported test host")
}

type request struct {
	path string
	body []byte
	at   time.Time
}

// receiver is a fake webhook endpoint with a scripted response queue.
type receiver struct {
	srv *httptest.Server

	mu      sync.Mutex
	reqs    []request
	script  []func(http.ResponseWriter) // consumed one per request; empty means 204
	arrived chan struct{}
}

func newReceiver(t *testing.T) *receiver {
	t.Helper()
	r := &receiver{arrived: make(chan struct{}, 100)}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.reqs = append(r.reqs, request{req.URL.Path, b, time.Now()})
		var f func(http.ResponseWriter)
		if len(r.script) > 0 {
			f, r.script = r.script[0], r.script[1:]
		}
		r.mu.Unlock()
		if f != nil {
			f(w)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
		r.arrived <- struct{}{}
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func status(code int) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) { w.WriteHeader(code) }
}

func (r *receiver) url(kind string) string { return r.srv.URL + "/" + kind + "/" + secret }

func (r *receiver) wait(t *testing.T, n int) []request {
	t.Helper()
	for range n {
		select {
		case <-r.arrived:
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for request (got %d of %d)", r.count(), n)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]request(nil), r.reqs...)
}

func (r *receiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reqs)
}

func (r *receiver) none(t *testing.T) {
	t.Helper()
	select {
	case <-r.arrived:
		t.Fatalf("unexpected request: %s", r.reqs[len(r.reqs)-1].body)
	case <-time.After(150 * time.Millisecond):
	}
}

type env struct {
	svc *mail.Service
	mgr *notify.Manager
}

func newEnv(t *testing.T, opts ...notify.Option) *env {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "aim.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc := mail.New(st, nil)
	if err := svc.EnsureOperator(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Register(ctx, mail.Caller{}, mail.RegisterInput{Address: alice, Role: "dev", Description: "d"}); err != nil {
		t.Fatal(err)
	}
	all := append([]notify.Option{
		notify.WithHostCheck(testCheck), notify.WithCoalesce(300 * time.Millisecond), notify.WithBackoff(time.Millisecond),
	}, opts...)
	mgr := notify.New(svc, slog.New(slog.NewTextHandler(io.Discard, nil)), all...)
	svc.SetOnOperatorDelivery(mgr.Handle)
	done := make(chan struct{})
	go func() { mgr.Run(ctx); close(done) }()
	t.Cleanup(func() {
		cancel()
		<-done
		_ = st.Close()
	})
	return &env{svc: svc, mgr: mgr}
}

func (e *env) configure(t *testing.T, publicURL string, hooks ...mail.Webhook) mail.NotifySettings {
	t.Helper()
	n, err := e.mgr.UpdateConfig(context.Background(), mail.NotifySettings{PublicURL: publicURL, Webhooks: hooks})
	if err != nil {
		t.Fatal(err)
	}
	cur, err := e.svc.Notify(context.Background())
	if err != nil || len(cur.Webhooks) != len(n.Webhooks) {
		t.Fatalf("notify = %+v, %v", cur, err)
	}
	return cur
}

func (e *env) sendOp(t *testing.T, subject, body string) mail.Message {
	t.Helper()
	c := mail.Caller{SessionID: "s", Identity: address.MustParse(alice), SignedOn: true, Client: "claude-code", Machine: "devbox"}
	m, err := e.svc.Send(context.Background(), c, mail.SendInput{To: []string{"operator"}, Subject: subject, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("payload %q: %v", b, err)
	}
	return v
}

func TestDiscordCard(t *testing.T) {
	t.Parallel()
	rc := newReceiver(t)
	e := newEnv(t)
	e.configure(t, "http://devbox:7172", mail.Webhook{Name: "team", URL: rc.url("discord"), Enabled: true})
	m := e.sendOp(t, "Need approval", "Please **approve** the deploy.")
	reqs := rc.wait(t, 1)
	if reqs[0].path != "/discord/"+secret {
		t.Errorf("path = %s", reqs[0].path)
	}
	p := decode(t, reqs[0].body)
	embeds, _ := p["embeds"].([]any)
	if len(embeds) != 1 {
		t.Fatalf("payload = %s", reqs[0].body)
	}
	em := embeds[0].(map[string]any)
	if em["title"] != "Need approval" || !strings.Contains(em["description"].(string), "approve the deploy") ||
		strings.Contains(em["description"].(string), "**") {
		t.Errorf("embed = %v", em)
	}
	if want := "http://devbox:7172/mail?view=operator&thread=" + m.ThreadID; em["url"] != want {
		t.Errorf("url = %v, want %s", em["url"], want)
	}
	footer := em["footer"].(map[string]any)["text"].(string)
	if !strings.Contains(footer, alice) || !strings.Contains(footer, "claude-code") || !strings.Contains(footer, "devbox") {
		t.Errorf("footer = %q", footer)
	}
	if am := p["allowed_mentions"].(map[string]any)["parse"].([]any); len(am) != 0 {
		t.Errorf("allowed_mentions = %v", am)
	}
	// Delivery status is recorded.
	waitFor(t, func() bool {
		n, _ := e.svc.Notify(context.Background())
		return n.Webhooks[0].LastStatus == http.StatusNoContent && n.Webhooks[0].LastSentAt != nil && n.Webhooks[0].LastError == ""
	})
}

func TestSlackCard(t *testing.T) {
	t.Parallel()
	rc := newReceiver(t)
	e := newEnv(t)
	e.configure(t, "http://devbox:7172", mail.Webhook{Name: "team", URL: rc.url("slack"), Enabled: true})
	m := e.sendOp(t, "Q <@here> & more", "body text")
	reqs := rc.wait(t, 1)
	p := decode(t, reqs[0].body)
	blocks, _ := p["blocks"].([]any)
	if len(blocks) != 2 {
		t.Fatalf("payload = %s", reqs[0].body)
	}
	text := blocks[0].(map[string]any)["text"].(map[string]any)["text"].(string)
	if !strings.Contains(text, "&lt;@here&gt; &amp; more") || strings.Contains(text, "<@here>") ||
		!strings.Contains(text, "<http://devbox:7172/mail?view=operator&thread="+m.ThreadID+"|") || !strings.Contains(text, "body text") {
		t.Errorf("text = %q", text)
	}
}

func TestSkipsOwnMutedDisabledAndNonOperatorMail(t *testing.T) {
	t.Parallel()
	rc := newReceiver(t)
	off := newReceiver(t)
	e := newEnv(t, notify.WithCoalesce(10*time.Millisecond))
	e.configure(t, "http://h", mail.Webhook{Name: "on", URL: rc.url("discord"), Enabled: true},
		mail.Webhook{Name: "off", URL: off.url("slack"), Enabled: false})
	ctx := context.Background()

	// Operator mailing itself never notifies.
	if _, err := e.svc.Send(ctx, mail.OperatorCaller("ui", "aim-web", "devbox"), mail.SendInput{To: []string{"operator"}, Subject: "note to self", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	// Agent to agent mail does not notify.
	c := mail.Caller{SessionID: "s", Identity: address.MustParse(alice), SignedOn: true}
	if _, err := e.svc.Send(ctx, c, mail.SendInput{To: []string{alice}, Subject: "to self", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	rc.none(t)

	// Muted threads are quiet, including replies to them.
	m := e.sendOp(t, "first", "b")
	rc.wait(t, 1)
	if err := e.svc.MuteThread(ctx, m.ThreadID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Send(ctx, c, mail.SendInput{To: []string{"operator"}, InReplyTo: m.ID, Subject: "Re: first", Body: "again"}); err != nil {
		t.Fatal(err)
	}
	rc.none(t)
	if err := e.svc.UnmuteThread(ctx, m.ThreadID); err != nil {
		t.Fatal(err)
	}
	e.sendOp(t, "after unmute", "b")
	rc.wait(t, 1)
	off.none(t) // disabled webhook never posts
}

func TestCoalescesBursts(t *testing.T) {
	t.Parallel()
	rc := newReceiver(t)
	e := newEnv(t, notify.WithCoalesce(400*time.Millisecond))
	e.configure(t, "http://h", mail.Webhook{Name: "w", URL: rc.url("discord"), Enabled: true})
	e.sendOp(t, "one", "b")
	rc.wait(t, 1)
	for _, s := range []string{"two", "three", "four"} {
		e.sendOp(t, s, "b")
	}
	rc.wait(t, 1)
	reqs := rc.wait(t, 0)
	if len(reqs) != 2 {
		t.Fatalf("posts = %d, want 2 (one card, one summary)", len(reqs))
	}
	if gap := reqs[1].at.Sub(reqs[0].at); gap < 350*time.Millisecond {
		t.Errorf("posts %v apart, want >= coalescing window", gap)
	}
	em := decode(t, reqs[1].body)["embeds"].([]any)[0].(map[string]any)
	if em["title"] != "3 new messages for the operator" {
		t.Errorf("title = %v", em["title"])
	}
	d := em["description"].(string)
	for _, s := range []string{"two", "three", "four", alice} {
		if !strings.Contains(d, s) {
			t.Errorf("summary missing %q: %q", s, d)
		}
	}
	if em["url"] != "http://h/mail?view=operator" {
		t.Errorf("url = %v", em["url"])
	}
	rc.none(t)
}

func TestRetriesAndRecordsStatus(t *testing.T) {
	t.Parallel()
	rc := newReceiver(t)
	rc.script = []func(http.ResponseWriter){status(500), status(502)}
	e := newEnv(t)
	e.configure(t, "", mail.Webhook{Name: "w", URL: rc.url("discord"), Enabled: true})
	e.sendOp(t, "retry me", "b")
	if reqs := rc.wait(t, 3); len(reqs) != 3 {
		t.Fatalf("requests = %d", len(reqs))
	}
	waitFor(t, func() bool {
		n, _ := e.svc.Notify(context.Background())
		return n.Webhooks[0].LastStatus == http.StatusNoContent && n.Webhooks[0].LastError == ""
	})
}

func TestHonorsRetryAfter(t *testing.T) {
	t.Parallel()
	for name, resp := range map[string]func(http.ResponseWriter){
		"discord body": func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"message":"You are being rate limited.","retry_after":0.3,"global":false}`)
		},
		"slack header": func(w http.ResponseWriter) {
			w.Header().Set("Retry-After", "0.3")
			w.WriteHeader(http.StatusTooManyRequests)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rc := newReceiver(t)
			rc.script = []func(http.ResponseWriter){resp}
			e := newEnv(t)
			e.configure(t, "", mail.Webhook{Name: "w", URL: rc.url("discord"), Enabled: true})
			e.sendOp(t, "limited", "b")
			reqs := rc.wait(t, 2)
			if gap := reqs[1].at.Sub(reqs[0].at); gap < 250*time.Millisecond {
				t.Errorf("retried after %v, want >= retry_after (backoff is 1ms)", gap)
			}
		})
	}
}

func TestPermanentFailureIsNotRetried(t *testing.T) {
	t.Parallel()
	rc := newReceiver(t)
	rc.script = []func(http.ResponseWriter){func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "no_service")
	}}
	e := newEnv(t)
	e.configure(t, "", mail.Webhook{Name: "w", URL: rc.url("slack"), Enabled: true})
	e.sendOp(t, "x", "b")
	rc.wait(t, 1)
	waitFor(t, func() bool {
		n, _ := e.svc.Notify(context.Background())
		w := n.Webhooks[0]
		return w.LastStatus == http.StatusNotFound && strings.Contains(w.LastError, "HTTP 404: no_service")
	})
	rc.none(t)
}

func TestErrorsNeverLeakTheURL(t *testing.T) {
	t.Parallel()
	rc := newReceiver(t)
	target := rc.url("discord")
	rc.srv.Close() // connection refused
	e := newEnv(t)
	e.configure(t, "", mail.Webhook{Name: "w", URL: target, Enabled: true})
	res, err := e.mgr.SendTest(context.Background(), webhookID(t, e))
	if err != nil {
		t.Fatal(err)
	}
	if res.OK() || res.Status != 0 || strings.Contains(res.Error, secret) {
		t.Errorf("result = %+v", res)
	}
	n, _ := e.svc.Notify(context.Background())
	if strings.Contains(n.Webhooks[0].LastError, secret) || n.Webhooks[0].LastError == "" {
		t.Errorf("stored error = %q", n.Webhooks[0].LastError)
	}
}

func webhookID(t *testing.T, e *env) string {
	t.Helper()
	n, err := e.svc.Notify(context.Background())
	if err != nil || len(n.Webhooks) == 0 {
		t.Fatalf("no webhook: %+v, %v", n, err)
	}
	return n.Webhooks[0].ID
}

func TestSendTest(t *testing.T) {
	t.Parallel()
	rc := newReceiver(t)
	rc.script = []func(http.ResponseWriter){status(http.StatusInternalServerError)}
	e := newEnv(t)
	e.configure(t, "http://h", mail.Webhook{Name: "w", URL: rc.url("discord"), Enabled: false})
	id := webhookID(t, e)
	res, err := e.mgr.SendTest(context.Background(), id) // works even when disabled; single attempt
	if err != nil || res.OK() || res.Status != 500 {
		t.Fatalf("first = %+v, %v", res, err)
	}
	if n := rc.count(); n != 1 {
		t.Errorf("requests = %d, want 1 (no retries for a test)", n)
	}
	res, err = e.mgr.SendTest(context.Background(), id)
	if err != nil || !res.OK() || res.Status != 204 {
		t.Fatalf("second = %+v, %v", res, err)
	}
	em := decode(t, rc.wait(t, 0)[1].body)["embeds"].([]any)[0].(map[string]any)
	if em["title"] != "AIM test notification" {
		t.Errorf("title = %v", em["title"])
	}
	n, _ := e.svc.Notify(context.Background())
	if n.Webhooks[0].LastStatus != 204 || n.Webhooks[0].LastError != "" {
		t.Errorf("recorded = %+v", n.Webhooks[0])
	}
	if _, err := e.mgr.SendTest(context.Background(), "whk-missing"); !errors.Is(err, mail.ErrNotFound) {
		t.Errorf("unknown id err = %v", err)
	}
}

func TestConfigMasksAndValidates(t *testing.T) {
	t.Parallel()
	e := newEnv(t, notify.WithHostCheck(func(u *url.URL) (notify.Kind, error) {
		if u.Hostname() == "bad.example" {
			return "", errors.New("host not allowed")
		}
		return notify.KindDiscord, nil
	}), notify.WithDefaultPublicURL("http://box:7172"))
	ctx := context.Background()
	if _, err := e.mgr.UpdateConfig(ctx, mail.NotifySettings{Webhooks: []mail.Webhook{{Name: "x", URL: "https://bad.example/hook", Enabled: true}}}); !errors.Is(err, mail.ErrInvalidInput) {
		t.Fatalf("bad host err = %v", err)
	}
	v, err := e.mgr.UpdateConfig(ctx, mail.NotifySettings{Webhooks: []mail.Webhook{
		{Name: "d", URL: "https://discord.com/api/webhooks/123/" + secret, Enabled: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if v.DefaultPublicURL != "http://box:7172" || v.PublicURL != "" {
		t.Errorf("view = %+v", v)
	}
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), secret) || !strings.Contains(string(b), "https://discord.com/api/webhooks/…/****") {
		t.Errorf("view JSON leaks or is unmasked: %s", b)
	}
	// Edit without a URL: keeps the secret, never echoes it.
	v2, err := e.mgr.UpdateConfig(ctx, mail.NotifySettings{PublicURL: "https://aim.example", Webhooks: []mail.Webhook{{ID: v.Webhooks[0].ID, Name: "renamed", Enabled: false}}})
	if err != nil {
		t.Fatal(err)
	}
	if v2.Webhooks[0].Name != "renamed" || v2.Webhooks[0].Enabled || v2.PublicURL != "https://aim.example" {
		t.Errorf("v2 = %+v", v2)
	}
	cur, _ := e.svc.Notify(ctx)
	if !strings.HasSuffix(cur.Webhooks[0].URL, secret) {
		t.Errorf("stored URL lost: %q", cur.Webhooks[0].URL)
	}
	got, err := e.mgr.Config(ctx)
	if err != nil || got.Webhooks[0].URL != v2.Webhooks[0].URL {
		t.Errorf("Config = %+v, %v", got, err)
	}
}

func TestSeedWebhooks(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	ok, err := e.mgr.SeedWebhooks(ctx, []string{"https://hooks.slack.com/services/T/B/x", "https://example.com/nope"})
	// The test host check accepts only /discord|/slack paths, so both are skipped.
	if err != nil || ok {
		t.Fatalf("seed with test check = %v, %v", ok, err)
	}
	if done, _ := e.svc.NotifyConfigured(ctx); !done {
		t.Fatal("first start must mark settings configured")
	}
	// Later starts never re-seed, even with valid URLs.
	if ok, err := e.mgr.SeedWebhooks(ctx, []string{"http://x/discord/abc"}); err != nil || ok {
		t.Errorf("re-seed = %v, %v", ok, err)
	}

	e2 := newEnv(t)
	ok, err = e2.mgr.SeedWebhooks(ctx, []string{"http://x/discord/abc", "junk", "http://y/slack/def"})
	if err != nil || !ok {
		t.Fatalf("seed = %v, %v", ok, err)
	}
	n, _ := e2.svc.Notify(ctx)
	if len(n.Webhooks) != 2 || !n.Webhooks[0].Enabled || n.Webhooks[0].Name != "env-1" || n.Webhooks[1].Name != "env-3" {
		t.Errorf("seeded = %+v", n.Webhooks)
	}
}

func TestHandleNeverBlocks(t *testing.T) {
	t.Parallel()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "aim.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	m := notify.New(mail.New(st, nil), slog.New(slog.NewTextHandler(io.Discard, nil))) // Run is not started
	done := make(chan struct{})
	go func() {
		for range 1000 {
			m.Handle(mail.Message{ID: "msg-1", ThreadID: "msg-1", From: alice})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Handle blocked on a full queue")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
