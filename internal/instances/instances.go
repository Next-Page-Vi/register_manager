// Package instances manages business instances (calendars): creation,
// deletion, operator bindings, status, settings and registration invite
// codes.
package instances

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"register_manager/internal/apperr"
	"register_manager/internal/audit"
	"register_manager/internal/auth"
	"register_manager/internal/booking"
	"register_manager/internal/schedule"
	"register_manager/internal/storage"
	"register_manager/internal/validate"
)

// Service implements instance operations.
type Service struct {
	DB  *storage.DB
	Now func() time.Time
}

// Instance is an instance's settings.
type Instance struct {
	ID              string    `json:"id"`
	Slug            string    `json:"slug"`
	Name            string    `json:"name"`
	Status          string    `json:"status" enum:"active,paused"`
	Timezone        string    `json:"timezone"`
	DefaultLocale   string    `json:"default_locale"`
	ContactKind     string    `json:"contact_kind" enum:"phone,email,text"`
	ContactRequired bool      `json:"contact_required"`
	BrandColor      *string   `json:"brand_color" doc:"Colour for the top of customer pages, \"#rrggbb\"; null for the standard look."`
	CreatedAt       time.Time `json:"created_at"`
}

// ToView converts a row.
func ToView(i storage.Instance) Instance {
	return Instance{
		ID: i.ID, Slug: i.Slug, Name: i.Name, Status: i.Status, Timezone: i.Timezone,
		DefaultLocale: i.DefaultLocale, ContactKind: i.ContactKind, ContactRequired: i.ContactRequired,
		BrandColor: i.BrandColor, CreatedAt: storage.FromMillis(i.CreatedAt),
	}
}

// Membership is an instance the account can open.
type Membership struct {
	Instance
	Role auth.Role `json:"role" enum:"operator,super"`
}

// ForAccount lists the instances the account can open: every instance for
// the super admin, the bound instances for an operator.
func (s *Service) ForAccount(ctx context.Context, a *auth.Account) ([]Membership, error) {
	q := s.DB.Read()
	rows, role := []storage.Instance(nil), auth.RoleOperator
	var err error
	if a.SuperAdmin {
		rows, err = listInstances(ctx, q, nil)
		role = auth.RoleSuper
	} else {
		rows, err = listInstances(ctx, q, &a.ID)
	}
	if err != nil {
		return nil, err
	}
	out := make([]Membership, 0, len(rows))
	for _, i := range rows {
		out = append(out, Membership{Instance: ToView(i), Role: role})
	}
	return out, nil
}

// AccountRef is an operator bound to an instance.
type AccountRef struct {
	ID       string `json:"id"`
	Login    string `json:"login"`
	Name     string `json:"name"`
	Disabled bool   `json:"disabled"`
}

// PlatformInstance is an instance with its bound operators, for the super
// admin.
type PlatformInstance struct {
	Instance
	Operators []AccountRef `json:"operators"`
}

// List returns all instances for the super admin.
func (s *Service) List(ctx context.Context, actor *auth.Account) ([]PlatformInstance, error) {
	if !actor.SuperAdmin {
		return nil, apperr.ErrForbidden
	}
	var out []PlatformInstance
	err := s.DB.ReadTx(ctx, func(tx bun.Tx) error {
		rows, err := listInstances(ctx, tx, nil)
		if err != nil {
			return err
		}
		ops, err := listOperators(ctx, tx, nil)
		if err != nil {
			return err
		}
		byInstance := map[string][]AccountRef{}
		for _, o := range ops {
			byInstance[o.InstanceID] = append(byInstance[o.InstanceID], AccountRef{ID: o.ID, Login: o.Login, Name: o.Name, Disabled: o.DisabledAt != nil})
		}
		out = make([]PlatformInstance, 0, len(rows))
		for _, i := range rows {
			refs := byInstance[i.ID]
			if refs == nil {
				refs = []AccountRef{}
			}
			out = append(out, PlatformInstance{Instance: ToView(i), Operators: refs})
		}
		return nil
	})
	return out, err
}

func platformView(ctx context.Context, q bun.IDB, i storage.Instance) (PlatformInstance, error) {
	ops, err := listOperators(ctx, q, &i.ID)
	if err != nil {
		return PlatformInstance{}, err
	}
	refs := make([]AccountRef, 0, len(ops))
	for _, o := range ops {
		refs = append(refs, AccountRef{ID: o.ID, Login: o.Login, Name: o.Name, Disabled: o.DisabledAt != nil})
	}
	return PlatformInstance{Instance: ToView(i), Operators: refs}, nil
}

// CreateInput is the data for a new instance. With CopyFrom set, the new
// instance starts with that instance's schedule, booking rules and page
// colour; customers, appointments, special dates, blocks, invite code and
// operators are never copied. (The form fills time zone and language from
// the source instance, but they are sent like any other field.)
type CreateInput struct {
	Slug            string
	Name            string
	Timezone        string
	DefaultLocale   string
	ContactKind     string
	ContactRequired bool
	CopyFrom        *string
}

// DefaultAnchor is a Monday, so the default 7-day cycle lines up with
// weekdays.
var DefaultAnchor = schedule.NewDate(2024, time.January, 1)

func defaultCycle() schedule.Cycle {
	work, _ := schedule.NewDayPlan([]schedule.Period{{Start: 9 * 60, End: 12 * 60}, {Start: 13 * 60, End: 17 * 60}})
	c, _ := schedule.NewCycle(DefaultAnchor, []schedule.DayPlan{work, work, work, work, work, schedule.Off, schedule.Off})
	return c
}

func validContactKind(k string) error {
	switch k {
	case "phone", "email", "text":
		return nil
	}
	return apperr.Invalid("invalid_contact_kind")
}

// Create creates an instance in one transaction together with its schedule,
// booking policy and invite code. New instances are paused and have no
// operators; the super admin binds them afterwards.
func (s *Service) Create(ctx context.Context, actor *auth.Account, in CreateInput) (PlatformInstance, error) {
	var err error
	if in.Slug, err = validate.Slug(in.Slug); err != nil {
		return PlatformInstance{}, err
	}
	if in.Name, err = validate.Name(in.Name, 100); err != nil {
		return PlatformInstance{}, err
	}
	location, err := validate.Timezone(in.Timezone)
	if err != nil {
		return PlatformInstance{}, err
	}
	in.Timezone = location.String()
	if in.DefaultLocale, err = validate.Locale(in.DefaultLocale); err != nil {
		return PlatformInstance{}, err
	}
	if err = validContactKind(in.ContactKind); err != nil {
		return PlatformInstance{}, err
	}
	var pv PlatformInstance
	err = s.DB.WriteTx(ctx, func(tx bun.Tx) error {
		if err := actor.RecheckSuperAdmin(ctx, tx); err != nil {
			return err
		}
		now := s.Now()
		ms := now.UnixMilli()

		var brand *string
		if in.CopyFrom != nil {
			src, err := getInstance(ctx, tx, *in.CopyFrom)
			if err != nil {
				return err
			}
			brand = src.BrandColor
		}

		id := validate.NewID()
		_, err := tx.NewInsert().
			Model(&storage.Instance{
				ID:                     id,
				Slug:                   in.Slug,
				Name:                   in.Name,
				Timezone:               in.Timezone,
				DefaultLocale:          in.DefaultLocale,
				RegistrationInviteCode: auth.NewInviteCode(),
				ContactKind:            in.ContactKind,
				ContactRequired:        in.ContactRequired,
				BrandColor:             brand,
				CreatedAt:              ms,
				UpdatedAt:              ms,
				Status:                 "paused",
			}).
			Exec(ctx)
		if errors.Is(storage.MapError(err), storage.ErrUnique) {
			return apperr.Conflict("slug_taken")
		}
		if err != nil {
			return err
		}
		if err := initSettings(ctx, tx, id, in.CopyFrom, ms); err != nil {
			return err
		}
		inst, err := storage.GetInstance(ctx, tx, id)
		if err != nil {
			return err
		}
		if pv, err = platformView(ctx, tx, inst); err != nil {
			return err
		}
		after := map[string]any{"instance": pv.Instance}
		if in.CopyFrom != nil {
			after["copied_from"] = in.CopyFrom
		}
		return audit.Write(ctx, tx, audit.Entry{
			At: now, Actor: audit.AccountActor(actor.ID), Action: "instance.create",
			Target: audit.Record("instance", id), After: after,
		})
	})
	return pv, err
}

// initSettings gives a new instance its booking rules and schedule: copied
// from the source instance, or the defaults when source is empty.
func initSettings(ctx context.Context, q bun.IDB, id string, source *string, ms int64) error {
	if source != nil {
		return copySettings(ctx, q, id, *source, ms)
	}
	p := booking.DefaultPolicy()
	if err := booking.StorePolicy(ctx, q, id, p, ms); err != nil {
		return err
	}
	c := defaultCycle()
	return storage.StoreSchedule(ctx, q, &storage.Schedule{
		InstanceID:      id,
		CycleLengthDays: int64(c.Length()),
		AnchorDate:      c.Anchor().String(),
		CycleDays:       string(c.DaysJSON()),
		UpdatedAt:       ms,
	})
}

func getInstance(ctx context.Context, q bun.IDB, id string) (storage.Instance, error) {
	i, err := storage.GetInstance(ctx, q, id)
	if errors.Is(err, sql.ErrNoRows) {
		return i, apperr.NotFound("instance_not_found")
	}
	return i, err
}

// SetOperator binds an operator to an instance or removes the binding. Only
// enabled operator accounts can be bound; the super admin reaches every
// instance and is never bound. Both directions are idempotent.
func (s *Service) SetOperator(ctx context.Context, actor *auth.Account, instanceID, accountID string, bind bool) (PlatformInstance, error) {
	var pv PlatformInstance
	err := s.DB.WriteTx(ctx, func(tx bun.Tx) error {
		if err := actor.RecheckSuperAdmin(ctx, tx); err != nil {
			return err
		}
		inst, err := getInstance(ctx, tx, instanceID)
		if err != nil {
			return err
		}
		now := s.Now()
		action := "instance.unbind_operator"
		if bind {
			action = "instance.bind_operator"
			a, err := storage.GetAccount(ctx, tx, accountID)
			if errors.Is(err, sql.ErrNoRows) {
				return apperr.NotFound("account_not_found")
			}
			if err != nil {
				return err
			}
			if a.SuperAdmin {
				return apperr.Conflict("cannot_bind_super_admin")
			}
			if a.DisabledAt != nil {
				return apperr.Conflict("account_disabled")
			}
			if _, err := tx.NewInsert().
				Model(&storage.InstanceOperator{InstanceID: instanceID, AccountID: accountID, CreatedAt: now.UnixMilli()}).
				On("CONFLICT (instance_id, account_id) DO NOTHING").
				Exec(ctx); err != nil {
				return err
			}
		} else if _, err := tx.NewDelete().
			Model((*storage.InstanceOperator)(nil)).
			Where("instance_id = ? AND account_id = ?", instanceID, accountID).
			Exec(ctx); err != nil {
			return err
		}
		if pv, err = platformView(ctx, tx, inst); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			At: now, InstanceID: &instanceID, Actor: audit.AccountActor(actor.ID), Action: action,
			Target: audit.Record("account", accountID),
		})
	})
	return pv, err
}

// DeletionImpact is what deleting an instance removes, shown before the
// super admin confirms.
type DeletionImpact struct {
	Customers          int64 `json:"customers"`
	FutureAppointments int64 `json:"future_appointments" doc:"Booked appointments that have not started yet"`
	Appointments       int64 `json:"appointments" doc:"All appointments, including past and cancelled ones"`
}

func impact(ctx context.Context, q bun.IDB, instanceID string, now time.Time) (DeletionImpact, error) {
	var out DeletionImpact
	err := q.NewRaw(`SELECT
 (SELECT count(*) FROM customers WHERE instance_id = ?) AS customers,
 (SELECT count(*) FROM appointments WHERE instance_id = ? AND status = 'booked' AND starts_at >= ?) AS future_appointments,
 (SELECT count(*) FROM appointments WHERE instance_id = ?) AS appointments`, instanceID, instanceID, now.UnixMilli(), instanceID).
		Scan(ctx, &out)
	return out, err
}

// DeletionPreview returns what deleting the instance would remove.
func (s *Service) DeletionPreview(ctx context.Context, actor *auth.Account, instanceID string) (DeletionImpact, error) {
	if !actor.SuperAdmin {
		return DeletionImpact{}, apperr.ErrForbidden
	}
	var out DeletionImpact
	err := s.DB.ReadTx(ctx, func(tx bun.Tx) error {
		if _, err := getInstance(ctx, tx, instanceID); err != nil {
			return err
		}
		var err error
		out, err = impact(ctx, tx, instanceID, s.Now())
		return err
	})
	return out, err
}

// Delete removes the instance and everything that belongs to it in one
// transaction: appointments, blocks, special dates, schedule, rules,
// customers, operator bindings and its audit history, including platform
// entries about it. One platform audit entry records the deletion, with
// counts but no customer data. The super admin confirms by typing the
// instance's name. Deletion is allowed at any time, also with future
// appointments.
//
// secure_delete zeroes the deleted rows in the database pages; the WAL is
// then checkpointed so the old pages leave the WAL as well. A checkpoint
// that cannot complete because a reader is active is finished by a later
// one and does not affect the result.
func (s *Service) Delete(ctx context.Context, actor *auth.Account, instanceID, confirmName string) (DeletionImpact, error) {
	var removed DeletionImpact
	err := s.DB.WriteTx(ctx, func(tx bun.Tx) error {
		if err := actor.RecheckSuperAdmin(ctx, tx); err != nil {
			return err
		}
		inst, err := getInstance(ctx, tx, instanceID)
		if err != nil {
			return err
		}
		if strings.TrimSpace(confirmName) != inst.Name {
			return apperr.Invalid("confirm_name_mismatch")
		}
		now := s.Now()
		if removed, err = impact(ctx, tx, instanceID, now); err != nil {
			return err
		}
		for _, table := range []string{"appointments", "blocked_periods", "schedule_overrides", "schedules", "booking_policies", "audit_log", "customers", "instance_operators", "instances"} {
			query := tx.NewDelete().Table(table)
			switch table {
			case "instances":
				query.Where("id = ?", instanceID)
			case "audit_log":
				query.Where("instance_id = ? OR (scope = 'platform' AND target_type = 'instance' AND target_id = ?)", instanceID, instanceID)
			default:
				query.Where("instance_id = ?", instanceID)
			}
			if _, err := query.Exec(ctx); err != nil {
				return err
			}
		}
		return audit.Write(ctx, tx, audit.Entry{
			At: now, Actor: audit.AccountActor(actor.ID), Action: "instance.deleted",
			Target: audit.Record("instance", instanceID),
			Before: map[string]any{"name": inst.Name, "slug": inst.Slug},
			After:  removed,
		})
	})
	if err != nil {
		return DeletionImpact{}, err
	}
	if done, err := s.DB.Checkpoint(ctx); err != nil || !done {
		slog.Warn("checkpoint after instance deletion did not complete; a later checkpoint finishes it", "err", err)
	}
	return removed, nil
}

// SetStatus activates or pauses an instance. Any manager of the instance may
// do so. Existing appointments and links are unaffected.
func (s *Service) SetStatus(ctx context.Context, actor *auth.Account, instanceID, status string) (Instance, error) {
	if status != "active" && status != "paused" {
		return Instance{}, apperr.Invalid("invalid_status")
	}
	var v Instance
	err := auth.InstanceWrite(ctx, s.DB, actor, instanceID, func(tx bun.Tx, acc *auth.Access) error {
		v = ToView(acc.Instance)
		if acc.Instance.Status == status {
			return nil
		}
		now := s.Now()
		if _, err := tx.NewUpdate().
			Model((*storage.Instance)(nil)).
			Set("status = ?", status).
			Set("updated_at = ?", now.UnixMilli()).
			Where("id = ?", instanceID).
			Exec(ctx); err != nil {
			return err
		}
		v.Status = status
		return audit.Write(ctx, tx, audit.Entry{
			At: now, InstanceID: &instanceID, Actor: audit.AccountActor(actor.ID), Action: "instance.status",
			Target: audit.Record("instance", instanceID),
			Before: map[string]any{"status": acc.Instance.Status}, After: map[string]any{"status": status},
		})
	})
	return v, err
}

// SettingsInput is the editable instance settings.
type SettingsInput struct {
	Name                  string
	Timezone              string
	DefaultLocale         string
	ContactKind           string
	ContactRequired       bool
	BrandColor            *string // nil clears it
	ConfirmTimezoneChange bool
}

// UpdateSettings changes instance settings. Changing the time zone while
// future appointments exist requires explicit confirmation.
func (s *Service) UpdateSettings(ctx context.Context, actor *auth.Account, instanceID string, in SettingsInput) (Instance, error) {
	var err error
	if in.Name, err = validate.Name(in.Name, 100); err != nil {
		return Instance{}, err
	}
	location, err := validate.Timezone(in.Timezone)
	if err != nil {
		return Instance{}, err
	}
	in.Timezone = location.String()
	if in.DefaultLocale, err = validate.Locale(in.DefaultLocale); err != nil {
		return Instance{}, err
	}
	if err = validContactKind(in.ContactKind); err != nil {
		return Instance{}, err
	}
	if in.BrandColor, err = validate.OptionalHexColor(in.BrandColor); err != nil {
		return Instance{}, err
	}
	var v Instance
	err = auth.InstanceWrite(ctx, s.DB, actor, instanceID, func(tx bun.Tx, acc *auth.Access) error {
		now := s.Now()
		if in.Timezone != acc.Instance.Timezone && !in.ConfirmTimezoneChange {
			n, err := tx.NewSelect().
				Model((*storage.Appointment)(nil)).
				Where("instance_id = ? AND status <> ? AND ends_at > ?", instanceID, "cancelled", now.UnixMilli()).Count(ctx)
			if err != nil {
				return err
			}
			if n > 0 {
				return apperr.Conflict("timezone_change_needs_confirmation").With("count", n)
			}
		}
		if _, err := tx.NewUpdate().
			Model((*storage.Instance)(nil)).
			Set("name = ?", in.Name).
			Set("timezone = ?", in.Timezone).
			Set("default_locale = ?", in.DefaultLocale).
			Set("contact_kind = ?", in.ContactKind).
			Set("contact_required = ?", in.ContactRequired).
			Set("brand_color = ?", in.BrandColor).
			Set("updated_at = ?", now.UnixMilli()).
			Where("id = ?", instanceID).
			Exec(ctx); err != nil {
			return err
		}
		inst, err := storage.GetInstance(ctx, tx, instanceID)
		if err != nil {
			return err
		}
		v = ToView(inst)
		return audit.Write(ctx, tx, audit.Entry{
			At: now, InstanceID: &instanceID, Actor: audit.AccountActor(actor.ID), Action: "instance.settings",
			Target: audit.Record("instance", instanceID), Before: ToView(acc.Instance), After: v,
		})
	})
	return v, err
}

// Invite returns the current registration invite code.
func (s *Service) Invite(ctx context.Context, actor *auth.Account, instanceID string) (Invite, error) {
	var out Invite
	err := auth.InstanceRead(ctx, s.DB, actor, instanceID, func(_ bun.Tx, acc *auth.Access) error {
		out = inviteView(acc.Instance.Slug, acc.Instance.RegistrationInviteCode)
		return nil
	})
	return out, err
}

// RotateInvite replaces the invite code; old QR codes stop working.
// Approved customers' links are unaffected.
func (s *Service) RotateInvite(ctx context.Context, actor *auth.Account, instanceID string) (Invite, error) {
	code := auth.NewInviteCode()
	var out Invite
	err := auth.InstanceWrite(ctx, s.DB, actor, instanceID, func(tx bun.Tx, acc *auth.Access) error {
		now := s.Now()
		out = inviteView(acc.Instance.Slug, code)
		if _, err := tx.NewUpdate().
			Model((*storage.Instance)(nil)).
			Set("registration_invite_code = ?", code).
			Set("updated_at = ?", now.UnixMilli()).
			Where("id = ?", instanceID).
			Exec(ctx); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			At: now, InstanceID: &instanceID, Actor: audit.AccountActor(actor.ID), Action: "instance.rotate_invite",
			Target: audit.Record("instance", instanceID),
		})
	})
	return out, err
}

// RecordSuperAdminEntry audits a super admin entering an instance they do
// not otherwise manage.
func (s *Service) RecordSuperAdminEntry(ctx context.Context, actor *auth.Account, instanceID string) error {
	return auth.InstanceWrite(ctx, s.DB, actor, instanceID, func(tx bun.Tx, _ *auth.Access) error {
		if !actor.SuperAdmin {
			return apperr.ErrForbidden
		}
		return audit.Write(ctx, tx, audit.Entry{
			At: s.Now(), InstanceID: &instanceID, Actor: audit.AccountActor(actor.ID), Action: "instance.super_admin_enter",
			Target: audit.Record("instance", instanceID),
		})
	})
}
