package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	msqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/scrothers/aim/internal/store/db"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const baseDSNOptions = "_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"

// Store is the SQLite-backed persistence layer.
type Store struct {
	writer *sql.DB
	reader *sql.DB
	wq     *db.Queries
	rq     *db.Queries
}

func dsn(path, extra string) string {
	esc := strings.NewReplacer("?", "%3f", "#", "%23", "%", "%25").Replace(path)
	return "file:" + esc + "?" + baseDSNOptions + extra
}

// Open opens (creating if needed) the database at path, applies all pending
// migrations, and returns a Store. The parent directory is created with mode
// 0o700.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("store: create dir: %w", err)
	}
	if err := migrateUp(ctx, path); err != nil {
		return nil, err
	}

	writer, err := sql.Open("sqlite", dsn(path, "&_txlock=immediate"))
	if err != nil {
		return nil, fmt.Errorf("store: open writer: %w", err)
	}
	writer.SetMaxOpenConns(1)

	reader, err := sql.Open("sqlite", dsn(path, "&_pragma=query_only(1)"))
	if err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("store: open reader: %w", err)
	}
	return &Store{writer: writer, reader: reader, wq: db.New(writer), rq: db.New(reader)}, nil
}

// migrateUp applies pending migrations on a dedicated connection pool, because
// the migrate driver closes the *sql.DB it is given.
func migrateUp(ctx context.Context, path string) error {
	conn, err := sql.Open("sqlite", dsn(path, "&_txlock=immediate"))
	if err != nil {
		return fmt.Errorf("store: open migration conn: %w", err)
	}
	conn.SetMaxOpenConns(1)
	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return fmt.Errorf("store: ping migration conn: %w", err)
	}
	drv, err := msqlite.WithInstance(conn, &msqlite.Config{})
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("store: migrate driver: %w", err)
	}
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		_ = drv.Close()
		return fmt.Errorf("store: migrate source: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	if err != nil {
		_ = drv.Close()
		_ = src.Close()
		return fmt.Errorf("store: migrate init: %w", err)
	}
	upErr := m.Up()
	srcErr, dbErr := m.Close() // closes conn
	if upErr != nil && !errors.Is(upErr, migrate.ErrNoChange) {
		return fmt.Errorf("store: migrate up: %w", upErr)
	}
	if err := errors.Join(srcErr, dbErr); err != nil {
		return fmt.Errorf("store: migrate close: %w", err)
	}
	return nil
}

// Close closes both connection pools.
func (s *Store) Close() error {
	if err := errors.Join(s.writer.Close(), s.reader.Close()); err != nil {
		return fmt.Errorf("store: close: %w", err)
	}
	return nil
}

// Ping verifies both pools can reach the database.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.writer.PingContext(ctx); err != nil {
		return fmt.Errorf("store: ping writer: %w", err)
	}
	if err := s.reader.PingContext(ctx); err != nil {
		return fmt.Errorf("store: ping reader: %w", err)
	}
	return nil
}

// Read returns queries bound to the read-only reader pool.
func (s *Store) Read() *db.Queries { return s.rq }

// WithTx runs fn inside an immediate write transaction, rolling back when fn
// returns an error and committing otherwise.
func (s *Store) WithTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	if err := fn(s.wq.WithTx(tx)); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return errors.Join(err, fmt.Errorf("store: rollback: %w", rbErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}
