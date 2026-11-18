package drivers

import (
	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/harness/claude"
	"github.com/scrothers/aim/internal/harness/codex"
	"github.com/scrothers/aim/internal/harness/gemini"
	"github.com/scrothers/aim/internal/harness/grok"
	"github.com/scrothers/aim/internal/harness/opencode"
)

// Installers returns every runtime installer in a stable order.
func Installers() []harness.Installer {
	return []harness.Installer{
		claude.New(),
		codex.New(),
		gemini.New(),
		grok.New(),
		opencode.New(),
	}
}

// Installer returns the installer with the given runtime name.
func Installer(name string) (harness.Installer, bool) {
	for _, in := range Installers() {
		if in.Name() == name {
			return in, true
		}
	}
	return nil, false
}
