package instances

import (
	"context"
	"database/sql"
	"errors"
	"net/url"

	"github.com/uptrace/bun"

	"register_manager/internal/apperr"
	"register_manager/internal/auth"
	"register_manager/internal/storage"
)

type InstanceAccess struct {
	Instance Instance  `json:"instance"`
	Role     auth.Role `json:"role" enum:"operator,super"`
}

func (s *Service) Access(ctx context.Context, actor *auth.Account, id string) (InstanceAccess, error) {
	var out InstanceAccess
	err := auth.InstanceRead(ctx, s.DB, actor, id, func(_ bun.Tx, acc *auth.Access) error {
		out = InstanceAccess{Instance: ToView(acc.Instance), Role: acc.Role}
		return nil
	})
	return out, err
}

func (s *Service) AccessBySlug(ctx context.Context, actor *auth.Account, slug string) (InstanceAccess, error) {
	var out InstanceAccess
	err := s.DB.ReadTx(ctx, func(tx bun.Tx) error {
		inst, err := bySlug(ctx, tx, slug)
		if err != nil {
			return err
		}
		role, err := auth.InstanceRole(ctx, tx, actor, inst)
		if err != nil {
			return err
		}
		out = InstanceAccess{Instance: ToView(inst), Role: role}
		return nil
	})
	return out, err
}

func bySlug(ctx context.Context, q bun.IDB, slug string) (storage.Instance, error) {
	inst, err := storage.GetInstanceBySlug(ctx, q, slug)
	if errors.Is(err, sql.ErrNoRows) {
		return inst, apperr.NotFound("instance_not_found")
	}
	return inst, err
}

// BySlug supplies the instance used to resolve a customer's session.
func (s *Service) BySlug(ctx context.Context, slug string) (storage.Instance, error) {
	return bySlug(ctx, s.DB.Read(), slug)
}

type PublicInstance struct {
	Slug            string  `json:"slug"`
	Name            string  `json:"name"`
	Status          string  `json:"status" enum:"active,paused"`
	Timezone        string  `json:"timezone"`
	DefaultLocale   string  `json:"default_locale"`
	ContactKind     string  `json:"contact_kind" enum:"phone,email,text"`
	ContactRequired bool    `json:"contact_required"`
	BrandColor      *string `json:"brand_color"`
}

func PublicView(i storage.Instance) PublicInstance {
	return PublicInstance{Slug: i.Slug, Name: i.Name, Status: i.Status, Timezone: i.Timezone,
		DefaultLocale: i.DefaultLocale, ContactKind: i.ContactKind, ContactRequired: i.ContactRequired,
		BrandColor: i.BrandColor}
}

func (s *Service) Public(ctx context.Context, slug string) (PublicInstance, error) {
	i, err := s.BySlug(ctx, slug)
	if err != nil {
		return PublicInstance{}, err
	}
	return PublicView(i), nil
}

type Invite struct {
	Code         string `json:"code"`
	RegisterPath string `json:"register_path"`
}

func inviteView(slug, code string) Invite {
	return Invite{Code: code, RegisterPath: "/i/" + url.PathEscape(slug) + "/register?invite=" + url.QueryEscape(code)}
}

func listInstances(ctx context.Context, db bun.IDB, accountID *string) ([]storage.Instance, error) {
	rows := []storage.Instance{}
	q := db.NewSelect().
		Model(&rows).
		Column("instances.*").
		Order("instances.name", "instances.slug")
	if accountID != nil {
		q.Join("JOIN instance_operators AS o ON o.instance_id = instances.id").
			Where("o.account_id = ?", *accountID)
	}
	err := q.Scan(ctx)
	return rows, err
}

type operatorRow struct {
	InstanceID string
	ID         string
	Login      string
	Name       string
	DisabledAt *int64
}

func listOperators(ctx context.Context, db bun.IDB, instanceID *string) ([]operatorRow, error) {
	rows := []operatorRow{}
	q := db.NewSelect().TableExpr("instance_operators AS o").
		Column("o.instance_id", "a.id", "a.login", "a.name", "a.disabled_at").
		Join("JOIN accounts AS a ON a.id = o.account_id").
		Order("a.login")
	if instanceID != nil {
		q.Where("o.instance_id = ?", *instanceID)
	}
	err := q.Scan(ctx, &rows)
	return rows, err
}

func copySettings(ctx context.Context, db bun.IDB, id, source string, at int64) error {
	_, err := db.NewRaw(`INSERT INTO booking_policies
 (instance_id, slot_step_minutes, service_duration_minutes, buffer_minutes, daily_customer_limit, per_customer_daily_limit, per_customer_weekly_limit, max_future_bookings, booking_horizon_days, min_booking_notice_minutes, min_cancellation_notice_minutes, updated_at)
 SELECT ?, slot_step_minutes, service_duration_minutes, buffer_minutes, daily_customer_limit, per_customer_daily_limit, per_customer_weekly_limit, max_future_bookings, booking_horizon_days, min_booking_notice_minutes, min_cancellation_notice_minutes, ?
 FROM booking_policies WHERE instance_id = ?`, id, at, source).
		Exec(ctx)
	if err != nil {
		return err
	}
	_, err = db.NewRaw(`INSERT INTO schedules (instance_id, cycle_length_days, anchor_date, cycle_days, updated_at)
 SELECT ?, cycle_length_days, anchor_date, cycle_days, ? FROM schedules WHERE instance_id = ?`, id, at, source).
		Exec(ctx)
	return err
}
