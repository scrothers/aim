package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/scrothers/aim/internal/daemon"
	"github.com/scrothers/aim/internal/shim"
)

// version reports the main module version from the embedded build info.
func version() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

// envOr returns the environment value for key, or def when unset or empty.
func envOr(getenv func(string) string, key, def string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return def
}

// envSet is like envOr but an explicitly set empty value wins, so that
// AIM_LISTEN="" disables a listener.
func envSet(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func defaultDB(getenv func(string) string, home string) string {
	if x := getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "aim", "aim.db")
	}
	return filepath.Join(home, ".local", "state", "aim", "aim.db")
}

func parseLevel(s string) (slog.Level, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return 0, fmt.Errorf("invalid --log-level %q: %w", s, err)
	}
	return l, nil
}

func newLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "aim",
		Short:         "Agent Instant Messaging",
		Version:       version(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newDaemonCmd(), newMCPCmd())
	return root
}

func newDaemonCmd() *cobra.Command {
	home, _ := os.UserHomeDir()
	var cfg daemon.Config
	var level string
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the AIM daemon",
		RunE: func(cmd *cobra.Command, _ []string) error {
			lvl, err := parseLevel(level)
			if err != nil {
				return err
			}
			cfg.Version = version()
			return daemon.Run(cmd.Context(), cfg, newLogger(cmd.ErrOrStderr(), lvl))
		},
	}
	f := cmd.Flags()
	f.StringVar(&cfg.DBPath, "db", envOr(os.Getenv, "AIM_DB", defaultDB(os.Getenv, home)), "SQLite database path (AIM_DB)")
	f.StringVar(&cfg.Socket, "socket", envOr(os.Getenv, "AIM_SOCKET", shim.DefaultAddr(func(k string) string {
		if k == "AIM_ADDR" {
			return ""
		}
		return os.Getenv(k)
	})), "unix socket path; empty disables (AIM_SOCKET)")
	f.StringVar(&cfg.Listen, "listen", envSet("AIM_LISTEN", ":7171"), "TCP listen address; empty disables (AIM_LISTEN)")
	f.StringVar(&cfg.HTTP, "http", envSet("AIM_HTTP", ":7172"), "HTTP listen address; empty disables (AIM_HTTP)")
	f.StringVar(&level, "log-level", "info", "log level: debug, info, warn, error")
	return cmd
}

func newMCPCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run the stdio MCP shim (connects to the daemon)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			p := shim.BuildPreamble(ctx, os.Getenv, shim.GitRemote)
			return shim.Run(ctx, shim.Config{Addr: addr}, p, cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&addr, "addr", shim.DefaultAddr(os.Getenv), "daemon address: unix socket path or host:port (AIM_ADDR)")
	return cmd
}
