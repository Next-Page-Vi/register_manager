// Package auth handles authentication for back-office accounts and
// customers: password and token secrets, the two independent session
// managers, request principals, CSRF protection, rate limiting and instance
// role checks.
package auth

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"register_manager/internal/apperr"
	"register_manager/internal/storage"
)

// Account is an authenticated back-office account for the current request:
// the one super admin or an operator.
type Account struct {
	ID          string
	Login       string
	Name        string
	Locale      *string
	AuthVersion int64
	SuperAdmin  bool
	// MustChangePassword is set for the super admin until it replaces the
	// generated initial password; the API then allows little else.
	MustChangePassword bool
}

// Customer is an authenticated customer for the current request.
type Customer struct {
	ID          string
	InstanceID  string
	Name        string
	Locale      *string
	AuthVersion int64
}

type ctxKey int

const (
	accountKey ctxKey = iota
	requestKey
)

// WithAccount stores the account principal in ctx.
func WithAccount(ctx context.Context, a *Account) context.Context {
	return context.WithValue(ctx, accountKey, a)
}

// AccountFrom returns the authenticated account or ErrUnauthenticated.
func AccountFrom(ctx context.Context) (*Account, error) {
	if a, ok := ctx.Value(accountKey).(*Account); ok && a != nil {
		return a, nil
	}
	return nil, apperr.ErrUnauthenticated
}

// SuperAdminFrom returns the authenticated account if it is a super admin.
func SuperAdminFrom(ctx context.Context) (*Account, error) {
	a, err := AccountFrom(ctx)
	if err != nil {
		return nil, err
	}
	if !a.SuperAdmin {
		return nil, apperr.ErrForbidden
	}
	return a, nil
}

// LoadAccount loads and validates the account for a session: it must exist,
// not be disabled, and match the session's auth version.
func LoadAccount(ctx context.Context, q bun.IDB, id string, version int64) (*Account, error) {
	row, err := storage.GetAccount(ctx, q, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.ErrUnauthenticated
	}
	if err != nil {
		return nil, err
	}
	if row.DisabledAt != nil || row.AuthVersion != version {
		return nil, apperr.ErrUnauthenticated
	}
	return &Account{
		ID: row.ID, Login: row.Login, Name: row.Name, Locale: row.Locale,
		AuthVersion: row.AuthVersion, SuperAdmin: row.SuperAdmin,
		MustChangePassword: row.MustChangePassword,
	}, nil
}

// Recheck re-validates the account inside a write transaction.
func (a *Account) Recheck(ctx context.Context, q bun.IDB) error {
	fresh, err := LoadAccount(ctx, q, a.ID, a.AuthVersion)
	if err != nil {
		return err
	}
	a.SuperAdmin, a.MustChangePassword = fresh.SuperAdmin, fresh.MustChangePassword
	return nil
}

// RecheckSuperAdmin re-validates the account and requires super admin.
func (a *Account) RecheckSuperAdmin(ctx context.Context, q bun.IDB) error {
	if err := a.Recheck(ctx, q); err != nil {
		return err
	}
	if !a.SuperAdmin {
		return apperr.ErrForbidden
	}
	return nil
}

// LoadCustomer loads and validates the customer for a session.
func LoadCustomer(ctx context.Context, q bun.IDB, instanceID, id string, version int64) (*Customer, error) {
	row, err := storage.GetCustomer(ctx, q, instanceID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.ErrUnauthenticated
	}
	if err != nil {
		return nil, err
	}
	if row.Status != "approved" || row.AuthVersion != version {
		return nil, apperr.ErrUnauthenticated
	}
	return &Customer{
		ID: row.ID, InstanceID: row.InstanceID, Name: row.Name,
		Locale: row.Locale, AuthVersion: row.AuthVersion,
	}, nil
}

// Recheck re-validates the customer inside a write transaction.
func (c *Customer) Recheck(ctx context.Context, q bun.IDB) error {
	_, err := LoadCustomer(ctx, q, c.InstanceID, c.ID, c.AuthVersion)
	return err
}

// Role is an account's relationship to an instance.
type Role string

const (
	RoleOperator Role = "operator"
	RoleSuper    Role = "super"
)

// InstanceRole returns the account's role for inst: the super admin reaches
// every instance, an operator only the instances it is bound to. Accounts
// without access get a not-found error so instance existence is not
// revealed.
func InstanceRole(ctx context.Context, q bun.IDB, a *Account, inst storage.Instance) (Role, error) {
	if a.SuperAdmin {
		return RoleSuper, nil
	}
	bound, err := q.NewSelect().
		Model((*storage.InstanceOperator)(nil)).
		Where("instance_id = ? AND account_id = ?", inst.ID, a.ID).Exists(ctx)
	if err != nil {
		return "", err
	}
	if bound {
		return RoleOperator, nil
	}
	return "", apperr.NotFound("instance_not_found")
}

// Access is a verified account access to an instance.
type Access struct {
	Account  *Account
	Instance storage.Instance
	Role     Role
}

// AuthorizeInstance loads the instance and verifies the account's access.
func AuthorizeInstance(ctx context.Context, q bun.IDB, a *Account, instanceID string) (*Access, error) {
	inst, err := storage.GetInstance(ctx, q, instanceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("instance_not_found")
	}
	if err != nil {
		return nil, err
	}
	role, err := InstanceRole(ctx, q, a, inst)
	if err != nil {
		return nil, err
	}
	return &Access{Account: a, Instance: inst, Role: role}, nil
}

// AuthorizeInstanceTx re-validates the account and its instance access inside
// a write transaction, returning the current instance row.
func AuthorizeInstanceTx(ctx context.Context, q bun.IDB, a *Account, instanceID string) (*Access, error) {
	if err := a.Recheck(ctx, q); err != nil {
		return nil, err
	}
	return AuthorizeInstance(ctx, q, a, instanceID)
}

// InstanceWrite runs fn in a write transaction once the account and its
// access to the instance are re-validated inside it.
func InstanceWrite(ctx context.Context, db *storage.DB, a *Account, instanceID string, fn func(tx bun.Tx, acc *Access) error) error {
	return db.WriteTx(ctx, func(tx bun.Tx) error {
		acc, err := AuthorizeInstanceTx(ctx, tx, a, instanceID)
		if err != nil {
			return err
		}
		return fn(tx, acc)
	})
}

// InstanceRead runs fn in a read transaction once the account's access to
// the instance is checked in the same snapshot.
func InstanceRead(ctx context.Context, db *storage.DB, a *Account, instanceID string, fn func(tx bun.Tx, acc *Access) error) error {
	return db.ReadTx(ctx, func(tx bun.Tx) error {
		acc, err := AuthorizeInstance(ctx, tx, a, instanceID)
		if err != nil {
			return err
		}
		return fn(tx, acc)
	})
}
