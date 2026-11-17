package shim

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/scrothers/aim/internal/wire"
)

// Config configures Run.
type Config struct {
	// Addr is a unix socket path (absolute, or "unix:/path") or "host:port".
	Addr string
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

// Run dials the daemon, sends p, and pipes stdin to the daemon and the daemon
// to stdout. It returns when the daemon closes the connection or ctx ends.
func Run(ctx context.Context, cfg Config, p wire.Preamble, stdin io.Reader, stdout io.Writer) error {
	nw, addr := network(cfg.Addr)
	var d net.Dialer
	conn, err := d.DialContext(ctx, nw, addr)
	if err != nil {
		return fmt.Errorf("aim daemon not reachable at %s: %w", cfg.Addr, err)
	}
	defer func() { _ = conn.Close() }()

	// Unblock the copy below when ctx is canceled.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	if err := wire.WritePreamble(conn, p); err != nil {
		return fmt.Errorf("shim: %w", err)
	}

	go func() {
		_, _ = io.Copy(conn, stdin)
		if cw, ok := conn.(closeWriter); ok {
			_ = cw.CloseWrite()
		}
	}()

	if _, err := io.Copy(stdout, conn); err != nil && ctx.Err() == nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("shim: copy from daemon: %w", err)
	}
	return ctx.Err()
}
