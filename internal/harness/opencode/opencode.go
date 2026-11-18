// Package opencode integrates AIM with OpenCode: hook payload translation for
// the bundled plugin (hook.go) and plugin installation (install.go).
package opencode

// Runtime is the OpenCode integration.
type Runtime struct{}

// New returns the OpenCode integration.
func New() Runtime { return Runtime{} }

// Name returns the runtime's CLI name.
func (Runtime) Name() string { return "opencode" }
