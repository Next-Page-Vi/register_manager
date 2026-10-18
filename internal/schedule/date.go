// Package schedule implements the calendar side of booking: civil dates,
// N-day cycles, date overrides and the resolution of local wall-clock times
// to absolute instants, including daylight-saving transitions. It has no
// database dependency.
package schedule

import (
	"encoding/json"
	"fmt"
	"time"
)

// Date is a civil (proleptic Gregorian) calendar date without a time zone,
// represented as the number of days since 1970-01-01.
type Date int

const dateLayout = "2006-01-02"

// NewDate returns the date for year, month and day. Out-of-range values are
// normalized the same way time.Date normalizes them.
func NewDate(year int, month time.Month, day int) Date {
	return Date(time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}

// DateOf returns the civil date of t in t's location.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return NewDate(y, m, d)
}

// DateIn returns the civil date of instant t in loc.
func DateIn(t time.Time, loc *time.Location) Date { return DateOf(t.In(loc)) }

// ParseDate parses YYYY-MM-DD.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return 0, fmt.Errorf("invalid date %q", s)
	}
	return DateOf(t), nil
}

func (d Date) utc() time.Time { return time.Unix(int64(d)*86400, 0).UTC() }

// YMD returns the year, month and day.
func (d Date) YMD() (int, time.Month, int) { return d.utc().Date() }

func (d Date) String() string { return d.utc().Format(dateLayout) }

// AddDays returns d shifted by n days.
func (d Date) AddDays(n int) Date { return d + Date(n) }

// Sub returns the number of calendar days from o to d.
func (d Date) Sub(o Date) int { return int(d - o) }

// Weekday returns the day of the week.
func (d Date) Weekday() time.Weekday { return d.utc().Weekday() }

// WeekStart returns the Monday on or before d.
func (d Date) WeekStart() Date {
	return d - Date((int(d.Weekday())+6)%7)
}

func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// CycleIndex returns the 0-based position of date in an n-day cycle starting
// at anchor. Dates before the anchor are handled by normalizing the
// remainder.
func CycleIndex(date, anchor Date, n int) int {
	return ((date.Sub(anchor) % n) + n) % n
}
