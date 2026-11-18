package opencode

import (
	"context"
	_ "embed" // plugin templates
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/scrothers/aim/internal/harness"
)

var _ harness.Installer = Runtime{}

//go:embed plugin/v1.js
var pluginV1 string

//go:embed plugin/v2.js
var pluginV2 string

// binPlaceholder is replaced by the JSON string literal of the aim binary.
const binPlaceholder = "__AIM_BIN__"

// RenderPlugin returns the plugin source for OpenCode major version (1 or
// 2) with bin substituted.
func RenderPlugin(major int, bin string) string {
	tmpl := pluginV1
	if major >= 2 {
		tmpl = pluginV2
	}
	lit, _ := json.Marshal(bin)
	return strings.ReplaceAll(tmpl, binPlaceholder, string(lit))
}

// detectMajor returns OpenCode's major version, defaulting to 1 with a note
// when it cannot be determined.
func detectMajor(ctx context.Context, sys harness.System, plan *harness.Plan) int {
	if _, err := sys.LookPath("opencode"); err != nil {
		plan.Notes = append(plan.Notes, "opencode not found on PATH; assuming the v1 plugin API")
		return 1
	}
	out, err := sys.Output(ctx, "opencode", "--version")
	if err == nil {
		fields := strings.FieldsFunc(strings.TrimSpace(string(out)), func(r rune) bool { return r < '0' || r > '9' })
		if len(fields) > 0 {
			if n, err := strconv.Atoi(fields[0]); err == nil && n > 0 {
				return n
			}
		}
	}
	plan.Notes = append(plan.Notes, "could not read opencode --version; assuming the v1 plugin API")
	return 1
}

func serverEntry(major int, bin string) *harness.JSONObject {
	command := []any{bin, "mcp"}
	if major >= 2 {
		return harness.Obj("type", "local", "command", command)
	}
	return harness.Obj("type", "local", "command", command, "enabled", true)
}

// setServer places the AIM server in the version's mcp layout.
func setServer(root *harness.JSONObject, major int, bin string) {
	mcp, ok := root.Child("mcp", true)
	if !ok {
		return
	}
	if major >= 2 {
		if servers, ok := mcp.Child("servers", true); ok {
			servers.Set("aim", serverEntry(major, bin))
		}
		return
	}
	mcp.Set("aim", serverEntry(major, bin))
}

// isAimServer reports whether v looks like AIM's server entry, so uninstall
// never removes a user's own server that happens to be named aim.
func isAimServer(v any) bool {
	o, ok := v.(*harness.JSONObject)
	if !ok {
		return false
	}
	cmd, _ := o.Get("command")
	args, _ := cmd.([]any)
	if len(args) == 0 {
		return false
	}
	last, _ := args[len(args)-1].(string)
	first, _ := args[0].(string)
	return last == "mcp" && strings.Contains(first, "aim")
}

// removeServer deletes AIM's entries from either mcp layout, dropping
// containers it emptied, and reports whether it removed anything.
func removeServer(root *harness.JSONObject) bool {
	mcp, ok := root.Child("mcp", false)
	if !ok {
		return false
	}
	removed := false
	if v, ok := mcp.Get("aim"); ok && isAimServer(v) {
		mcp.Delete("aim")
		removed = true
	}
	if servers, ok := mcp.Child("servers", false); ok {
		if v, ok := servers.Get("aim"); ok && isAimServer(v) {
			servers.Delete("aim")
			removed = true
			if servers.Len() == 0 {
				mcp.Delete("servers")
			}
		}
	}
	if removed && mcp.Len() == 0 {
		root.Delete("mcp")
	}
	return removed
}

// Plan plans the AIM plugin file and the opencode.json MCP entry.
func (Runtime) Plan(ctx context.Context, opts harness.InstallOptions, sys harness.System) (harness.Plan, error) {
	var plan harness.Plan
	pluginPath, err := harness.ScopedPath(opts, ".config/opencode/plugins/aim.js", ".opencode/plugins/aim.js")
	if err != nil {
		return plan, err
	}
	major := detectMajor(ctx, sys, &plan)
	before, err := harness.ReadOptional(sys, pluginPath)
	if err != nil {
		return plan, err
	}
	var after []byte
	if !opts.Uninstall {
		after = []byte(RenderPlugin(major, opts.Bin))
	}
	if c, ok := harness.NewChange(pluginPath, before, after); ok {
		plan.Changes = append(plan.Changes, c)
	}
	if !opts.MCP {
		return plan, nil
	}
	cfgPath, err := harness.ScopedPath(opts, ".config/opencode/opencode.json", "opencode.json")
	if err != nil {
		return plan, err
	}
	cur, err := harness.ReadOptional(sys, cfgPath)
	if err != nil {
		return plan, err
	}
	snippet := func() []byte {
		root := harness.NewJSONObject()
		setServer(root, major, opts.Bin)
		return root.Bytes()
	}
	if cur == nil {
		jsoncPath := strings.TrimSuffix(cfgPath, ".json") + ".jsonc"
		if jc, err := harness.ReadOptional(sys, jsoncPath); err != nil {
			return plan, err
		} else if jc != nil {
			if opts.Uninstall {
				plan.Notes = append(plan.Notes, fmt.Sprintf("%s uses JSONC; remove its mcp \"aim\" entry by hand.", jsoncPath))
			} else {
				plan.Notes = append(plan.Notes, fmt.Sprintf("%s uses JSONC and was not edited; merge this into it by hand:\n%s", jsoncPath, snippet()))
			}
			return plan, nil
		}
	}
	if cur == nil && opts.Uninstall {
		return plan, nil
	}
	root, err := harness.ParseJSONObject(cur)
	if err != nil {
		if opts.Uninstall {
			return plan, harness.ConfigParseError(cfgPath, err, nil)
		}
		return plan, harness.ConfigParseError(cfgPath, err, snippet())
	}
	if opts.Uninstall {
		if !removeServer(root) {
			return plan, nil
		}
	} else {
		setServer(root, major, opts.Bin)
	}
	if c, ok := harness.NewChange(cfgPath, cur, root.Bytes()); ok {
		plan.Changes = append(plan.Changes, c)
	}
	return plan, nil
}
