package main

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/harness"
)

// noCLISys sees no runtime CLIs, so installs only write files.
type noCLISys struct{ harness.HostSystem }

func (noCLISys) LookPath(string) (string, error) { return "", errors.New("not found") }

func baseCfg(home string) installConfig {
	return installConfig{Home: home, Bin: "/opt/aim", MCP: true, ProjectDir: func() string { return filepath.Join(home, "proj") }}
}

func TestRunInstallDryRunWritesNothing(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	cfg := baseCfg(home)
	cfg.DryRun = true
	var out bytes.Buffer
	if err := runInstall(t.Context(), &out, cfg, noCLISys{}, nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"== claude ==", "== opencode ==", "+++ " + home + "/.claude/settings.json (create)", "/opt/aim hook claude watch", "note: claude not found on PATH"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in\n%s", want, out.String())
		}
	}
	if entries, _ := os.ReadDir(home); len(entries) != 0 {
		t.Fatalf("dry run wrote %v", entries)
	}
}

func TestRunInstallApplyAndUninstall(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	cfg := baseCfg(home)
	cfg.Runtimes = []string{"claude", "grok", "claude"}
	var out bytes.Buffer
	if err := runInstall(t.Context(), &out, cfg, noCLISys{}, nil); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".claude/settings.json", ".grok/hooks/aim.json", ".grok/config.toml"} {
		if _, err := os.Stat(filepath.Join(home, p)); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
	if !strings.Contains(out.String(), "created "+filepath.Join(home, ".grok/hooks/aim.json")) {
		t.Errorf("summary:\n%s", out.String())
	}
	out.Reset()
	if err := runInstall(t.Context(), &out, cfg, noCLISys{}, nil); err != nil || strings.Count(out.String(), "already up to date") != 2 {
		t.Fatalf("second run: %v\n%s", err, out.String())
	}
	cfg.Uninstall = true
	out.Reset()
	if err := runInstall(t.Context(), &out, cfg, noCLISys{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".grok/hooks/aim.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("grok hooks file should be moved away")
	}
	if !strings.Contains(out.String(), "removed ") {
		t.Errorf("summary:\n%s", out.String())
	}
}

func TestRunInstallProjectScope(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	cfg := baseCfg(home)
	cfg.Project, cfg.Runtimes, cfg.DryRun = true, []string{"codex"}, true
	var out bytes.Buffer
	if err := runInstall(t.Context(), &out, cfg, noCLISys{}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), filepath.Join(home, "proj", ".codex", "hooks.json")) {
		t.Fatalf("project path missing:\n%s", out.String())
	}
}

func TestRunInstallErrors(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte("{//"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		mod  func(*installConfig)
		want string
	}{
		{"unknown runtime", func(c *installConfig) { c.Runtimes = []string{"vim"} }, `unknown runtime "vim"`},
		{"relative bin", func(c *installConfig) { c.Bin = "aim" }, "must be an absolute path"},
		{"jsonc aborts before any write", func(c *installConfig) { c.Runtimes = []string{"grok", "claude"} }, "add this by hand"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := baseCfg(home)
			tc.mod(&cfg)
			err := runInstall(t.Context(), &bytes.Buffer{}, cfg, noCLISys{}, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err %v want %q", err, tc.want)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(home, ".grok")); err == nil {
		t.Fatal("planning error must abort before writes")
	}
}

func TestRunInstallAllAndRunnerError(t *testing.T) {
	t.Parallel()
	ins, err := selectInstallers([]string{"codex", "all"})
	if err != nil || len(ins) != 5 {
		t.Fatalf("%d %v", len(ins), err)
	}
	// With a CLI on PATH the registration command goes to the runner.
	home := t.TempDir()
	cfg := baseCfg(home)
	cfg.Runtimes = []string{"claude"}
	boom := errors.New("boom")
	var got []string
	err = runInstall(t.Context(), &bytes.Buffer{}, cfg, claudeSys{}, func(_ context.Context, argv []string) error {
		got = argv
		return boom
	})
	if !errors.Is(err, boom) || strings.Join(got, " ") != "claude mcp add --scope user aim -- /opt/aim mcp" {
		t.Fatalf("err %v argv %v", err, got)
	}
}

type claudeSys struct{ harness.HostSystem }

func (claudeSys) LookPath(string) (string, error) { return "/bin/claude", nil }
func (claudeSys) Output(context.Context, ...string) ([]byte, error) {
	return nil, errors.New("not registered")
}

func TestInstallHelpers(t *testing.T) {
	t.Parallel()
	bin, err := currentBinary()
	if err != nil || !filepath.IsAbs(bin) {
		t.Fatalf("%q %v", bin, err)
	}
	if top := gitToplevel(); top == "" {
		t.Fatal("empty toplevel")
	}
	if err := runCommand(t.Context(), []string{"true"}); err != nil {
		t.Fatal(err)
	}
	err = runCommand(t.Context(), []string{"sh", "-c", "echo bad >&2; exit 3"})
	if err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("got %v", err)
	}
	if err := runCommand(t.Context(), []string{"sh", "-c", "exit 4"}); err == nil {
		t.Fatal("want error")
	}
}

func TestInstallCmdFlags(t *testing.T) {
	// Not parallel: t.Setenv.
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"install", "grok", "--dry-run", "--no-mcp", "--bin", "/opt/aim"})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "/opt/aim hook grok") || strings.Contains(out.String(), "config.toml") {
		t.Fatalf("output:\n%s", out.String())
	}
}
