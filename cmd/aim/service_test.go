package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/service"
)

type fakeRunner struct {
	calls [][]string
	fail  string
}

func (f *fakeRunner) run(_ context.Context, argv []string) error {
	f.calls = append(f.calls, argv)
	if f.fail != "" && strings.Contains(strings.Join(argv, " "), f.fail) {
		return errors.New("boom")
	}
	return nil
}

func newTestSystemd(t *testing.T) *service.Systemd {
	t.Helper()
	return service.NewSystemd(t.TempDir())
}

func testServiceCfg(env map[string]string) serviceConfig {
	return serviceConfig{Bin: "/usr/local/bin/aim", Env: env, Home: "/home/u"}
}

func TestServiceInstallFlow(t *testing.T) {
	t.Parallel()
	mgr := newTestSystemd(t)
	r := &fakeRunner{}
	var healthURL string
	deps := serviceDeps{
		Run:       r.run,
		Health:    func(_ context.Context, u string) error { healthURL = u; return nil },
		LingerOff: func(context.Context) bool { return true },
	}
	var out bytes.Buffer
	cfg := testServiceCfg(map[string]string{"AIM_HTTP": "127.0.0.1:27172"})
	if err := runServiceInstall(t.Context(), &out, cfg, mgr, deps); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(mgr.Path())
	if err != nil || !strings.Contains(string(data), "ExecStart=/usr/local/bin/aim daemon") ||
		!strings.Contains(string(data), "Environment=AIM_HTTP=127.0.0.1:27172") {
		t.Fatalf("unit not written: %v\n%s", err, data)
	}
	want := [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "--now", "aim"},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls = %v, want %v", r.calls, want)
	}
	if healthURL != "http://127.0.0.1:27172/healthz" {
		t.Errorf("health url = %q", healthURL)
	}
	if s := out.String(); !strings.Contains(s, "created") || !strings.Contains(s, "console at http://localhost:27172") ||
		!strings.Contains(s, "enable-linger") {
		t.Errorf("output: %s", s)
	}

	// Same spec again: nothing rewritten, no daemon-reload, no restart.
	before, err := os.Stat(mgr.Path())
	if err != nil {
		t.Fatal(err)
	}
	r.calls = nil
	out.Reset()
	if err := runServiceInstall(t.Context(), &out, cfg, mgr, deps); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(mgr.Path())
	if err != nil || !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("unchanged install rewrote the file (%v)", err)
	}
	if want := [][]string{{"systemctl", "--user", "enable", "--now", "aim"}}; !reflect.DeepEqual(r.calls, want) {
		t.Errorf("rerun calls = %v, want %v", r.calls, want)
	}
	if !strings.Contains(out.String(), "unchanged") {
		t.Errorf("output: %s", out.String())
	}

	// Changed env: rewrite and restart.
	r.calls = nil
	cfg.Env = map[string]string{"AIM_HTTP": "127.0.0.1:27173"}
	if err := runServiceInstall(t.Context(), &out, cfg, mgr, deps); err != nil {
		t.Fatal(err)
	}
	want = [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "--now", "aim"},
		{"systemctl", "--user", "restart", "aim"},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("changed calls = %v, want %v", r.calls, want)
	}
}

func TestServiceInstallDryRunWritesNothing(t *testing.T) {
	t.Parallel()
	mgr := newTestSystemd(t)
	r := &fakeRunner{}
	var out bytes.Buffer
	cfg := testServiceCfg(nil)
	cfg.DryRun = true
	if err := runServiceInstall(t.Context(), &out, cfg, mgr, serviceDeps{Run: r.run}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mgr.Path()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dry run wrote the unit: %v", err)
	}
	if len(r.calls) != 0 {
		t.Errorf("dry run ran %v", r.calls)
	}
	if s := out.String(); !strings.Contains(s, "would write") || !strings.Contains(s, "would run systemctl --user enable --now aim") {
		t.Errorf("output: %s", s)
	}
}

func TestServiceInstallNoStartAndFailures(t *testing.T) {
	t.Parallel()
	mgr := newTestSystemd(t)
	r := &fakeRunner{}
	var out bytes.Buffer
	cfg := testServiceCfg(nil)
	cfg.NoStart = true
	if err := runServiceInstall(t.Context(), &out, cfg, mgr, serviceDeps{Run: r.run}); err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{"systemctl", "--user", "daemon-reload"}, {"systemctl", "--user", "enable", "aim"}}; !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls = %v", r.calls)
	}
	if !strings.Contains(out.String(), "not started") {
		t.Errorf("output: %s", out.String())
	}

	bad := &fakeRunner{fail: "daemon-reload"}
	cfg.NoStart = false
	cfg.Env = map[string]string{"AIM_X": "1"}
	err := runServiceInstall(t.Context(), &out, cfg, mgr, serviceDeps{Run: bad.run})
	if err == nil || !strings.Contains(err.Error(), "daemon-reload") {
		t.Errorf("err = %v", err)
	}
}

func TestServiceInstallHealthFailure(t *testing.T) {
	t.Parallel()
	mgr := newTestSystemd(t)
	var out bytes.Buffer
	deps := serviceDeps{
		Run:    (&fakeRunner{}).run,
		Health: func(context.Context, string) error { return errors.New("refused") },
	}
	if err := runServiceInstall(t.Context(), &out, testServiceCfg(nil), mgr, deps); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "did not answer") || !strings.Contains(out.String(), "aim service logs") {
		t.Errorf("output: %s", out.String())
	}
}

func TestServiceInstallRejectsTempBinary(t *testing.T) {
	t.Parallel()
	for _, bin := range []string{"/home/u/.cache/go-build/ab/exe/aim", "/tmp/go-build123/b001/exe/aim", os.TempDir() + "/aim", "relative/aim"} {
		cfg := testServiceCfg(nil)
		cfg.Bin = bin
		r := &fakeRunner{}
		err := runServiceInstall(t.Context(), &bytes.Buffer{}, cfg, newTestSystemd(t), serviceDeps{Run: r.run})
		if err == nil {
			t.Errorf("%s accepted", bin)
		}
		if len(r.calls) != 0 {
			t.Errorf("%s ran commands", bin)
		}
	}
}

func TestParseServiceEnv(t *testing.T) {
	t.Parallel()
	got, err := parseServiceEnv([]string{"AIM_HTTP=:1", "AIM_LISTEN=", "AIM_X=a=b"})
	if err != nil || !reflect.DeepEqual(got, map[string]string{"AIM_HTTP": ":1", "AIM_LISTEN": "", "AIM_X": "a=b"}) {
		t.Errorf("got %v, %v", got, err)
	}
	for _, bad := range []string{"HTTP=:1", "AIM_HTTP", "AIM_=1", "AIM_a=1", "AIM_X=a\nb", ""} {
		if _, err := parseServiceEnv([]string{bad}); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestHealthURLs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		env             map[string]string
		health, console string
		ok              bool
	}{
		{nil, "http://127.0.0.1:7172/healthz", "http://localhost:7172", true},
		{map[string]string{"AIM_HTTP": "0.0.0.0:9"}, "http://127.0.0.1:9/healthz", "http://localhost:9", true},
		{map[string]string{"AIM_HTTP": "10.0.0.5:9"}, "http://10.0.0.5:9/healthz", "http://localhost:9", true},
		{map[string]string{"AIM_HTTP": ""}, "", "", false},
		{map[string]string{"AIM_HTTP": "junk"}, "", "", false},
	}
	for _, tc := range tests {
		h, c, ok := healthURLs(tc.env)
		if h != tc.health || c != tc.console || ok != tc.ok {
			t.Errorf("%v: got %q %q %v", tc.env, h, c, ok)
		}
	}
}

func TestServiceUninstall(t *testing.T) {
	t.Parallel()
	mgr := newTestSystemd(t)
	r := &fakeRunner{}
	var out bytes.Buffer
	deps := serviceDeps{Run: r.run}

	if err := runServiceUninstall(t.Context(), &out, false, mgr, deps); err != nil || !strings.Contains(out.String(), "not installed") {
		t.Fatalf("missing unit: %v %s", err, out.String())
	}
	if len(r.calls) != 0 {
		t.Errorf("ran %v for a missing unit", r.calls)
	}

	if err := runServiceInstall(t.Context(), &out, testServiceCfg(nil), mgr, serviceDeps{Run: r.run}); err != nil {
		t.Fatal(err)
	}
	r.calls = nil
	out.Reset()
	if err := runServiceUninstall(t.Context(), &out, true, mgr, deps); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mgr.Path()); err != nil || len(r.calls) != 0 {
		t.Errorf("dry run changed state: %v %v", err, r.calls)
	}

	out.Reset()
	if err := runServiceUninstall(t.Context(), &out, false, mgr, deps); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mgr.Path()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("unit still present: %v", err)
	}
	want := [][]string{{"systemctl", "--user", "disable", "--now", "aim"}, {"systemctl", "--user", "daemon-reload"}}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls = %v", r.calls)
	}
	if !strings.Contains(out.String(), "database and backups were not touched") {
		t.Errorf("output: %s", out.String())
	}
}

func TestRunServiceCmdsIgnoreErr(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{fail: "bootout"}
	cmds := service.NewLaunchd("/h", 1).Install(service.InstallOpts{Changed: true})
	if err := runServiceCmds(t.Context(), cmds, r.run); err != nil {
		t.Errorf("ignorable failure surfaced: %v", err)
	}
	if len(r.calls) != 2 {
		t.Errorf("calls = %v", r.calls)
	}
	if err := runServiceCmds(t.Context(), service.NewLaunchd("/h", 1).Start(), (&fakeRunner{fail: "kickstart"}).run); err == nil {
		t.Error("start failure swallowed")
	}
}

func TestWaitHealthy(t *testing.T) {
	t.Parallel()
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(ok.Close)
	if err := waitHealthy(t.Context(), ok.URL, 2*time.Second); err != nil {
		t.Errorf("healthy server: %v", err)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	t.Cleanup(bad.Close)
	if err := waitHealthy(t.Context(), bad.URL, 300*time.Millisecond); err == nil {
		t.Error("unhealthy server reported healthy")
	}
}

func TestServiceCmdRegistered(t *testing.T) {
	t.Parallel()
	c, _, err := newRootCmd().Find([]string{"service", "logs"})
	if err != nil || c.Name() != "logs" {
		t.Fatalf("find: %v %v", c, err)
	}
}
