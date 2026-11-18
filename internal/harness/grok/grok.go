// Package grok integrates AIM with xAI Grok Build: hook payload translation
// (hook.go) and ~/.grok/hooks installation (install.go).
package grok

// Runtime is the Grok Build integration.
type Runtime struct{}

// New returns the Grok Build integration.
func New() Runtime { return Runtime{} }

// Name returns the runtime's CLI name.
func (Runtime) Name() string { return "grok" }
