package claude

import (
	"context"

	"github.com/scrothers/aim/internal/harness"
)

var _ harness.Installer = Runtime{}

// hookEntries returns the handlers AIM installs into settings.json.
func hookEntries(bin string) []harness.HookEntry {
	cmd := harness.HookCommand(bin, "claude")
	handler := func(timeout int) *harness.JSONObject {
		return harness.Obj("type", "command", "command", cmd, "timeout", timeout)
	}
	events := []string{"SessionStart", "UserPromptSubmit", "SubagentStart", "SessionEnd"}
	entries := make([]harness.HookEntry, 0, len(events)+2)
	for _, ev := range events {
		entries = append(entries, harness.HookEntry{Event: ev, Handler: handler(10)})
	}
	entries = append(entries,
		harness.HookEntry{Event: "PreToolUse", Matcher: "mcp__aim__.*", Handler: handler(5)},
		harness.HookEntry{Event: "Stop", Handler: harness.Obj(
			"type", "command",
			"command", harness.HookCommand(bin, "claude", "watch"),
			"asyncRewake", true,
			"timeout", 86400,
		)},
	)
	return entries
}

// Plan plans the Claude Code settings.json hooks and, through the claude
// CLI, the MCP server registration. It never touches ~/.claude.json.
func (Runtime) Plan(ctx context.Context, opts harness.InstallOptions, sys harness.System) (harness.Plan, error) {
	var plan harness.Plan
	path, err := harness.ScopedPath(opts, ".claude/settings.json", ".claude/settings.json")
	if err != nil {
		return plan, err
	}
	change, ok, err := harness.HooksChange(sys, path, "claude", hookEntries(opts.Bin), opts.Uninstall)
	if err != nil {
		return plan, err
	}
	if ok {
		plan.Changes = append(plan.Changes, change)
	}
	if opts.MCP {
		scope := harness.ScopeFlag(opts.Scope)
		harness.PlanCLIMCP(ctx, sys, &plan, harness.CLIMCP{
			CLI:    "claude",
			Add:    []string{"mcp", "add", "--scope", scope, "aim", "--", opts.Bin, "mcp"},
			Remove: []string{"mcp", "remove", "--scope", scope, "aim"},
			Probe:  []string{"mcp", "get", "aim"},
		}, opts.Uninstall)
	}
	return plan, nil
}
