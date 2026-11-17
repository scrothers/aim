package webui_test

import (
	"io/fs"
	"testing"

	"github.com/scrothers/aim/internal/webui"
)

func TestFSAndBuilt(t *testing.T) {
	t.Parallel()
	fsys := webui.FS()
	if fsys == nil {
		t.Fatal("FS() is nil")
	}
	if _, err := fs.Stat(fsys, "."); err != nil {
		t.Errorf("stat root: %v", err)
	}
	_, err := fs.Stat(fsys, "index.html")
	if got, want := webui.Built(), err == nil; got != want {
		t.Errorf("Built() = %v, index.html present = %v", got, want)
	}
}
