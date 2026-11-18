package grok_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/harness/grok"
)

type fakeSys struct{ files map[string]string }

func (f fakeSys) ReadFile(p string) ([]byte, error) {
	if s, ok := f.files[p]; ok {
		return []byte(s), nil
	}
	return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
}
func (fakeSys) LookPath(string) (string, error)                   { return "", errors.New("none") }
func (fakeSys) Output(context.Context, ...string) ([]byte, error) { return nil, errors.New("none") }

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

func TestPlanFresh(t *testing.T) {
	t.Parallel()
	plan, err := grok.New().Plan(t.Context(), harness.InstallOptions{Home: "/h", Bin: "/opt/aim", MCP: true}, fakeSys{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 2 || plan.Changes[0].Path != "/h/.grok/hooks/aim.json" || plan.Changes[1].Path != "/h/.grok/config.toml" {
		t.Fatalf("changes %+v", plan.Changes)
	}
	goldenFile(t, "aim.json.golden", plan.Changes[0].After)
	goldenFile(t, "config.toml.golden", plan.Changes[1].After)
	if len(plan.Notes) != 0 {
		t.Fatalf("notes %v", plan.Notes)
	}
}

func TestPlanProjectNoMCP(t *testing.T) {
	t.Parallel()
	plan, err := grok.New().Plan(t.Context(), harness.InstallOptions{Scope: harness.ScopeProject, ProjectDir: "/r", Bin: "/aim"}, fakeSys{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Path != "/r/.grok/hooks/aim.json" || !strings.Contains(strings.Join(plan.Notes, ""), "/hooks-trust") {
		t.Fatalf("%+v %v", plan.Changes, plan.Notes)
	}
}

func TestTOMLEditing(t *testing.T) {
	t.Parallel()
	const header = "[mcp_servers.aim]\ncommand = \"/aim\"\nargs = [\"mcp\"]\n"
	tests := []struct {
		name      string
		in        string
		uninstall bool
		want      string // empty means no change
	}{
		{"append after content", "model = \"x\"\n", false, "model = \"x\"\n\n" + header},
		{"append no trailing newline", "model = \"x\"", false, "model = \"x\"\n\n" + header},
		{"already present", "[mcp_servers.aim]\ncommand = \"/other\"\n", false, ""},
		{"remove middle", "a = 1\n\n" + header + "\n[other]\nb = 2\n", true, "a = 1\n\n[other]\nb = 2\n"},
		{"remove last with subtable", "a = 1\n\n" + header + "[mcp_servers.aim.env]\nK = \"v\"\n", true, "a = 1\n"},
		{"remove keeps neighbours", "[mcp_servers.aimx]\nc = 1\n\n" + header, true, "[mcp_servers.aimx]\nc = 1\n"},
		{"remove absent", "a = 1", true, ""},
		{"remove only table", header, true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sys := fakeSys{files: map[string]string{"/h/.grok/config.toml": tc.in}}
			plan, err := grok.New().Plan(t.Context(), harness.InstallOptions{Home: "/h", Bin: "/aim", MCP: true, Uninstall: tc.uninstall}, sys)
			if err != nil {
				t.Fatal(err)
			}
			var got string
			for _, c := range plan.Changes {
				if strings.HasSuffix(c.Path, "config.toml") {
					got = string(c.After)
				}
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestUninstallRemovesHooksFile(t *testing.T) {
	t.Parallel()
	sys := fakeSys{files: map[string]string{"/h/.grok/hooks/aim.json": "{}"}}
	plan, err := grok.New().Plan(t.Context(), harness.InstallOptions{Home: "/h", Uninstall: true}, sys)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].After != nil {
		t.Fatalf("%+v", plan.Changes)
	}
	if _, err := grok.New().Plan(t.Context(), harness.InstallOptions{Bin: "/a"}, sys); err == nil {
		t.Fatal("want home error")
	}
}
