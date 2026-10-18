package storage

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"
)

func open(t *testing.T) *DB {
	t.Helper()
	d, err := Open(context.Background(), filepath.Join(t.TempDir(), "s.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestPragmasOnEveryConnection(t *testing.T) {
	d := open(t)
	// Force several read connections to exist and check each.
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := d.R.Conn(context.Background())
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			var fk int
			if err := conn.QueryRowContext(context.Background(), "PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
				t.Errorf("foreign_keys=%d err=%v", fk, err)
			}
			var sd int
			if err := conn.QueryRowContext(context.Background(), "PRAGMA secure_delete").Scan(&sd); err != nil || sd != 1 {
				t.Errorf("secure_delete=%d err=%v", sd, err)
			}
			if _, err := conn.ExecContext(context.Background(), "CREATE TABLE x (a INT)"); err == nil {
				t.Error("read pool accepted a write")
			}
		}()
	}
	wg.Wait()
}

func TestReadsAreNotBlockedByWriter(t *testing.T) {
	d := open(t)
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- d.WriteTx(context.Background(), func(tx bun.Tx) error {
			if _, err := tx.Tx.Exec(`INSERT INTO accounts (id, login, name, password_hash, created_at, updated_at) VALUES ('a', 'a', 'A', 'x', 0, 0)`); err != nil {
				return err
			}
			close(started)
			<-release
			return nil
		})
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var n int
	// The uncommitted row is invisible and the read does not wait.
	if err := d.R.QueryRowContext(ctx, "SELECT count(*) FROM accounts").Scan(&n); err != nil || n != 0 {
		t.Errorf("read during write: n=%d err=%v", n, err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := d.R.QueryRow("SELECT count(*) FROM accounts").Scan(&n); err != nil || n != 1 {
		t.Errorf("after commit: %d %v", n, err)
	}
}

func TestConcurrentWritersAreSerialized(t *testing.T) {
	d := open(t)
	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for i := range 50 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- d.WriteTx(context.Background(), func(tx bun.Tx) error {
				var n int
				if err := tx.Tx.QueryRow("SELECT count(*) FROM accounts").Scan(&n); err != nil {
					return err
				}
				_, err := tx.Tx.Exec(`INSERT INTO accounts (id, login, name, password_hash, created_at, updated_at) VALUES (?, ?, 'A', 'x', 0, 0)`,
					string(rune('a'+i%26))+time.Now().String(), time.Now().String()+string(rune(i)))
				return err
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("writer failed (busy error leaked?): %v", err)
		}
	}
}

func TestCheckConstraints(t *testing.T) {
	d := open(t)
	err := d.WriteTx(context.Background(), func(tx bun.Tx) error {
		_, err := tx.Tx.Exec(`INSERT INTO schedule_overrides (instance_id, date, periods, updated_at) VALUES ('i', '2026-02-30', '[]', 0)`)
		return err
	})
	if err == nil {
		t.Error("invalid date accepted")
	}
}

func TestSessionCommitSkipsSmallExpiryMoves(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := open(t)
	s := NewSessionStore(ctx, d, "account_sessions", nil)
	stored := func() int64 {
		var e int64
		if err := d.R.QueryRow("SELECT expiry FROM account_sessions WHERE token = 't'").Scan(&e); err != nil {
			t.Fatal(err)
		}
		return e
	}
	base := time.Now().Add(24 * time.Hour)
	if err := s.CommitCtx(ctx, "t", []byte("a"), base); err != nil {
		t.Fatal(err)
	}
	steps := []struct {
		data   string
		expiry time.Time
		want   time.Time
	}{
		{"a", base.Add(time.Minute), base},                                              // unchanged, small move: skipped
		{"a", base.Add(2 * time.Hour), base.Add(2 * time.Hour)},                         // unchanged, large move: written
		{"b", base.Add(2*time.Hour + time.Minute), base.Add(2*time.Hour + time.Minute)}, // changed: written
	}
	for i, st := range steps {
		if err := s.CommitCtx(ctx, "t", []byte(st.data), st.expiry); err != nil {
			t.Fatal(err)
		}
		if got := stored(); got != st.want.UnixMilli() {
			t.Errorf("step %d: stored expiry %d, want %d", i, got, st.want.UnixMilli())
		}
	}
}
