package schedule

import (
	"slices"
	"sync"
	"time"

	_ "time/tzdata" // embed the IANA database so behaviour does not depend on the host
)

// locations caches loaded time zones. time.LoadLocation parses the
// embedded zone database on every call; only valid names are cached, so the
// cache is bounded by the number of IANA zones.
var locations sync.Map // string -> *time.Location

// LoadLocation is time.LoadLocation with a cache.
func LoadLocation(name string) (*time.Location, error) {
	if l, ok := locations.Load(name); ok {
		return l.(*time.Location), nil
	}
	l, err := time.LoadLocation(name)
	if err != nil {
		return nil, err
	}
	locations.Store(name, l)
	return l, nil
}

// Go's time.Date does not specify which offset it picks for a wall time that
// is skipped or repeated by a transition, so wall times are resolved here
// with explicit rules:
//
//   - Candidate start times (ResolveStart): a skipped wall time does not
//     exist; a repeated wall time resolves to the earlier instant.
//   - Boundaries (ResolveBoundary) of periods, days and weeks: a skipped wall
//     time moves to the first valid instant after the gap (the transition);
//     a repeated wall time resolves to the earlier instant.

// wallInstants returns every instant whose wall clock in loc is date+minute
// (zero, one or two instants, ascending). When there is none, gapEnd is the
// transition instant that ends the gap.
func wallInstants(loc *time.Location, d Date, minute int) (insts []time.Time, gapEnd time.Time) {
	y, m, day := d.YMD()
	wall := time.Date(y, m, day, 0, 0, 0, 0, time.UTC).Unix() + int64(minute)*60
	offAt := func(u int64) int {
		_, off := time.Unix(u, 0).In(loc).Zone()
		return off
	}
	offs := []int{offAt(wall - 86400), offAt(wall), offAt(wall + 86400)}
	slices.Sort(offs)
	offs = slices.Compact(offs)
	for _, off := range offs {
		u := wall - int64(off)
		if offAt(u) == off {
			insts = append(insts, time.Unix(u, 0).UTC())
		}
	}
	slices.SortFunc(insts, func(a, b time.Time) int { return a.Compare(b) })
	insts = slices.CompactFunc(insts, func(a, b time.Time) bool { return a.Equal(b) })
	if len(insts) == 0 {
		// In a forward gap the wall time minus the smallest offset lands
		// after the transition; the zone containing it starts at the gap end.
		u := wall - int64(offs[0])
		start, _ := time.Unix(u, 0).In(loc).ZoneBounds()
		gapEnd = start.UTC()
	}
	return insts, gapEnd
}

// ResolveStart resolves a candidate start time. ok is false when the wall
// time does not exist on that date.
func ResolveStart(loc *time.Location, d Date, minute int) (t time.Time, ok bool) {
	insts, _ := wallInstants(loc, d, minute)
	if len(insts) == 0 {
		return time.Time{}, false
	}
	return insts[0], true
}

// ResolveBoundary resolves a boundary wall time. Minute may be 1440, meaning
// midnight at the start of the next day.
func ResolveBoundary(loc *time.Location, d Date, minute int) time.Time {
	if minute >= MinutesPerDay {
		d = d.AddDays(minute / MinutesPerDay)
		minute %= MinutesPerDay
	}
	insts, gapEnd := wallInstants(loc, d, minute)
	if len(insts) == 0 {
		return gapEnd
	}
	return insts[0]
}

// DayStart returns the instant local date d begins.
func DayStart(loc *time.Location, d Date) time.Time { return ResolveBoundary(loc, d, 0) }

// Day resolves wall times on one local date. Most days have no offset
// change between their midnights; on those every wall time is the day's
// start plus its minutes, which skips the general search of wallInstants.
// Other days fall back to ResolveStart and ResolveBoundary.
type Day struct {
	loc   *time.Location
	date  Date
	start time.Time
	plain bool
}

// NewDay prepares resolution of wall times on d in loc.
func NewDay(loc *time.Location, d Date) Day {
	start := DayStart(loc, d)
	wall := start.In(loc)
	// Plain when midnight exists (it is not inside a gap) and the zone in
	// effect at midnight lasts beyond the next midnight.
	_, end := wall.ZoneBounds()
	plain := wall.Hour() == 0 && wall.Minute() == 0 && wall.Second() == 0 &&
		(end.IsZero() || end.After(start.Add(MinutesPerDay*time.Minute)))
	return Day{loc: loc, date: d, start: start, plain: plain}
}

// Start resolves a candidate start time like ResolveStart.
func (day Day) Start(minute int) (time.Time, bool) {
	if day.plain {
		return day.start.Add(time.Duration(minute) * time.Minute), true
	}
	return ResolveStart(day.loc, day.date, minute)
}

// Boundary resolves a boundary wall time (0..1440) like ResolveBoundary.
func (day Day) Boundary(minute int) time.Time {
	if day.plain {
		return day.start.Add(time.Duration(minute) * time.Minute)
	}
	return ResolveBoundary(day.loc, day.date, minute)
}

// Periods resolves every period of plan on the day.
func (day Day) Periods(plan DayPlan) []ResolvedPeriod {
	out := make([]ResolvedPeriod, 0, len(plan.periods))
	for _, p := range plan.periods {
		s, e := day.Boundary(p.Start), day.Boundary(p.End)
		out = append(out, ResolvedPeriod{Period: p, Abs: Interval{Start: s, End: e}, Empty: !s.Before(e)})
	}
	return out
}

// Interval is a half-open absolute time range [Start, End).
type Interval struct {
	Start time.Time
	End   time.Time
}

// Overlaps reports whether two half-open intervals intersect.
func (i Interval) Overlaps(o Interval) bool {
	return i.Start.Before(o.End) && o.Start.Before(i.End)
}

// Contains reports whether o lies within i.
func (i Interval) Contains(o Interval) bool {
	return !o.Start.Before(i.Start) && !o.End.After(i.End)
}

// ResolvedPeriod is a working period resolved to absolute instants.
type ResolvedPeriod struct {
	Period Period
	Abs    Interval
	// Empty is true when DST collapsed or inverted the period; it is then
	// closed for booking.
	Empty bool
}

// ResolvePeriods resolves every period of plan on date d.
func ResolvePeriods(loc *time.Location, d Date, plan DayPlan) []ResolvedPeriod {
	if plan.IsOff() {
		return nil
	}
	return NewDay(loc, d).Periods(plan)
}
