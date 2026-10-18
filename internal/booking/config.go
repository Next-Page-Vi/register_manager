package booking

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"register_manager/internal/apperr"
	"register_manager/internal/audit"
	"register_manager/internal/auth"
	"register_manager/internal/schedule"
	"register_manager/internal/storage"
	"register_manager/internal/validate"
)

// CycleView is the repeating schedule as exchanged with the API. Each entry
// of Days is one cycle day; an empty list is a day off.
type CycleView struct {
	CycleLengthDays int                 `json:"cycle_length_days" minimum:"1" maximum:"366"`
	AnchorDate      string              `json:"anchor_date" format:"date"`
	Days            [][]schedule.Period `json:"days"`
}

func cycleView(c schedule.Cycle) CycleView {
	v := CycleView{CycleLengthDays: c.Length(), AnchorDate: c.Anchor().String()}
	for _, d := range c.Days() {
		v.Days = append(v.Days, d.Periods())
	}
	return v
}

// ParseCycle validates a cycle from the API.
func ParseCycle(v CycleView) (schedule.Cycle, error) {
	anchor, err := schedule.ParseDate(v.AnchorDate)
	if err != nil {
		return schedule.Cycle{}, apperr.Invalid("invalid_date")
	}
	if len(v.Days) != v.CycleLengthDays {
		return schedule.Cycle{}, apperr.Invalid("cycle_length_mismatch")
	}
	days := make([]schedule.DayPlan, 0, len(v.Days))
	for i, ps := range v.Days {
		p, err := schedule.NewDayPlan(ps)
		if err != nil {
			var ae *apperr.Error
			if errors.As(err, &ae) {
				return schedule.Cycle{}, ae.With("day", i)
			}
			return schedule.Cycle{}, err
		}
		days = append(days, p)
	}
	return schedule.NewCycle(anchor, days)
}

// DayPreview lists the open slots of one upcoming local date.
type DayPreview struct {
	Date      string      `json:"date" format:"date"`
	Starts    []time.Time `json:"starts"`
	Collapsed bool        `json:"collapsed"`
}

// Preview shows the effect of a proposed configuration.
type Preview struct {
	Timezone string       `json:"timezone"`
	Days     []DayPreview `json:"days"`
}

const previewDays = 14

func preview(f *Facts) []DayPreview {
	today := f.Today()
	out := make([]DayPreview, 0, previewDays)
	collapsed := map[schedule.Date]bool{}
	for _, d := range f.CollapsedPeriods(today, today.AddDays(previewDays-1)) {
		collapsed[d] = true
	}
	for d := today; d < today.AddDays(previewDays); d++ {
		dp := DayPreview{Date: d.String(), Starts: []time.Time{}, Collapsed: collapsed[d]}
		if d <= f.LastDate() {
			for _, c := range f.Candidates(d) {
				if f.bookable(c, d) {
					dp.Starts = append(dp.Starts, c.StartsAt)
				}
			}
		}
		out = append(out, dp)
	}
	return out
}

// CycleState is the repeating schedule together with the longest cycle the
// super admin currently allows. A schedule saved before the limit was
// lowered can be longer; it keeps working until it is next saved.
type CycleState struct {
	CycleView
	MaxCycleDays int `json:"max_cycle_days" doc:"Longest cycle that can be saved (system setting)"`
}

// maxCycleDays reads the system-wide limit on the cycle length.
func maxCycleDays(ctx context.Context, q bun.IDB) (int, error) {
	st, err := storage.GetPlatformSettings(ctx, q)
	return int(st.MaxCycleDays), err
}

// checkCycleLimit refuses a cycle longer than the system-wide limit.
func checkCycleLimit(ctx context.Context, q bun.IDB, c schedule.Cycle) (int, error) {
	limit, err := maxCycleDays(ctx, q)
	if err != nil {
		return 0, err
	}
	if c.Length() > limit {
		return limit, apperr.Invalid("cycle_length_over_limit").With("max", limit)
	}
	return limit, nil
}

// GetCycle returns the current repeating schedule and the cycle limit.
func (s *Service) GetCycle(ctx context.Context, actor *auth.Account, instanceID string) (CycleState, error) {
	var out CycleState
	err := auth.InstanceRead(ctx, s.DB, actor, instanceID, func(tx bun.Tx, _ *auth.Access) error {
		c, err := loadCycle(ctx, tx, instanceID)
		if err != nil {
			return err
		}
		limit, err := maxCycleDays(ctx, tx)
		out = CycleState{CycleView: cycleView(c), MaxCycleDays: limit}
		return err
	})
	return out, err
}

// PreviewCycle shows customer slots for a proposed cycle without
// saving it.
func (s *Service) PreviewCycle(ctx context.Context, actor *auth.Account, instanceID string, v CycleView) (Preview, error) {
	c, err := ParseCycle(v)
	if err != nil {
		return Preview{}, err
	}
	var p Preview
	err = auth.InstanceRead(ctx, s.DB, actor, instanceID, func(tx bun.Tx, acc *auth.Access) error {
		if _, err := checkCycleLimit(ctx, tx, c); err != nil {
			return err
		}
		now := s.Now()
		f, err := loadFacts(ctx, tx, acc.Instance, now, factsOptions{cycle: &c})
		if err != nil {
			return err
		}
		p = Preview{Timezone: acc.Instance.Timezone, Days: preview(f)}
		return nil
	})
	return p, err
}

// SaveCycle replaces the repeating schedule without changing appointments.
func (s *Service) SaveCycle(ctx context.Context, actor *auth.Account, instanceID string, v CycleView) (CycleState, error) {
	c, err := ParseCycle(v)
	if err != nil {
		return CycleState{}, err
	}
	limit := 0
	err = auth.InstanceWrite(ctx, s.DB, actor, instanceID, func(tx bun.Tx, _ *auth.Access) error {
		var err error
		if limit, err = checkCycleLimit(ctx, tx, c); err != nil {
			return err
		}
		now := s.Now()
		old, err := loadCycle(ctx, tx, instanceID)
		if err != nil {
			return err
		}
		if err := storage.StoreSchedule(ctx, tx, &storage.Schedule{
			InstanceID:      instanceID,
			CycleLengthDays: int64(c.Length()),
			AnchorDate:      c.Anchor().String(),
			CycleDays:       string(c.DaysJSON()),
			UpdatedAt:       now.UnixMilli(),
		}); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			At: now, InstanceID: &instanceID, Actor: audit.AccountActor(actor.ID), Action: "schedule.update",
			Target: audit.Record("schedule", instanceID), Before: cycleView(old), After: cycleView(c),
		})
	})
	return CycleState{CycleView: cycleView(c), MaxCycleDays: limit}, err
}

// OverrideView is a date-specific plan. Periods empty means a day off.
type OverrideView struct {
	Date    string            `json:"date" format:"date"`
	Periods []schedule.Period `json:"periods"`
}

// ListOverrides returns overrides between two local dates (inclusive).
func (s *Service) ListOverrides(ctx context.Context, actor *auth.Account, instanceID, from, to string) ([]OverrideView, error) {
	q := s.DB.Read()
	acc, err := auth.AuthorizeInstance(ctx, q, actor, instanceID)
	if err != nil {
		return nil, err
	}
	loc, err := loadLocation(acc.Instance)
	if err != nil {
		return nil, err
	}
	today := schedule.DateIn(s.Now(), loc)
	fd, td := today.AddDays(-30), today.AddDays(400)
	if from != "" {
		if fd, err = schedule.ParseDate(from); err != nil {
			return nil, apperr.Invalid("invalid_date")
		}
	}
	if to != "" {
		if td, err = schedule.ParseDate(to); err != nil {
			return nil, apperr.Invalid("invalid_date")
		}
	}
	m, err := loadOverrides(ctx, q, instanceID, fd, td)
	if err != nil {
		return nil, err
	}
	out := make([]OverrideView, 0, len(m))
	for d := fd; d <= td && len(out) < len(m); d++ {
		if p, ok := m[d]; ok {
			out = append(out, OverrideView{Date: d.String(), Periods: p.Periods()})
		}
	}
	return out, nil
}

// PutOverride creates or replaces a date override (empty periods = day off).
func (s *Service) PutOverride(ctx context.Context, actor *auth.Account, instanceID, date string, periods []schedule.Period) (OverrideView, error) {
	d, err := schedule.ParseDate(date)
	if err != nil {
		return OverrideView{}, apperr.Invalid("invalid_date")
	}
	plan, err := schedule.NewDayPlan(periods)
	if err != nil {
		return OverrideView{}, err
	}
	err = auth.InstanceWrite(ctx, s.DB, actor, instanceID, func(tx bun.Tx, _ *auth.Access) error {
		now := s.Now()
		b, _ := plan.MarshalJSON()
		if _, err := tx.NewInsert().
			Model(&storage.ScheduleOverride{InstanceID: instanceID, Date: d.String(), Periods: string(b), UpdatedAt: now.UnixMilli()}).
			On("CONFLICT (instance_id, date) DO UPDATE").
			Set("periods = EXCLUDED.periods").
			Set("updated_at = EXCLUDED.updated_at").
			Exec(ctx); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			At: now, InstanceID: &instanceID, Actor: audit.AccountActor(actor.ID), Action: "schedule.override_put",
			Target: audit.Record("override", d.String()), After: plan,
		})
	})
	return OverrideView{Date: d.String(), Periods: plan.Periods()}, err
}

// DeleteOverride removes an override so the date follows the cycle again.
func (s *Service) DeleteOverride(ctx context.Context, actor *auth.Account, instanceID, date string) error {
	d, err := schedule.ParseDate(date)
	if err != nil {
		return apperr.Invalid("invalid_date")
	}
	return auth.InstanceWrite(ctx, s.DB, actor, instanceID, func(tx bun.Tx, _ *auth.Access) error {
		if _, err := loadOverride(ctx, tx, instanceID, d.String()); errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		now := s.Now()
		if _, err := tx.NewDelete().
			Model((*storage.ScheduleOverride)(nil)).
			Where("instance_id = ? AND date = ?", instanceID, d.String()).
			Exec(ctx); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			At: now, InstanceID: &instanceID, Actor: audit.AccountActor(actor.ID), Action: "schedule.override_delete",
			Target: audit.Record("override", d.String()),
		})
	})
}

// BlockView is a temporary block.
type BlockView struct {
	ID       string    `json:"id"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Reason   *string   `json:"reason"`
}

// ListBlocks returns blocks that end after 30 days ago.
func (s *Service) ListBlocks(ctx context.Context, actor *auth.Account, instanceID string) ([]BlockView, error) {
	q := s.DB.Read()
	if _, err := auth.AuthorizeInstance(ctx, q, actor, instanceID); err != nil {
		return nil, err
	}
	rows, err := listBlocks(ctx, q, instanceID, s.Now().Add(-30*24*time.Hour).UnixMilli(), 1<<62)
	if err != nil {
		return nil, err
	}
	out := make([]BlockView, 0, len(rows))
	for _, b := range rows {
		out = append(out, BlockView{ID: b.ID, StartsAt: storage.FromMillis(b.StartsAt), EndsAt: storage.FromMillis(b.EndsAt), Reason: b.Reason})
	}
	return out, nil
}

// CreateBlock closes a period to customers without changing appointments.
func (s *Service) CreateBlock(ctx context.Context, actor *auth.Account, instanceID string, start, end time.Time, reason *string) (BlockView, error) {
	if !start.Before(end) {
		return BlockView{}, apperr.Invalid("invalid_interval")
	}
	reason, err := validate.OptionalText(reason, 200)
	if err != nil {
		return BlockView{}, err
	}
	var v BlockView
	err = auth.InstanceWrite(ctx, s.DB, actor, instanceID, func(tx bun.Tx, _ *auth.Access) error {
		now := s.Now()
		id := validate.NewID()
		if _, err := tx.NewInsert().
			Model(&storage.BlockedPeriod{
				ID:         id,
				InstanceID: instanceID,
				StartsAt:   start.UnixMilli(),
				EndsAt:     end.UnixMilli(),
				Reason:     reason,
				CreatedAt:  now.UnixMilli(),
			}).
			Exec(ctx); err != nil {
			return err
		}
		v = BlockView{ID: id, StartsAt: start.UTC(), EndsAt: end.UTC(), Reason: reason}
		return audit.Write(ctx, tx, audit.Entry{
			At: now, InstanceID: &instanceID, Actor: audit.AccountActor(actor.ID), Action: "block.create",
			Target: audit.Record("block", id), After: v,
		})
	})
	return v, err
}

// DeleteBlock removes a block.
func (s *Service) DeleteBlock(ctx context.Context, actor *auth.Account, instanceID, id string) error {
	return auth.InstanceWrite(ctx, s.DB, actor, instanceID, func(tx bun.Tx, _ *auth.Access) error {
		b, err := loadBlock(ctx, tx, instanceID, id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := tx.NewDelete().
			Model((*storage.BlockedPeriod)(nil)).
			Where("instance_id = ? AND id = ?", instanceID, id).
			Exec(ctx); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			At: s.Now(), InstanceID: &instanceID, Actor: audit.AccountActor(actor.ID), Action: "block.delete",
			Target: audit.Record("block", id),
			Before: BlockView{ID: b.ID, StartsAt: storage.FromMillis(b.StartsAt), EndsAt: storage.FromMillis(b.EndsAt), Reason: b.Reason},
		})
	})
}

// GetPolicy returns the instance's booking rules.
func (s *Service) GetPolicy(ctx context.Context, actor *auth.Account, instanceID string) (Policy, error) {
	q := s.DB.Read()
	if _, err := auth.AuthorizeInstance(ctx, q, actor, instanceID); err != nil {
		return Policy{}, err
	}
	row, err := storage.GetPolicy(ctx, q, instanceID)
	if err != nil {
		return Policy{}, err
	}
	return PolicyFromRow(row), nil
}

// PreviewPolicy shows customer slots for a proposed policy.
func (s *Service) PreviewPolicy(ctx context.Context, actor *auth.Account, instanceID string, p Policy) (Preview, error) {
	if err := ValidatePolicy(p); err != nil {
		return Preview{}, err
	}
	var pv Preview
	err := auth.InstanceRead(ctx, s.DB, actor, instanceID, func(tx bun.Tx, acc *auth.Access) error {
		f, err := loadFacts(ctx, tx, acc.Instance, s.Now(), factsOptions{policy: &p})
		if err != nil {
			return err
		}
		pv = Preview{Timezone: acc.Instance.Timezone, Days: preview(f)}
		return nil
	})
	return pv, err
}

// PolicySaved contains the saved policy.
type PolicySaved struct {
	Policy Policy `json:"policy"`
}

// SavePolicy stores new customer rules without checking existing appointments.
func (s *Service) SavePolicy(ctx context.Context, actor *auth.Account, instanceID string, p Policy) (PolicySaved, error) {
	if err := ValidatePolicy(p); err != nil {
		return PolicySaved{}, err
	}
	var out PolicySaved
	err := auth.InstanceWrite(ctx, s.DB, actor, instanceID, func(tx bun.Tx, _ *auth.Access) error {
		old, err := storage.GetPolicy(ctx, tx, instanceID)
		if err != nil {
			return err
		}
		now := s.Now()
		if err := StorePolicy(ctx, tx, instanceID, p, now.UnixMilli()); err != nil {
			return err
		}
		out = PolicySaved{Policy: p}
		return audit.Write(ctx, tx, audit.Entry{
			At: now, InstanceID: &instanceID, Actor: audit.AccountActor(actor.ID), Action: "policy.update",
			Target: audit.Record("policy", instanceID), Before: PolicyFromRow(old), After: p,
		})
	})
	return out, err
}
