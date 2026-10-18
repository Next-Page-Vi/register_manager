package booking

import (
	"testing"
	"time"

	"register_manager/internal/apperr"
	"register_manager/internal/schedule"
)

func ptr(i int) *int { return &i }

func snapshot(t testing.TB, input *FactsInput) *Facts {
	t.Helper()
	f, err := NewFacts(*input)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestFactsOwnsItsInput(t *testing.T) {
	l := time.UTC
	start := at(l, 2026, 6, 1, 9, 0)
	in := baseFacts(t, l, start.Add(-time.Hour), calendar(t, schedule.Period{Start: 540, End: 720}))
	in.Appts = []Appt{{ID: "a", CustomerID: "c", Start: start.Add(time.Hour), End: start.Add(90 * time.Minute), Status: "booked"}}
	in.Policy.DailyCustomerLimit = ptr(2)
	in.Calendar.Overrides = map[schedule.Date]schedule.DayPlan{}
	in.Customer = &CustomerContext{ID: "c"}
	f := snapshot(t, in)
	in.Appts[0].Start, in.Appts[0].End = start, start.Add(30*time.Minute)
	if err := f.Check(start, start.Add(30*time.Minute)); err != nil {
		t.Fatalf("snapshot changed: %v", err)
	}
	if err := snapshot(t, in).Check(start, start.Add(30*time.Minute)); apperr.Code(err) != "slot_taken" {
		t.Fatalf("fresh snapshot missed occupancy: %v", err)
	}
	*in.Policy.DailyCustomerLimit = 0
	in.Calendar.Overrides[schedule.DateIn(start, l)] = schedule.Off
	in.Customer.ID = "other"
	if err := f.Check(start, start.Add(30*time.Minute)); err != nil {
		t.Fatalf("aliased input changed snapshot: %v", err)
	}
}

func loc(t testing.TB, name string) *time.Location {
	t.Helper()
	l, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func calendar(t testing.TB, periods ...schedule.Period) schedule.Calendar {
	t.Helper()
	plan, err := schedule.NewDayPlan(periods)
	if err != nil {
		t.Fatal(err)
	}
	c, err := schedule.NewCycle(schedule.NewDate(2026, 1, 5), []schedule.DayPlan{plan})
	if err != nil {
		t.Fatal(err)
	}
	return schedule.Calendar{Cycle: c}
}

func baseFacts(t testing.TB, l *time.Location, now time.Time, cal schedule.Calendar) *FactsInput {
	return &FactsInput{
		Loc: l,
		Now: now,
		Policy: Policy{
			SlotStepMinutes:        15,
			ServiceDurationMinutes: 30,
			BookingHorizonDays:     10,
		},
		Calendar: cal,
	}
}

func at(l *time.Location, y int, m time.Month, d, hh, mm int) time.Time {
	return time.Date(y, m, d, hh, mm, 0, 0, l)
}

func TestGridAlignedToLocalMidnight(t *testing.T) {
	l := loc(t, "Asia/Tokyo")
	f := baseFacts(t, l, at(l, 2026, 6, 1, 0, 0), calendar(t, schedule.Period{Start: 9*60 + 10, End: 10 * 60}))
	got := snapshot(t, f).Candidates(schedule.NewDate(2026, 6, 1))
	if len(got) == 0 || !got[0].StartsAt.Equal(at(l, 2026, 6, 1, 9, 15)) {
		t.Fatalf("first candidate %v", got)
	}
	// 09:15, 09:30 fit; 09:45 would end 10:15 > 10:00.
	if len(got) != 2 {
		t.Errorf("want 2 candidates, got %d", len(got))
	}
}

func TestBufferMayUseNonWorkingTime(t *testing.T) {
	l := loc(t, "Europe/London")
	f := baseFacts(t, l, at(l, 2026, 6, 1, 0, 0), calendar(t, schedule.Period{Start: 9 * 60, End: 10 * 60}))
	f.Policy.BufferMinutes = 60
	// The last service may end at 10:00 even though its buffer would not fit.
	if err := snapshot(t, f).Check(at(l, 2026, 6, 1, 9, 30), at(l, 2026, 6, 1, 10, 0)); err != nil {
		t.Errorf("last slot rejected: %v", err)
	}
}

func TestBufferBetweenAppointments(t *testing.T) {
	l := loc(t, "Europe/London")
	f := baseFacts(t, l, at(l, 2026, 6, 1, 0, 0), calendar(t, schedule.Period{Start: 9 * 60, End: 17 * 60}))
	f.Policy.BufferMinutes = 15
	f.Appts = []Appt{{ID: "a", CustomerID: "x", Start: at(l, 2026, 6, 1, 10, 0), End: at(l, 2026, 6, 1, 10, 30), Status: "booked"}}
	cases := []struct {
		h, m int
		ok   bool
	}{
		{9, 15, true},   // ends 9:45, +15 = 10:00
		{9, 30, false},  // ends 10:00, buffer violated
		{10, 30, false}, // starts at E.end, buffer violated
		{10, 45, true},  // starts at E.end + 15
	}
	for _, c := range cases {
		s := at(l, 2026, 6, 1, c.h, c.m)
		err := snapshot(t, f).Check(s, s.Add(30*time.Minute))
		if (err == nil) != c.ok {
			t.Errorf("%02d:%02d: err=%v want ok=%v", c.h, c.m, err, c.ok)
		}
	}
	// Rescheduling the appointment itself ignores its own occupancy.
	f2 := *f
	f2.ExcludeID = new("a")
	if err := snapshot(t, &f2).Check(at(l, 2026, 6, 1, 10, 15), at(l, 2026, 6, 1, 10, 45)); err != nil {
		t.Errorf("reschedule overlapping itself: %v", err)
	}
}

func TestWindowRules(t *testing.T) {
	l := loc(t, "America/New_York")
	now := at(l, 2026, 6, 1, 9, 0)
	f := baseFacts(t, l, now, calendar(t, schedule.Period{Start: 9 * 60, End: 17 * 60}))
	f.Policy.MinBookingNoticeMinutes = 60
	f.Policy.BookingHorizonDays = 2
	check := func(s time.Time) string { return apperr.Code(snapshot(t, f).Check(s, s.Add(30*time.Minute))) }
	if c := check(at(l, 2026, 6, 1, 9, 45)); c != "too_soon" {
		t.Errorf("notice: %s", c)
	}
	if c := check(at(l, 2026, 6, 1, 10, 0)); c != "" {
		t.Errorf("exactly at notice: %s", c)
	}
	if c := check(at(l, 2026, 6, 3, 16, 0)); c != "" {
		t.Errorf("last horizon day: %s", c)
	}
	if c := check(at(l, 2026, 6, 4, 9, 0)); c != "beyond_horizon" {
		t.Errorf("beyond horizon: %s", c)
	}
	if c := check(at(l, 2026, 6, 2, 9, 5)); c != "slot_unavailable" {
		t.Errorf("off grid: %s", c)
	}
	s := at(l, 2026, 6, 2, 10, 0)
	if c := apperr.Code(snapshot(t, f).Check(s, s.Add(45*time.Minute))); c != "duration_changed" {
		t.Errorf("duration: %s", c)
	}
}

func TestBlocksUseServiceTimeOnly(t *testing.T) {
	l := loc(t, "UTC")
	f := baseFacts(t, l, at(l, 2026, 6, 1, 0, 0), calendar(t, schedule.Period{Start: 9 * 60, End: 17 * 60}))
	f.Policy.BufferMinutes = 30
	f.Blocks = []schedule.Interval{{Start: at(l, 2026, 6, 1, 12, 0), End: at(l, 2026, 6, 2, 13, 0)}}
	if c := apperr.Code(snapshot(t, f).Check(at(l, 2026, 6, 1, 11, 30), at(l, 2026, 6, 1, 12, 0))); c != "" {
		t.Errorf("service ending at block start: %s", c)
	}
	if c := apperr.Code(snapshot(t, f).Check(at(l, 2026, 6, 1, 11, 45), at(l, 2026, 6, 1, 12, 15))); c != "slot_blocked" {
		t.Errorf("overlapping block: %s", c)
	}
	if c := apperr.Code(snapshot(t, f).Check(at(l, 2026, 6, 2, 10, 0), at(l, 2026, 6, 2, 10, 30))); c != "slot_blocked" {
		t.Errorf("multi-day block: %s", c)
	}
}

func TestQuotas(t *testing.T) {
	l := loc(t, "Asia/Shanghai")
	now := at(l, 2026, 6, 1, 0, 0) // Monday
	f := baseFacts(t, l, now, calendar(t, schedule.Period{Start: 9 * 60, End: 17 * 60}))
	appt := func(id, cust string, d, h int, status string) Appt {
		s := at(l, 2026, 6, d, h, 0)
		return Appt{ID: id, CustomerID: cust, Start: s, End: s.Add(30 * time.Minute), Status: status}
	}
	f.Appts = []Appt{
		appt("1", "alice", 2, 9, "booked"),
		appt("2", "bob", 2, 10, "completed"),
		appt("3", "carol", 2, 11, "cancelled"), // cancelled returns quota
		appt("4", "alice", 3, 9, "no_show"),
	}
	f.Policy.DailyCustomerLimit = ptr(2)
	f.Customer = &CustomerContext{ID: "dave"}
	slot := at(l, 2026, 6, 2, 14, 0)
	if c := apperr.Code(snapshot(t, f).Check(slot, slot.Add(30*time.Minute))); c != "daily_capacity_full" {
		t.Errorf("daily distinct: %s", c)
	}
	g := *f
	g.Customer = &CustomerContext{ID: "alice"} // already counted that day
	if c := apperr.Code(snapshot(t, &g).Check(slot, slot.Add(30*time.Minute))); c != "" {
		t.Errorf("existing customer same day: %s", c)
	}
	g.Policy.PerCustomerDailyLimit = ptr(1)
	if c := apperr.Code(snapshot(t, &g).Check(slot, slot.Add(30*time.Minute))); c != "customer_daily_limit" {
		t.Errorf("per-customer daily: %s", c)
	}
	h := *f
	h.Policy.DailyCustomerLimit = nil
	h.Customer = &CustomerContext{ID: "alice"}
	h.Policy.PerCustomerWeeklyLimit = ptr(2)
	thu := at(l, 2026, 6, 4, 9, 0)
	if c := apperr.Code(snapshot(t, &h).Check(thu, thu.Add(30*time.Minute))); c != "customer_weekly_limit" {
		t.Errorf("weekly (booked + no_show count): %s", c)
	}
	nextMon := at(l, 2026, 6, 8, 9, 0)
	if c := apperr.Code(snapshot(t, &h).Check(nextMon, nextMon.Add(30*time.Minute))); c != "" {
		t.Errorf("next week: %s", c)
	}
	h.Policy.MaxFutureBookings = ptr(1)
	h.Customer.FutureBooked = 1
	if c := apperr.Code(snapshot(t, &h).Check(nextMon, nextMon.Add(30*time.Minute))); c != "customer_future_limit" {
		t.Errorf("future limit: %s", c)
	}
	z := *f
	z.Policy.DailyCustomerLimit = nil
	z.Policy.PerCustomerDailyLimit = ptr(0)
	if c := apperr.Code(snapshot(t, &z).Check(nextMon, nextMon.Add(30*time.Minute))); c != "customer_daily_limit" {
		t.Errorf("zero limit means none allowed: %s", c)
	}
}

func TestDSTCandidates(t *testing.T) {
	l := loc(t, "America/New_York")
	f := baseFacts(t, l, at(l, 2026, 3, 1, 0, 0), calendar(t, schedule.Period{Start: 0, End: 6 * 60}))
	f.Policy.SlotStepMinutes = 30
	f.Policy.ServiceDurationMinutes = 30

	spring := snapshot(t, f).Candidates(schedule.NewDate(2026, 3, 8))
	for _, c := range spring {
		if lt := c.StartsAt.In(l); lt.Hour() == 2 {
			t.Errorf("nonexistent 02:xx candidate %v", lt)
		}
	}
	// 00:00..05:30 is 12 wall slots minus 02:00 and 02:30.
	if len(spring) != 10 {
		t.Errorf("spring candidates = %d", len(spring))
	}

	fall := snapshot(t, f).Candidates(schedule.NewDate(2026, 11, 1))
	seen := map[int64]bool{}
	for i, c := range fall {
		if seen[c.StartsAt.Unix()] {
			t.Errorf("duplicate candidate %v", c.StartsAt)
		}
		seen[c.StartsAt.Unix()] = true
		if i > 0 && !c.StartsAt.After(fall[i-1].StartsAt) {
			t.Errorf("not ascending at %d", i)
		}
	}
	// Repeated 01:00 and 01:30 are offered once (earlier instance): 12 slots.
	if len(fall) != 12 {
		t.Errorf("fall candidates = %d", len(fall))
	}
	// 01:30 EDT + 30 min = 01:00 EST: service of real 30 minutes.
	for _, c := range fall {
		if c.EndsAt.Sub(c.StartsAt) != 30*time.Minute {
			t.Errorf("duration %v", c.EndsAt.Sub(c.StartsAt))
		}
	}
}

func TestSlotsPagination(t *testing.T) {
	l := loc(t, "UTC")
	f := baseFacts(t, l, at(l, 2026, 6, 1, 9, 50), calendar(t, schedule.Period{Start: 9 * 60, End: 11 * 60}))
	f.Policy.SlotStepMinutes = 30
	first := snapshot(t, f).Slots(nil, ptr(3))
	if len(first) != 3 || !first[0].StartsAt.Equal(at(l, 2026, 6, 1, 10, 0)) {
		t.Fatalf("first page %v", first)
	}
	next := snapshot(t, f).Slots(&first[2].StartsAt, ptr(2))
	if len(next) != 2 || !next[0].StartsAt.After(first[2].StartsAt) {
		t.Fatalf("next page %v", next)
	}
	// Cursor inside a day continues with the rest of that day.
	if !next[0].StartsAt.Equal(at(l, 2026, 6, 2, 9, 30)) {
		t.Errorf("next[0] = %v", next[0].StartsAt)
	}
}

func FuzzSlotsAreValid(f *testing.F) {
	f.Add(int64(0), 15, 30, 10, 540, 1020, "America/New_York")
	f.Add(int64(86400*70), 7, 45, 0, 0, 1440, "Australia/Lord_Howe")
	f.Add(int64(86400*300), 20, 25, 5, 60, 300, "Europe/London")
	f.Fuzz(func(t *testing.T, offset int64, step, dur, buf, ps, pe int, zone string) {
		l, err := time.LoadLocation(zone)
		if err != nil || step < 1 || step > 240 || dur < 1 || dur > 600 || buf < 0 || buf > 240 {
			t.Skip()
		}
		plan, err := schedule.NewDayPlan([]schedule.Period{{Start: ps, End: pe}})
		if err != nil {
			t.Skip()
		}
		c, _ := schedule.NewCycle(schedule.NewDate(2026, 1, 1), []schedule.DayPlan{plan})
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(offset%(86400*365)) * time.Second)
		fc := &FactsInput{
			Loc: l, Now: now, Calendar: schedule.Calendar{Cycle: c},
			Policy: Policy{SlotStepMinutes: step, ServiceDurationMinutes: dur, BufferMinutes: buf, BookingHorizonDays: 3},
		}
		slots := snapshot(t, fc).Slots(nil, nil)
		for i, s := range slots {
			if i > 0 && !s.StartsAt.After(slots[i-1].StartsAt) {
				t.Fatalf("not ascending")
			}
			if err := snapshot(t, fc).Check(s.StartsAt, s.EndsAt); err != nil {
				t.Fatalf("slot %v fails check: %v", s.StartsAt, err)
			}
			if lt := s.StartsAt.In(l); (lt.Hour()*60+lt.Minute())%step != 0 {
				t.Fatalf("slot %v off grid", lt)
			}
		}
	})
}

func TestEmptyReason(t *testing.T) {
	l := loc(t, "UTC")
	// 2026-06-01 is a Monday; two 30-minute slots a day at 09:00 and 09:30.
	open := calendar(t, schedule.Period{Start: 9 * 60, End: 10 * 60})
	mk := func(now time.Time, cal schedule.Calendar) *FactsInput {
		f := baseFacts(t, l, now, cal)
		f.Policy.SlotStepMinutes = 30
		f.Policy.BookingHorizonDays = 3
		f.Customer = &CustomerContext{ID: "c"}
		return f
	}
	midnight := at(l, 2026, 6, 1, 0, 0)
	own := Appt{ID: "a", CustomerID: "c", Start: at(l, 2026, 6, 2, 9, 0), End: at(l, 2026, 6, 2, 9, 30), Status: "booked"}
	cases := []struct {
		name  string
		setup func(f *FactsInput)
		cal   schedule.Calendar
		now   time.Time
		want  EmptyReason
	}{
		{"no working time", func(f *FactsInput) {}, calendar(t), midnight, EmptyMisconfigured},
		{"daily capacity zero", func(f *FactsInput) { f.Policy.DailyCustomerLimit = ptr(0) }, open, midnight, EmptyMisconfigured},
		{"future limit zero", func(f *FactsInput) { f.Policy.MaxFutureBookings = ptr(0) }, open, midnight, EmptyMisconfigured},
		{"notice beyond horizon", func(f *FactsInput) { f.Policy.MinBookingNoticeMinutes = 4 * 24 * 60 }, open, midnight, EmptyMisconfigured},
		{"same-day horizon passed", func(f *FactsInput) { f.Policy.BookingHorizonDays = 0 }, open, at(l, 2026, 6, 1, 9, 45), EmptyFull},
		{"all blocked", func(f *FactsInput) {
			f.Blocks = []schedule.Interval{{Start: midnight, End: midnight.AddDate(0, 0, 10)}}
		}, open, midnight, EmptyFull},
		{"future limit", func(f *FactsInput) {
			f.Policy.MaxFutureBookings = ptr(1)
			f.Customer.FutureBooked = 1
		}, open, midnight, EmptyCustomerLimit},
		{"weekly limit", func(f *FactsInput) {
			f.Policy.PerCustomerWeeklyLimit = ptr(1)
			f.Appts = []Appt{own}
			f.Customer.FutureBooked = 1
		}, open, midnight, EmptyPeriodLimit},
		{"future limit wins over weekly", func(f *FactsInput) {
			f.Policy.PerCustomerWeeklyLimit = ptr(1)
			f.Policy.MaxFutureBookings = ptr(1)
			f.Appts = []Appt{own}
			f.Customer.FutureBooked = 1
		}, open, midnight, EmptyCustomerLimit},
		{"full beats personal limit", func(f *FactsInput) {
			f.Policy.MaxFutureBookings = ptr(1)
			f.Customer.FutureBooked = 1
			f.Blocks = []schedule.Interval{{Start: midnight, End: midnight.AddDate(0, 0, 10)}}
		}, open, midnight, EmptyFull},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := mk(tc.now, tc.cal)
			tc.setup(f)
			if s := snapshot(t, f).Slots(nil, nil); len(s) != 0 {
				t.Fatalf("expected no slots, got %v", s)
			}
			if got := snapshot(t, f).EmptyReason(nil); got != tc.want {
				t.Errorf("reason = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFactsCopiesBlocksAndExclusion(t *testing.T) {
	start := at(time.UTC, 2026, 6, 1, 9, 0)
	in := baseFacts(t, time.UTC, start.Add(-time.Hour), calendar(t, schedule.Period{Start: 540, End: 720}))
	excluded := "existing"
	in.ExcludeID = &excluded
	in.Appts = []Appt{{ID: excluded, CustomerID: "customer", Start: start, End: start.Add(30 * time.Minute), Status: "booked"}}
	in.Blocks = []schedule.Interval{{Start: start.Add(time.Hour), End: start.Add(2 * time.Hour)}}
	f := snapshot(t, in)
	excluded = "different"
	in.Blocks[0].Start = start
	if err := f.Check(start, start.Add(30*time.Minute)); err != nil {
		t.Fatalf("mutating input changed the snapshot: %v", err)
	}
	if err := snapshot(t, in).Check(start, start.Add(30*time.Minute)); err == nil {
		t.Fatal("fresh snapshot did not see the changed block")
	}
}

func TestDateAvailabilityAcrossDSTAndPagination(t *testing.T) {
	l := loc(t, "Europe/London")
	for _, date := range []schedule.Date{schedule.NewDate(2026, 3, 29), schedule.NewDate(2026, 10, 25)} {
		input := baseFacts(t, l, schedule.DayStart(l, date), calendar(t, schedule.Period{Start: 0, End: 240}))
		f := snapshot(t, input)
		expected := f.Candidates(date)
		var after *time.Time
		got := []Slot{}
		for {
			result := page(f, &date, after, 3)
			got = append(got, result.Items...)
			if result.NextCursor == nil {
				break
			}
			var err error
			after, err = availabilityCursor(result.NextCursor)
			if err != nil {
				t.Fatal(err)
			}
		}
		if len(got) != len(expected) {
			t.Fatalf("%s: got %d, want %d", date, len(got), len(expected))
		}
		for i, slot := range got {
			if !slot.StartsAt.Equal(expected[i].StartsAt) || schedule.DateIn(slot.StartsAt, l) != date {
				t.Fatalf("DST date mismatch: %v", slot)
			}
		}
	}
}

func TestDateAvailabilityReasonsAndReschedule(t *testing.T) {
	date := schedule.NewDate(2026, 6, 1)
	input := baseFacts(t, time.UTC, at(time.UTC, 2026, 6, 1, 0, 0), calendar(t, schedule.Period{Start: 540, End: 570}))
	input.Customer = &CustomerContext{ID: "customer", FutureBooked: 1}
	input.Policy.MaxFutureBookings = ptr(1)
	check := func(want EmptyReason) {
		t.Helper()
		p := page(snapshot(t, input), &date, nil, 20)
		if len(p.Items) != 0 || p.EmptyReason == nil || *p.EmptyReason != want {
			t.Fatalf("want %s, got %+v", want, p)
		}
	}
	check(EmptyCustomerLimit)
	input.Policy.MaxFutureBookings = nil
	input.Policy.PerCustomerWeeklyLimit = ptr(1)
	input.Appts = []Appt{{ID: "other", CustomerID: "customer", Start: at(time.UTC, 2026, 6, 2, 9, 0), End: at(time.UTC, 2026, 6, 2, 9, 30), Status: "booked"}}
	check(EmptyPeriodLimit)
	input.Policy.PerCustomerWeeklyLimit = nil
	input.Appts = []Appt{{ID: "booked", CustomerID: "customer", Start: at(time.UTC, 2026, 6, 1, 9, 0), End: at(time.UTC, 2026, 6, 1, 9, 30), Status: "booked"}}
	check(EmptyOnDate)
	input.ExcludeID = new("booked")
	if p := page(snapshot(t, input), &date, nil, 20); len(p.Items) != 1 {
		t.Fatalf("reschedule: %+v", p)
	}
	input.Policy.DailyCustomerLimit = ptr(0)
	check(EmptyMisconfigured)
	input.Policy.DailyCustomerLimit = nil
	input.Blocks = []schedule.Interval{{Start: at(time.UTC, 2026, 6, 1, 9, 0), End: at(time.UTC, 2026, 6, 1, 10, 0)}}
	check(EmptyOnDate)
}
