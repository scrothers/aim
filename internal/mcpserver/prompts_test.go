package mcpserver

import "testing"

func TestPromptPanicsOnMissingFile(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("prompt of a missing file did not panic")
		}
	}()
	_ = toolDescription("no_such_tool")
}

func TestPromptsTrimmedAndNonEmpty(t *testing.T) {
	t.Parallel()
	if got := instructions(); got == "" || got[len(got)-1] == '\n' {
		t.Fatalf("instructions() = %q, want trimmed non-empty text", got)
	}
	if got := toolDescription("wait"); got == "" || got[0] == ' ' {
		t.Fatalf("toolDescription(wait) = %q, want trimmed non-empty text", got)
	}
}
