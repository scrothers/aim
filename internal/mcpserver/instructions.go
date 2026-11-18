package mcpserver

import (
	_ "embed"
	"strings"
)

// instructionsMD is the server-level instructions sent at initialize. Editing
// instructions.md changes what agents are told without touching Go code.
//
//go:embed instructions.md
var instructionsMD string

// instructions returns the trimmed server instructions.
func instructions() string { return strings.TrimSpace(instructionsMD) }
