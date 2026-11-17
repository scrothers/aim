package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/store"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&strings.Builder{})
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestBackupCmd(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db := filepath.Join(dir, "aim.db")
	dest := filepath.Join(dir, "copy.db")
	out, err := run(t, "backup", dest, "--db", db)
	if err != nil || !strings.Contains(out, "backup written to "+dest) {
		t.Fatalf("backup to path: %q, %v", out, err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "backup", dest, "--db", db); err == nil {
		t.Error("overwriting an existing backup should fail")
	}
	bdir := filepath.Join(dir, "backups")
	out, err = run(t, "backup", "--db", db, "--backup-dir", bdir)
	if err != nil || !strings.Contains(out, bdir) {
		t.Fatalf("backup to dir: %q, %v", out, err)
	}
	ents, _ := os.ReadDir(bdir)
	if len(ents) != 1 || !strings.HasPrefix(ents[0].Name(), "aim-") {
		t.Errorf("backups = %v", ents)
	}
}

func TestPruneCmd(t *testing.T) {
	t.Parallel()
	db := filepath.Join(t.TempDir(), "aim.db")
	out, err := run(t, "maintenance", "prune", "--db", db)
	if err != nil || !strings.Contains(out, "retention is off") {
		t.Fatalf("off: %q, %v", out, err)
	}
	out, err = run(t, "maintenance", "prune", "--db", db, "--days", "30", "--dry-run")
	if err != nil || !strings.Contains(out, "would delete 0 messages") {
		t.Fatalf("dry run: %q, %v", out, err)
	}
	st, err := store.Open(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if err := mail.New(st, nil).SetRetention(context.Background(), mail.RetentionSettings{PruneArchivedAfterDays: 7}); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	out, err = run(t, "maintenance", "prune", "--db", db)
	if err != nil || !strings.Contains(out, "deleted 0 messages") {
		t.Fatalf("prune: %q, %v", out, err)
	}
}
