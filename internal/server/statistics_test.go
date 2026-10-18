package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"register_manager/internal/booking"
)

func statisticsResult(t *testing.T, c *client, base, query string) booking.Statistics {
	t.Helper()
	var out booking.Statistics
	if err := json.Unmarshal(c.get(base+"/statistics?"+query).Raw, &out); err != nil {
		t.Fatal(err)
	}
	var sum booking.StatusCounts
	for _, bucket := range out.Series {
		sum.Booked += bucket.Counts.Booked
		sum.Completed += bucket.Counts.Completed
		sum.Cancelled += bucket.Counts.Cancelled
		sum.NoShow += bucket.Counts.NoShow
	}
	if sum != out.Summary.StatusCounts || sum.Booked+sum.Completed+sum.Cancelled+sum.NoShow != out.Summary.Total {
		t.Fatalf("series and summary differ: %+v", out)
	}
	return out
}

func insertStatisticsAppointment(t *testing.T, e *env, w *world, customer, start, created, status string) string {
	t.Helper()
	parse := func(value string) int64 {
		x, err := time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatal(err)
		}
		return x.UnixMilli()
	}
	id, at, made := newUUID(), parse(start), parse(created)
	_, err := e.db.W.Exec(`INSERT INTO appointments (id, instance_id, customer_id, starts_at, ends_at, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, w.instanceID, customer, at, at+30*60*1000, status, made, made)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestStatisticsCountsBasisAndChanges(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("statistics", "stats-worker")
	_, first, _ := e.newCustomer(w, "First")
	_, second, _ := e.newCustomer(w, "Second")
	base := "/api/v1/instances/" + w.instanceID
	insertStatisticsAppointment(t, e, w, first, "2026-06-02T09:00:00Z", "2026-06-01T23:30:00Z", "completed")
	insertStatisticsAppointment(t, e, w, first, "2026-06-02T10:00:00Z", "2026-06-01T08:00:00Z", "cancelled")
	insertStatisticsAppointment(t, e, w, second, "2026-06-03T09:00:00Z", "2026-06-01T09:00:00Z", "no_show")
	id := insertStatisticsAppointment(t, e, w, second, "2026-06-03T10:00:00Z", "2026-06-01T10:00:00Z", "booked")
	query := "from=2026-06-02&to=2026-06-03"
	got := statisticsResult(t, w.worker, base, query)
	if got.Basis != "starts_at" || got.Timezone != "Europe/London" || got.Summary.Total != 4 || got.Summary.UniqueCustomers != 2 || got.Summary.StatusCounts != (booking.StatusCounts{Booked: 1, Completed: 1, Cancelled: 1, NoShow: 1}) {
		t.Fatalf("unexpected summary: %+v", got)
	}
	if len(got.Series) != 2 || got.Series[0].Counts.Completed != 1 || got.Series[1].Counts.NoShow != 1 {
		t.Fatalf("unexpected daily counts: %+v", got.Series)
	}
	created := statisticsResult(t, w.worker, base, "from=2026-06-01&to=2026-06-01&basis=created_at")
	if created.Summary.Total != 3 || created.Summary.Completed != 0 {
		t.Fatalf("creation dates did not use local midnight: %+v", created)
	}
	// Moving an appointment changes its scheduled bucket, not its creation bucket.
	w.worker.must("POST", base+"/appointments/"+id+"/reschedule", map[string]any{
		"expected_starts_at": "2026-06-03T10:00:00Z", "expected_ends_at": "2026-06-03T10:30:00Z",
		"starts_at": "2026-06-04T10:00:00Z", "ends_at": "2026-06-04T10:30:00Z",
	}, 200)
	if moved := statisticsResult(t, w.worker, base, query); moved.Summary.Total != 3 {
		t.Fatalf("reschedule not reflected: %+v", moved)
	}
	w.worker.must("POST", base+"/appointments/"+id+"/status", map[string]any{"status": "cancelled"}, 200)
	created = statisticsResult(t, w.worker, base, "from=2026-06-01&to=2026-06-01&basis=created_at")
	if created.Summary.Total != 3 || created.Summary.Cancelled != 2 || created.Summary.Booked != 0 {
		t.Fatalf("creation cohort did not reflect current status: %+v", created)
	}
	empty := statisticsResult(t, w.worker, base, "from=2026-05-01&to=2026-05-03")
	if empty.Summary != (booking.StatisticsSummary{}) || len(empty.Series) != 3 {
		t.Fatalf("empty period must have zero buckets: %+v", empty)
	}
	// Elapsed time alone never changes a booked appointment to no-show.
	insertStatisticsAppointment(t, e, w, first, "2026-05-01T09:00:00Z", "2026-04-01T09:00:00Z", "booked")
	past := statisticsResult(t, w.worker, base, "from=2026-05-01&to=2026-05-01")
	if past.Summary.Booked != 1 || past.Summary.NoShow != 0 {
		t.Fatal("past booking was inferred to be a no-show")
	}
}

func TestStatisticsDSTPermissionsAndValidation(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("statistics-dst", "stats-dst-worker")
	c, customer, _ := e.newCustomer(w, "DST")
	base := "/api/v1/instances/" + w.instanceID
	for _, tc := range []struct {
		date, before, first, last, after string
	}{
		{"2026-03-29", "2026-03-28T23:59:00Z", "2026-03-29T00:00:00Z", "2026-03-29T22:59:00Z", "2026-03-29T23:00:00Z"},
		{"2026-10-25", "2026-10-24T22:59:00Z", "2026-10-24T23:00:00Z", "2026-10-25T23:59:00Z", "2026-10-26T00:00:00Z"},
	} {
		for _, at := range []string{tc.before, tc.first, tc.last, tc.after} {
			insertStatisticsAppointment(t, e, w, customer, at, at, "cancelled")
		}
		for _, basis := range []string{"starts_at", "created_at"} {
			got := statisticsResult(t, w.worker, base, fmt.Sprintf("from=%s&to=%s&basis=%s", tc.date, tc.date, basis))
			if got.Summary.Total != 2 {
				t.Fatalf("%s %s: %+v", tc.date, basis, got)
			}
		}
	}
	query := "from=2026-03-29&to=2026-03-29"
	other := e.setupInstance("statistics-other", "stats-other-worker")
	other.worker.must("GET", base+"/statistics?"+query, nil, 404)
	e.client().must("GET", base+"/statistics?"+query, nil, 401)
	c.must("GET", base+"/statistics?"+query, nil, 401)
	statisticsResult(t, w.admin, base, query)
	if got := statisticsResult(t, other.worker, "/api/v1/instances/"+other.instanceID, query); got.Summary.Total != 0 {
		t.Fatal("statistics leaked between instances")
	}
	for _, query := range []string{
		"", "from=2026-01-01", "from=&to=2026-01-01", "from=2026-02-30&to=2026-03-01",
		"from=2026-01-02&to=2026-01-01", "from=2026-01-01&to=2027-01-02",
		"from=2026-01-01&to=2026-01-02&basis=status", "from=2026-01-01&to=2026-01-02&basis=",
	} {
		w.worker.must("GET", base+"/statistics?"+query, nil, 422)
	}
	// Both time bases have an instance-scoped range index.
	for _, column := range []string{"starts_at", "created_at"} {
		rows, err := e.db.R.Query("EXPLAIN QUERY PLAN SELECT status FROM appointments WHERE instance_id = ? AND "+column+" >= ? AND "+column+" < ?", w.instanceID, 1, 2)
		if err != nil {
			t.Fatal(err)
		}
		var details []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			details = append(details, detail)
		}
		rows.Close()
		if !strings.Contains(strings.Join(details, " "), "USING INDEX appointments_instance_") {
			t.Fatalf("%s did not use range index: %v", column, details)
		}
	}
}
