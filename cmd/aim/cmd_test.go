package main

import (
	"log/slog"
	"strings"
	"testing"
)

func TestDefaultDB(t *testing.T) {
	t.Parallel()
	get := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got := defaultDB(get(map[string]string{"XDG_STATE_HOME": "/x"}), "/h"); got != "/x/aim/aim.db" {
		t.Fatalf("got %s", got)
	}
	if got := defaultDB(get(nil), "/h"); got != "/h/.local/state/aim/aim.db" {
		t.Fatalf("got %s", got)
	}
}

func TestEnvOr(t *testing.T) {
	t.Parallel()
	get := func(k string) string { return map[string]string{"A": "1"}[k] }
	if envOr(get, "A", "d") != "1" || envOr(get, "B", "d") != "d" {
		t.Fatal("envOr")
	}
}

// An empty AIM_SOCKET disables the unix socket, as AIM_LISTEN and AIM_HTTP
// do; the container image relies on it.
func TestDaemonEmptyEnvDisablesListeners(t *testing.T) {
	t.Setenv("AIM_SOCKET", "")
	t.Setenv("AIM_LISTEN", "")
	t.Setenv("AIM_HTTP", "")
	f := newDaemonCmd().Flags()
	for _, name := range []string{"socket", "listen", "http"} {
		if got := f.Lookup(name).DefValue; got != "" {
			t.Errorf("--%s default = %q, want empty", name, got)
		}
	}
}

func TestParseLevel(t *testing.T) {
	t.Parallel()
	if l, err := parseLevel("debug"); err != nil || l != slog.LevelDebug {
		t.Fatalf("%v %v", l, err)
	}
	if _, err := parseLevel("loud"); err == nil {
		t.Fatal("want error")
	}
}

func TestSplitList(t *testing.T) {
	t.Parallel()
	got := splitList(" a.lan, ,b.lan:9000,")
	if len(got) != 2 || got[0] != "a.lan" || got[1] != "b.lan:9000" {
		t.Errorf("splitList = %q", got)
	}
	if splitList("") != nil {
		t.Error("empty list should be nil")
	}
}

func TestOpenAPICmd(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	var out strings.Builder
	root.SetOut(&out)
	root.SetArgs([]string{"openapi"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"openapi":"3.1`) || !strings.Contains(out.String(), `"operationId":"getViews"`) {
		t.Errorf("unexpected spec: %.200s", out.String())
	}
}

func TestRootCmd(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	if len(root.Commands()) != 7 {
		t.Fatalf("commands = %d", len(root.Commands()))
	}
	root.SetArgs([]string{"daemon", "--log-level", "loud"})
	root.SetErr(&strings.Builder{})
	if err := root.Execute(); err == nil {
		t.Fatal("want bad log level error")
	}
}
