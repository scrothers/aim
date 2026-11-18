package opencode_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/harness/opencode"
)

type fakeSys struct {
	files map[string]string
	paths map[string]bool
	out   map[string]string
}

func (f fakeSys) ReadFile(p string) ([]byte, error) {
	if s, ok := f.files[p]; ok {
		return []byte(s), nil
	}
	return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
}

func (f fakeSys) LookPath(n string) (string, error) {
	if f.paths[n] {
		return "/bin/" + n, nil
	}
	return "", errors.New("not found")
}

func (f fakeSys) Output(_ context.Context, argv ...string) ([]byte, error) {
	if s, ok := f.out[strings.Join(argv, " ")]; ok {
		return []byte(s), nil
	}
	return nil, errors.New("exit 1")
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if os.Getenv("UPDATE_SNAPSHOTS") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got) {
		t.Fatalf("golden mismatch %s; rerun with UPDATE_SNAPSHOTS=1\n%s", name, got)
	}
}

func versioned(v string) fakeSys {
	return fakeSys{paths: map[string]bool{"opencode": true}, out: map[string]string{"opencode --version": v}}
}

func changeFor(plan harness.Plan, suffix string) (harness.Change, bool) {
	for _, c := range plan.Changes {
		if strings.HasSuffix(c.Path, suffix) {
			return c, true
		}
	}
	return harness.Change{}, false
}

func TestRenderPluginSyntax(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	for _, major := range []int{1, 2} {
		src := opencode.RenderPlugin(major, "/my dir/aim")
		if strings.Contains(src, "__AIM_BIN__") || !strings.Contains(src, `const BIN = "/my dir/aim"`) {
			t.Fatalf("v%d placeholder not substituted", major)
		}
		if err != nil {
			continue
		}
		p := filepath.Join(t.TempDir(), "aim.mjs")
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.CommandContext(t.Context(), node, "--check", p).CombinedOutput(); err != nil {
			t.Fatalf("node --check v%d: %v\n%s", major, err, out)
		}
	}
	if !strings.Contains(opencode.RenderPlugin(2, "/a"), "Plugin.define") || strings.Contains(opencode.RenderPlugin(1, "/a"), "Plugin.define") {
		t.Fatal("wrong template per major")
	}
}

func TestPlanGolden(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		version string
		suffix  string
	}{{"1.4.2\n", "v1"}, {"opencode 2.0.0\n", "v2"}} {
		plan, err := opencode.New().Plan(t.Context(), harness.InstallOptions{Home: "/h", Bin: "/opt/aim", MCP: true}, versioned(tc.version))
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Changes) != 2 || len(plan.Notes) != 0 {
			t.Fatalf("%s: %+v notes %v", tc.suffix, plan.Changes, plan.Notes)
		}
		plugin, _ := changeFor(plan, "plugins/aim.js")
		if plugin.Path != "/h/.config/opencode/plugins/aim.js" {
			t.Fatalf("plugin path %s", plugin.Path)
		}
		cfg, _ := changeFor(plan, "opencode.json")
		golden(t, "opencode."+tc.suffix+".json.golden", cfg.After)
		if (tc.suffix == "v2") != strings.Contains(string(plugin.After), "Plugin.define") {
			t.Fatalf("%s: wrong template", tc.suffix)
		}
	}
}

func TestPlanVersionFallback(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sys  fakeSys
		note string
	}{
		{"missing", fakeSys{}, "opencode not found on PATH"},
		{"unparsable", versioned("dev build"), "could not read opencode --version"},
		{"fails", fakeSys{paths: map[string]bool{"opencode": true}}, "could not read opencode --version"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plan, err := opencode.New().Plan(t.Context(), harness.InstallOptions{Home: "/h", Bin: "/aim"}, tc.sys)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(plan.Notes, "|"), tc.note) {
				t.Fatalf("notes %v", plan.Notes)
			}
			if c, _ := changeFor(plan, "aim.js"); strings.Contains(string(c.After), "Plugin.define") {
				t.Fatal("should default to v1")
			}
		})
	}
}

func TestPlanMCPEdits(t *testing.T) {
	t.Parallel()
	cfg := "/h/.config/opencode/opencode.json"
	t.Run("preserves user keys", func(t *testing.T) {
		t.Parallel()
		sys := versioned("1.0.0")
		sys.files = map[string]string{cfg: `{"theme":"x","mcp":{"mine":{"type":"remote","url":"u"}}}`}
		plan, err := opencode.New().Plan(t.Context(), harness.InstallOptions{Home: "/h", Bin: "/aim", MCP: true}, sys)
		if err != nil {
			t.Fatal(err)
		}
		c, _ := changeFor(plan, "opencode.json")
		out := string(c.After)
		if !strings.Contains(out, `"mine"`) || !strings.Contains(out, `"aim"`) || strings.Index(out, `"theme"`) > strings.Index(out, `"mcp"`) {
			t.Fatalf("bad merge:\n%s", out)
		}
		// Replanning against the result is a no-op.
		sys.files[cfg] = out
		sys.files["/h/.config/opencode/plugins/aim.js"] = opencode.RenderPlugin(1, "/aim")
		again, err := opencode.New().Plan(t.Context(), harness.InstallOptions{Home: "/h", Bin: "/aim", MCP: true}, sys)
		if err != nil || len(again.Changes) != 0 {
			t.Fatalf("not idempotent: %v %+v", err, again.Changes)
		}
	})
	t.Run("uninstall removes only aim", func(t *testing.T) {
		t.Parallel()
		sys := versioned("2.1.0")
		sys.files = map[string]string{
			cfg:                                  `{"mcp":{"servers":{"aim":{"type":"local","command":["/aim","mcp"]},"mine":{"type":"local","command":["x"]}},"aim":{"type":"local","command":["/aim","mcp"],"enabled":true}}}`,
			"/h/.config/opencode/plugins/aim.js": "js",
		}
		plan, err := opencode.New().Plan(t.Context(), harness.InstallOptions{Home: "/h", Bin: "/aim", MCP: true, Uninstall: true}, sys)
		if err != nil {
			t.Fatal(err)
		}
		c, _ := changeFor(plan, "opencode.json")
		if strings.Contains(string(c.After), `"/aim"`) || !strings.Contains(string(c.After), `"mine"`) {
			t.Fatalf("bad uninstall:\n%s", c.After)
		}
		p, _ := changeFor(plan, "aim.js")
		if p.After != nil || p.Before == nil {
			t.Fatal("plugin should be removed")
		}
	})
	t.Run("uninstall keeps users own aim", func(t *testing.T) {
		t.Parallel()
		sys := versioned("1.0.0")
		sys.files = map[string]string{cfg: `{"mcp":{"aim":{"type":"remote","url":"u"}}}`}
		plan, err := opencode.New().Plan(t.Context(), harness.InstallOptions{Home: "/h", Bin: "/aim", MCP: true, Uninstall: true}, sys)
		if err != nil || len(plan.Changes) != 0 {
			t.Fatalf("%v %+v", err, plan.Changes)
		}
	})
	t.Run("uninstall empties mcp", func(t *testing.T) {
		t.Parallel()
		sys := versioned("1.0.0")
		sys.files = map[string]string{cfg: `{"a":1,"mcp":{"aim":{"command":["/aim","mcp"]}}}`}
		plan, _ := opencode.New().Plan(t.Context(), harness.InstallOptions{Home: "/h", Bin: "/aim", MCP: true, Uninstall: true}, sys)
		if c, _ := changeFor(plan, "opencode.json"); string(c.After) != "{\n  \"a\": 1\n}\n" {
			t.Fatalf("%q", c.After)
		}
	})
	t.Run("uninstall nothing", func(t *testing.T) {
		t.Parallel()
		plan, err := opencode.New().Plan(t.Context(), harness.InstallOptions{Home: "/h", Bin: "/aim", MCP: true, Uninstall: true}, versioned("1.0.0"))
		if err != nil || len(plan.Changes) != 0 {
			t.Fatalf("%v %+v", err, plan.Changes)
		}
	})
	t.Run("jsonc only", func(t *testing.T) {
		t.Parallel()
		sys := versioned("1.0.0")
		sys.files = map[string]string{"/h/.config/opencode/opencode.jsonc": "{//\n}"}
		plan, err := opencode.New().Plan(t.Context(), harness.InstallOptions{Home: "/h", Bin: "/aim", MCP: true}, sys)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := changeFor(plan, "opencode.json"); ok || !strings.Contains(strings.Join(plan.Notes, "|"), "merge this into it by hand") {
			t.Fatalf("%+v %v", plan.Changes, plan.Notes)
		}
		up, _ := opencode.New().Plan(t.Context(), harness.InstallOptions{Home: "/h", Bin: "/aim", MCP: true, Uninstall: true}, sys)
		if !strings.Contains(strings.Join(up.Notes, "|"), "remove its mcp") {
			t.Fatalf("%v", up.Notes)
		}
	})
	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()
		sys := versioned("1.0.0")
		sys.files = map[string]string{cfg: "{//"}
		for _, un := range []bool{false, true} {
			_, err := opencode.New().Plan(t.Context(), harness.InstallOptions{Home: "/h", Bin: "/aim", MCP: true, Uninstall: un}, sys)
			if err == nil || !strings.Contains(err.Error(), "by hand") {
				t.Fatalf("uninstall=%v err %v", un, err)
			}
		}
	})
	t.Run("project scope", func(t *testing.T) {
		t.Parallel()
		plan, err := opencode.New().Plan(t.Context(), harness.InstallOptions{Scope: harness.ScopeProject, ProjectDir: "/r", Bin: "/aim", MCP: true}, versioned("1.0.0"))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := changeFor(plan, "/r/.opencode/plugins/aim.js"); !ok {
			t.Fatal("plugin path")
		}
		if _, ok := changeFor(plan, "/r/opencode.json"); !ok {
			t.Fatal("config path")
		}
	})
	t.Run("errors", func(t *testing.T) {
		t.Parallel()
		if _, err := opencode.New().Plan(t.Context(), harness.InstallOptions{Bin: "/aim"}, fakeSys{}); err == nil {
			t.Fatal("want home error")
		}
	})
}
