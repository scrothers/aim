package claude_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/harness/claude"
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

func goldenFile(t *testing.T, name string, got []byte) {
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

func TestPlanFreshUser(t *testing.T) {
	t.Parallel()
	sys := fakeSys{paths: map[string]bool{"claude": true}}
	plan, err := claude.New().Plan(t.Context(), harness.InstallOptions{Home: "/h", Bin: "/opt/aim", MCP: true}, sys)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Path != "/h/.claude/settings.json" || plan.Changes[0].Before != nil {
		t.Fatalf("changes %+v", plan.Changes)
	}
	goldenFile(t, "settings.json.golden", plan.Changes[0].After)
	if len(plan.Commands) != 1 || strings.Join(plan.Commands[0].Argv, " ") != "claude mcp add --scope user aim -- /opt/aim mcp" {
		t.Fatalf("commands %+v", plan.Commands)
	}
}

func TestPlanScenarios(t *testing.T) {
	t.Parallel()
	existing := `{"theme":"dark","hooks":{"Stop":[{"hooks":[{"type":"command","command":"mine"},{"type":"command","command":"/x/aim hook claude watch"}]}]}}`
	tests := []struct {
		name    string
		opts    harness.InstallOptions
		sys     fakeSys
		wantErr string
		changes int
		cmd     string
		note    string
	}{
		{"project scope", harness.InstallOptions{Scope: harness.ScopeProject, ProjectDir: "/r", Bin: "/a b/aim", MCP: true},
			fakeSys{paths: map[string]bool{"claude": true}}, "", 1, "claude mcp add --scope project aim -- /a b/aim mcp", ""},
		{"no cli", harness.InstallOptions{Home: "/h", Bin: "/aim", MCP: true}, fakeSys{}, "", 1, "", "claude not found on PATH"},
		{"no mcp", harness.InstallOptions{Home: "/h", Bin: "/aim"}, fakeSys{paths: map[string]bool{"claude": true}}, "", 1, "", ""},
		{"already registered", harness.InstallOptions{Home: "/h", Bin: "/aim", MCP: true},
			fakeSys{paths: map[string]bool{"claude": true}, out: map[string]string{"claude mcp get aim": ""}}, "", 1, "", "already registered"},
		{"uninstall", harness.InstallOptions{Home: "/h", Bin: "/aim", MCP: true, Uninstall: true},
			fakeSys{paths: map[string]bool{"claude": true}, out: map[string]string{"claude mcp get aim": ""}, files: map[string]string{"/h/.claude/settings.json": existing}},
			"", 1, "claude mcp remove --scope user aim", ""},
		{"uninstall nothing", harness.InstallOptions{Home: "/h", Bin: "/aim", Uninstall: true}, fakeSys{}, "", 0, "", ""},
		{"jsonc", harness.InstallOptions{Home: "/h", Bin: "/aim"}, fakeSys{files: map[string]string{"/h/.claude/settings.json": "{//x\n}"}}, "add this by hand", 0, "", ""},
		{"no home", harness.InstallOptions{Bin: "/aim"}, fakeSys{}, "home directory", 0, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plan, err := claude.New().Plan(t.Context(), tc.opts, tc.sys)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err %v want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Changes) != tc.changes {
				t.Fatalf("changes %d want %d", len(plan.Changes), tc.changes)
			}
			cmd := ""
			if len(plan.Commands) > 0 {
				cmd = strings.Join(plan.Commands[0].Argv, " ")
			}
			if cmd != tc.cmd {
				t.Errorf("cmd %q want %q", cmd, tc.cmd)
			}
			if got := strings.Join(plan.Notes, "|"); !strings.Contains(got, tc.note) {
				t.Errorf("notes %q want %q", got, tc.note)
			}
		})
	}
}

func TestInstallIdempotentAndUninstallKeepsUserHooks(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "settings.json")
	user := `{"theme":"dark","hooks":{"Stop":[{"hooks":[{"type":"command","command":"mine"}]}]}}` + "\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(user), 0o600); err != nil {
		t.Fatal(err)
	}
	sys := harness.HostSystem{}
	opts := harness.InstallOptions{Home: home, Bin: "/opt/aim"}
	apply := func(o harness.InstallOptions) harness.Plan {
		plan, err := claude.New().Plan(t.Context(), o, sys)
		if err != nil {
			t.Fatal(err)
		}
		if err := harness.Apply(t.Context(), plan, nil); err != nil {
			t.Fatal(err)
		}
		return plan
	}
	apply(opts)
	first, _ := os.ReadFile(path)
	if p := apply(opts); len(p.Changes) != 0 {
		t.Fatal("second install should be a no-op")
	}
	if second, _ := os.ReadFile(path); string(second) != string(first) {
		t.Fatal("not idempotent")
	}
	if bak, _ := os.ReadFile(path + ".aim-bak"); string(bak) != user {
		t.Fatalf("backup %q", bak)
	}
	if !strings.Contains(string(first), `"asyncRewake": true`) || !strings.Contains(string(first), "/opt/aim hook claude watch") {
		t.Fatalf("stop hook missing:\n%s", first)
	}
	opts.Uninstall = true
	apply(opts)
	after, _ := os.ReadFile(path)
	want := "{\n  \"theme\": \"dark\",\n  \"hooks\": {\n    \"Stop\": [\n      {\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"mine\"\n          }\n        ]\n      }\n    ]\n  }\n}\n"
	if string(after) != want {
		t.Fatalf("uninstall left:\n%s", after)
	}
}
