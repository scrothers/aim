package mcpserver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMarkdownIsolated keeps each feature package's Markdown in its
// markdown/ directory and Go code out of it.
func TestMarkdownIsolated(t *testing.T) {
	t.Parallel()
	for _, pkg := range []string{"tools", "resources", "prompts"} {
		entries, err := os.ReadDir(pkg)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".md") {
				t.Errorf("%s/%s belongs in %s/markdown/", pkg, e.Name(), pkg)
			}
		}
		md, err := os.ReadDir(filepath.Join(pkg, "markdown"))
		if err != nil {
			t.Fatal(err)
		}
		if len(md) == 0 {
			t.Errorf("%s/markdown is empty", pkg)
		}
		for _, e := range md {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				t.Errorf("%s/markdown/%s is not a Markdown file", pkg, e.Name())
			}
		}
	}
}
