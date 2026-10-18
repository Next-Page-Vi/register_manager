// Package settings manages system-wide settings, which only the super admin
// can read and change. Today there is one: the longest repeating schedule
// an instance may save.
package settings

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"register_manager/internal/apperr"
	"register_manager/internal/audit"
	"register_manager/internal/auth"
	"register_manager/internal/schedule"
	"register_manager/internal/storage"
)

// Service implements the settings operations.
type Service struct {
	DB  *storage.DB
	Now func() time.Time
}

// OverLimit is an instance whose saved cycle is longer than the current
// limit. It keeps working; saving its schedule again needs a shorter cycle.
type OverLimit struct {
	InstanceID      string `json:"instance_id"`
	Name            string `json:"name"`
	Slug            string `json:"slug"`
	CycleLengthDays int    `json:"cycle_length_days"`
}

// Settings are the system-wide settings.
type Settings struct {
	MaxCycleDays int         `json:"max_cycle_days" doc:"Longest repeating schedule an instance may save (1-366)"`
	OverLimit    []OverLimit `json:"over_limit" doc:"Instances whose saved cycle is longer than max_cycle_days"`
}

func load(ctx context.Context, q bun.IDB) (Settings, error) {
	st, err := storage.GetPlatformSettings(ctx, q)
	if err != nil {
		return Settings{}, err
	}
	rows := []OverLimit{}
	err = q.NewSelect().TableExpr("schedules AS s").
		ColumnExpr("i.id AS instance_id").
		Column("i.name", "i.slug", "s.cycle_length_days").
		Join("JOIN instances AS i ON i.id = s.instance_id").
		Where("s.cycle_length_days > ?", st.MaxCycleDays).
		Order("i.name", "i.slug").
		Scan(ctx, &rows)
	if err != nil {
		return Settings{}, err
	}
	return Settings{MaxCycleDays: int(st.MaxCycleDays), OverLimit: rows}, nil
}

// Get returns the settings.
func (s *Service) Get(ctx context.Context, actor *auth.Account) (Settings, error) {
	if !actor.SuperAdmin {
		return Settings{}, apperr.ErrForbidden
	}
	var out Settings
	err := s.DB.ReadTx(ctx, func(tx bun.Tx) error {
		var err error
		out, err = load(ctx, tx)
		return err
	})
	return out, err
}

// Update changes the longest cycle instances may save. Lowering it does not
// change any saved schedule.
func (s *Service) Update(ctx context.Context, actor *auth.Account, maxCycleDays int) (Settings, error) {
	if maxCycleDays < 1 || maxCycleDays > schedule.MaxCycleLength {
		return Settings{}, apperr.Invalid("invalid_max_cycle_days").With("max", schedule.MaxCycleLength)
	}
	var out Settings
	err := s.DB.WriteTx(ctx, func(tx bun.Tx) error {
		if err := actor.RecheckSuperAdmin(ctx, tx); err != nil {
			return err
		}
		before, err := storage.GetPlatformSettings(ctx, tx)
		if err != nil {
			return err
		}
		now := s.Now()
		if _, err := tx.NewUpdate().
			Model((*storage.PlatformSetting)(nil)).
			Set("max_cycle_days = ?", int64(maxCycleDays)).
			Set("updated_at = ?", now.UnixMilli()).
			Where("id = ?", 1).
			Exec(ctx); err != nil {
			return err
		}
		if out, err = load(ctx, tx); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			At: now, Actor: audit.AccountActor(actor.ID), Action: "platform.settings",
			Target: audit.Resource("settings"),
			Before: map[string]any{"max_cycle_days": before.MaxCycleDays},
			After:  map[string]any{"max_cycle_days": maxCycleDays},
		})
	})
	return out, err
}
