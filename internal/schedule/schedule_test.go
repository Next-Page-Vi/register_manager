package schedule

import (
	"testing"
	"time"

	"register_manager/internal/apperr"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestDateArithmetic(t *testing.T) {
	d := NewDate(2024, time.February, 28)
	if got := d.AddDays(1).String(); got != "2024-02-29" {
		t.Errorf("leap day: %s", got)
	}
	if got := d.AddDays(2).String(); got != "2024-03-01" {
		t.Errorf("after leap: %s", got)
	}
	if got := NewDate(1969, time.December, 31).String(); got != "1969-12-31" {
		t.Errorf("pre-epoch: %s", got)
	}
	if NewDate(2026, time.March, 9).Weekday() != time.Monday {
		t.Error("2026-03-09 should be Monday")
	}
	if got := NewDate(2026, time.March, 15).WeekStart().String(); got != "2026-03-09" {
		t.Errorf("week start of Sunday: %s", got)
	}
	if got := NewDate(2026, time.March, 9).WeekStart().String(); got != "2026-03-09" {
		t.Errorf("week start of Monday: %s", got)
	}
	p, err := ParseDate("2026-10-14")
	if err != nil || p != NewDate(2026, 10, 14) {
		t.Errorf("parse: %v %v", p, err)
	}
	if _, err := ParseDate("2026-02-30"); err == nil {
		t.Error("invalid date accepted")
	}
}

func TestCycleIndex(t *testing.T) {
	anchor := NewDate(2026, 1, 1)
	cases := []struct {
		date Date
		n    int
		want int
	}{
		{anchor, 5, 0},
		{anchor.AddDays(4), 5, 4},
		{anchor.AddDays(5), 5, 0},
		{anchor.AddDays(-1), 5, 4},
		{anchor.AddDays(-5), 5, 0},
		{anchor.AddDays(-6), 5, 4},
		{anchor.AddDays(100), 7, 2},
		{anchor.AddDays(-100), 7, 5},
		{anchor.AddDays(3), 1, 0},
	}
	for _, c := range cases {
		if got := CycleIndex(c.date, anchor, c.n); got != c.want {
			t.Errorf("CycleIndex(%s, n=%d) = %d, want %d", c.date, c.n, got, c.want)
		}
	}
}

func TestCycleAcrossDSTUsesCalendarDays(t *testing.T) {
	// A naive (midnight difference / 24h) computation goes wrong across the
	// 23-hour spring-forward day; civil date arithmetic must not.
	anchor := NewDate(2026, 3, 7)
	if got := CycleIndex(NewDate(2026, 3, 9), anchor, 2); got != 0 {
		t.Errorf("got %d", got)
	}
}

func TestNewDayPlan(t *testing.T) {
	p, err := NewDayPlan([]Period{{780, 1020}, {540, 720}})
	if err != nil {
		t.Fatal(err)
	}
	if ps := p.Periods(); len(ps) != 2 || ps[0].Start != 540 {
		t.Errorf("not sorted: %v", ps)
	}
	merged, err := NewDayPlan([]Period{{540, 720}, {720, 1020}})
	if err != nil || len(merged.Periods()) != 1 || merged.Periods()[0].End != 1020 {
		t.Errorf("touching periods not merged: %v %v", merged.Periods(), err)
	}
	if _, err := NewDayPlan([]Period{{1200, 1440}}); err != nil {
		t.Errorf("end at midnight rejected: %v", err)
	}
	bad := map[string][]Period{
		"periods_overlap":             {{540, 720}, {700, 800}},
		"period_start_not_before_end": {{600, 600}},
		"period_out_of_range":         {{1380, 1500}},
	}
	for code, ps := range bad {
		if _, err := NewDayPlan(ps); apperr.Code(err) != code {
			t.Errorf("%v: got %v, want %s", ps, err, code)
		}
	}
	off, _ := NewDayPlan(nil)
	if !off.IsOff() {
		t.Error("empty plan should be off")
	}
}

func TestCalendarOverrides(t *testing.T) {
	work, _ := NewDayPlan([]Period{{540, 1020}})
	c, err := NewCycle(NewDate(2026, 1, 1), []DayPlan{work, work, work, Off, Off})
	if err != nil {
		t.Fatal(err)
	}
	special, _ := NewDayPlan([]Period{{600, 660}})
	cal := Calendar{Cycle: c, Overrides: map[Date]DayPlan{
		NewDate(2026, 1, 2): Off,
		NewDate(2026, 1, 4): special,
	}}
	if !cal.PlanOn(NewDate(2026, 1, 2)).IsOff() {
		t.Error("explicit day off override ignored")
	}
	if cal.PlanOn(NewDate(2026, 1, 3)).IsOff() {
		t.Error("inherit should follow cycle")
	}
	if p := cal.PlanOn(NewDate(2026, 1, 4)).Periods(); len(p) != 1 || p[0].Start != 600 {
		t.Error("replacement override ignored on cycle day off")
	}
	if !cal.PlanOn(NewDate(2026, 1, 5)).IsOff() || cal.PlanOn(NewDate(2026, 1, 6)).IsOff() {
		t.Error("cycle repetition wrong")
	}
}

func TestResolveDST(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	// 2026-03-08 02:00 -> 03:00 (spring forward); 2026-11-01 02:00 -> 01:00.
	spring := NewDate(2026, 3, 8)
	fall := NewDate(2026, 11, 1)

	if _, ok := ResolveStart(ny, spring, 2*60+30); ok {
		t.Error("02:30 on spring-forward day should not exist")
	}
	gapEnd := time.Date(2026, 3, 8, 7, 0, 0, 0, time.UTC) // 03:00 EDT
	if got := ResolveBoundary(ny, spring, 2*60+30); !got.Equal(gapEnd) {
		t.Errorf("boundary in gap = %v, want %v", got, gapEnd)
	}
	got, ok := ResolveStart(ny, fall, 60+30)
	want := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC) // 01:30 EDT, the earlier
	if !ok || !got.Equal(want) {
		t.Errorf("repeated 01:30 = %v, want %v", got, want)
	}
	if got := ResolveBoundary(ny, fall, 60+30); !got.Equal(want) {
		t.Errorf("repeated boundary = %v", got)
	}
	if got := ResolveBoundary(ny, NewDate(2026, 3, 7), 1440); !got.Equal(DayStart(ny, spring)) {
		t.Error("1440 should equal next day start")
	}

	// Lord Howe shifts by 30 minutes.
	lh := mustLoc(t, "Australia/Lord_Howe")
	// 2026-10-04 02:00 -> 02:30 local.
	if _, ok := ResolveStart(lh, NewDate(2026, 10, 4), 2*60+15); ok {
		t.Error("Lord Howe 02:15 should not exist")
	}
	if _, ok := ResolveStart(lh, NewDate(2026, 10, 4), 2*60+30); !ok {
		t.Error("Lord Howe 02:30 should exist")
	}

	// Southern hemisphere: Sydney falls back on 2026-04-05 03:00 -> 02:00.
	syd := mustLoc(t, "Australia/Sydney")
	got, _ = ResolveStart(syd, NewDate(2026, 4, 5), 2*60+30)
	if want := time.Date(2026, 4, 4, 15, 30, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Sydney repeated 02:30 = %v, want %v (AEDT)", got, want)
	}

	// No DST, non-whole-hour offset.
	kol := mustLoc(t, "Asia/Kolkata")
	got, _ = ResolveStart(kol, NewDate(2026, 6, 1), 9*60)
	if want := time.Date(2026, 6, 1, 3, 30, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Kolkata 09:00 = %v", got)
	}
}

func TestResolvePeriodsCollapse(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	plan, _ := NewDayPlan([]Period{{2*60 + 10, 2*60 + 50}, {9 * 60, 12 * 60}})
	rps := ResolvePeriods(ny, NewDate(2026, 3, 8), plan)
	if !rps[0].Empty {
		t.Error("period entirely inside the gap should collapse")
	}
	if rps[1].Empty || rps[1].Abs.End.Sub(rps[1].Abs.Start) != 3*time.Hour {
		t.Error("normal period wrong")
	}
	// A period spanning the gap loses an hour of real time.
	span, _ := NewDayPlan([]Period{{60, 4 * 60}})
	rp := ResolvePeriods(ny, NewDate(2026, 3, 8), span)[0]
	if rp.Abs.End.Sub(rp.Abs.Start) != 2*time.Hour {
		t.Errorf("spanning period length %v", rp.Abs.End.Sub(rp.Abs.Start))
	}
}

// Day's fast path for days without a transition must agree with the
// general resolution everywhere, including zones that switch at midnight.
func TestDayMatchesGeneralResolution(t *testing.T) {
	zones := []string{
		"America/New_York", "Europe/London", "Australia/Lord_Howe", "Asia/Shanghai",
		"America/Havana", "America/Santiago", "America/Sao_Paulo", "Pacific/Apia",
	}
	minutes := []int{1439, 1440}
	for m := 0; m < MinutesPerDay; m += 5 {
		minutes = append(minutes, m)
	}
	for _, name := range zones {
		loc := mustLoc(t, name)
		for _, year := range []int{2018, 2026} {
			first, last := NewDate(year, time.January, 1), NewDate(year, time.December, 31)
			for d := first; d <= last; d++ {
				day := NewDay(loc, d)
				for _, m := range minutes {
					if got, want := day.Boundary(m), ResolveBoundary(loc, d, m); !got.Equal(want) {
						t.Fatalf("%s %s boundary %d: %v, want %v", name, d, m, got, want)
					}
					if m == MinutesPerDay {
						continue
					}
					got, gotOK := day.Start(m)
					want, wantOK := ResolveStart(loc, d, m)
					if gotOK != wantOK || !got.Equal(want) {
						t.Fatalf("%s %s start %d: %v %v, want %v %v", name, d, m, got, gotOK, want, wantOK)
					}
				}
			}
		}
	}
}
