// Package accounts manages back-office accounts. There is exactly one super
// admin, created on first start (EnsureSuperAdmin) and never created,
// deleted, disabled or demoted afterwards; every other account is an
// operator. The super admin manages operators; every account edits its own
// profile and password on "My account"; the server CLI resets the super
// admin's password.
package accounts

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"register_manager/internal/apperr"
	"register_manager/internal/audit"
	"register_manager/internal/auth"
	"register_manager/internal/storage"
	"register_manager/internal/validate"
)

// Service implements account operations.
type Service struct {
	DB  *storage.DB
	Now func() time.Time
}

// Account is an account as shown to the super admin.
type Account struct {
	ID          string    `json:"id"`
	Login       string    `json:"login"`
	Name        string    `json:"name"`
	Locale      *string   `json:"locale"`
	Disabled    bool      `json:"disabled"`
	SuperAdmin  bool      `json:"super_admin"`
	InstanceIDs []string  `json:"instance_ids" doc:"Instances the operator is bound to; empty for the super admin"`
	CreatedAt   time.Time `json:"created_at"`
}

func toView(a storage.Account, instanceIDs []string) Account {
	if instanceIDs == nil {
		instanceIDs = []string{}
	}
	return Account{
		ID: a.ID, Login: a.Login, Name: a.Name, Locale: a.Locale,
		Disabled: a.DisabledAt != nil, SuperAdmin: a.SuperAdmin, InstanceIDs: instanceIDs,
		CreatedAt: storage.FromMillis(a.CreatedAt),
	}
}

var (
	errInvalidCredentials = apperr.Unauthorized("invalid_credentials")
	// The super admin is edited only through "My account" and the CLI.
	errSuperAdminProtected = apperr.Forbidden("super_admin_protected")
)

// DefaultSuperAdminLogin is the login name of the super admin created on
// first start; the super admin can change it later.
const DefaultSuperAdminLogin = "admin"

// Authenticate verifies login and password. Unknown logins and disabled
// accounts fail the same way and take the same time.
func (s *Service) Authenticate(ctx context.Context, login, password string) (storage.Account, error) {
	a, err := storage.GetAccountByLogin(ctx, s.DB.Read(), login)
	if errors.Is(err, sql.ErrNoRows) {
		if err := auth.VerifyDummy(ctx, password); err != nil {
			return storage.Account{}, err
		}
		return storage.Account{}, errInvalidCredentials
	}
	if err != nil {
		return storage.Account{}, err
	}
	ok, err := auth.VerifyPassword(ctx, a.PasswordHash, password)
	if err != nil {
		return storage.Account{}, err
	}
	if !ok || a.DisabledAt != nil {
		return storage.Account{}, errInvalidCredentials
	}
	return a, nil
}

// List returns all accounts with the instances each operator is bound to.
func (s *Service) List(ctx context.Context, actor *auth.Account) ([]Account, error) {
	if !actor.SuperAdmin {
		return nil, apperr.ErrForbidden
	}
	var out []Account
	err := s.DB.ReadTx(ctx, func(tx bun.Tx) error {
		var rows []storage.Account
		err := tx.NewSelect().
			Model(&rows).
			Order("login").
			Scan(ctx)
		if err != nil {
			return err
		}
		var bindings []storage.InstanceOperator
		err = tx.NewSelect().
			Model(&bindings).
			Scan(ctx)
		if err != nil {
			return err
		}
		bound := map[string][]string{}
		for _, b := range bindings {
			bound[b.AccountID] = append(bound[b.AccountID], b.InstanceID)
		}
		out = make([]Account, 0, len(rows))
		for _, a := range rows {
			out = append(out, toView(a, bound[a.ID]))
		}
		return nil
	})
	return out, err
}

// CreateInput is the data for a new operator account.
type CreateInput struct {
	Login    string
	Name     string
	Password string
	Locale   *string
}

func validateProfile(login, name string) (string, string, error) {
	login, err := validate.Login(login)
	if err != nil {
		return "", "", err
	}
	name, err = validate.Name(name, 100)
	return login, name, err
}

func hashNew(ctx context.Context, password string) (string, error) {
	if err := validate.Password(password); err != nil {
		return "", err
	}
	return auth.HashPassword(ctx, password)
}

type newAccount struct {
	login, name, hash  string
	locale             *string
	superAdmin, mustCh bool
}

func insert(ctx context.Context, tx bun.Tx, n newAccount, now time.Time) (storage.Account, error) {
	id := validate.NewID()
	_, err := tx.NewInsert().
		Model(&storage.Account{
			ID:                 id,
			Login:              n.login,
			Name:               n.name,
			PasswordHash:       n.hash,
			Locale:             n.locale,
			SuperAdmin:         n.superAdmin,
			MustChangePassword: n.mustCh,
			CreatedAt:          now.UnixMilli(),
			UpdatedAt:          now.UnixMilli(),
			AuthVersion:        1,
		}).
		Exec(ctx)
	if errors.Is(storage.MapError(err), storage.ErrUnique) {
		return storage.Account{}, apperr.Conflict("login_taken")
	}
	if err != nil {
		return storage.Account{}, err
	}
	return storage.GetAccount(ctx, tx, id)
}

// Create adds an operator account. It is bound to no instance.
func (s *Service) Create(ctx context.Context, actor *auth.Account, in CreateInput) (Account, error) {
	login, name, err := validateProfile(in.Login, in.Name)
	if err != nil {
		return Account{}, err
	}
	locale, err := validate.OptionalLocale(in.Locale)
	if err != nil {
		return Account{}, err
	}
	hash, err := hashNew(ctx, in.Password)
	if err != nil {
		return Account{}, err
	}
	var v Account
	err = s.DB.WriteTx(ctx, func(tx bun.Tx) error {
		if err := actor.RecheckSuperAdmin(ctx, tx); err != nil {
			return err
		}
		now := s.Now()
		a, err := insert(ctx, tx, newAccount{login: login, name: name, hash: hash, locale: locale}, now)
		if err != nil {
			return err
		}
		v = toView(a, nil)
		return audit.Write(ctx, tx, audit.Entry{
			At: now, Actor: audit.AccountActor(actor.ID), Action: "account.create",
			Target: audit.Record("account", a.ID),
			After:  map[string]any{"login": a.Login, "name": a.Name},
		})
	})
	return v, err
}

// getOperator loads an operator account; the super admin is not managed
// through the account management operations.
func getOperator(ctx context.Context, q bun.IDB, id string) (storage.Account, error) {
	a, err := storage.GetAccount(ctx, q, id)
	if errors.Is(err, sql.ErrNoRows) {
		return a, apperr.NotFound("account_not_found")
	}
	if err != nil {
		return a, err
	}
	if a.SuperAdmin {
		return a, errSuperAdminProtected
	}
	return a, nil
}

func (s *Service) updateProfile(ctx context.Context, tx bun.Tx, actor *auth.Account, a storage.Account, login, name, action string) error {
	now := s.Now()
	_, err := tx.NewUpdate().
		Model((*storage.Account)(nil)).
		Set("login = ?", login).
		Set("name = ?", name).
		Set("updated_at = ?", now.UnixMilli()).
		Where("id = ?", a.ID).
		Exec(ctx)
	if errors.Is(storage.MapError(err), storage.ErrUnique) {
		return apperr.Conflict("login_taken")
	}
	if err != nil {
		return err
	}
	return audit.Write(ctx, tx, audit.Entry{
		At: now, Actor: audit.AccountActor(actor.ID), Action: action,
		Target: audit.Record("account", a.ID),
		Before: map[string]any{"login": a.Login, "name": a.Name},
		After:  map[string]any{"login": login, "name": name},
	})
}

// UpdateProfile changes an operator's login and name.
func (s *Service) UpdateProfile(ctx context.Context, actor *auth.Account, id, login, name string) error {
	login, name, err := validateProfile(login, name)
	if err != nil {
		return err
	}
	return s.DB.WriteTx(ctx, func(tx bun.Tx) error {
		if err := actor.RecheckSuperAdmin(ctx, tx); err != nil {
			return err
		}
		a, err := getOperator(ctx, tx, id)
		if err != nil {
			return err
		}
		return s.updateProfile(ctx, tx, actor, a, login, name, "account.update")
	})
}

// ResetPassword sets a new password for an operator and ends all of its
// sessions.
func (s *Service) ResetPassword(ctx context.Context, actor *auth.Account, id, password string) error {
	hash, err := hashNew(ctx, password)
	if err != nil {
		return err
	}
	return s.DB.WriteTx(ctx, func(tx bun.Tx) error {
		if err := actor.RecheckSuperAdmin(ctx, tx); err != nil {
			return err
		}
		if _, err := getOperator(ctx, tx, id); err != nil {
			return err
		}
		now := s.Now()
		if err := setPassword(ctx, tx, id, hash, now.UnixMilli()); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			At: now, Actor: audit.AccountActor(actor.ID), Action: "account.reset_password",
			Target: audit.Record("account", id),
		})
	})
}

// SetDisabled disables or re-enables an operator. Disabling ends all of its
// sessions; its instance bindings stay.
func (s *Service) SetDisabled(ctx context.Context, actor *auth.Account, id string, disabled bool) error {
	return s.DB.WriteTx(ctx, func(tx bun.Tx) error {
		if err := actor.RecheckSuperAdmin(ctx, tx); err != nil {
			return err
		}
		a, err := getOperator(ctx, tx, id)
		if err != nil {
			return err
		}
		if (a.DisabledAt != nil) == disabled {
			return nil
		}
		now := s.Now()
		var at *int64
		action := "account.enable"
		if disabled {
			ms := now.UnixMilli()
			at = &ms
			action = "account.disable"
		}
		if _, err := tx.NewUpdate().
			Model((*storage.Account)(nil)).
			Set("disabled_at = ?", at).
			Set("updated_at = ?", now.UnixMilli()).
			Set("auth_version = auth_version + 1").
			Where("id = ?", id).
			Exec(ctx); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			At: now, Actor: audit.AccountActor(actor.ID), Action: action,
			Target: audit.Record("account", id),
		})
	})
}

// SetLocale stores the account's own interface language (nil clears it).
func (s *Service) SetLocale(ctx context.Context, actor *auth.Account, locale *string) error {
	l, err := validate.OptionalLocale(locale)
	if err != nil {
		return err
	}
	return s.DB.WriteTx(ctx, func(tx bun.Tx) error {
		if err := actor.Recheck(ctx, tx); err != nil {
			return err
		}
		_, writeErr := tx.NewUpdate().
			Model((*storage.Account)(nil)).
			Set("locale = ?", l).
			Set("updated_at = ?", s.Now().UnixMilli()).
			Where("id = ?", actor.ID).
			Exec(ctx)
		return storage.MapError(writeErr)
	})
}

// UpdateOwnProfile changes the signed-in account's own login and name.
func (s *Service) UpdateOwnProfile(ctx context.Context, actor *auth.Account, login, name string) error {
	login, name, err := validateProfile(login, name)
	if err != nil {
		return err
	}
	return s.DB.WriteTx(ctx, func(tx bun.Tx) error {
		if err := actor.Recheck(ctx, tx); err != nil {
			return err
		}
		a, err := storage.GetAccount(ctx, tx, actor.ID)
		if err != nil {
			return err
		}
		return s.updateProfile(ctx, tx, actor, a, login, name, "account.update_own")
	})
}

var errCurrentPassword = apperr.Invalid("current_password_invalid")

// ChangeOwnPassword replaces the signed-in account's password after checking
// the current one. It clears the must-change flag and ends every session of
// the account; the caller starts a new session for the current browser with
// the returned auth version.
func (s *Service) ChangeOwnPassword(ctx context.Context, actor *auth.Account, current, next string) (int64, error) {
	// Password hashing is slow and stays outside the write transaction; the
	// hash checked here must still be current when the change is written.
	row, err := storage.GetAccount(ctx, s.DB.Read(), actor.ID)
	if err != nil {
		return 0, err
	}
	ok, err := auth.VerifyPassword(ctx, row.PasswordHash, current)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, errCurrentPassword
	}
	hash, err := hashNew(ctx, next)
	if err != nil {
		return 0, err
	}
	var version int64
	err = s.DB.WriteTx(ctx, func(tx bun.Tx) error {
		if err := actor.Recheck(ctx, tx); err != nil {
			return err
		}
		a, err := storage.GetAccount(ctx, tx, actor.ID)
		if err != nil {
			return err
		}
		if a.PasswordHash != row.PasswordHash {
			return errCurrentPassword
		}
		now := s.Now()
		if err := setPassword(ctx, tx, a.ID, hash, now.UnixMilli()); err != nil {
			return err
		}
		version = a.AuthVersion + 1
		return audit.Write(ctx, tx, audit.Entry{
			At: now, Actor: audit.AccountActor(actor.ID), Action: "account.change_own_password",
			Target: audit.Record("account", a.ID),
		})
	})
	return version, err
}

// BootstrapCredentials are shown once, only when the super admin is created.
type BootstrapCredentials struct {
	Login    string
	Password string
}

// EnsureSuperAdmin returns nil when the super admin already exists, without
// hashing a password. A newly created account must change its initial password.
func (s *Service) EnsureSuperAdmin(ctx context.Context) (*BootstrapCredentials, error) {
	if _, err := storage.GetSuperAdmin(ctx, s.DB.Read()); !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	password := auth.NewToken(18)
	hash, err := auth.HashPassword(ctx, password)
	if err != nil {
		return nil, err
	}
	var credentials *BootstrapCredentials
	err = s.DB.WriteTx(ctx, func(tx bun.Tx) error {
		a, err := s.ensureSuperAdmin(ctx, tx, hash, true)
		if err == nil && a.PasswordHash == hash {
			credentials = &BootstrapCredentials{Login: a.Login, Password: password}
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return credentials, nil
}

// ensureSuperAdmin returns the super admin, creating it with hash when there
// is none. The partial unique index guarantees a single super admin even if
// two processes start at once.
func (s *Service) ensureSuperAdmin(ctx context.Context, tx bun.Tx, hash string, mustChange bool) (storage.Account, error) {
	a, err := storage.GetSuperAdmin(ctx, tx)
	if err == nil || !errors.Is(err, sql.ErrNoRows) {
		return a, err
	}
	now := s.Now()
	a, err = insert(ctx, tx, newAccount{
		login: DefaultSuperAdminLogin, name: "Administrator", hash: hash, superAdmin: true, mustCh: mustChange,
	}, now)
	if err != nil {
		return a, err
	}
	return a, audit.Write(ctx, tx, audit.Entry{
		At: now, Actor: audit.CLI, Action: "account.create_super_admin",
		Target: audit.Record("account", a.ID),
		After:  map[string]any{"login": a.Login, "name": a.Name},
	})
}

// CLIResetPassword sets the super admin's password from the server command
// line and returns its login name, which the super admin may have changed and
// forgotten. On a database without a super admin it creates one with this
// password. The deployer typed the password, so it need not be changed at
// the next sign-in; all existing super admin sessions end.
func (s *Service) CLIResetPassword(ctx context.Context, password string) (string, error) {
	hash, err := hashNew(ctx, password)
	if err != nil {
		return "", err
	}
	var login string
	err = s.DB.WriteTx(ctx, func(tx bun.Tx) error {
		a, err := s.ensureSuperAdmin(ctx, tx, hash, false)
		if err != nil {
			return err
		}
		login = a.Login
		if a.PasswordHash == hash {
			return nil // just created with this password
		}
		now := s.Now()
		if err := setPassword(ctx, tx, a.ID, hash, now.UnixMilli()); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			At: now, Actor: audit.CLI, Action: "cli.reset_password",
			Target: audit.Record("account", a.ID),
		})
	})
	return login, err
}

func setPassword(ctx context.Context, db bun.IDB, id, hash string, at int64) error {
	_, err := db.NewUpdate().
		Model((*storage.Account)(nil)).
		Set("password_hash = ?", hash).
		Set("must_change_password = ?", false).
		Set("auth_version = auth_version + 1").
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Exec(ctx)
	return storage.MapError(err)
}
