package mcpserver

import (
	"io/fs"
	"strings"
	"testing"
	"unicode/utf8"
)

// maxPromptChars is Claude Code's cap on server instructions; longer text is
// truncated silently, so the end would be lost.
const maxPromptChars = 2048

// maxWorkflowChars is the sanity cap for workflow prompt scripts (8 KB).
const maxWorkflowChars = 8 * 1024

func TestInstructionsFitClientCap(t *testing.T) {
	t.Parallel()
	got := instructions()
	if got == "" || got[len(got)-1] == '\n' {
		t.Fatalf("instructions() = %q, want trimmed non-empty text", got)
	}
	if n := utf8.RuneCountInString(got); n > maxPromptChars {
		t.Errorf("instructions.md is %d characters; keep it at most %d", n, maxPromptChars)
	}
}

func TestWorkflowsFitCap(t *testing.T) {
	t.Parallel()
	err := fs.WalkDir(promptFS, "prompts", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		// Workflow prompts load only when invoked; they have a sanity cap.
		if n := utf8.RuneCountInString(prompt(path)); n > maxWorkflowChars {
			t.Errorf("%s is %d characters; keep it at most %d", path, n, maxWorkflowChars)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPromptPanicsOnMissingFile(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("prompt of a missing file did not panic")
		}
	}()
	_ = prompt("prompts/no_such_file.md")
}
