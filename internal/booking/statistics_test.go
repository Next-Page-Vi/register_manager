package booking

import (
	"testing"

	"register_manager/internal/schedule"
)

func TestStatisticsBucketBoundaries(t *testing.T) {
	for _, tc := range []struct {
		from, to, grain, firstEnd, lastStart string
		count                                int
	}{
		{"2026-01-01", "2026-01-31", "day", "2026-01-01", "2026-01-31", 31},
		{"2026-01-01", "2026-02-01", "week", "2026-01-04", "2026-01-26", 5},
		{"2026-01-01", "2026-06-29", "week", "2026-01-04", "2026-06-29", 27},
		{"2026-01-01", "2026-06-30", "month", "2026-01-31", "2026-06-01", 6},
		{"2025-12-15", "2026-12-15", "month", "2025-12-31", "2026-12-01", 13},
	} {
		t.Run(tc.from+"_"+tc.to, func(t *testing.T) {
			from, _ := schedule.ParseDate(tc.from)
			to, _ := schedule.ParseDate(tc.to)
			grain, buckets, indices := statisticsBuckets(from, to)
			if grain != tc.grain || len(buckets) != tc.count || buckets[0].From != tc.from || buckets[0].To != tc.firstEnd || buckets[len(buckets)-1].From != tc.lastStart || buckets[len(buckets)-1].To != tc.to {
				t.Fatalf("unexpected buckets: %s %+v", grain, buckets)
			}
			if len(indices) != to.Sub(from)+1 {
				t.Fatal("dates omitted from buckets")
			}
			for day := from; day <= to; day++ {
				bucket := buckets[indices[day]]
				if date := day.String(); date < bucket.From || date > bucket.To {
					t.Fatalf("%s mapped to %+v", date, bucket)
				}
			}
		})
	}
}
