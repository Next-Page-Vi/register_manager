package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"

	"register_manager/internal/storage"
)

type countingStore struct {
	scs.CtxStore
	commits atomic.Int32
}

func (c *countingStore) CommitCtx(ctx context.Context, token string, b []byte, expiry time.Time) error {
	c.commits.Add(1)
	return c.CtxStore.CommitCtx(ctx, token, b, expiry)
}

// With an idle timeout, scs commits the session on every request to push
// the expiry forward, even when nothing in it changed. SessionStore skips
// rewriting the row in that case.
func TestSessionCommittedOnEveryRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "s.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewSessions(ctx, db, false, nil)
	store := &countingStore{CtxStore: s.Account.Store.(scs.CtxStore)}
	s.Account.Store = store

	login := s.Account.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Account.Put(r.Context(), keyAccountID, "a")
	}))
	read := s.Account.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = s.Account.GetString(r.Context(), keyAccountID)
	}))

	rec := httptest.NewRecorder()
	login.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	cookies := rec.Result().Cookies()
	before := store.commits.Load()
	expiry := func() (e int64) {
		if err := db.R.QueryRow("SELECT expiry FROM account_sessions").Scan(&e); err != nil {
			t.Fatal(err)
		}
		return e
	}
	stored := expiry()
	for range 2 {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		read.ServeHTTP(httptest.NewRecorder(), req)
	}
	if got := store.commits.Load() - before; got != 2 {
		t.Fatalf("read-only requests committed %d times, want 2", got)
	}
	// The commits found the session unchanged, so the row was not rewritten.
	if expiry() != stored {
		t.Error("unchanged session was rewritten")
	}
}
