package storage

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"
)

// SessionStore implements scs.Store and scs.CtxStore on one of the session
// tables. Account and customer sessions use separate tables.
type SessionStore struct {
	db    *DB
	table string
}

// NewSessionStore returns a store on table, which must be "account_sessions"
// or "customer_sessions", and starts a cleanup goroutine that stops when ctx
// is done.
func NewSessionStore(ctx context.Context, d *DB, table string, log *slog.Logger) *SessionStore {
	switch table {
	case "account_sessions", "customer_sessions":
	default:
		panic("unknown session table " + table)
	}
	s := &SessionStore{db: d, table: table}
	go s.cleanup(ctx, 10*time.Minute, log)
	return s
}

func (s *SessionStore) Find(token string) ([]byte, bool, error) {
	return s.FindCtx(context.Background(), token)
}

func (s *SessionStore) Commit(token string, b []byte, expiry time.Time) error {
	return s.CommitCtx(context.Background(), token, b, expiry)
}

func (s *SessionStore) Delete(token string) error {
	return s.DeleteCtx(context.Background(), token)
}

func (s *SessionStore) FindCtx(ctx context.Context, token string) ([]byte, bool, error) {
	var b []byte
	err := s.db.R.QueryRowContext(ctx,
		"SELECT data FROM "+s.table+" WHERE token = ? AND expiry > ?",
		token, time.Now().UnixMilli()).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

// commitSlack is how far a session's stored expiry may lag behind. With an
// idle timeout scs commits on every request just to move the expiry
// forward; an unchanged session is rewritten only once it lags by more than
// this, so most requests change no page and cause no WAL write or fsync.
// Idle timeouts are days, so an hour of lag is immaterial.
const commitSlack = time.Hour

func (s *SessionStore) CommitCtx(ctx context.Context, token string, b []byte, expiry time.Time) error {
	_, err := s.db.W.ExecContext(ctx,
		"INSERT INTO "+s.table+" (token, data, expiry) VALUES (?, ?, ?) "+
			"ON CONFLICT (token) DO UPDATE SET data = excluded.data, expiry = excluded.expiry "+
			"WHERE data IS NOT excluded.data OR excluded.expiry - expiry > ?",
		token, b, expiry.UnixMilli(), commitSlack.Milliseconds())
	return err
}

func (s *SessionStore) DeleteCtx(ctx context.Context, token string) error {
	_, err := s.db.W.ExecContext(ctx, "DELETE FROM "+s.table+" WHERE token = ?", token)
	return err
}

func (s *SessionStore) cleanup(ctx context.Context, every time.Duration, log *slog.Logger) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, err := s.db.W.ExecContext(ctx, "DELETE FROM "+s.table+" WHERE expiry <= ?", time.Now().UnixMilli())
			if err != nil && ctx.Err() == nil && log != nil {
				log.Warn("session cleanup failed", "table", s.table, "err", err)
			}
		}
	}
}
