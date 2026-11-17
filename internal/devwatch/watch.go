// Package devwatch rebuilds and restarts a development daemon when Go sources
// change. It is dev tooling only and is not used by the shipped binaries.
package devwatch

import (
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

// skipDirs are root-relative directories (slash separated) never scanned.
var skipDirs = map[string]bool{
	".git":                true,
	".dev":                true,
	"web":                 true,
	"internal/webui/dist": true,
	"test/e2e":            true,
}

// watched reports whether a file name is a build input.
func watched(name string) bool {
	switch name {
	case "go.mod", "go.sum":
		return true
	}
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

// Watcher polls a tree for build-input changes and debounces them.
type Watcher struct {
	root     string
	debounce time.Duration
	prev     map[string]time.Time
	pending  bool
	last     time.Time
}

// NewWatcher snapshots root so that only later changes are reported.
func NewWatcher(root string, debounce time.Duration) *Watcher {
	w := &Watcher{root: root, debounce: debounce}
	w.prev = w.snapshot()
	return w
}

// Poll rescans the tree at time now. It returns true once changes have been
// seen and no further change has occurred for the debounce interval.
func (w *Watcher) Poll(now time.Time) bool {
	cur := w.snapshot()
	if !sameSnapshot(w.prev, cur) {
		w.pending = true
		w.last = now
	}
	w.prev = cur
	if w.pending && now.Sub(w.last) >= w.debounce {
		w.pending = false
		return true
	}
	return false
}

func (w *Watcher) snapshot() map[string]time.Time {
	snap := make(map[string]time.Time)
	_ = filepath.WalkDir(w.root, func(path string, d fs.DirEntry, _ error) error {
		if d == nil { // unreadable root
			return nil
		}
		// Per-entry errors are ignored: files vanish mid-edit.
		rel, _ := filepath.Rel(w.root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && (skipDirs[rel] || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !watched(d.Name()) {
			return nil
		}
		if info, ierr := d.Info(); ierr == nil {
			snap[rel] = info.ModTime()
		}
		return nil
	})
	return snap
}

func sameSnapshot(a, b map[string]time.Time) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if t, ok := b[k]; !ok || !t.Equal(v) {
			return false
		}
	}
	return true
}
