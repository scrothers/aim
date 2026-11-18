package mcpserver

import (
	"testing"

	"github.com/scrothers/aim/internal/mcpserver/limits"
)

func TestInstructionsFitBudget(t *testing.T) {
	t.Parallel()
	got := instructions()
	if got == "" || got[len(got)-1] == '\n' {
		t.Fatalf("instructions() = %q, want trimmed non-empty text", got)
	}
	if n := len(got); n > limits.MaxDescriptionBytes {
		t.Errorf("instructions.md is %d bytes; keep it at most %d", n, limits.MaxDescriptionBytes)
	}
}
