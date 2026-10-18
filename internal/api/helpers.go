package api

import (
	"context"

	"register_manager/internal/auth"
	"register_manager/internal/storage"
)

// loadAccountPrincipal builds a context carrying a freshly loaded account,
// used right after login before the next request's middleware runs.
func loadAccountPrincipal(ctx context.Context, db *storage.DB, id string, version int64) (context.Context, error) {
	a, err := auth.LoadAccount(ctx, db.Read(), id, version)
	if err != nil {
		return nil, err
	}
	return auth.WithAccount(ctx, a), nil
}
