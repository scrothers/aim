package harness_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/harness"
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

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestApplyWritesAndBacksUp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "a", "b", "f.json")
	plan := harness.Plan{Changes: []harness.Change{{Path: p, After: []byte("one")}}}
	if err := harness.Apply(t.Context(), plan, nil); err != nil {
		t.Fatal(err)
	}
	if read(t, p) != "one" {
		t.Fatal("create failed")
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v", st.Mode())
	}
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, next := range []string{"two", "three"} {
		plan.Changes[0].After = []byte(next)
		if err := harness.Apply(t.Context(), plan, nil); err != nil {
			t.Fatal(err)
		}
	}
	if read(t, p) != "three" || read(t, p+".aim-bak") != "one" {
		t.Fatalf("content %q backup %q", read(t, p), read(t, p+".aim-bak"))
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode not preserved: %v", st.Mode())
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("leftover temp %s", e.Name())
		}
	}
}

func TestApplyRemoveMovesToBackup(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "f")
	for _, content := range []string{"v1", "v2"} {
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := harness.Apply(t.Context(), harness.Plan{Changes: []harness.Change{{Path: p, Before: []byte(content)}}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(p); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("file should be gone")
	}
	if read(t, p+".aim-bak") != "v1" || read(t, p+".aim-bak.1") != "v2" {
		t.Fatal("backups overwritten")
	}
	// Removing a missing file is a no-op.
	if err := harness.Apply(t.Context(), harness.Plan{Changes: []harness.Change{{Path: p}}}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestApplyFollowsSymlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target, link := filepath.Join(dir, "target"), filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := harness.Apply(t.Context(), harness.Plan{Changes: []harness.Change{{Path: link, After: []byte("new")}}}, nil); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Lstat(link); st.Mode()&os.ModeSymlink == 0 || read(t, target) != "new" {
		t.Fatal("symlink replaced")
	}
}

func TestApplyCommands(t *testing.T) {
	t.Parallel()
	plan := harness.Plan{Commands: []harness.Command{{Argv: []string{"a", "b"}}, {Argv: []string{"c"}}}}
	var ran []string
	err := harness.Apply(t.Context(), plan, func(_ context.Context, argv []string) error {
		ran = append(ran, strings.Join(argv, " "))
		return nil
	})
	if err != nil || strings.Join(ran, ";") != "a b;c" {
		t.Fatalf("ran %v err %v", ran, err)
	}
	if err := harness.Apply(t.Context(), plan, nil); err == nil {
		t.Fatal("nil runner should fail")
	}
	boom := errors.New("boom")
	err = harness.Apply(t.Context(), plan, func(context.Context, []string) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

func TestDescribe(t *testing.T) {
	t.Parallel()
	plan := harness.Plan{
		Changes: []harness.Change{
			{Path: "/n", After: []byte("a\nb\n")},
			{Path: "/m", Before: []byte("a\nb\nc\n"), After: []byte("a\nx\nc\nd\n")},
			{Path: "/r", Before: []byte("z\n")},
		},
		Commands: []harness.Command{{Argv: []string{"x", "y"}, Note: "why"}},
		Notes:    []string{"careful"},
	}
	want := `+++ /n (create)
+ a
+ b
+++ /m (modify; original kept as m.aim-bak)
- b
+ x
+ d
--- /r (remove; kept as r.aim-bak)
- z
$ x y
  # why
note: careful
`
	if got := harness.Describe(plan); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestShellQuoteAndHookCommand(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"/usr/bin/aim", "/usr/bin/aim"},
		{"/my dir/aim", "'/my dir/aim'"},
		{"/a'b", `'/a'\''b'`},
		{"", "''"},
		{"/x;rm", "'/x;rm'"},
	}
	for _, tc := range tests {
		if got := harness.ShellQuote(tc.in); got != tc.want {
			t.Errorf("ShellQuote(%q)=%s want %s", tc.in, got, tc.want)
		}
	}
	if got := harness.HookCommand("/a b/aim", "claude", "watch"); got != "'/a b/aim' hook claude watch" {
		t.Fatal(got)
	}
}

func TestNewChangeAndScopedPath(t *testing.T) {
	t.Parallel()
	if _, ok := harness.NewChange("p", []byte("x"), []byte("x")); ok {
		t.Fatal("same bytes")
	}
	if _, ok := harness.NewChange("p", nil, nil); ok {
		t.Fatal("both absent")
	}
	if _, ok := harness.NewChange("p", nil, []byte{}); !ok {
		t.Fatal("create empty file")
	}
	p, err := harness.ScopedPath(harness.InstallOptions{Home: "/h"}, ".u", ".p")
	if err != nil || p != "/h/.u" {
		t.Fatal(p, err)
	}
	p, err = harness.ScopedPath(harness.InstallOptions{Scope: harness.ScopeProject, ProjectDir: "/r"}, ".u", ".p")
	if err != nil || p != "/r/.p" {
		t.Fatal(p, err)
	}
	if _, err = harness.ScopedPath(harness.InstallOptions{Scope: harness.ScopeProject}, "u", "p"); err == nil {
		t.Fatal("want project error")
	}
	if _, err = harness.ScopedPath(harness.InstallOptions{}, "u", "p"); err == nil {
		t.Fatal("want home error")
	}
	if harness.ScopeFlag(harness.ScopeProject) != "project" || harness.ScopeFlag(harness.ScopeUser) != "user" {
		t.Fatal("scope flag")
	}
}

func TestHooksChange(t *testing.T) {
	t.Parallel()
	sys := fakeSys{files: map[string]string{"/bad": "{//"}}
	if _, ok, err := harness.HooksChange(sys, "/none", "claude", entries(), true); ok || err != nil {
		t.Fatalf("uninstall missing: %v %v", ok, err)
	}
	c, ok, err := harness.HooksChange(sys, "/none", "claude", entries(), false)
	if !ok || err != nil || c.Before != nil {
		t.Fatalf("install: %v %v", ok, err)
	}
	if _, _, err := harness.HooksChange(sys, "/bad", "claude", entries(), false); err == nil {
		t.Fatal("want error")
	}
}

func TestPlanCLIMCP(t *testing.T) {
	t.Parallel()
	m := harness.CLIMCP{
		CLI: "x", Add: []string{"add"}, Remove: []string{"rm"},
		Probe: []string{"get"}, Check: []string{"help"},
	}
	tests := []struct {
		name      string
		sys       fakeSys
		uninstall bool
		cmd       string
		note      string
	}{
		{"missing cli", fakeSys{}, false, "", "x not found on PATH; register the AIM MCP server by running: x add"},
		{"missing cli uninstall", fakeSys{}, true, "", "unregister the AIM MCP server by running: x rm"},
		{"check fails", fakeSys{paths: map[string]bool{"x": true}}, false, "", "could not confirm x's MCP syntax"},
		{"add", fakeSys{paths: map[string]bool{"x": true}, out: map[string]string{"x help": ""}}, false, "x add", ""},
		{"already", fakeSys{paths: map[string]bool{"x": true}, out: map[string]string{"x help": "", "x get": ""}}, false, "", "already registered"},
		{"remove", fakeSys{paths: map[string]bool{"x": true}, out: map[string]string{"x help": "", "x get": ""}}, true, "x rm", ""},
		{"remove absent", fakeSys{paths: map[string]bool{"x": true}, out: map[string]string{"x help": ""}}, true, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var plan harness.Plan
			harness.PlanCLIMCP(t.Context(), tc.sys, &plan, m, tc.uninstall)
			got := ""
			if len(plan.Commands) > 0 {
				got = strings.Join(plan.Commands[0].Argv, " ")
			}
			if got != tc.cmd {
				t.Errorf("command %q want %q", got, tc.cmd)
			}
			notes := strings.Join(plan.Notes, "|")
			if !strings.Contains(notes, tc.note) || (tc.note == "" && notes != "") {
				t.Errorf("notes %q want %q", notes, tc.note)
			}
		})
	}
}

func TestMCPRegisteredContains(t *testing.T) {
	t.Parallel()
	sys := fakeSys{out: map[string]string{"l": "foo\naim: ok\n"}}
	if !harness.MCPRegistered(t.Context(), sys, "aim", "l") || harness.MCPRegistered(t.Context(), sys, "zzz", "l") || harness.MCPRegistered(t.Context(), sys, "", "nope") {
		t.Fatal("MCPRegistered")
	}
}

func TestHostSystem(t *testing.T) {
	t.Parallel()
	var h harness.HostSystem
	p := filepath.Join(t.TempDir(), "f")
	if _, err := h.ReadFile(p); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if _, err := h.LookPath("definitely-not-a-real-binary-aim"); err == nil {
		t.Fatal("want error")
	}
	out, err := h.Output(t.Context(), "echo", "hi")
	if err != nil || string(out) != "hi\n" {
		t.Fatalf("%q %v", out, err)
	}
	if _, err := h.Output(t.Context()); err == nil {
		t.Fatal("empty argv")
	}
	if _, err := h.Output(t.Context(), "false"); err == nil {
		t.Fatal("false should fail")
	}
}
