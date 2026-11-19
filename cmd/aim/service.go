package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/scrothers/aim/internal/service"
)

const (
	defaultServiceHTTP = ":7172"
	healthTimeout      = 5 * time.Second
	healthPoll         = 250 * time.Millisecond
)

// serviceConfig is everything runServiceInstall needs, so tests can fake the host.
type serviceConfig struct {
	Bin     string
	Env     map[string]string
	Home    string
	NoStart bool
	DryRun  bool
}

// serviceDeps are the host effects of a service operation.
type serviceDeps struct {
	// Run executes one service-manager command.
	Run func(ctx context.Context, argv []string) error
	// Health waits for the daemon's /healthz; nil skips the check.
	Health func(ctx context.Context, url string) error
	// LingerOff reports whether the user's systemd instance stops at logout;
	// nil skips the hint.
	LingerOff func(ctx context.Context) bool
}

func newServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Run the daemon as a per-user service (systemd or launchd)",
		Long: "Installs and manages a per-user service for `aim daemon`: a systemd user unit on Linux\n" +
			"or a launchd LaunchAgent on macOS.",
	}
	cmd.AddCommand(newServiceInstallCmd(), newServiceUninstallCmd(), newServiceLogsCmd())
	for _, a := range []struct {
		use, short string
		cmds       func(service.Manager) []service.Cmd
	}{
		{"start", "Start the service", service.Manager.Start},
		{"stop", "Stop the service", service.Manager.Stop},
		{"restart", "Restart the service", service.Manager.Restart},
		{"status", "Show the service status", service.Manager.Status},
	} {
		cmd.AddCommand(&cobra.Command{
			Use:   a.use,
			Short: a.short,
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				mgr, err := detectServiceManager()
				if err != nil {
					return err
				}
				return runServiceCmds(cmd.Context(), a.cmds(mgr), streamCommand(cmd.OutOrStdout(), cmd.ErrOrStderr()))
			},
		})
	}
	return cmd
}

func newServiceInstallCmd() *cobra.Command {
	var cfg serviceConfig
	var bin string
	var envPairs []string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install and start the per-user service",
		Long: "Writes the systemd unit or LaunchAgent plist, enables it and starts the daemon.\n" +
			"Re-running is safe: an unchanged file is left alone and a changed one is rewritten and restarted.\n" +
			"Use --env to set daemon variables (AIM_*) for the service; the shell environment is not inherited.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			mgr, err := detectServiceManager()
			if err != nil {
				return err
			}
			if cfg.Env, err = parseServiceEnv(envPairs); err != nil {
				return err
			}
			if cfg.Home, err = os.UserHomeDir(); err != nil {
				return fmt.Errorf("find home directory: %w", err)
			}
			if bin == "" {
				// No EvalSymlinks: a Homebrew or ~/.local/bin symlink survives upgrades.
				if bin, err = os.Executable(); err != nil {
					return fmt.Errorf("find aim binary: %w", err)
				}
			}
			if cfg.Bin, err = filepath.Abs(bin); err != nil {
				return fmt.Errorf("resolve --bin: %w", err)
			}
			deps := serviceDeps{
				Run: runCommand,
				Health: func(ctx context.Context, url string) error {
					return waitHealthy(ctx, url, healthTimeout)
				},
			}
			if mgr.LingerNote() != "" {
				deps.LingerOff = lingerOff
			}
			return runServiceInstall(cmd.Context(), cmd.OutOrStdout(), cfg, mgr, deps)
		},
	}
	f := cmd.Flags()
	f.StringVar(&bin, "bin", "", "aim binary the service runs (default: this executable)")
	f.StringArrayVar(&envPairs, "env", nil, "daemon environment `KEY=VALUE`, AIM_ prefix required (repeatable)")
	f.BoolVar(&cfg.NoStart, "no-start", false, "write and enable the service without starting it")
	f.BoolVar(&cfg.DryRun, "dry-run", false, "print the file and commands without changing anything")
	return cmd
}

func newServiceUninstallCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Stop and remove the per-user service",
		Long:  "Stops the service and removes its unit or plist. The database and backups are never touched.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			mgr, err := detectServiceManager()
			if err != nil {
				return err
			}
			return runServiceUninstall(cmd.Context(), cmd.OutOrStdout(), dryRun, mgr, serviceDeps{Run: runCommand})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the commands without changing anything")
	return cmd
}

func newServiceLogsCmd() *cobra.Command {
	var follow bool
	var lines int
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show the service logs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			mgr, err := detectServiceManager()
			if err != nil {
				return err
			}
			return runServiceCmds(cmd.Context(), mgr.Logs(follow, lines), streamCommand(cmd.OutOrStdout(), cmd.ErrOrStderr()))
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "stream new log lines")
	cmd.Flags().IntVarP(&lines, "lines", "n", 100, "number of recent lines to show")
	return cmd
}

func detectServiceManager() (service.Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find home directory: %w", err)
	}
	return service.Detect(service.Host{
		GOOS:          runtime.GOOS,
		Home:          home,
		XDGConfigHome: os.Getenv("XDG_CONFIG_HOME"),
		UID:           os.Getuid(),
		LookPath:      exec.LookPath,
		Exists: func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		},
	})
}

// parseServiceEnv validates KEY=VALUE pairs; the AIM_ prefix catches typos.
func parseServiceEnv(pairs []string) (map[string]string, error) {
	env := make(map[string]string, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || !strings.HasPrefix(k, "AIM_") || len(k) == len("AIM_") {
			return nil, fmt.Errorf("--env %q: want AIM_NAME=VALUE", p)
		}
		for _, r := range k {
			if (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
				return nil, fmt.Errorf("--env %q: invalid variable name", p)
			}
		}
		if strings.ContainsAny(v, "\r\n") {
			return nil, fmt.Errorf("--env %q: value may not contain a newline", p)
		}
		env[k] = v
	}
	return env, nil
}

// checkServiceBin refuses a binary that will not survive: a `go run` build or
// anything under the temp directory.
func checkServiceBin(bin, tmpDir string) error {
	if !filepath.IsAbs(bin) {
		return fmt.Errorf("binary path %q must be absolute", bin)
	}
	clean := filepath.Clean(bin)
	inTemp := tmpDir != "" && strings.HasPrefix(clean, filepath.Clean(tmpDir)+string(filepath.Separator))
	if inTemp || strings.Contains(clean, string(filepath.Separator)+"go-build") {
		return fmt.Errorf("%s is a temporary build (go run?) that will disappear; pass --bin with a permanent aim binary", bin)
	}
	return nil
}

// healthURLs derives the daemon's /healthz and console URLs from the service
// environment. It reports false when the HTTP listener is disabled.
func healthURLs(env map[string]string) (health, console string, ok bool) {
	addr, set := env["AIM_HTTP"]
	if !set {
		addr = defaultServiceHTTP
	}
	if addr == "" {
		return "", "", false
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", false
	}
	console = "http://localhost:" + port
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/healthz", console, true
}

func runServiceInstall(ctx context.Context, out io.Writer, cfg serviceConfig, mgr service.Manager, deps serviceDeps) error {
	if err := checkServiceBin(cfg.Bin, os.TempDir()); err != nil {
		return err
	}
	content := mgr.Render(service.Spec{Bin: cfg.Bin, Env: cfg.Env, Home: cfg.Home})
	path := mgr.Path()
	existing, err := os.ReadFile(path)
	existed := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	changed := !existed || !bytes.Equal(existing, content)
	cmds := mgr.Install(service.InstallOpts{NoStart: cfg.NoStart, Changed: changed, Existed: existed})

	if cfg.DryRun {
		emit(out, "would write %s:\n%s", path, content)
		for _, c := range cmds {
			emit(out, "would run %s\n", strings.Join(c.Argv, " "))
		}
		return nil
	}

	if changed {
		for _, d := range append([]string{filepath.Dir(path)}, mgr.Dirs()...) {
			if err := os.MkdirAll(d, 0o755); err != nil {
				return fmt.Errorf("create %s: %w", d, err)
			}
		}
		if err := os.WriteFile(path, content, 0o644); err != nil { //nolint:gosec // unit files are world-readable by convention
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	switch {
	case !existed:
		emit(out, "created %s\n", path)
	case changed:
		emit(out, "updated %s\n", path)
	default:
		emit(out, "unchanged %s\n", path)
	}
	if err := runServiceCmds(ctx, cmds, deps.Run); err != nil {
		return err
	}
	if cfg.NoStart {
		emit(out, "service %s installed, not started\n", mgr.Name())
		return nil
	}
	reportHealth(ctx, out, cfg, mgr, deps)
	if deps.LingerOff != nil && deps.LingerOff(ctx) {
		emit(out, "note: %s\n", mgr.LingerNote())
	}
	return nil
}

func reportHealth(ctx context.Context, out io.Writer, cfg serviceConfig, mgr service.Manager, deps serviceDeps) {
	health, console, ok := healthURLs(cfg.Env)
	if !ok || deps.Health == nil {
		emit(out, "started; check it with `aim service status`\n")
		return
	}
	if err := deps.Health(ctx, health); err != nil {
		emit(out, "started, but %s did not answer: %v\n", health, err)
		emit(out, "check `aim service status` and `aim service logs`; a manually started `aim daemon` may hold the port\n")
		return
	}
	emit(out, "running (%s), console at %s\n", mgr.Name(), console)
}

func runServiceUninstall(ctx context.Context, out io.Writer, dryRun bool, mgr service.Manager, deps serviceDeps) error {
	path := mgr.Path()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		emit(out, "not installed (%s does not exist)\n", path)
		return nil
	} else if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	before, after := mgr.Uninstall()
	if dryRun {
		for _, c := range before {
			emit(out, "would run %s\n", strings.Join(c.Argv, " "))
		}
		emit(out, "would remove %s\n", path)
		for _, c := range after {
			emit(out, "would run %s\n", strings.Join(c.Argv, " "))
		}
		return nil
	}
	if err := runServiceCmds(ctx, before, deps.Run); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	emit(out, "removed %s\n", path)
	if err := runServiceCmds(ctx, after, deps.Run); err != nil {
		return err
	}
	emit(out, "the database and backups were not touched\n")
	return nil
}

// runServiceCmds runs cmds in order, stopping at the first failure that is not
// marked IgnoreErr.
func runServiceCmds(ctx context.Context, cmds []service.Cmd, run func(context.Context, []string) error) error {
	for _, c := range cmds {
		if err := run(ctx, c.Argv); err != nil && !c.IgnoreErr {
			return fmt.Errorf("%s: %w", strings.Join(c.Argv, " "), err)
		}
	}
	return nil
}

// streamCommand runs a command with its output wired straight through, for
// status and logs.
func streamCommand(out, errOut io.Writer) func(context.Context, []string) error {
	return func(ctx context.Context, argv []string) error {
		c := exec.CommandContext(ctx, argv[0], argv[1:]...)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, out, errOut
		return c.Run()
	}
}

// waitHealthy polls url until it answers 200 or timeout passes.
func waitHealthy(ctx context.Context, url string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client := &http.Client{Timeout: time.Second}
	tick := time.NewTicker(healthPoll)
	defer tick.Stop()
	var last error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			err = fmt.Errorf("status %d", resp.StatusCode)
		}
		last = err
		select {
		case <-ctx.Done():
			return fmt.Errorf("not healthy after %s: %w", timeout, last)
		case <-tick.C:
		}
	}
}

// lingerOff reports whether systemd would stop the user's services at logout.
func lingerOff(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	outb, err := exec.CommandContext(ctx, "loginctl", "show-user", strconv.Itoa(os.Getuid()), "-p", "Linger").Output()
	return err == nil && strings.TrimSpace(string(outb)) == "Linger=no"
}
