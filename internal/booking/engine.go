// Package booking computes availability and performs booking transactions.
// The rule engine in this file is pure: it works on a snapshot of facts and
// is shared by availability queries and by the write path, which re-checks
// the same rules inside the serialized write transaction.
package booking

import (
	"maps"
	"slices"
	"sort"
	"time"

	"register_manager/internal/apperr"
	"register_manager/internal/schedule"
)

// Policy is an instance's booking rules. Nil limits mean unlimited.
type Policy struct {
	SlotStepMinutes              int  `json:"slot_step_minutes"`
	ServiceDurationMinutes       int  `json:"service_duration_minutes"`
	BufferMinutes                int  `json:"buffer_minutes"`
	DailyCustomerLimit           *int `json:"daily_customer_limit" required:"true" nullable:"true"`
	PerCustomerDailyLimit        *int `json:"per_customer_daily_limit" required:"true" nullable:"true"`
	PerCustomerWeeklyLimit       *int `json:"per_customer_weekly_limit" required:"true" nullable:"true"`
	MaxFutureBookings            *int `json:"max_future_bookings" required:"true" nullable:"true"`
	BookingHorizonDays           int  `json:"booking_horizon_days"`
	MinBookingNoticeMinutes      int  `json:"min_booking_notice_minutes"`
	MinCancellationNoticeMinutes int  `json:"min_cancellation_notice_minutes"`
}

// DefaultPolicy is used for new instances.
func DefaultPolicy() Policy {
	return Policy{
		SlotStepMinutes:              30,
		ServiceDurationMinutes:       30,
		BufferMinutes:                0,
		BookingHorizonDays:           30,
		MinBookingNoticeMinutes:      60,
		MinCancellationNoticeMinutes: 120,
	}
}

func (p Policy) Duration() time.Duration {
	return time.Duration(p.ServiceDurationMinutes) * time.Minute
}
func (p Policy) Buffer() time.Duration { return time.Duration(p.BufferMinutes) * time.Minute }

// Appt is an existing appointment that occupies time (not cancelled).
type Appt struct {
	ID         string
	CustomerID string
	Start      time.Time
	End        time.Time
	Status     string
}

// Slot is a bookable service interval.
type Slot struct {
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}

// CustomerContext enables personal quotas for one customer.
type CustomerContext struct {
	ID           string
	FutureBooked int
}

// FactsInput is mutable input; NewFacts takes an independent snapshot of it.
type FactsInput struct {
	Loc       *time.Location
	Now       time.Time
	Policy    Policy
	Calendar  schedule.Calendar
	Blocks    []schedule.Interval
	Appts     []Appt
	Customer  *CustomerContext
	ExcludeID *string
}

// Facts is an immutable, indexed snapshot shared by reads and writes.
type Facts struct {
	input    FactsInput
	sorted   []Appt
	dayCust  map[schedule.Date]map[string]bool
	custDay  map[schedule.Date]int
	custWeek map[schedule.Date]int
}

func NewFacts(input FactsInput) (*Facts, error) {
	if input.Loc == nil || input.Now.IsZero() || input.Calendar.Cycle.Length() == 0 {
		return nil, apperr.Invalid("invalid_facts")
	}
	if err := ValidatePolicy(input.Policy); err != nil {
		return nil, err
	}
	if input.Customer != nil {
		if input.Customer.ID == "" || input.Customer.FutureBooked < 0 {
			return nil, apperr.Invalid("invalid_facts")
		}
		customer := *input.Customer
		input.Customer = &customer
	}
	if input.ExcludeID != nil {
		if *input.ExcludeID == "" {
			return nil, apperr.Invalid("invalid_facts")
		}
		id := *input.ExcludeID
		input.ExcludeID = &id
	}
	input.Blocks = slices.Clone(input.Blocks)
	input.Appts = slices.Clone(input.Appts)
	input.Calendar.Overrides = maps.Clone(input.Calendar.Overrides)
	// Optional quota pointers belong to the snapshot as well.
	for _, quota := range []**int{&input.Policy.DailyCustomerLimit, &input.Policy.PerCustomerDailyLimit, &input.Policy.PerCustomerWeeklyLimit, &input.Policy.MaxFutureBookings} {
		if *quota != nil {
			value := **quota
			*quota = &value
		}
	}
	f := &Facts{input: input, sorted: []Appt{}, dayCust: map[schedule.Date]map[string]bool{}, custDay: map[schedule.Date]int{}, custWeek: map[schedule.Date]int{}}
	for _, a := range input.Appts {
		if !a.Start.Before(a.End) {
			return nil, apperr.Invalid("invalid_facts")
		}
		if (input.ExcludeID != nil && a.ID == *input.ExcludeID) || a.Status == "cancelled" {
			continue
		}
		f.sorted = append(f.sorted, a)
		d := schedule.DateIn(a.Start, input.Loc)
		if f.dayCust[d] == nil {
			f.dayCust[d] = map[string]bool{}
		}
		f.dayCust[d][a.CustomerID] = true
		if input.Customer != nil && a.CustomerID == input.Customer.ID {
			f.custDay[d]++
			f.custWeek[d.WeekStart()]++
		}
	}
	slices.SortFunc(f.sorted, func(a, b Appt) int { return a.Start.Compare(b.Start) })
	return f, nil
}

// Today is the instance-local date of Now.
func (f *Facts) Today() schedule.Date { return schedule.DateIn(f.input.Now, f.input.Loc) }

// LastDate is the last bookable local date (today + horizon, inclusive).
func (f *Facts) LastDate() schedule.Date { return f.Today().AddDays(f.input.Policy.BookingHorizonDays) }

// Candidates returns the grid-aligned start times on date d whose full
// service fits inside one working period, ignoring all other rules.
func (f *Facts) Candidates(d schedule.Date) []Slot {
	step := f.input.Policy.SlotStepMinutes
	dur := f.input.Policy.Duration()
	plan := f.input.Calendar.PlanOn(d)
	if plan.IsOff() {
		return nil
	}
	day := schedule.NewDay(f.input.Loc, d)
	var out []Slot
	for _, rp := range day.Periods(plan) {
		if rp.Empty {
			continue
		}
		first := (rp.Period.Start + step - 1) / step * step
		for m := first; m < rp.Period.End; m += step {
			start, ok := day.Start(m)
			if !ok {
				continue
			}
			end := start.Add(dur)
			if start.Before(rp.Abs.Start) || end.After(rp.Abs.End) {
				continue
			}
			out = append(out, Slot{StartsAt: start, EndsAt: end})
		}
	}
	// Merged periods are disjoint, so the list is already ordered; sort
	// defensively for DST edge cases.
	slices.SortFunc(out, func(a, b Slot) int { return a.StartsAt.Compare(b.StartsAt) })
	return out
}

// Check validates a requested service interval against every rule.
func (f *Facts) Check(start, end time.Time) error {
	if end.Sub(start) != f.input.Policy.Duration() {
		return apperr.Conflict("duration_changed").With("service_duration_minutes", f.input.Policy.ServiceDurationMinutes)
	}
	d := schedule.DateIn(start, f.input.Loc)
	if err := f.checkWindow(start, d); err != nil {
		return err
	}
	found := false
	for _, c := range f.Candidates(d) {
		if c.StartsAt.Equal(start) {
			found = true
			break
		}
	}
	if !found {
		return apperr.Conflict("slot_unavailable")
	}
	return f.checkOccupancy(start, end, d)
}

func (f *Facts) checkWindow(start time.Time, d schedule.Date) error {
	notice := time.Duration(f.input.Policy.MinBookingNoticeMinutes) * time.Minute
	if start.Before(f.input.Now.Add(notice)) {
		return apperr.Conflict("too_soon").With("min_booking_notice_minutes", f.input.Policy.MinBookingNoticeMinutes)
	}
	if d > f.LastDate() {
		return apperr.Conflict("beyond_horizon").With("booking_horizon_days", f.input.Policy.BookingHorizonDays)
	}
	return nil
}

func (f *Facts) checkOccupancy(start, end time.Time, d schedule.Date) error {
	if err := f.checkShared(start, end, d); err != nil {
		return err
	}
	return f.checkPersonal(d)
}

// checkShared applies the rules that hold for every customer alike.
func (f *Facts) checkShared(start, end time.Time, d schedule.Date) error {
	svc := schedule.Interval{Start: start, End: end}
	for _, b := range f.input.Blocks {
		if b.Overlaps(svc) {
			return apperr.Conflict("slot_blocked")
		}
	}
	if f.conflicts(start, end) {
		return apperr.Conflict("slot_taken")
	}
	p := f.input.Policy
	if p.DailyCustomerLimit != nil {
		set := f.dayCust[d]
		if !(f.input.Customer != nil && set[f.input.Customer.ID]) && len(set) >= *p.DailyCustomerLimit {
			return apperr.Conflict("daily_capacity_full")
		}
	}
	return nil
}

// checkPersonal applies the customer's own quotas.
func (f *Facts) checkPersonal(d schedule.Date) error {
	p := f.input.Policy
	if f.input.Customer == nil {
		return nil
	}
	if p.PerCustomerDailyLimit != nil && f.custDay[d] >= *p.PerCustomerDailyLimit {
		return apperr.Conflict("customer_daily_limit").With("limit", *p.PerCustomerDailyLimit)
	}
	if p.PerCustomerWeeklyLimit != nil && f.custWeek[d.WeekStart()] >= *p.PerCustomerWeeklyLimit {
		return apperr.Conflict("customer_weekly_limit").With("limit", *p.PerCustomerWeeklyLimit)
	}
	if p.MaxFutureBookings != nil && f.input.Customer.FutureBooked >= *p.MaxFutureBookings {
		return apperr.Conflict("customer_future_limit").With("limit", *p.MaxFutureBookings)
	}
	return nil
}

// conflicts reports whether [start, end) violates the current buffer against
// any existing appointment. Appointments never overlap, so sorting by start
// also sorts by end.
func (f *Facts) conflicts(start, end time.Time) bool {
	buf := f.input.Policy.Buffer()
	i := sort.Search(len(f.sorted), func(i int) bool {
		return f.sorted[i].End.Add(buf).After(start)
	})
	// The first appointment ending (plus buffer) after start is the only one
	// that can conflict unless it also starts before end+buffer.
	return i < len(f.sorted) && f.sorted[i].Start.Before(end.Add(buf))
}

// eachCandidate calls fn with every candidate on the days up to the last
// bookable date that starts strictly after `after` (a nil `after` means
// from today), in time order, until fn returns false.
func (f *Facts) eachCandidate(date *schedule.Date, after *time.Time, fn func(c Slot, d schedule.Date) bool) {
	from, to := f.Today(), f.LastDate()
	if date != nil {
		from, to = *date, *date
	}
	if after != nil {
		if d := schedule.DateIn(*after, f.input.Loc); d > from {
			from = d
		}
	}
	for d := from; d <= to; d++ {
		for _, c := range f.Candidates(d) {
			if after != nil && !c.StartsAt.After(*after) {
				continue
			}
			if !fn(c, d) {
				return
			}
		}
	}
}

// bookable reports whether candidate c on date d passes every rule.
func (f *Facts) bookable(c Slot, d schedule.Date) bool {
	return f.checkWindow(c.StartsAt, d) == nil && f.checkOccupancy(c.StartsAt, c.EndsAt, d) == nil
}

// Slots returns up to limit bookable slots starting strictly after `after`,
// in time order. A nil `after` means from now.
func (f *Facts) Slots(after *time.Time, limit *int) []Slot {
	return f.slotsOn(nil, after, limit)
}

func (f *Facts) slotsOn(date *schedule.Date, after *time.Time, limit *int) []Slot {
	var out []Slot
	f.eachCandidate(date, after, func(c Slot, d schedule.Date) bool {
		if f.bookable(c, d) {
			out = append(out, c)
		}
		return limit == nil || len(out) < *limit
	})
	return out
}

// EmptyReason says why there is nothing to book.
type EmptyReason string

const (
	// EmptyPaused: the instance is not taking bookings.
	EmptyPaused EmptyReason = "paused"
	// EmptyMisconfigured: the schedule and rules leave nothing bookable at
	// all, e.g. no working periods in the horizon or a limit of zero.
	EmptyMisconfigured EmptyReason = "misconfigured"
	// EmptyCustomerLimit: the customer holds MaxFutureBookings.
	EmptyCustomerLimit EmptyReason = "customer_limit"
	// EmptyPeriodLimit: the customer's daily or weekly quota is used up on
	// every day that is otherwise free.
	EmptyPeriodLimit EmptyReason = "period_limit"
	// EmptyFull: times exist but are taken, blocked or have passed.
	EmptyFull   EmptyReason = "full"
	EmptyOnDate EmptyReason = "no_slots_on_date"
)

// EmptyReason explains why Slots(after, ...) returned nothing. When several
// causes apply, the first in the order misconfigured, customer_limit,
// period_limit, full wins.
func (f *Facts) EmptyReason(after *time.Time) EmptyReason {
	return f.emptyReasonOn(nil, after)
}

func (f *Facts) emptyReasonOn(date *schedule.Date, after *time.Time) EmptyReason {
	p := f.input.Policy
	isZero := func(l *int) bool { return l != nil && *l == 0 }
	if isZero(p.DailyCustomerLimit) || f.input.Customer != nil &&
		(isZero(p.PerCustomerDailyLimit) || isZero(p.PerCustomerWeeklyLimit) || isZero(p.MaxFutureBookings)) {
		return EmptyMisconfigured
	}
	// Even at local midnight the notice would reach past the horizon.
	if p.MinBookingNoticeMinutes >= (p.BookingHorizonDays+1)*24*60 {
		return EmptyMisconfigured
	}
	var inWindow, tooSoon, sharedFree bool
	f.eachCandidate(date, after, func(c Slot, d schedule.Date) bool {
		if f.checkWindow(c.StartsAt, d) != nil {
			tooSoon = true
			return true
		}
		inWindow = true
		if f.checkShared(c.StartsAt, c.EndsAt, d) == nil {
			sharedFree = true
		}
		// Once a candidate is in the window and free for everyone, the
		// rest cannot change the answer.
		return !sharedFree
	})
	if date != nil && (!inWindow || !sharedFree) {
		return EmptyOnDate
	}
	switch {
	case !inWindow:
		// With a same-day horizon, today's times having passed is normal;
		// otherwise the remaining days have no working time.
		if tooSoon && p.BookingHorizonDays == 0 {
			return EmptyFull
		}
		return EmptyMisconfigured
	case !sharedFree:
		return EmptyFull
	case f.input.Customer != nil && p.MaxFutureBookings != nil && f.input.Customer.FutureBooked >= *p.MaxFutureBookings:
		return EmptyCustomerLimit
	default:
		return EmptyPeriodLimit
	}
}

// CollapsedPeriods returns the dates in [from, to] on which DST collapses a
// working period so that it cannot be booked.
func (f *Facts) CollapsedPeriods(from, to schedule.Date) []schedule.Date {
	var out []schedule.Date
	for d := from; d <= to; d++ {
		for _, rp := range schedule.ResolvePeriods(f.input.Loc, d, f.input.Calendar.PlanOn(d)) {
			if rp.Empty {
				out = append(out, d)
				break
			}
		}
	}
	return out
}
