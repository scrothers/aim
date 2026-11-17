package mcpserver

import (
	"embed"
	"fmt"
	"strings"
)

// promptFS holds the server instructions (prompts/instructions.md) and one
// Markdown description per tool (prompts/tools/<name>.md). Editing these files
// changes what agents are told without touching Go code.
//
//go:embed prompts
var promptFS embed.FS

// instructionsPath is the server-level instructions sent at initialize.
const instructionsPath = "prompts/instructions.md"

// toolPromptDir holds one <tool name>.md file per registered tool.
const toolPromptDir = "prompts/tools"

// prompt returns the trimmed contents of an embedded prompt file. The files are
// compiled in and covered by tests, so a missing one is a programming error.
func prompt(path string) string {
	b, err := promptFS.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("mcpserver: missing embedded prompt %s: %v", path, err))
	}
	return strings.TrimSpace(string(b))
}

// instructions returns the server instructions.
func instructions() string { return prompt(instructionsPath) }

// toolDescription returns the description for the named tool.
func toolDescription(name string) string { return prompt(toolPromptDir + "/" + name + ".md") }
