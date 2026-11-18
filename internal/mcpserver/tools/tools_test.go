package tools

import (
	"io/fs"
	"strings"
	"testing"
	"unicode/utf8"
)

// maxDescriptionChars is Claude Code's cap on each tool description; longer
// text is truncated silently, so the end would be lost.
const maxDescriptionChars = 2048

func TestDescriptionsTrimmedAndFitClientCap(t *testing.T) {
	t.Parallel()
	entries, err := fs.ReadDir(descriptionFS, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no embedded descriptions")
	}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".md")
		got := description(name)
		if got == "" || got[0] == ' ' || got[len(got)-1] == '\n' {
			t.Errorf("description(%s) = %q, want trimmed non-empty text", name, got)
		}
		if n := utf8.RuneCountInString(got); n > maxDescriptionChars {
			t.Errorf("%s is %d characters; keep it at most %d", e.Name(), n, maxDescriptionChars)
		}
	}
}

func TestDescriptionPanicsOnMissingFile(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("description of a missing tool did not panic")
		}
	}()
	_ = description("no_such_tool")
}
