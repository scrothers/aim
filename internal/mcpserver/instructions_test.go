package mcpserver

import (
	"testing"
	"unicode/utf8"
)

// maxInstructionsChars is Claude Code's cap on server instructions; longer
// text is truncated silently, so the end would be lost.
const maxInstructionsChars = 2048

func TestInstructionsFitClientCap(t *testing.T) {
	t.Parallel()
	got := instructions()
	if got == "" || got[len(got)-1] == '\n' {
		t.Fatalf("instructions() = %q, want trimmed non-empty text", got)
	}
	if n := utf8.RuneCountInString(got); n > maxInstructionsChars {
		t.Errorf("instructions.md is %d characters; keep it at most %d", n, maxInstructionsChars)
	}
}
