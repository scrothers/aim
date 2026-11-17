package shim_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/scrothers/aim/internal/shim"
	"github.com/scrothers/aim/internal/wire"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDefaultAddr(t *testing.T) {
	t.Parallel()
	if got := shim.DefaultAddr(env(map[string]string{"AIM_ADDR": "h:1"})); got != "h:1" {
		t.Errorf("got %q", got)
	}
	if got := shim.DefaultAddr(env(map[string]string{"XDG_RUNTIME_DIR": "/x"})); got != "/x/aim/aim.sock" {
		t.Errorf("got %q", got)
	}
	if got := shim.DefaultAddr(env(nil)); !strings.HasPrefix(got, "/run/user/") || !strings.HasSuffix(got, "/aim/aim.sock") {
		t.Errorf("got %q", got)
	}
}

func TestBuildPreamble(t *testing.T) {
	t.Parallel()
	getenv := env(map[string]string{"CLAUDE_CODE_SESSION_ID": "sess", "CLAUDE_PROJECT_DIR": "/proj"})
	var gotDir string
	git := func(_ context.Context, dir string) (string, error) {
		gotDir = dir
		return "git@github.com:Org/Repo.git", nil
	}
	p := shim.BuildPreamble(context.Background(), getenv, git)
	if p.V != wire.Version || p.SessionID != "sess" || p.ProjectDir != "/proj" || gotDir != "/proj" {
		t.Errorf("preamble %+v dir %q", p, gotDir)
	}
	if p.Machine == "" || p.GitRemote != "git@github.com:Org/Repo.git" || p.Namespace != "github.com/org/repo" {
		t.Errorf("preamble %+v", p)
	}
}

func TestBuildPreambleGitFailureAndCwdFallback(t *testing.T) {
	t.Parallel()
	git := func(context.Context, string) (string, error) { return "", errors.New("no repo") }
	p := shim.BuildPreamble(context.Background(), env(nil), git)
	if p.ProjectDir == "" {
		t.Error("expected cwd fallback")
	}
	if p.GitRemote != "" || p.Namespace != "" {
		t.Errorf("preamble %+v", p)
	}
}

func TestGitRemote(t *testing.T) {
	t.Parallel()
	if _, err := shim.GitRemote(context.Background(), t.TempDir()); err == nil {
		t.Error("expected error outside a repository")
	}
}

type fakeDaemon struct {
	pre  wire.Preamble
	rest []byte
	err  error
}

// serve accepts one connection, reads the preamble, echoes the remaining bytes
// after the peer half-closes, then closes.
func serve(t *testing.T, ln net.Listener) (*fakeDaemon, func()) {
	t.Helper()
	fd := &fakeDaemon{}
	var wg sync.WaitGroup
	wg.Go(func() {
		c, err := ln.Accept()
		if err != nil {
			fd.err = err
			return
		}
		defer c.Close()
		br := bufio.NewReader(c)
		fd.pre, fd.err = wire.ReadPreamble(br)
		if fd.err != nil {
			return
		}
		fd.rest, fd.err = io.ReadAll(br) // returns only after the shim half-closes
		if fd.err != nil {
			return
		}
		_, fd.err = c.Write(fd.rest)
	})
	return fd, wg.Wait
}

func TestRunUnix(t *testing.T) {
	t.Parallel()
	sock := filepath.Join(t.TempDir(), "d.sock")
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	fd, wait := serve(t, ln)

	var out bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1}` + "\n")
	p := wire.Preamble{SessionID: "s", ProjectDir: "/p", Machine: "m"}
	if err := shim.Run(context.Background(), shim.Config{Addr: "unix:" + sock}, p, in, &out); err != nil {
		t.Fatal(err)
	}
	wait()
	if fd.err != nil {
		t.Fatal(fd.err)
	}
	if fd.pre.SessionID != "s" || fd.pre.V != wire.Version {
		t.Errorf("preamble %+v", fd.pre)
	}
	if out.String() != `{"jsonrpc":"2.0","id":1}`+"\n" {
		t.Errorf("out = %q", out.String())
	}
}

func TestRunTCP(t *testing.T) {
	t.Parallel()
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	fd, wait := serve(t, ln)

	var out bytes.Buffer
	err = shim.Run(context.Background(), shim.Config{Addr: ln.Addr().String()}, wire.Preamble{SessionID: "t"}, strings.NewReader("hi\n"), &out)
	if err != nil {
		t.Fatal(err)
	}
	wait()
	if fd.err != nil || fd.pre.SessionID != "t" || out.String() != "hi\n" {
		t.Errorf("err=%v pre=%+v out=%q", fd.err, fd.pre, out.String())
	}
}

func TestRunDialFailure(t *testing.T) {
	t.Parallel()
	sock := filepath.Join(t.TempDir(), "none.sock")
	err := shim.Run(context.Background(), shim.Config{Addr: sock}, wire.Preamble{}, strings.NewReader(""), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "aim daemon not reachable at "+sock) {
		t.Errorf("err = %v", err)
	}
}

func TestRunContextCanceled(t *testing.T) {
	t.Parallel()
	sock := filepath.Join(t.TempDir(), "d.sock")
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		accepted <- c
	}()

	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	t.Cleanup(func() { pw.Close() })
	done := make(chan error, 1)
	go func() { done <- shim.Run(ctx, shim.Config{Addr: sock}, wire.Preamble{}, pr, io.Discard) }()
	c := <-accepted
	t.Cleanup(func() { c.Close() })
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}
