package devwatch

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// lockedBuf is a bytes.Buffer safe for the daemon-wait goroutine and the test.
type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

const helperEnv = "DEVWATCH_FAKE_DAEMON"

// TestMain lets the test binary double as a fake daemon.
func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "":
		os.Exit(m.Run())
	case "exit":
		os.Exit(3)
	default: // "serve", "stubborn"
		os.Exit(fakeDaemon(os.Getenv(helperEnv) == "stubborn"))
	}
}

func fakeDaemon(stubborn bool) int {
	if stubborn {
		ignoreTerm()
	}
	os.Stdout.WriteString("up\n")
	blockForever()
	return 0
}

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func bump(t *testing.T, path string, at time.Time) {
	t.Helper()
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

func TestWatcherDebounce(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "a.go")
	write(t, f)
	base := time.Now()
	w := NewWatcher(root, 300*time.Millisecond)

	if w.Poll(base) {
		t.Fatal("no change yet")
	}
	bump(t, f, base.Add(time.Hour))
	if w.Poll(base.Add(100 * time.Millisecond)) {
		t.Fatal("must wait for debounce")
	}
	bump(t, f, base.Add(2*time.Hour)) // another edit resets the window
	if w.Poll(base.Add(350 * time.Millisecond)) {
		t.Fatal("debounce should have restarted")
	}
	if !w.Poll(base.Add(700 * time.Millisecond)) {
		t.Fatal("expected change after quiet period")
	}
	if w.Poll(base.Add(2 * time.Second)) {
		t.Fatal("change reported twice")
	}
}

func TestWatcherDetectsAddAndRemove(t *testing.T) {
	root := t.TempDir()
	w := NewWatcher(root, 0)
	now := time.Now()
	f := filepath.Join(root, "go.mod")
	write(t, f)
	if !w.Poll(now) {
		t.Fatal("added file not detected")
	}
	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	if !w.Poll(now.Add(time.Second)) {
		t.Fatal("removed file not detected")
	}
}

func TestWatcherSkipRules(t *testing.T) {
	root := t.TempDir()
	w := NewWatcher(root, 0)
	now := time.Now()
	ignored := []string{
		"x_test.go", "README.md", ".git/a.go", ".dev/a.go", "web/a.go",
		"web/node_modules/a.go", "sub/node_modules/a.go",
		"internal/webui/dist/a.go", "test/e2e/a.go",
	}
	for _, p := range ignored {
		write(t, filepath.Join(root, p))
	}
	if w.Poll(now) {
		t.Fatal("ignored files triggered a change")
	}
	for _, p := range []string{"cmd/aim/main.go", "go.sum", "internal/webui/embed.go"} {
		write(t, filepath.Join(root, p))
		if !w.Poll(now) {
			t.Fatalf("%s should trigger", p)
		}
	}
}

func testManager(t *testing.T, mode string, buildFails bool, out *lockedBuf) *Manager {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Root:        t.TempDir(),
		Bin:         self,
		StopTimeout: 300 * time.Millisecond,
		Stdout:      io.Discard,
		Stderr:      out,
		Logger:      slog.New(slog.NewTextHandler(out, nil)),
		BuildCommand: func(ctx context.Context) *exec.Cmd {
			if buildFails {
				return exec.CommandContext(ctx, "sh", "-c", "echo compile-error >&2; exit 1")
			}
			return exec.CommandContext(ctx, "true")
		},
	}
	t.Setenv(helperEnv, mode)
	return NewManager(cfg)
}

func TestRestartReplacesProcess(t *testing.T) {
	var out lockedBuf
	m := testManager(t, "serve", false, &out)
	t.Cleanup(m.Stop)
	ctx := t.Context()
	if err := m.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	first := m.cur
	if err := m.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first.done:
	default:
		t.Fatal("old daemon still running")
	}
	if m.cur == first || m.count != 2 || !strings.Contains(out.String(), "restarted (2)") {
		t.Fatalf("bad state count=%d log=%s", m.count, out.String())
	}
}

func TestBuildFailureKeepsDaemon(t *testing.T) {
	var out lockedBuf
	m := testManager(t, "serve", false, &out)
	t.Cleanup(m.Stop)
	if err := m.Rebuild(t.Context()); err != nil {
		t.Fatal(err)
	}
	running := m.cur
	m.cfg.BuildCommand = func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "echo compile-error >&2; exit 1")
	}
	if err := m.Rebuild(t.Context()); err == nil {
		t.Fatal("expected build error")
	}
	select {
	case <-running.done:
		t.Fatal("daemon was stopped despite build failure")
	default:
	}
	if m.cur != running || !strings.Contains(out.String(), "compile-error") {
		t.Fatalf("unexpected state, log=%s", out.String())
	}
}

func TestStopEscalatesToKill(t *testing.T) {
	var out lockedBuf
	m := testManager(t, "stubborn", false, &out)
	if err := m.Rebuild(t.Context()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond) // let the child install its handler
	p := m.cur
	m.Stop()
	select {
	case <-p.done:
	default:
		t.Fatal("daemon not stopped")
	}
	if !strings.Contains(out.String(), "SIGKILL") {
		t.Fatalf("missing escalation log: %s", out.String())
	}
}

func TestSelfExitDoesNotRestart(t *testing.T) {
	var out lockedBuf
	m := testManager(t, "exit", false, &out)
	t.Cleanup(m.Stop)
	if err := m.Rebuild(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-m.cur.done:
	case <-time.After(time.Second):
		t.Fatal("daemon did not exit")
	}
	if m.count != 1 || !strings.Contains(out.String(), "daemon exited") {
		t.Fatalf("count=%d log=%s", m.count, out.String())
	}
	m.Stop() // must not hang or panic on an already-exited daemon
}
