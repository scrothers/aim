package drivers_test

import (
	"testing"

	"github.com/scrothers/aim/internal/harness/drivers"
)

func TestHooks(t *testing.T) {
	t.Parallel()
	want := []string{"claude", "codex", "gemini", "grok", "opencode"}
	hs := drivers.Hooks()
	if len(hs) != len(want) {
		t.Fatalf("len = %d, want %d", len(hs), len(want))
	}
	for i, h := range hs {
		if h.Name() != want[i] {
			t.Errorf("hook %d = %q, want %q", i, h.Name(), want[i])
		}
		if h.Client() == "" {
			t.Errorf("%s has no client name", h.Name())
		}
		got, ok := drivers.Hook(want[i])
		if !ok || got.Name() != want[i] {
			t.Errorf("Hook(%q) = %v, %v", want[i], got, ok)
		}
	}
	if _, ok := drivers.Hook("nope"); ok {
		t.Error("Hook(nope) found")
	}
}
