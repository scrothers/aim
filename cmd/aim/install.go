package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/harness/drivers"
)

// installConfig is everything runInstall needs, so tests can fake the host.
type installConfig struct {
	Runtimes  []string
	Project   bool
	DryRun    bool
	MCP       bool
	Uninstall bool
	Bin       string
	Home      string
	// ProjectDir is resolved lazily so user-scope installs never run git.
	ProjectDir func() string
}

// runCommandTimeout bounds each registration command such as `claude mcp add`.
const runCommandTimeout = 30 * time.Second

func newInstallCmd() *cobra.Command {
	var cfg installConfig
	var bin string
	var noMCP bool
	cmd := &cobra.Command{
		Use:   "install [runtime...|all]",
		Short: "Install AIM's hooks and MCP server into coding-agent runtimes",
		Long: "Writes hook configuration and registers the AIM MCP server for claude, codex, gemini,\n" +
			"grok and opencode (all of them when none are named). Existing files are backed up to\n" +
			"<file>.aim-bak before their first modification.",
		ValidArgs: runtimeNames(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Runtimes = args
			cfg.MCP = !noMCP
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("find home directory: %w", err)
			}
			cfg.Home = home
			cfg.ProjectDir = gitToplevel
			if bin == "" {
				if bin, err = currentBinary(); err != nil {
					return err
				}
			}
			cfg.Bin = bin
			return runInstall(cmd.Context(), cmd.OutOrStdout(), cfg, harness.HostSystem{}, runCommand)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&cfg.Project, "project", false, "install into the current repository instead of the user config")
	f.BoolVar(&cfg.DryRun, "dry-run", false, "print the planned changes without writing")
	f.BoolVar(&noMCP, "no-mcp", false, "skip registering the MCP server")
	f.BoolVar(&cfg.Uninstall, "uninstall", false, "remove AIM's entries")
	f.StringVar(&bin, "bin", "", "aim binary path written into hooks (default: this executable)")
	return cmd
}

func runtimeNames() []string {
	var names []string
	for _, in := range drivers.Installers() {
		names = append(names, in.Name())
	}
	return append(names, "all")
}

// currentBinary returns this executable's absolute, symlink-resolved path.
func currentBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find aim binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// gitToplevel returns the repository root of the working directory, falling
// back to the working directory itself.
func gitToplevel() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output(); err == nil {
		if top := strings.TrimSpace(string(out)); top != "" {
			return top
		}
	}
	wd, _ := os.Getwd()
	return wd
}

// runCommand runs a registration command, folding its output into the error.
func runCommand(ctx context.Context, argv []string) error {
	ctx, cancel := context.WithTimeout(ctx, runCommandTimeout)
	defer cancel()
	var buf bytes.Buffer
	c := exec.CommandContext(ctx, argv[0], argv[1:]...)
	c.Stdout, c.Stderr = &buf, &buf
	if err := c.Run(); err != nil {
		if msg := strings.TrimSpace(buf.String()); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

// selectInstallers resolves runtime names ("all" or none means every one).
func selectInstallers(names []string) ([]harness.Installer, error) {
	if len(names) == 0 {
		return drivers.Installers(), nil
	}
	var out []harness.Installer
	seen := map[string]bool{}
	for _, n := range names {
		if n == "all" {
			return drivers.Installers(), nil
		}
		in, ok := drivers.Installer(n)
		if !ok {
			return nil, fmt.Errorf("unknown runtime %q (want one of %s)", n, strings.Join(runtimeNames(), ", "))
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, in)
		}
	}
	return out, nil
}

// runInstall plans every selected runtime first, so a planning error aborts
// before anything is written, then describes or applies the plans.
func runInstall(ctx context.Context, out io.Writer, cfg installConfig, sys harness.System,
	run func(context.Context, []string) error) error {
	installers, err := selectInstallers(cfg.Runtimes)
	if err != nil {
		return err
	}
	opts := harness.InstallOptions{
		Home:      cfg.Home,
		Uninstall: cfg.Uninstall,
		MCP:       cfg.MCP,
		Bin:       cfg.Bin,
	}
	if cfg.Project {
		opts.Scope = harness.ScopeProject
		if cfg.ProjectDir != nil {
			opts.ProjectDir = cfg.ProjectDir()
		}
	}
	if !cfg.Uninstall && !filepath.IsAbs(cfg.Bin) {
		return fmt.Errorf("--bin %q must be an absolute path", cfg.Bin)
	}
	plans := make([]harness.Plan, len(installers))
	var errs []error
	for i, in := range installers {
		p, err := in.Plan(ctx, opts, sys)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", in.Name(), err))
			continue
		}
		plans[i] = p
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	for i, in := range installers {
		emit(out, "== %s ==\n", in.Name())
		if cfg.DryRun {
			if d := harness.Describe(plans[i]); d != "" {
				emit(out, "%s", d)
			} else {
				emit(out, "nothing to do\n")
			}
			continue
		}
		if err := harness.Apply(ctx, plans[i], run); err != nil {
			return fmt.Errorf("%s: %w", in.Name(), err)
		}
		writeSummary(out, plans[i])
	}
	return nil
}

// emit writes to the command's output; a failed terminal write is not
// actionable.
func emit(out io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(out, format, args...)
}

func writeSummary(out io.Writer, p harness.Plan) {
	if len(p.Changes) == 0 && len(p.Commands) == 0 {
		emit(out, "already up to date\n")
	}
	for _, c := range p.Changes {
		verb := "updated"
		switch {
		case c.After == nil:
			verb = "removed"
		case c.Before == nil:
			verb = "created"
		}
		emit(out, "%s %s\n", verb, c.Path)
	}
	for _, c := range p.Commands {
		emit(out, "ran %s\n", strings.Join(c.Argv, " "))
	}
	for _, n := range p.Notes {
		emit(out, "note: %s\n", n)
	}
}
