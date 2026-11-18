package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// BackupSuffix is appended to a file's path for the copy AIM keeps of the
// original before its first modification.
const BackupSuffix = ".aim-bak"

// Apply writes the plan's file changes, then runs its commands. Parents are
// created 0o755; an existing file keeps its mode, else 0o644; writes are
// atomic. Before the first modification of an existing file it is copied to
// <path>.aim-bak (kept if already present); a removal moves the file there
// instead of deleting it.
func Apply(ctx context.Context, plan Plan, run func(ctx context.Context, argv []string) error) error {
	for _, c := range plan.Changes {
		if err := applyChange(c); err != nil {
			return err
		}
	}
	if len(plan.Commands) > 0 && run == nil {
		return errors.New("plan has commands but no runner")
	}
	for _, cmd := range plan.Commands {
		if err := run(ctx, cmd.Argv); err != nil {
			return fmt.Errorf("run %s: %w", strings.Join(cmd.Argv, " "), err)
		}
	}
	return nil
}

func applyChange(c Change) error {
	target := c.Path
	if resolved, err := filepath.EvalSymlinks(c.Path); err == nil {
		target = resolved
	}
	info, statErr := os.Stat(target)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", c.Path, statErr)
	}
	if c.After == nil {
		if !exists {
			return nil
		}
		dst, err := freeBackupPath(c.Path + BackupSuffix)
		if err != nil {
			return err
		}
		if err := os.Rename(target, dst); err != nil {
			return fmt.Errorf("move %s to backup: %w", c.Path, err)
		}
		return nil
	}
	mode := fs.FileMode(0o644)
	if exists {
		mode = info.Mode().Perm()
		if err := backupOnce(target, c.Path+BackupSuffix, mode); err != nil {
			return err
		}
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".aim-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", c.Path, err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(c.After); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write %s: %w", c.Path, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("write %s: %w", c.Path, err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		cleanup()
		return fmt.Errorf("chmod %s: %w", c.Path, err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		cleanup()
		return fmt.Errorf("replace %s: %w", c.Path, err)
	}
	return nil
}

// backupOnce copies src to backup unless backup already exists.
func backupOnce(src, backup string, mode fs.FileMode) error {
	if _, err := os.Lstat(backup); err == nil {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s for backup: %w", src, err)
	}
	if err := os.WriteFile(backup, data, mode); err != nil {
		return fmt.Errorf("write backup %s: %w", backup, err)
	}
	return nil
}

// freeBackupPath returns base, or base.N, whichever does not exist yet, so a
// removal never overwrites an earlier backup.
func freeBackupPath(base string) (string, error) {
	p := base
	for i := 1; i < 1000; i++ {
		if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
			return p, nil
		}
		p = fmt.Sprintf("%s.%d", base, i)
	}
	return "", fmt.Errorf("no free backup name for %s", base)
}

// Describe renders a dry-run view of the plan: each path with a compact
// -/+ line diff, then commands and notes.
func Describe(plan Plan) string {
	var b strings.Builder
	for _, c := range plan.Changes {
		switch {
		case c.After == nil:
			fmt.Fprintf(&b, "--- %s (remove; kept as %s%s)\n", c.Path, filepath.Base(c.Path), BackupSuffix)
		case c.Before == nil:
			fmt.Fprintf(&b, "+++ %s (create)\n", c.Path)
		default:
			fmt.Fprintf(&b, "+++ %s (modify; original kept as %s%s)\n", c.Path, filepath.Base(c.Path), BackupSuffix)
		}
		for _, l := range lineDiff(c.Before, c.After) {
			b.WriteString(l)
			b.WriteByte('\n')
		}
	}
	for _, cmd := range plan.Commands {
		fmt.Fprintf(&b, "$ %s\n", strings.Join(cmd.Argv, " "))
		if cmd.Note != "" {
			fmt.Fprintf(&b, "  # %s\n", cmd.Note)
		}
	}
	for _, n := range plan.Notes {
		fmt.Fprintf(&b, "note: %s\n", n)
	}
	return b.String()
}

func splitLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// lineDiff returns "- line" and "+ line" entries from an LCS diff.
func lineDiff(before, after []byte) []string {
	if bytes.Equal(before, after) {
		return nil
	}
	a, b := splitLines(before), splitLines(after)
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []string
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			i++
			j++
		case j >= len(b) || (i < len(a) && lcs[i+1][j] >= lcs[i][j+1]):
			out = append(out, "- "+a[i])
			i++
		default:
			out = append(out, "+ "+b[j])
			j++
		}
	}
	return out
}

// ReadOptional reads path through sys, returning nil data when it does not
// exist.
func ReadOptional(sys System, path string) ([]byte, error) {
	data, err := sys.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

// ShellQuote returns s quoted for a POSIX shell, unchanged when it is made of
// safe characters only.
func ShellQuote(s string) string {
	safe := s != ""
	for _, r := range s {
		letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
		if !letter && !strings.ContainsRune("/._-+:@%", r) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// HookCommand returns the shell string `<bin> hook <runtime> [args...]`.
func HookCommand(bin, runtime string, args ...string) string {
	return strings.Join(append([]string{ShellQuote(bin), "hook", runtime}, args...), " ")
}

// NewChange returns a Change from before to after, or false when they are
// identical (including both absent).
func NewChange(path string, before, after []byte) (Change, bool) {
	if bytes.Equal(before, after) && (before == nil) == (after == nil) {
		return Change{}, false
	}
	return Change{Path: path, Before: before, After: after}, true
}

// HooksChange reads path, merges entries into its Claude-shaped hooks object
// and returns the planned change. A missing file on uninstall plans nothing.
func HooksChange(sys System, path, runtime string, entries []HookEntry, uninstall bool) (Change, bool, error) {
	before, err := ReadOptional(sys, path)
	if err != nil {
		return Change{}, false, err
	}
	if before == nil && uninstall {
		return Change{}, false, nil
	}
	after, err := MergeHooks(path, before, runtime, entries, uninstall)
	if err != nil {
		return Change{}, false, err
	}
	c, ok := NewChange(path, before, after)
	return c, ok, nil
}

// MCPRegistered reports whether `argv` (a "get"/"list" style probe) succeeds
// and, when contains is non-empty, mentions it in its output.
func MCPRegistered(ctx context.Context, sys System, contains string, argv ...string) bool {
	out, err := sys.Output(ctx, argv...)
	if err != nil {
		return false
	}
	return contains == "" || bytes.Contains(out, []byte(contains))
}
