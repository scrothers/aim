package hooks_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/hooks"
	"github.com/scrothers/aim/internal/tasks"
	"github.com/scrothers/aim/internal/wire"
)

type fakeTasks struct {
	mu      sync.Mutex
	touched []string
	counts  tasks.Counts
	err     error
}

func (f *fakeTasks) Touch(_ context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.touched = append(f.touched, sessionID)
	return f.err
}

func (f *fakeTasks) Counts(_ context.Context, identity string) (tasks.Counts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if identity != alice {
		return tasks.Counts{}, errors.New("unexpected identity " + identity)
	}
	return f.counts, f.err
}

func (f *fakeTasks) touches() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.touched...)
}

func TestHooksTouchTasksAndNudge(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ft := &fakeTasks{counts: tasks.Counts{Held: 2, Stale: 1, Assigned: 1}}
	e.srv = hooks.New(e.svc, e.reg, hooks.WithDebounce(10*time.Millisecond), hooks.WithClock(e.clock), hooks.WithTasks(ft))
	e.connect(t, 21, alice)
	p := hook(21)
	p.SessionID = "sess-21"

	r := e.event(t, p, wire.HookRequest{Event: wire.HookSessionStart})
	const want = "You hold 2 tasks (1 stale) and have 1 assigned task to claim; `task_list` shows them."
	if !strings.Contains(r.Context, want) || !strings.Contains(r.Context, "signed on to AIM") {
		t.Errorf("session start context = %q", r.Context)
	}
	if r := e.event(t, p, wire.HookRequest{Event: wire.HookPrompt}); strings.TrimSpace(r.Context) != want {
		t.Errorf("prompt context = %q", r.Context)
	}
	if r := e.event(t, p, wire.HookRequest{Event: wire.HookPostTool}); r.Context != "" {
		t.Errorf("post-tool context = %q", r.Context)
	}
	if got := ft.touches(); len(got) != 3 || got[0] != "sess-21" {
		t.Errorf("touches = %v", got)
	}
	// No session id: nothing to touch.
	e.event(t, hook(21), wire.HookRequest{Event: wire.HookPreTool})
	if got := ft.touches(); len(got) != 3 {
		t.Errorf("touches without session = %v", got)
	}
	// Failures are best-effort: no nudge line, no hook error.
	ft.mu.Lock()
	ft.err = errors.New("db down")
	ft.mu.Unlock()
	if r := e.event(t, p, wire.HookRequest{Event: wire.HookPrompt}); r.Context != "" {
		t.Errorf("prompt context on error = %q", r.Context)
	}
	// Zero counts stay silent.
	ft.mu.Lock()
	ft.err, ft.counts = nil, tasks.Counts{}
	ft.mu.Unlock()
	if r := e.event(t, p, wire.HookRequest{Event: wire.HookPrompt}); r.Context != "" {
		t.Errorf("prompt context with no tasks = %q", r.Context)
	}
}
