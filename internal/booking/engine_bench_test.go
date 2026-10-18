package booking

import (
	"fmt"
	"testing"

	"register_manager/internal/schedule"
)

// benchFacts is a working day of 9:00–12:00 and 13:00–17:00 every day, with
// one appointment every other hour so occupancy checks do real work.
func benchFacts(b *testing.B, horizon, step int) *Facts {
	l := loc(b, "Europe/London")
	now := at(l, 2026, 3, 1, 0, 0)
	f := baseFacts(b, l, now, calendar(b, schedule.Period{Start: 9 * 60, End: 12 * 60}, schedule.Period{Start: 13 * 60, End: 17 * 60}))
	f.Policy.BookingHorizonDays = horizon
	f.Policy.SlotStepMinutes = step
	for d := 0; d <= horizon; d++ {
		for h := 9; h < 17; h += 2 {
			s := at(l, 2026, 3, 1+d, h, 0)
			f.Appts = append(f.Appts, Appt{ID: fmt.Sprint(d, h), CustomerID: "c", Start: s, End: s.Add(f.Policy.Duration()), Status: "booked"})
		}
	}
	return snapshot(b, f)
}

func BenchmarkSlots(b *testing.B) {
	for _, horizon := range []int{30, 366} {
		for _, step := range []int{5, 30} {
			b.Run(fmt.Sprintf("horizon=%d/step=%d", horizon, step), func(b *testing.B) {
				f := benchFacts(b, horizon, step)
				for b.Loop() {
					f.Slots(&f.input.Now, nil)
				}
			})
		}
	}
}

func BenchmarkEmptyReason(b *testing.B) {
	f := benchFacts(b, 366, 5)
	for b.Loop() {
		f.EmptyReason(&f.input.Now)
	}
}
