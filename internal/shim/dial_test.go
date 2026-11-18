package shim_test

import (
	"context"
	"net"
	"path/filepath"
	"testing"

	"github.com/scrothers/aim/internal/shim"
)

func TestDial(t *testing.T) {
	t.Parallel()
	sock := filepath.Join(t.TempDir(), "d.sock")
	ul, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ul.Close() })
	tl, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tl.Close() })

	for _, addr := range []string{sock, "unix:" + sock, tl.Addr().String()} {
		c, err := shim.Dial(context.Background(), addr)
		if err != nil {
			t.Errorf("Dial(%q): %v", addr, err)
			continue
		}
		_ = c.Close()
	}
	if _, err := shim.Dial(context.Background(), filepath.Join(t.TempDir(), "none.sock")); err == nil {
		t.Error("Dial to a missing socket succeeded")
	}
}
