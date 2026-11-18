package grok

import (
	"context"
	"strconv"
	"strings"

	"github.com/scrothers/aim/internal/harness"
)

var _ harness.Installer = Runtime{}

const mcpHeader = "[mcp_servers.aim]"

func hookEntries(bin string) []harness.HookEntry {
	cmd := harness.HookCommand(bin, "grok")
	events := []string{"SessionStart", "PostToolUse", "SessionEnd"}
	entries := make([]harness.HookEntry, 0, len(events))
	for _, ev := range events {
		entries = append(entries, harness.HookEntry{
			Event:   ev,
			Handler: harness.Obj("type", "command", "command", cmd, "timeout", 10),
		})
	}
	return entries
}

// Plan plans Grok Build's AIM-owned hooks file (hooks/aim.json) and the
// [mcp_servers.aim] table in config.toml.
func (Runtime) Plan(_ context.Context, opts harness.InstallOptions, sys harness.System) (harness.Plan, error) {
	var plan harness.Plan
	hooksPath, err := harness.ScopedPath(opts, ".grok/hooks/aim.json", ".grok/hooks/aim.json")
	if err != nil {
		return plan, err
	}
	before, err := harness.ReadOptional(sys, hooksPath)
	if err != nil {
		return plan, err
	}
	var after []byte
	if !opts.Uninstall {
		// AIM owns this file, so it is generated from scratch.
		after, err = harness.MergeHooks(hooksPath, nil, "grok", hookEntries(opts.Bin), false)
		if err != nil {
			return plan, err
		}
	}
	if c, ok := harness.NewChange(hooksPath, before, after); ok {
		plan.Changes = append(plan.Changes, c)
	}
	if opts.MCP {
		tomlPath, err := harness.ScopedPath(opts, ".grok/config.toml", ".grok/config.toml")
		if err != nil {
			return plan, err
		}
		cur, err := harness.ReadOptional(sys, tomlPath)
		if err != nil {
			return plan, err
		}
		var next []byte
		if opts.Uninstall {
			next = removeTable(cur)
		} else {
			next = addTable(cur, opts.Bin)
		}
		if c, ok := harness.NewChange(tomlPath, cur, next); ok {
			plan.Changes = append(plan.Changes, c)
		}
	}
	if opts.Scope == harness.ScopeProject && !opts.Uninstall {
		plan.Notes = append(plan.Notes, "Grok Build runs project hooks only after /hooks-trust.")
	}
	return plan, nil
}

func isHeader(line string) bool { return strings.HasPrefix(strings.TrimSpace(line), "[") }

func isAimHeader(line string) bool {
	t := strings.TrimSpace(line)
	return t == mcpHeader || t == `[mcp_servers."aim"]` ||
		strings.HasPrefix(t, "[mcp_servers.aim.") || strings.HasPrefix(t, `[mcp_servers."aim".`)
}

func hasTable(cur []byte) bool {
	for _, l := range strings.Split(string(cur), "\n") {
		if t := strings.TrimSpace(l); t == mcpHeader || t == `[mcp_servers."aim"]` {
			return true
		}
	}
	return false
}

// addTable appends the AIM table unless one exists. Nothing else in the
// file is touched.
func addTable(cur []byte, bin string) []byte {
	if hasTable(cur) {
		return cur
	}
	var b strings.Builder
	b.Write(cur)
	if len(cur) > 0 {
		if !strings.HasSuffix(string(cur), "\n") {
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	b.WriteString(mcpHeader + "\ncommand = " + strconv.Quote(bin) + "\nargs = [\"mcp\"]\n")
	return []byte(b.String())
}

// removeTable drops the AIM table (and its sub-tables), from its header
// through the line before the next unrelated header, plus the blank line
// that separated it.
func removeTable(cur []byte) []byte {
	lines := strings.Split(string(cur), "\n")
	found := false
	for _, l := range lines {
		found = found || isAimHeader(l)
	}
	if !found {
		return cur
	}
	var out []string
	skipping := false
	for _, l := range lines {
		switch {
		case isAimHeader(l):
			skipping = true
			// Drop the blank separator before the table.
			if n := len(out); n > 0 && strings.TrimSpace(out[n-1]) == "" {
				out = out[:n-1]
			}
		case isHeader(l):
			// Keep a blank separator where the removed table used to be.
			if n := len(out); skipping && n > 0 && strings.TrimSpace(out[n-1]) != "" {
				out = append(out, "")
			}
			skipping = false
			out = append(out, l)
		case !skipping:
			out = append(out, l)
		}
	}
	res := strings.Join(out, "\n")
	if res != "" && !strings.HasSuffix(res, "\n") {
		res += "\n"
	}
	if strings.TrimSpace(res) == "" {
		res = ""
	}
	return []byte(res)
}
