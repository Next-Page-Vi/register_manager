package schedule

import (
	"encoding/json"
	"slices"

	"register_manager/internal/apperr"
)

// MinutesPerDay is the exclusive upper bound of a start minute and the
// inclusive upper bound of an end minute (the next midnight).
const MinutesPerDay = 1440

// MaxCycleLength bounds the cycle length.
const MaxCycleLength = 366

// Period is a working period within one local day, in minutes from local
// midnight: Start in [0, 1439], End in [1, 1440], Start < End.
type Period struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// DayPlan is the working periods of one day. An empty plan is a day off.
// Valid plans are sorted, non-overlapping and have touching periods merged;
// construct them with NewDayPlan.
type DayPlan struct {
	periods []Period
}

// Off is a day without working periods.
var Off = DayPlan{}

// NewDayPlan validates and normalizes periods. Touching periods (one ends
// where the next starts) are merged; overlapping periods are rejected.
func NewDayPlan(periods []Period) (DayPlan, error) {
	ps := slices.Clone(periods)
	for _, p := range ps {
		if p.Start < 0 || p.Start >= MinutesPerDay || p.End <= 0 || p.End > MinutesPerDay {
			return DayPlan{}, apperr.Invalid("period_out_of_range")
		}
		if p.Start >= p.End {
			return DayPlan{}, apperr.Invalid("period_start_not_before_end")
		}
	}
	slices.SortFunc(ps, func(a, b Period) int { return a.Start - b.Start })
	out := make([]Period, 0, len(ps))
	for _, p := range ps {
		if n := len(out); n > 0 {
			last := &out[n-1]
			if p.Start < last.End {
				return DayPlan{}, apperr.Invalid("periods_overlap")
			}
			if p.Start == last.End {
				last.End = p.End
				continue
			}
		}
		out = append(out, p)
	}
	return DayPlan{periods: out}, nil
}

// Periods returns a copy of the periods; a day off gives an empty, non-nil
// slice so it encodes as [].
func (p DayPlan) Periods() []Period { return append([]Period{}, p.periods...) }

// IsOff reports whether the plan has no working periods.
func (p DayPlan) IsOff() bool { return len(p.periods) == 0 }

func (p DayPlan) MarshalJSON() ([]byte, error) {
	if p.periods == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(p.periods)
}

func (p *DayPlan) UnmarshalJSON(b []byte) error {
	var ps []Period
	if err := json.Unmarshal(b, &ps); err != nil {
		return err
	}
	v, err := NewDayPlan(ps)
	if err != nil {
		return err
	}
	*p = v
	return nil
}

// Cycle is a repeating N-day working pattern anchored at a local date.
type Cycle struct {
	anchor Date
	days   []DayPlan
}

// NewCycle validates a cycle: 1..MaxCycleLength days, each with a valid plan.
func NewCycle(anchor Date, days []DayPlan) (Cycle, error) {
	if len(days) < 1 || len(days) > MaxCycleLength {
		return Cycle{}, apperr.Invalid("cycle_length_invalid").With("max", MaxCycleLength)
	}
	return Cycle{anchor: anchor, days: slices.Clone(days)}, nil
}

// NewCycleFromJSON builds a cycle from the stored JSON array of day plans.
func NewCycleFromJSON(anchor Date, length int, daysJSON []byte) (Cycle, error) {
	var days []DayPlan
	if err := json.Unmarshal(daysJSON, &days); err != nil {
		return Cycle{}, err
	}
	if len(days) != length {
		return Cycle{}, apperr.Invalid("cycle_length_mismatch")
	}
	return NewCycle(anchor, days)
}

func (c Cycle) Anchor() Date     { return c.anchor }
func (c Cycle) Length() int      { return len(c.days) }
func (c Cycle) Days() []DayPlan  { return slices.Clone(c.days) }
func (c Cycle) DaysJSON() []byte { b, _ := json.Marshal(c.days); return b }
func (c Cycle) PlanOn(d Date) DayPlan {
	return c.days[CycleIndex(d, c.anchor, len(c.days))]
}

// Calendar combines a cycle with date overrides. A date with an override uses
// the override plan (possibly Off); other dates follow the cycle.
type Calendar struct {
	Cycle     Cycle
	Overrides map[Date]DayPlan
}

// PlanOn returns the effective plan for d.
func (c Calendar) PlanOn(d Date) DayPlan {
	if p, ok := c.Overrides[d]; ok {
		return p
	}
	return c.Cycle.PlanOn(d)
}
