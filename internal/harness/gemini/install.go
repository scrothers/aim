package gemini

import (
	"context"

	"github.com/scrothers/aim/internal/harness"
)

var _ harness.Installer = Runtime{}

func hookEntries(bin string) []harness.HookEntry {
	cmd := harness.HookCommand(bin, "gemini")
	events := []struct {
		name    string
		timeout int
	}{
		{"SessionStart", 10000}, {"BeforeAgent", 10000}, {"AfterTool", 5000},
		{"Notification", 5000}, {"SessionEnd", 10000},
	}
	entries := make([]harness.HookEntry, 0, len(events))
	for _, ev := range events {
		entries = append(entries, harness.HookEntry{
			Event:   ev.name,
			Matcher: "*",
			// Gemini timeouts are milliseconds.
			Handler: harness.Obj("name", "aim", "type", "command", "command", cmd, "timeout", ev.timeout),
		})
	}
	return entries
}

// Plan plans Gemini CLI's settings.json hooks and, through the gemini CLI,
// the MCP server registration.
func (Runtime) Plan(ctx context.Context, opts harness.InstallOptions, sys harness.System) (harness.Plan, error) {
	var plan harness.Plan
	path, err := harness.ScopedPath(opts, ".gemini/settings.json", ".gemini/settings.json")
	if err != nil {
		return plan, err
	}
	change, ok, err := harness.HooksChange(sys, path, "gemini", hookEntries(opts.Bin), opts.Uninstall)
	if err != nil {
		return plan, err
	}
	if ok {
		plan.Changes = append(plan.Changes, change)
	}
	if !opts.Uninstall {
		plan.Notes = append(plan.Notes, "Gemini CLI may need hooks enabled: run /hooks enable-all in Gemini CLI.")
	}
	if opts.MCP {
		scope := harness.ScopeFlag(opts.Scope)
		harness.PlanCLIMCP(ctx, sys, &plan, harness.CLIMCP{
			CLI:           "gemini",
			Add:           []string{"mcp", "add", "--scope", scope, "aim", opts.Bin, "mcp"},
			Remove:        []string{"mcp", "remove", "--scope", scope, "aim"},
			Probe:         []string{"mcp", "list"},
			ProbeContains: "aim",
			Check:         []string{"mcp", "add", "--help"},
		}, opts.Uninstall)
	}
	return plan, nil
}
