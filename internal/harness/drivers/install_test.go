package drivers_test

import (
	"testing"

	"github.com/scrothers/aim/internal/harness/drivers"
)

func TestInstallers(t *testing.T) {
	t.Parallel()
	want := []string{"claude", "codex", "gemini", "grok", "opencode"}
	got := drivers.Installers()
	if len(got) != len(want) {
		t.Fatalf("got %d installers", len(got))
	}
	for i, in := range got {
		if in.Name() != want[i] {
			t.Errorf("installer %d is %s want %s", i, in.Name(), want[i])
		}
		if byName, ok := drivers.Installer(want[i]); !ok || byName.Name() != want[i] {
			t.Errorf("lookup %s failed", want[i])
		}
	}
	if _, ok := drivers.Installer("vim"); ok {
		t.Error("unknown runtime found")
	}
}
