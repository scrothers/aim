// Package gemini integrates AIM with Gemini CLI: hook payload translation
// (hook.go) and settings.json installation (install.go).
package gemini

// Runtime is the Gemini CLI integration.
type Runtime struct{}

// New returns the Gemini CLI integration.
func New() Runtime { return Runtime{} }

// Name returns the runtime's CLI name.
func (Runtime) Name() string { return "gemini" }
