package devwatch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Config configures Run.
type Config struct {
	Root         string        // module root to watch and build in
	Interval     time.Duration // poll interval
	Debounce     time.Duration // quiet period after the last change
	Bin          string        // output binary path
	Args         []string      // daemon arguments
	StopTimeout  time.Duration // SIGTERM grace before SIGKILL
	Stdout       io.Writer
	Stderr       io.Writer
	Logger       *slog.Logger
	BuildCommand func(ctx context.Context) *exec.Cmd // nil: go build ./cmd/aim
}

// Manager builds and supervises a single daemon process.
type Manager struct {
	cfg   Config
	cur   *proc
	count int
}

type proc struct {
	cmd  *exec.Cmd
	done chan struct{}
}

// NewManager returns a Manager with defaults applied.
func NewManager(cfg Config) *Manager {
	if cfg.StopTimeout == 0 {
		cfg.StopTimeout = 10 * time.Second
	}
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Manager{cfg: cfg}
}

func (m *Manager) build(ctx context.Context) error {
	var cmd *exec.Cmd
	if m.cfg.BuildCommand != nil {
		cmd = m.cfg.BuildCommand(ctx)
	} else {
		cmd = exec.CommandContext(ctx, "go", "build", "-o", m.cfg.Bin, "./cmd/aim")
		cmd.Dir = m.cfg.Root
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		_, _ = m.cfg.Stderr.Write(out.Bytes())
		return fmt.Errorf("build: %w", err)
	}
	return nil
}

// Rebuild builds the daemon; on success it replaces the running process. On
// build failure the running process is left untouched.
func (m *Manager) Rebuild(ctx context.Context) error {
	if err := m.build(ctx); err != nil {
		m.cfg.Logger.Error("build failed, keeping current daemon", "err", err)
		return err
	}
	m.Stop()
	cmd := exec.Command(m.cfg.Bin, m.cfg.Args...) //nolint:gosec // dev tool runs its own build output
	cmd.Dir = m.cfg.Root
	cmd.Stdout, cmd.Stderr = m.cfg.Stdout, m.cfg.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	p := &proc{cmd: cmd, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		m.cfg.Logger.Info("daemon exited", "pid", cmd.Process.Pid, "err", err)
		close(p.done)
	}()
	m.cur = p
	m.count++
	m.cfg.Logger.Info(fmt.Sprintf("restarted (%d)", m.count), "pid", cmd.Process.Pid)
	return nil
}

// Stop terminates the running daemon: SIGTERM, then SIGKILL after StopTimeout.
func (m *Manager) Stop() {
	p := m.cur
	m.cur = nil
	if p == nil {
		return
	}
	select {
	case <-p.done:
		return
	default:
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(m.cfg.StopTimeout):
		m.cfg.Logger.Warn("daemon ignored SIGTERM, sending SIGKILL", "timeout", m.cfg.StopTimeout)
		_ = p.cmd.Process.Kill()
		<-p.done
	}
}

// Run builds and starts the daemon, then rebuilds on source changes until ctx
// is cancelled, at which point the daemon is stopped.
func Run(ctx context.Context, cfg Config) error {
	m := NewManager(cfg)
	defer m.Stop()
	w := NewWatcher(cfg.Root, cfg.Debounce)
	if err := m.Rebuild(ctx); err != nil && ctx.Err() == nil {
		m.cfg.Logger.Warn("initial start failed; waiting for changes", "err", err)
	}
	t := time.NewTicker(cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-t.C:
			if w.Poll(now) {
				m.cfg.Logger.Info("change detected, rebuilding")
				if err := m.Rebuild(ctx); err != nil && errors.Is(err, context.Canceled) {
					return nil
				}
			}
		}
	}
}
