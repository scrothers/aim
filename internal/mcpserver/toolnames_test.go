package mcpserver_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// wantTools is the full tool list, grouped by area.
var wantTools = []string{
	"agent_whoami", "agent_register", "agent_roster", "agent_sign_on", "agent_sign_off",
	"mail_send", "mail_list", "mail_read", "mail_thread", "mail_mark", "mail_search", "mail_wait",
	"task_create", "task_list", "task_get", "task_update",
}

// oldToolNames were the unprefixed names; none may be registered or appear
// in agent-facing text as a backticked tool.
var oldToolNames = []string{
	"send", "list", "read", "thread", "mark", "search", "wait",
	"whoami", "register", "roster", "sign_on", "sign_off",
}

func TestToolListIsExactlyThePrefixedNames(t *testing.T) {
	t.Parallel()
	cs := newEnv(t).Client(t, "a")
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		got = append(got, tool.Name)
	}
	// The SDK lists tools sorted by name, so compare as sets.
	slices.Sort(got)
	want := slices.Sorted(slices.Values(wantTools))
	if !slices.Equal(got, want) {
		t.Errorf("tools = %v, want %v", got, want)
	}
	for _, old := range oldToolNames {
		if slices.Contains(got, old) {
			t.Errorf("old tool name %q is still registered", old)
		}
	}
}

// TestMarkdownNamesOnlyRealTools fails when any embedded Markdown (tool,
// resource and prompt text, server instructions) names a tool that does not
// exist: a stale old name, or a typo in a prefixed one.
func TestMarkdownNamesOnlyRealTools(t *testing.T) {
	t.Parallel()
	ref := regexp.MustCompile("`([a-z_]+)`")
	prefixed := regexp.MustCompile(`^(mail|agent|task)_[a-z_]+$`)
	// Field names that share the prefix pattern but are not tools.
	notTools := map[string]bool{"task_id": true, "mail_id": true}

	var files []string
	for _, dir := range []string{"tools/markdown", "resources/markdown", "prompts/markdown"} {
		m, err := filepath.Glob(filepath.Join(dir, "*.md"))
		if err != nil || len(m) == 0 {
			t.Fatalf("no Markdown in %s: %v", dir, err)
		}
		files = append(files, m...)
	}
	files = append(files, "instructions.md")

	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			for _, m := range ref.FindAllStringSubmatch(line, -1) {
				name := m[1]
				switch {
				case prefixed.MatchString(name):
					if !slices.Contains(wantTools, name) && !notTools[name] {
						t.Errorf("%s:%d names unknown tool `%s`", f, i+1, name)
					}
				case slices.Contains(oldToolNames, name):
					// Field values such as the mail_list state `read` are not tools.
					if !isFieldValue(f, line, name) {
						t.Errorf("%s:%d names old tool `%s`", f, i+1, name)
					}
				}
			}
		}
	}
}

// isFieldValue reports whether a backticked bare word on line is an argument
// value rather than a tool: the read state in the mail_list and mail_mark
// state lists.
func isFieldValue(file, line, name string) bool {
	if name != "read" || !strings.Contains(line, "`state`") {
		return false
	}
	base := filepath.Base(file)
	return base == "mail_list.md" || base == "mail_mark.md"
}
