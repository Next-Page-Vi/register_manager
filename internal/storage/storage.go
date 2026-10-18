// Package storage owns the SQLite database: connection pools, pragmas,
// migrations, transactions, backups and the session stores.
//
// All writes go through a single connection whose transactions begin with
// BEGIN IMMEDIATE, so SQLite serializes every write transaction. Reads use a
// separate query_only pool; in WAL mode each read transaction sees a
// consistent snapshot and never blocks or is blocked by the writer.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"register_manager/db"
)

// DB holds the write and read pools.
type DB struct {
	Path  string
	W     *sql.DB
	R     *sql.DB
	read  *bun.DB
	write *bun.DB
}

func dsn(path string, readOnly bool) string {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(FULL)")
	// Deleted content is overwritten with zeros, so deleting an instance
	// leaves no recoverable customer data in the file.
	q.Add("_pragma", "secure_delete(1)")
	if readOnly {
		q.Add("_pragma", "query_only(1)")
	} else {
		q.Add("_pragma", "journal_mode(WAL)")
		q.Set("_txlock", "immediate")
	}
	return path + "?" + q.Encode()
}

// Open opens (creating if needed) the database file, applies pending
// migrations and verifies the connection configuration.
func Open(ctx context.Context, path string, log *slog.Logger) (*DB, error) {
	if path == "" {
		return nil, errors.New("database path is empty")
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	w, err := sql.Open("sqlite", dsn(path, false))
	if err != nil {
		return nil, err
	}
	w.SetMaxOpenConns(1)
	w.SetMaxIdleConns(1)
	w.SetConnMaxLifetime(0)
	w.SetConnMaxIdleTime(0)

	// The writer must exist first so WAL mode is set before readers connect.
	if err := w.PingContext(ctx); err != nil {
		w.Close()
		return nil, fmt.Errorf("open database: %w", err)
	}

	r, err := sql.Open("sqlite", dsn(path, true))
	if err != nil {
		w.Close()
		return nil, err
	}
	n := max(4, runtime.NumCPU())
	r.SetMaxOpenConns(n)
	r.SetMaxIdleConns(n)

	d := &DB{Path: path, W: w, R: r, read: bun.NewDB(r, sqlitedialect.New()), write: bun.NewDB(w, sqlitedialect.New())}
	if err := d.migrate(ctx, log); err != nil {
		d.Close()
		return nil, err
	}
	if err := d.SelfCheck(ctx); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) Close() error {
	return errors.Join(d.read.Close(), d.write.Close())
}

// SelfCheck verifies that pragmas are in effect on both pools and that the
// database passes a quick integrity check.
func (d *DB) SelfCheck(ctx context.Context) error {
	for _, p := range []struct {
		name string
		db   *sql.DB
	}{{"write", d.W}, {"read", d.R}} {
		var fk int
		if err := p.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
			return fmt.Errorf("%s pool: %w", p.name, err)
		}
		if fk != 1 {
			return fmt.Errorf("%s pool: foreign_keys is not enabled", p.name)
		}
		var sd int
		if err := p.db.QueryRowContext(ctx, "PRAGMA secure_delete").Scan(&sd); err != nil {
			return fmt.Errorf("%s pool: %w", p.name, err)
		}
		if sd != 1 {
			return fmt.Errorf("%s pool: secure_delete is not enabled", p.name)
		}
		var mode string
		if err := p.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
			return fmt.Errorf("%s pool: %w", p.name, err)
		}
		if !strings.EqualFold(mode, "wal") {
			return fmt.Errorf("%s pool: journal_mode is %q, want wal", p.name, mode)
		}
	}
	var qc string
	if err := d.R.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&qc); err != nil {
		return err
	}
	if qc != "ok" {
		return fmt.Errorf("quick_check failed: %s", qc)
	}
	return nil
}

func (d *DB) migrate(ctx context.Context, log *slog.Logger) error {
	migrations, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(goose.DialectSQLite3, d.W, migrations)
	if err != nil {
		return err
	}
	pending, err := p.HasPending(ctx)
	if err != nil {
		return err
	}
	if !pending {
		return nil
	}
	current, err := p.GetDBVersion(ctx)
	if err != nil {
		return err
	}
	if current > 0 {
		backup := fmt.Sprintf("%s.pre-migrate-v%d-%s.bak", d.Path, current, time.Now().UTC().Format("20060102T150405Z"))
		if err := d.Backup(ctx, backup); err != nil {
			return fmt.Errorf("backup before migration: %w", err)
		}
		if log != nil {
			log.Info("database backed up before migration", "path", backup)
		}
	}
	results, err := p.Up(ctx)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	for _, r := range results {
		if log != nil {
			log.Info("applied migration", "version", r.Source.Version, "file", r.Source.Path)
		}
	}
	// Table rebuild migrations may toggle foreign keys; verify integrity.
	rows, err := d.W.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("foreign_key_check failed after migration")
	}
	return rows.Err()
}

// Backup writes a consistent snapshot of the database to path using
// VACUUM INTO. The target must not exist.
func (d *DB) Backup(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("backup target %s already exists", path)
	}
	_, err := d.W.ExecContext(ctx, "VACUUM INTO ?", path)
	return err
}

// WriteTx serializes a write transaction using the driver's BEGIN IMMEDIATE.
func (d *DB) WriteTx(ctx context.Context, fn func(tx bun.Tx) error) error {
	return MapError(d.write.RunInTx(ctx, nil, func(_ context.Context, tx bun.Tx) error { return fn(tx) }))
}

// ReadTx gives every query in fn the same read snapshot.
func (d *DB) ReadTx(ctx context.Context, fn func(tx bun.Tx) error) error {
	return MapError(d.read.RunInTx(ctx, nil, func(_ context.Context, tx bun.Tx) error { return fn(tx) }))
}

// Read returns the query-only Bun connection pool.
func (d *DB) Read() *bun.DB { return d.read }

// Checkpoint copies the WAL into the database file and truncates the WAL,
// so pages that held deleted rows leave the WAL too. It reports whether the
// checkpoint completed; an open read transaction can keep it from finishing,
// and a later checkpoint completes it.
func (d *DB) Checkpoint(ctx context.Context) (bool, error) {
	var busy, logFrames, done int
	if err := d.W.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &done); err != nil {
		return false, err
	}
	return busy == 0, nil
}

// Constraint errors surfaced to services.
var (
	ErrOverlap    = errors.New("appointment_overlap")
	ErrUnique     = errors.New("unique_violation")
	ErrForeignKey = errors.New("foreign_key_violation")
	ErrCheck      = errors.New("check_violation")
)

// MapError converts SQLite constraint errors into sentinel errors wrapped
// around the original, leaving other errors unchanged.
func MapError(err error) error {
	if err == nil {
		return nil
	}
	var se *sqlite.Error
	if !errors.As(err, &se) {
		return err
	}
	switch se.Code() {
	case sqlite3.SQLITE_CONSTRAINT_TRIGGER:
		if strings.Contains(se.Error(), "appointment_overlap") {
			return fmt.Errorf("%w: %w", ErrOverlap, err)
		}
	case sqlite3.SQLITE_CONSTRAINT_UNIQUE, sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY:
		return fmt.Errorf("%w: %w", ErrUnique, err)
	case sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY:
		return fmt.Errorf("%w: %w", ErrForeignKey, err)
	case sqlite3.SQLITE_CONSTRAINT_CHECK:
		return fmt.Errorf("%w: %w", ErrCheck, err)
	}
	return err
}

// FromMillis converts UTC Unix milliseconds to a UTC time.
func FromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }
