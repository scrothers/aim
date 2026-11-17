package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/maintenance"
	"github.com/scrothers/aim/internal/store"
)

func say(w io.Writer, format string, a ...any) error {
	if _, err := fmt.Fprintf(w, format, a...); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func defaultBackupDir() string {
	home, _ := os.UserHomeDir()
	return maintenance.DefaultDir(os.Getenv, home)
}

func newBackupCmd() *cobra.Command {
	home, _ := os.UserHomeDir()
	var dbPath, backupDir string
	cmd := &cobra.Command{
		Use:   "backup [path]",
		Short: "Write a consistent online backup of the database",
		Long: "Write a backup with VACUUM INTO, safe while the daemon runs. With a path, " +
			"the backup is written there (the file must not exist). Without one it goes " +
			"into the backup directory as aim-YYYYMMDDTHHMMSSZ.db, old backups beyond " +
			"the keep count are removed, and the outcome is recorded for the console.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			st, err := store.Open(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer func() { _ = st.Close() }()
			if len(args) == 1 {
				if err := st.Backup(ctx, args[0]); err != nil {
					return err
				}
				fi, err := os.Stat(args[0])
				if err != nil {
					return fmt.Errorf("stat backup: %w", err)
				}
				return say(cmd.OutOrStdout(), "backup written to %s (%d bytes)\n", args[0], fi.Size())
			}
			r := maintenance.New(st, mail.New(st, nil), newLogger(cmd.ErrOrStderr(), slog.LevelWarn), maintenance.WithDefaultDir(backupDir))
			res, err := r.BackupNow(ctx)
			if err != nil {
				return err
			}
			return say(cmd.OutOrStdout(), "backup written to %s (%d bytes)\n", res.LastPath, res.LastSize)
		},
	}
	f := cmd.Flags()
	f.StringVar(&dbPath, "db", envOr(os.Getenv, "AIM_DB", defaultDB(os.Getenv, home)), "SQLite database path (AIM_DB)")
	f.StringVar(&backupDir, "backup-dir", envOr(os.Getenv, "AIM_BACKUP_DIR", defaultBackupDir()), "backup directory when no path is given (AIM_BACKUP_DIR)")
	return cmd
}

func newMaintenanceCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "maintenance", Short: "Database housekeeping"}
	cmd.AddCommand(newPruneCmd())
	return cmd
}

func newPruneCmd() *cobra.Command {
	home, _ := os.UserHomeDir()
	var dbPath string
	var dryRun bool
	var days int
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Delete old fully archived mail under the retention setting",
		Long: "Delete messages older than the configured retention whose every delivery is " +
			"archived. Retention is off by default; set it in the console or override " +
			"it once with --days. Use --dry-run to count without deleting.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			st, err := store.Open(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer func() { _ = st.Close() }()
			svc := mail.New(st, nil)
			var res mail.PruneResult
			if cmd.Flags().Changed("days") {
				res, err = svc.PruneAfter(ctx, time.Now(), days, dryRun)
			} else {
				res, err = svc.Prune(ctx, time.Now(), dryRun)
			}
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			switch {
			case !res.Enabled:
				return say(out, "retention is off (prune_archived_after_days = 0); nothing to do\n")
			case dryRun:
				return say(out, "would delete %d messages created before %s\n", res.Messages, res.Cutoff.Format(time.RFC3339))
			default:
				return say(out, "deleted %d messages created before %s\n", res.Messages, res.Cutoff.Format(time.RFC3339))
			}
		},
	}
	f := cmd.Flags()
	f.StringVar(&dbPath, "db", envOr(os.Getenv, "AIM_DB", defaultDB(os.Getenv, home)), "SQLite database path (AIM_DB)")
	f.BoolVar(&dryRun, "dry-run", false, "count what would be deleted without deleting")
	f.IntVar(&days, "days", 0, "override the retention age in days for this run")
	return cmd
}
