package shim

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/scrothers/aim/internal/wire"
)

// Config configures Run.
type Config struct {
	// Addr is a unix socket path (absolute, or "unix:/path") or "host:port".
	Addr string
	// ReconnectTimeout bounds how long a request waits for the daemon to come
	// back before failing with a JSON-RPC unavailable error; zero means
	// DefaultReconnectTimeout.
	ReconnectTimeout time.Duration
	// BackoffMin and BackoffMax bound the redial delay, which doubles from
	// the first to the second; zero means 100ms and 2s.
	BackoffMin time.Duration
	BackoffMax time.Duration
	// Logger receives reconnect diagnostics (stderr in the CLI); nil discards.
	Logger *slog.Logger
}

// DefaultAddr returns AIM_ADDR, else $XDG_RUNTIME_DIR/aim/aim.sock, else
// /run/user/<uid>/aim/aim.sock.
func DefaultAddr(getenv func(string) string) string {
	if a := getenv("AIM_ADDR"); a != "" {
		return a
	}
	if x := getenv("XDG_RUNTIME_DIR"); x != "" {
		return filepath.Join(x, "aim", "aim.sock")
	}
	return filepath.Join("/run/user", strconv.Itoa(os.Getuid()), "aim", "aim.sock")
}

// GitRemote returns the origin remote URL of the repository containing dir.
func GitRemote(ctx context.Context, dir string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", fmt.Errorf("shim: git remote get-url: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// BuildPreamble assembles the session preamble from the environment. A git
// failure leaves the remote and namespace empty.
func BuildPreamble(
	ctx context.Context,
	getenv func(string) string,
	git func(ctx context.Context, dir string) (string, error),
) wire.Preamble {
	p := wire.Preamble{
		V:          wire.Version,
		SessionID:  getenv("CLAUDE_CODE_SESSION_ID"),
		ProjectDir: getenv("CLAUDE_PROJECT_DIR"),
	}
	if p.ProjectDir == "" {
		if wd, err := os.Getwd(); err == nil {
			p.ProjectDir = wd
		}
	}
	if h, err := os.Hostname(); err == nil {
		p.Machine = h
	}
	if remote, err := git(ctx, p.ProjectDir); err == nil && remote != "" {
		p.GitRemote = remote
		if ns, ok := wire.NamespaceFromRemote(remote); ok {
			p.Namespace = ns
		}
	}
	return p
}

func network(addr string) (string, string) {
	if rest, ok := strings.CutPrefix(addr, "unix:"); ok {
		return "unix", rest
	}
	if strings.HasPrefix(addr, "/") {
		return "unix", addr
	}
	return "tcp", addr
}

type closeWriter interface{ CloseWrite() error }
