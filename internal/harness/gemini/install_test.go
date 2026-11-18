package gemini_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/harness/gemini"
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

func TestPlan(t *testing.T) {
	t.Parallel()
	paths := map[string]bool{"gemini": true}
	ready := fakeSys{paths: paths, out: map[string]string{"gemini mcp add --help": "usage", "gemini mcp list": "other: ok"}}
	registered := fakeSys{paths: paths, out: map[string]string{"gemini mcp add --help": "", "gemini mcp list": "aim: connected"}}
	tests := []struct {
		name string
		opts harness.InstallOptions
		sys  fakeSys
		path string
		cmd  string
		note string
	}{
		{"user", harness.InstallOptions{Home: "/h", Bin: "/opt/aim", MCP: true}, ready, "/h/.gemini/settings.json", "gemini mcp add --scope user aim /opt/aim mcp", "enable-all"},
		{"project", harness.InstallOptions{Scope: harness.ScopeProject, ProjectDir: "/r", Bin: "/opt/aim", MCP: true}, ready, "/r/.gemini/settings.json", "gemini mcp add --scope project aim /opt/aim mcp", ""},
		{"no cli", harness.InstallOptions{Home: "/h", Bin: "/opt/aim", MCP: true}, fakeSys{}, "", "", "gemini not found on PATH"},
		{"already", harness.InstallOptions{Home: "/h", Bin: "/opt/aim", MCP: true}, registered, "", "", "already registered"},
		{"uninstall", harness.InstallOptions{Home: "/h", Bin: "/opt/aim", MCP: true, Uninstall: true}, registered, "", "gemini mcp remove --scope user aim", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plan, err := gemini.New().Plan(t.Context(), tc.opts, tc.sys)
			if err != nil {
				t.Fatal(err)
			}
			if tc.path != "" && plan.Changes[0].Path != tc.path {
				t.Errorf("path %v", plan.Changes)
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

func TestGolden(t *testing.T) {
	t.Parallel()
	plan, err := gemini.New().Plan(t.Context(), harness.InstallOptions{Home: "/h", Bin: "/opt/aim"}, fakeSys{})
	if err != nil {
		t.Fatal(err)
	}
	goldenFile(t, "settings.json.golden", plan.Changes[0].After)
}

func TestErrors(t *testing.T) {
	t.Parallel()
	if _, err := gemini.New().Plan(t.Context(), harness.InstallOptions{Bin: "/a"}, fakeSys{}); err == nil {
		t.Fatal("want home error")
	}
	bad := fakeSys{files: map[string]string{"/h/.gemini/settings.json": "{//"}}
	if _, err := gemini.New().Plan(t.Context(), harness.InstallOptions{Home: "/h", Bin: "/a"}, bad); err == nil {
		t.Fatal("want parse error")
	}
}
