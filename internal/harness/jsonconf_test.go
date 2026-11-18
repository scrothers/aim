package harness_test

import (
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/harness"
)

func entries() []harness.HookEntry {
	h := harness.Obj("type", "command", "command", "/opt/aim hook claude", "timeout", 10)
	return []harness.HookEntry{
		{Event: "SessionStart", Handler: h},
		{Event: "PreToolUse", Matcher: "mcp__aim__.*", Handler: h},
	}
}

func TestParseJSONObjectPreservesOrder(t *testing.T) {
	t.Parallel()
	in := `{"z":1,"a":{"y":[1,2.50,"x"],"b":null},"m":true}`
	o, err := harness.ParseJSONObject([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"z\": 1,\n  \"a\": {\n    \"y\": [\n      1,\n      2.50,\n      \"x\"\n    ],\n    \"b\": null\n  },\n  \"m\": true\n}\n"
	if got := string(o.Bytes()); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestParseJSONObjectErrors(t *testing.T) {
	t.Parallel()
	for name, in := range map[string]string{
		"comment":  "{\n// hi\n\"a\":1}",
		"trailing": `{"a":1} {}`,
		"array":    `[1]`,
		"truncate": `{"a":`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := harness.ParseJSONObject([]byte(in)); err == nil {
				t.Fatal("want error")
			}
		})
	}
	o, err := harness.ParseJSONObject([]byte("  \n"))
	if err != nil || o.Len() != 0 {
		t.Fatalf("blank: %v %v", o, err)
	}
}

func TestJSONObjectOps(t *testing.T) {
	t.Parallel()
	o := harness.Obj("a", 1, "b", "x")
	o.Set("a", 2)
	o.Set("c", true)
	o.Delete("b")
	o.Delete("missing")
	if got := strings.Join(o.Keys(), ","); got != "a,c" {
		t.Fatalf("keys %s", got)
	}
	if _, ok := o.Child("a", false); ok {
		t.Fatal("a is not an object")
	}
	if _, ok := o.Child("nope", false); ok {
		t.Fatal("nope missing")
	}
	c, ok := o.Child("nope", true)
	if !ok || c == nil || o.Len() != 3 {
		t.Fatal("create child")
	}
	if got := string(harness.NewJSONObject().Bytes()); got != "{}\n" {
		t.Fatalf("empty: %q", got)
	}
	if v, _ := o.Get("a"); v != 2 {
		t.Fatalf("a=%v", v)
	}
}

func TestMergeHooks(t *testing.T) {
	t.Parallel()
	user := `{
  "model": "x",
  "hooks": {
    "SessionStart": [
      {"hooks": [{"type": "command", "command": "echo hi"}]},
      {"hooks": [{"type": "command", "command": "/old/aim hook claude"}, {"type": "command", "command": "keep"}]}
    ],
    "Stop": [{"hooks": [{"type": "command", "command": "/old/aim hook claude watch"}]}]
  },
  "other": [1]
}`
	tests := []struct {
		name      string
		in        string
		uninstall bool
		check     func(t *testing.T, out string)
	}{
		{"empty", "", false, func(t *testing.T, out string) {
			for _, s := range []string{`"SessionStart"`, `"matcher": "mcp__aim__.*"`, `"timeout": 10`} {
				if !strings.Contains(out, s) {
					t.Errorf("missing %s in\n%s", s, out)
				}
			}
		}},
		{"merge keeps user content", user, false, func(t *testing.T, out string) {
			if strings.Count(out, "/opt/aim hook claude") != 2 || strings.Contains(out, "/old/aim") {
				t.Errorf("aim entries wrong:\n%s", out)
			}
			if !strings.Contains(out, `"echo hi"`) || !strings.Contains(out, `"keep"`) || strings.Index(out, `"model"`) > strings.Index(out, `"hooks"`) {
				t.Errorf("user content lost or reordered:\n%s", out)
			}
			if strings.Contains(out, `"Stop"`) {
				t.Errorf("empty Stop event should be dropped:\n%s", out)
			}
		}},
		{"uninstall", user, true, func(t *testing.T, out string) {
			if strings.Contains(out, "aim") || !strings.Contains(out, `"keep"`) {
				t.Errorf("bad uninstall:\n%s", out)
			}
		}},
		{"uninstall to empty", `{"hooks":{"Stop":[{"hooks":[{"command":"/x/aim hook claude"}]}]}}`, true, func(t *testing.T, out string) {
			if out != "{}\n" {
				t.Errorf("got %q", out)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := harness.MergeHooks("s.json", []byte(tc.in), "claude", entries(), tc.uninstall)
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, string(out))
			if !tc.uninstall {
				again, err := harness.MergeHooks("s.json", out, "claude", entries(), false)
				if err != nil || string(again) != string(out) {
					t.Fatalf("not idempotent: %v\n%s\nvs\n%s", err, out, again)
				}
			}
		})
	}
}

func TestMergeHooksUninstallUntouched(t *testing.T) {
	t.Parallel()
	for _, in := range []string{`{"a":1}`, `{ "hooks": {"Stop":[{"hooks":[{"command":"mine"}]}]} }`} {
		out, err := harness.MergeHooks("p", []byte(in), "claude", entries(), true)
		if err != nil || string(out) != in {
			t.Fatalf("%q -> %q (%v)", in, out, err)
		}
	}
}

func TestMergeHooksErrors(t *testing.T) {
	t.Parallel()
	_, err := harness.MergeHooks("/p/settings.json", []byte("{ // c\n}"), "claude", entries(), false)
	if err == nil || !strings.Contains(err.Error(), "add this by hand") || !strings.Contains(err.Error(), "SessionStart") {
		t.Fatalf("want snippet error, got %v", err)
	}
	_, err = harness.MergeHooks("/p/settings.json", []byte("{ // c\n}"), "claude", entries(), true)
	if err == nil || !strings.Contains(err.Error(), "by hand") {
		t.Fatalf("uninstall error: %v", err)
	}
	if _, err = harness.MergeHooks("p", []byte(`{"hooks":[]}`), "claude", entries(), false); err == nil {
		t.Fatal("non-object hooks should fail")
	}
}
