package storage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/uptrace/bun"
	sqlite3 "modernc.org/sqlite/lib"
)

func TestBunExistingDataAndNullValues(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "existing.db")
	db, err := Open(ctx, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Write through database/sql, so the read-side mapping is independent of Bun.
	at := int64(1780300800123)
	name := "中文 O'Reilly; DROP TABLE accounts; --"
	_, err = db.W.Exec(`INSERT INTO accounts (id, login, name, password_hash, auth_version, disabled_at, locale, super_admin, must_change_password, created_at, updated_at) VALUES (?, ?, ?, ?, 3, 0, '', 0, 1, ?, ?)`, "operator", "operator", name, "test-hash", at, at)
	if err != nil {
		t.Fatal(err)
	}
	type schemaEntry struct {
		Name string
		SQL  *string
	}
	schema := func(db *DB) []schemaEntry {
		t.Helper()
		rows, err := db.R.Query(`SELECT name, sql FROM sqlite_schema ORDER BY name`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []schemaEntry
		for rows.Next() {
			var s schemaEntry
			if err := rows.Scan(&s.Name, &s.SQL); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	before := schema(db)
	var version int64
	if err := db.R.QueryRow(`SELECT max(version_id) FROM goose_db_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(ctx, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	row, err := GetAccount(ctx, db.Read(), "operator")
	if err != nil {
		t.Fatal(err)
	}
	if row.Name != name || row.CreatedAt != at || row.UpdatedAt != at || row.DisabledAt == nil || *row.DisabledAt != 0 || row.Locale == nil || *row.Locale != "" || row.SuperAdmin || !row.MustChangePassword {
		t.Fatalf("incorrect existing row: %+v", row)
	}
	err = db.WriteTx(ctx, func(tx bun.Tx) error {
		row.Locale, row.DisabledAt, row.MustChangePassword = nil, nil, false
		row.UpdatedAt++
		_, err := tx.NewUpdate().Model(&row).Column("locale", "disabled_at", "must_change_password", "updated_at").WherePK().Exec(ctx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var locale sql.NullString
	var disabled sql.NullInt64
	var mustChange, updated int64
	if err := db.R.QueryRow(`SELECT locale, disabled_at, must_change_password, updated_at FROM accounts WHERE id = 'operator'`).Scan(&locale, &disabled, &mustChange, &updated); err != nil {
		t.Fatal(err)
	}
	if locale.Valid || disabled.Valid || mustChange != 0 || updated != at+1 {
		t.Fatalf("NULL/zero update: %v %v %d %d", locale, disabled, mustChange, updated)
	}
	var currentVersion int64
	if err := db.R.QueryRow(`SELECT max(version_id) FROM goose_db_version`).Scan(&currentVersion); err != nil {
		t.Fatal(err)
	}
	if currentVersion != version || !reflect.DeepEqual(before, schema(db)) {
		t.Fatal("opening existing data changed schema or migration version")
	}
}

func TestBunWriteBeginsImmediate(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	other, err := sql.Open("sqlite", dsn(db.Path, false))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	other.SetMaxOpenConns(1)
	if _, err := other.ExecContext(ctx, "PRAGMA busy_timeout=0"); err != nil {
		t.Fatal(err)
	}
	err = db.WriteTx(ctx, func(_ bun.Tx) error {
		// No statement has run in the first transaction: BEGIN itself must
		// already hold the write reservation, including across processes.
		tx, err := other.BeginTx(ctx, nil)
		if err == nil {
			tx.Rollback()
			t.Fatal("another connection acquired the write reservation")
		}
		var coded interface{ Code() int }
		if !errors.As(err, &coded) || coded.Code() != sqlite3.SQLITE_BUSY {
			t.Fatalf("expected SQLITE_BUSY, got %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBunTransactionRollbackAndCancellation(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	insert := func(tx bun.Tx, id string) error {
		_, err := tx.NewInsert().Model(&Account{ID: id, Login: id, Name: id, PasswordHash: "test", AuthVersion: 1}).Exec(ctx)
		return err
	}
	failure := errors.New("rollback requested")
	if err := db.WriteTx(ctx, func(tx bun.Tx) error {
		if err := insert(tx, "error"); err != nil {
			return err
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if got := recover(); got != failure {
				t.Errorf("panic changed: %v", got)
			}
		}()
		_ = db.WriteTx(ctx, func(tx bun.Tx) error {
			if err := insert(tx, "panic"); err != nil {
				t.Fatal(err)
			}
			panic(failure)
		})
	}()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := db.WriteTx(cancelled, func(tx bun.Tx) error { t.Error("cancelled transaction started"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled transaction: %v", err)
	}
	if err := db.Read().NewSelect().Model((*Account)(nil)).Scan(ctx, new(Account)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("rolled-back account exists: %v", err)
	}
	if err := db.WriteTx(ctx, func(tx bun.Tx) error { return insert(tx, "after") }); err != nil {
		t.Fatalf("writer not released: %v", err)
	}
}

func TestBunReadSnapshot(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	err := db.ReadTx(ctx, func(tx bun.Tx) error {
		before, err := tx.NewSelect().Model((*Account)(nil)).Count(ctx)
		if err != nil {
			return err
		}
		if err := db.WriteTx(ctx, func(writer bun.Tx) error {
			_, err := writer.NewInsert().Model(&Account{ID: "new", Login: "new", Name: "New", PasswordHash: "test", AuthVersion: 1}).Exec(ctx)
			return err
		}); err != nil {
			return err
		}
		after, err := tx.NewSelect().Model((*Account)(nil)).Count(ctx)
		if before != after {
			t.Errorf("read snapshot changed: %d -> %d", before, after)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GetAccount(ctx, db.Read(), "new"); err != nil {
		t.Fatal(err)
	}
}
