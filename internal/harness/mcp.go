package harness

import (
	"context"
	"fmt"
	"strings"
)

// CLIMCP describes registering the AIM MCP server through a runtime's own
// CLI. Argument slices exclude the CLI name.
type CLIMCP struct {
	CLI    string
	Add    []string
	Remove []string
	// Probe, when set, is run to learn whether AIM is already registered; its
	// output must contain ProbeContains (when non-empty) to count.
	Probe         []string
	ProbeContains string
	// Check, when set, is run first to confirm the CLI supports the
	// subcommands (for example `mcp add --help`); failure downgrades the
	// step to a note.
	Check []string
}

// PlanCLIMCP appends the command (or a note when the CLI is missing or
// unsuitable) that registers or unregisters the MCP server.
func PlanCLIMCP(ctx context.Context, sys System, plan *Plan, m CLIMCP, uninstall bool) {
	argv := append([]string{m.CLI}, m.Add...)
	verb := "register"
	if uninstall {
		argv = append([]string{m.CLI}, m.Remove...)
		verb = "unregister"
	}
	manual := strings.Join(argv, " ")
	if _, err := sys.LookPath(m.CLI); err != nil {
		plan.Notes = append(plan.Notes, fmt.Sprintf("%s not found on PATH; %s the AIM MCP server by running: %s", m.CLI, verb, manual))
		return
	}
	if m.Check != nil {
		if _, err := sys.Output(ctx, append([]string{m.CLI}, m.Check...)...); err != nil {
			plan.Notes = append(plan.Notes, fmt.Sprintf("could not confirm %s's MCP syntax; %s the AIM MCP server by running: %s", m.CLI, verb, manual))
			return
		}
	}
	registered := m.Probe != nil && MCPRegistered(ctx, sys, m.ProbeContains, append([]string{m.CLI}, m.Probe...)...)
	switch {
	case uninstall && m.Probe != nil && !registered:
		return
	case !uninstall && registered:
		plan.Notes = append(plan.Notes, fmt.Sprintf("the AIM MCP server is already registered with %s; left unchanged", m.CLI))
		return
	}
	plan.Commands = append(plan.Commands, Command{Argv: argv, Note: verb + " the AIM MCP server with " + m.CLI})
}
