package drivers

import (
	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/harness/claude"
	"github.com/scrothers/aim/internal/harness/codex"
	"github.com/scrothers/aim/internal/harness/gemini"
	"github.com/scrothers/aim/internal/harness/grok"
	"github.com/scrothers/aim/internal/harness/opencode"
)

// Hooks returns every runtime's hook translator.
func Hooks() []harness.Hook {
	return []harness.Hook{
		claude.New(),
		codex.New(),
		gemini.New(),
		grok.New(),
		opencode.New(),
	}
}

// Hook returns the hook translator with the given CLI name.
func Hook(name string) (harness.Hook, bool) {
	for _, h := range Hooks() {
		if h.Name() == name {
			return h, true
		}
	}
	return nil, false
}
