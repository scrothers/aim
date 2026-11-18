package tools

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/mcpserver/limits"
)

func TestDescriptionsTrimmedAndFitBudget(t *testing.T) {
	t.Parallel()
	entries, err := fs.ReadDir(descriptionFS, markdownDir)
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
		if n := len(got); n > limits.MaxDescriptionBytes {
			t.Errorf("%s is %d bytes; keep it at most %d", e.Name(), n, limits.MaxDescriptionBytes)
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
