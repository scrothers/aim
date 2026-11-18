package codex

import (
	"context"

	"github.com/scrothers/aim/internal/harness"
)

var _ harness.Installer = Runtime{}

func hookEntries(bin string) []harness.HookEntry {
	cmd := harness.HookCommand(bin, "codex")
	timeouts := []struct {
		event   string
		timeout int
	}{
		{"SessionStart", 10},
		{"UserPromptSubmit", 10},
		{"SubagentStart", 10},
		{"PostToolUse", 5},
		// Codex caps SessionEnd and Interrupt at 3 seconds.
		{"Interrupt", 2},
		{"SessionEnd", 2},
	}
	entries := make([]harness.HookEntry, 0, len(timeouts))
	for _, t := range timeouts {
		entries = append(entries, harness.HookEntry{
			Event:   t.event,
			Handler: harness.Obj("type", "command", "command", cmd, "timeout", t.timeout),
		})
	}
	return entries
}

// Plan plans Codex's hooks.json and, through the codex CLI, the MCP server
// registration.
func (Runtime) Plan(ctx context.Context, opts harness.InstallOptions, sys harness.System) (harness.Plan, error) {
	var plan harness.Plan
	path, err := harness.ScopedPath(opts, ".codex/hooks.json", ".codex/hooks.json")
	if err != nil {
		return plan, err
	}
	change, ok, err := harness.HooksChange(sys, path, "codex", hookEntries(opts.Bin), opts.Uninstall)
	if err != nil {
		return plan, err
	}
	if ok {
		plan.Changes = append(plan.Changes, change)
	}
	if !opts.Uninstall {
		plan.Notes = append(plan.Notes, "Codex requires trusting new hooks: run /hooks in Codex and approve the AIM entries.")
	}
	if opts.MCP {
		harness.PlanCLIMCP(ctx, sys, &plan, harness.CLIMCP{
			CLI:    "codex",
			Add:    []string{"mcp", "add", "aim", "--", opts.Bin, "mcp"},
			Remove: []string{"mcp", "remove", "aim"},
			Probe:  []string{"mcp", "get", "aim"},
			Check:  []string{"mcp", "add", "--help"},
		}, opts.Uninstall)
	}
	return plan, nil
}
