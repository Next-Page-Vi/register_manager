package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"register_manager/internal/accounts"
	"register_manager/internal/audit"
	"register_manager/internal/auth"
	"register_manager/internal/instances"
	"register_manager/internal/paging"
	"register_manager/internal/settings"
)

// AccountMe is the signed-in account with the instances it can open.
type AccountMe struct {
	ID                 string                 `json:"id"`
	Login              string                 `json:"login"`
	Name               string                 `json:"name"`
	Locale             *string                `json:"locale"`
	SuperAdmin         bool                   `json:"super_admin"`
	MustChangePassword bool                   `json:"must_change_password" doc:"The super admin must replace its generated initial password before anything else"`
	Instances          []instances.Membership `json:"instances"`
}

type accountMeOut struct {
	Body struct {
		Account *AccountMe `json:"account"`
	}
}

type loginIn struct {
	Body struct {
		Login    string `json:"login" maxLength:"64"`
		Password string `json:"password" maxLength:"256"`
	}
}

type localeIn struct {
	Body struct {
		Locale *string `json:"locale" required:"true" nullable:"true" maxLength:"35"`
	}
}

type profileIn struct {
	Body struct {
		Login string `json:"login" maxLength:"64"`
		Name  string `json:"name" maxLength:"100"`
	}
}

type changePasswordIn struct {
	Body struct {
		CurrentPassword string `json:"current_password" maxLength:"256"`
		NewPassword     string `json:"new_password" maxLength:"256"`
	}
}

func (s *server) accountMe(ctx context.Context) (*AccountMe, error) {
	a, err := signedIn(ctx)
	if err != nil {
		return nil, err
	}
	ms := []instances.Membership{}
	if !a.MustChangePassword {
		if ms, err = s.Instances.ForAccount(ctx, a); err != nil {
			return nil, err
		}
	}

	return &AccountMe{
		ID: a.ID, Login: a.Login, Name: a.Name, Locale: a.Locale, SuperAdmin: a.SuperAdmin,
		MustChangePassword: a.MustChangePassword, Instances: ms,
	}, nil
}

// signedInAs starts a fresh session for the account and returns it as the
// caller sees it. The session middleware ran before the change, so the
// principal is loaded again.
func (s *server) signedInAs(ctx context.Context, id string, version int64) (*out[AccountMe], error) {
	if err := s.Sessions.LoginAccount(ctx, id, version); err != nil {
		return nil, err
	}
	acct, err := loadAccountPrincipal(ctx, s.DB, id, version)
	if err != nil {
		return nil, err
	}
	me, err := s.accountMe(acct)
	if err != nil {
		return nil, err
	}
	return body(*me, nil)
}

func (s *server) registerAccount(a huma.API) {
	huma.Register(a, op(http.MethodGet, "/api/v1/account/me", "getAccountMe", "account", "Current back-office account"),
		func(ctx context.Context, _ *struct{}) (*accountMeOut, error) {
			o := &accountMeOut{}
			if _, err := signedIn(ctx); err != nil {
				return o, nil
			}
			me, err := s.accountMe(ctx)
			if err != nil {
				return nil, err
			}
			o.Body.Account = me
			return o, nil
		})

	huma.Register(a, op(http.MethodPost, "/api/v1/account/login", "login", "account", "Sign in with login name and password"),
		func(ctx context.Context, in *loginIn) (*out[AccountMe], error) {
			if err := s.Limiters.Login.Check(ctx, "login"); err != nil {
				return nil, err
			}
			row, err := s.Accounts.Authenticate(ctx, in.Body.Login, in.Body.Password)
			if err != nil {
				return nil, err
			}
			return s.signedInAs(ctx, row.ID, row.AuthVersion)
		})

	huma.Register(a, noContent(op(http.MethodPost, "/api/v1/account/logout", "logout", "account", "Sign out")),
		func(ctx context.Context, _ *struct{}) (*Empty, error) {
			return nil, s.Sessions.LogoutAccount(ctx)
		})

	huma.Register(a, noContent(op(http.MethodPut, "/api/v1/account/locale", "setAccountLocale", "account", "Set own interface language")),
		as(signedIn, func(ctx context.Context, acct *auth.Account, in *localeIn) (*Empty, error) {
			return nil, s.Accounts.SetLocale(ctx, acct, in.Body.Locale)
		}))

	huma.Register(a, noContent(op(http.MethodPut, "/api/v1/account/profile", "updateOwnProfile", "account", "Change own login name and name")),
		as(account, func(ctx context.Context, acct *auth.Account, in *profileIn) (*Empty, error) {
			return nil, s.Accounts.UpdateOwnProfile(ctx, acct, in.Body.Login, in.Body.Name)
		}))

	huma.Register(a, op(http.MethodPost, "/api/v1/account/password", "changeOwnPassword", "account",
		"Change own password; ends the account's other sessions"),
		as(signedIn, func(ctx context.Context, acct *auth.Account, in *changePasswordIn) (*out[AccountMe], error) {
			if err := s.Limiters.PasswordReset.Check(ctx, acct.ID); err != nil {
				return nil, err
			}
			version, err := s.Accounts.ChangeOwnPassword(ctx, acct, in.Body.CurrentPassword, in.Body.NewPassword)
			if err != nil {
				return nil, err
			}
			// The change ended every session; keep this browser signed in.
			return s.signedInAs(ctx, acct.ID, version)
		}))
}

type accountsOut struct {
	Body struct {
		Accounts []accounts.Account `json:"accounts"`
	}
}

type createAccountIn struct {
	Body struct {
		Login    string  `json:"login" maxLength:"64"`
		Name     string  `json:"name" maxLength:"100"`
		Password string  `json:"password" maxLength:"256"`
		Locale   *string `json:"locale,omitempty" nullable:"true" maxLength:"35"`
	}
}

type updateAccountIn struct {
	AccountID string `path:"account_id"`
	Body      struct {
		Login string `json:"login" maxLength:"64"`
		Name  string `json:"name" maxLength:"100"`
	}
}

type passwordIn struct {
	AccountID string `path:"account_id"`
	Body      struct {
		Password string `json:"password" maxLength:"256"`
	}
}

type disabledIn struct {
	AccountID string `path:"account_id"`
	Body      struct {
		Disabled bool `json:"disabled"`
	}
}

type platformInstancesOut struct {
	Body struct {
		Instances []instances.PlatformInstance `json:"instances"`
	}
}

type createInstanceIn struct {
	Body struct {
		Slug            string  `json:"slug" maxLength:"40"`
		Name            string  `json:"name" maxLength:"100"`
		Timezone        string  `json:"timezone" maxLength:"64"`
		DefaultLocale   string  `json:"default_locale" maxLength:"35"`
		ContactKind     string  `json:"contact_kind" enum:"phone,email,text"`
		ContactRequired bool    `json:"contact_required"`
		CopyFrom        *string `json:"copy_from,omitempty" nullable:"true" doc:"Instance ID to copy the schedule, booking rules and page colour from"`
	}
}

type operatorIn struct {
	InstanceID string `path:"instance_id"`
	AccountID  string `path:"account_id"`
}

type deleteInstanceIn struct {
	InstanceID string `path:"instance_id"`
	Body       struct {
		ConfirmName string `json:"confirm_name" maxLength:"100" doc:"The instance's name, typed by the super admin to confirm"`
	}
}

type platformSettingsIn struct {
	Body struct {
		MaxCycleDays int `json:"max_cycle_days" minimum:"1" maximum:"366"`
	}
}

func (s *server) registerPlatform(a huma.API) {
	const tag = "platform"
	huma.Register(a, op(http.MethodGet, "/api/v1/platform/accounts", "listAccounts", tag, "List back-office accounts"),
		as(superAdmin, func(ctx context.Context, actor *auth.Account, _ *struct{}) (*accountsOut, error) {
			list, err := s.Accounts.List(ctx, actor)
			if err != nil {
				return nil, err
			}
			o := &accountsOut{}
			o.Body.Accounts = list
			return o, nil
		}))

	huma.Register(a, op(http.MethodPost, "/api/v1/platform/accounts", "createAccount", tag, "Create an operator account"),
		as(superAdmin, func(ctx context.Context, actor *auth.Account, in *createAccountIn) (*out[accounts.Account], error) {
			return body(s.Accounts.Create(ctx, actor, accounts.CreateInput{
				Login: in.Body.Login, Name: in.Body.Name, Password: in.Body.Password, Locale: in.Body.Locale,
			}))
		}))

	huma.Register(a, noContent(op(http.MethodPut, "/api/v1/platform/accounts/{account_id}", "updateAccount", tag, "Edit an operator's login name and name")),
		as(superAdmin, func(ctx context.Context, actor *auth.Account, in *updateAccountIn) (*Empty, error) {
			return nil, s.Accounts.UpdateProfile(ctx, actor, in.AccountID, in.Body.Login, in.Body.Name)
		}))

	huma.Register(a, noContent(op(http.MethodPost, "/api/v1/platform/accounts/{account_id}/password", "resetAccountPassword", tag, "Reset an operator's password and end its sessions")),
		as(superAdmin, func(ctx context.Context, actor *auth.Account, in *passwordIn) (*Empty, error) {
			if err := s.Limiters.PasswordReset.Check(ctx, actor.ID); err != nil {
				return nil, err
			}
			return nil, s.Accounts.ResetPassword(ctx, actor, in.AccountID, in.Body.Password)
		}))

	huma.Register(a, noContent(op(http.MethodPut, "/api/v1/platform/accounts/{account_id}/disabled", "setAccountDisabled", tag, "Disable or enable an operator")),
		as(superAdmin, func(ctx context.Context, actor *auth.Account, in *disabledIn) (*Empty, error) {
			return nil, s.Accounts.SetDisabled(ctx, actor, in.AccountID, in.Body.Disabled)
		}))

	huma.Register(a, op(http.MethodGet, "/api/v1/platform/instances", "listPlatformInstances", tag, "List all instances"),
		as(superAdmin, func(ctx context.Context, actor *auth.Account, _ *struct{}) (*platformInstancesOut, error) {
			list, err := s.Instances.List(ctx, actor)
			if err != nil {
				return nil, err
			}
			o := &platformInstancesOut{}
			o.Body.Instances = list
			return o, nil
		}))

	huma.Register(a, op(http.MethodPost, "/api/v1/platform/instances", "createInstance", tag, "Create an instance (without operators)"),
		as(superAdmin, func(ctx context.Context, actor *auth.Account, in *createInstanceIn) (*out[instances.PlatformInstance], error) {
			b := in.Body
			return body(s.Instances.Create(ctx, actor, instances.CreateInput{
				Slug: b.Slug, Name: b.Name, Timezone: b.Timezone, DefaultLocale: b.DefaultLocale,
				ContactKind: b.ContactKind, ContactRequired: b.ContactRequired, CopyFrom: b.CopyFrom,
			}))
		}))

	huma.Register(a, op(http.MethodPut, "/api/v1/platform/instances/{instance_id}/operators/{account_id}", "bindOperator", tag, "Bind an operator to the instance"),
		as(superAdmin, func(ctx context.Context, actor *auth.Account, in *operatorIn) (*out[instances.PlatformInstance], error) {
			return body(s.Instances.SetOperator(ctx, actor, in.InstanceID, in.AccountID, true))
		}))

	huma.Register(a, op(http.MethodDelete, "/api/v1/platform/instances/{instance_id}/operators/{account_id}", "unbindOperator", tag, "Remove an operator from the instance"),
		as(superAdmin, func(ctx context.Context, actor *auth.Account, in *operatorIn) (*out[instances.PlatformInstance], error) {
			return body(s.Instances.SetOperator(ctx, actor, in.InstanceID, in.AccountID, false))
		}))

	huma.Register(a, op(http.MethodGet, "/api/v1/platform/instances/{instance_id}/deletion-preview", "instanceDeletionPreview", tag, "What deleting the instance would remove"),
		as(superAdmin, func(ctx context.Context, actor *auth.Account, in *instancePath) (*out[instances.DeletionImpact], error) {
			return body(s.Instances.DeletionPreview(ctx, actor, in.InstanceID))
		}))

	huma.Register(a, op(http.MethodDelete, "/api/v1/platform/instances/{instance_id}", "deleteInstance", tag,
		"Delete the instance and all of its data; returns what was removed"),
		as(superAdmin, func(ctx context.Context, actor *auth.Account, in *deleteInstanceIn) (*out[instances.DeletionImpact], error) {
			return body(s.Instances.Delete(ctx, actor, in.InstanceID, in.Body.ConfirmName))
		}))

	huma.Register(a, op(http.MethodGet, "/api/v1/platform/settings", "getPlatformSettings", tag, "System-wide settings"),
		as(superAdmin, func(ctx context.Context, actor *auth.Account, _ *struct{}) (*out[settings.Settings], error) {
			return body(s.Settings.Get(ctx, actor))
		}))

	huma.Register(a, op(http.MethodPut, "/api/v1/platform/settings", "updatePlatformSettings", tag, "Change system-wide settings"),
		as(superAdmin, func(ctx context.Context, actor *auth.Account, in *platformSettingsIn) (*out[settings.Settings], error) {
			return body(s.Settings.Update(ctx, actor, in.Body.MaxCycleDays))
		}))

	huma.Register(a, op(http.MethodGet, "/api/v1/platform/audit", "listPlatformAudit", tag, "Platform audit log"),
		as(superAdmin, func(ctx context.Context, actor *auth.Account, in *PageParams) (*out[paging.Page[audit.LogEntry]], error) {
			return body(s.Audit.List(ctx, actor, nil, in.Page()))
		}))
}
