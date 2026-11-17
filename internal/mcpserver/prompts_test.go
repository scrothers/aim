package mcpserver

import (
	"io/fs"
	"strings"
	"testing"
	"unicode/utf8"
)

// maxPromptChars is Claude Code's cap on server instructions and on each tool
// description; longer text is truncated silently, so the end would be lost.
const maxPromptChars = 2048

func TestPromptsFitClientCap(t *testing.T) {
	t.Parallel()
	err := fs.WalkDir(promptFS, "prompts", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		n := utf8.RuneCountInString(prompt(path))
		if n > maxPromptChars {
			t.Errorf("%s is %d characters; keep it at most %d or clients truncate it", path, n, maxPromptChars)
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
