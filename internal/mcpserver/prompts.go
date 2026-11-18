package mcpserver

import (
	"embed"
	"fmt"
	"strings"
)

// promptFS holds the user-invocable prompt workflows (prompts/workflows).
//
//go:embed prompts
var promptFS embed.FS

// prompt returns the trimmed contents of an embedded prompt file. The files are
// compiled in and covered by tests, so a missing one is a programming error.
func prompt(path string) string {
	b, err := promptFS.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("mcpserver: missing embedded prompt %s: %v", path, err))
	}
	return strings.TrimSpace(string(b))
}
