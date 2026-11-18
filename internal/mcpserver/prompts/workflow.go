package prompts

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"text/template"
)

// markdownDir holds one <prompt name>.md workflow script per user-invocable
// MCP prompt. Unlike tool descriptions these load only when invoked, so they
// are not subject to the client description floor (see package limits).
const markdownDir = "markdown"

//go:embed markdown/*.md
var workflowFS embed.FS

// workflowArg describes one prompt argument from a workflow's front matter.
type workflowArg struct {
	name, description string
	required          bool
}

// workflow is a parsed markdown/<name>.md file.
type workflow struct {
	name, title, description string
	args                     []workflowArg
	body                     *template.Template
}

// parseWorkflow parses a workflow file: a front matter block between two
// "---" lines followed by a text/template body. The front matter is a tiny
// YAML subset:
//
//	title: Text
//	description: Text
//	arguments:
//	  - name: arg
//	    required: true
//	    description: Text
func parseWorkflow(name, src string) (workflow, error) {
	w := workflow{name: name}
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return w, fmt.Errorf("prompts: workflow %s: missing front matter", name)
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return w, fmt.Errorf("prompts: workflow %s: unterminated front matter", name)
	}
	if err := w.parseFrontMatter(lines[1:end]); err != nil {
		return w, err
	}
	if w.title == "" || w.description == "" {
		return w, fmt.Errorf("prompts: workflow %s: title and description are required", name)
	}
	body := strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	t, err := template.New(name).Option("missingkey=zero").Parse(body)
	if err != nil {
		return w, fmt.Errorf("prompts: workflow %s: %w", name, err)
	}
	w.body = t
	return w, nil
}

func (w *workflow) parseFrontMatter(lines []string) error {
	inArgs := false
	for _, raw := range lines {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		bad := func(why string) error {
			return fmt.Errorf("prompts: workflow %s: front matter line %q: %s", w.name, raw, why)
		}
		if !strings.HasPrefix(raw, " ") {
			inArgs = false
			k, v, ok := strings.Cut(raw, ":")
			if !ok {
				return bad("want key: value")
			}
			v = strings.TrimSpace(v)
			switch k {
			case "title":
				w.title = v
			case "description":
				w.description = v
			case "arguments":
				inArgs = true
			default:
				return bad("unknown key")
			}
			continue
		}
		if !inArgs {
			return bad("indented line outside arguments")
		}
		item := strings.TrimSpace(raw)
		if rest, isNew := strings.CutPrefix(item, "- "); isNew {
			w.args = append(w.args, workflowArg{})
			item = rest
		}
		if len(w.args) == 0 {
			return bad("argument attribute before '- name:'")
		}
		k, v, ok := strings.Cut(item, ":")
		if !ok {
			return bad("want key: value")
		}
		v = strings.TrimSpace(v)
		a := &w.args[len(w.args)-1]
		switch k {
		case "name":
			a.name = v
		case "description":
			a.description = v
		case "required":
			switch v {
			case "true":
				a.required = true
			case "false":
				a.required = false
			default:
				return bad("required must be true or false")
			}
		default:
			return bad("unknown argument key")
		}
	}
	for _, a := range w.args {
		if a.name == "" {
			return fmt.Errorf("prompts: workflow %s: argument without a name", w.name)
		}
	}
	return nil
}

// workflows parses every embedded workflow. The files are compiled in and
// covered by tests, so a malformed one is a programming error.
func workflows() []workflow {
	entries, err := fs.ReadDir(workflowFS, markdownDir)
	if err != nil {
		panic(fmt.Sprintf("prompts: read workflows: %v", err))
	}
	var out []workflow
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".md")
		if e.IsDir() || !ok {
			continue
		}
		b, err := workflowFS.ReadFile(markdownDir + "/" + e.Name())
		if err != nil {
			panic(fmt.Sprintf("prompts: read %s/%s: %v", markdownDir, e.Name(), err))
		}
		w, err := parseWorkflow(name, strings.TrimSpace(string(b)))
		if err != nil {
			panic(err.Error())
		}
		out = append(out, w)
	}
	return out
}
