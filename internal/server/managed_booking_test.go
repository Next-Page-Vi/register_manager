package server

import (
	"testing"
	"time"
)

func managedBody(customerID string, start, end time.Time) map[string]any {
	return map[string]any{"id": newUUID(), "customer_id": customerID, "starts_at": start.Format(time.RFC3339), "ends_at": end.Format(time.RFC3339)}
}

func TestManagedBookingExemptions(t *testing.T) {
	cases := []struct {
		name     string
		start    time.Time
		duration time.Duration
		policy   map[string]any
		code     string
	}{
		{"duration", slot10, 17 * time.Minute, nil, "duration_changed"},
		{"grid", slot10.Add(7 * time.Minute), 30 * time.Minute, nil, "slot_unavailable"},
		{"off_hours", slot10.Add(10 * time.Hour), 30 * time.Minute, nil, "slot_unavailable"},
		{"day_off", slot10, 30 * time.Minute, nil, "slot_unavailable"},
		{"block", slot10, 30 * time.Minute, nil, "slot_blocked"},
		{"notice", slot10, 30 * time.Minute, map[string]any{"min_booking_notice_minutes": 60}, "too_soon"},
		{"horizon", slot10.AddDate(1, 0, 0), 30 * time.Minute, nil, "beyond_horizon"},
		{"daily_capacity", slot10, 30 * time.Minute, map[string]any{"daily_customer_limit": 0}, "daily_capacity_full"},
		{"customer_daily", slot10, 30 * time.Minute, map[string]any{"per_customer_daily_limit": 0}, "customer_daily_limit"},
		{"customer_weekly", slot10, 30 * time.Minute, map[string]any{"per_customer_weekly_limit": 0}, "customer_weekly_limit"},
		{"customer_future", slot10, 30 * time.Minute, map[string]any{"max_future_bookings": 0}, "customer_future_limit"},
		{"buffer", slot10, 30 * time.Minute, map[string]any{"buffer_minutes": 15}, "slot_taken"},
		{"paused", slot10, 30 * time.Minute, nil, "instance_paused"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.createSuperAdmin("root", "root-password")
			w := e.setupInstance("alpha", "worker1")
			cust, customerID, _ := e.newCustomer(w, "Alice")
			base := "/api/v1/instances/" + w.instanceID
			policy := w.worker.get(base + "/policy").Body
			for key, value := range tc.policy {
				policy[key] = value
			}
			w.worker.must("PUT", base+"/policy", policy, 200)
			switch tc.name {
			case "day_off":
				w.worker.must("PUT", base+"/overrides/2026-06-02", map[string]any{"periods": []any{}}, 200)
			case "block":
				w.worker.must("POST", base+"/blocks", map[string]any{"starts_at": tc.start, "ends_at": tc.start.Add(tc.duration)}, 200)
			case "notice":
				e.clock.Set(tc.start.Add(-30 * time.Minute))
			case "buffer":
				w.worker.must("POST", base+"/appointments", managedBody(customerID, tc.start.Add(-30*time.Minute), tc.start), 200)
			case "paused":
				w.worker.must("PUT", base+"/status", map[string]any{"status": "paused"}, 200)
			}
			body := managedBody(customerID, tc.start, tc.start.Add(tc.duration))
			// A customer cannot turn their request into a privileged booking.
			customerBody := map[string]any{"id": newUUID(), "starts_at": body["starts_at"], "ends_at": body["ends_at"]}
			if r := cust.do("POST", w.cust("/appointments"), customerBody); r.code() != tc.code {
				t.Fatalf("customer: want %s, got %d %s", tc.code, r.Status, r.Raw)
			}
			for _, manager := range []*client{w.worker, w.admin} {
				body["id"] = newUUID()
				r := manager.must("POST", base+"/appointments", body, 200)
				if r.Body["starts_at"] != body["starts_at"] || r.Body["ends_at"] != body["ends_at"] {
					t.Fatalf("changed manual time: %s", r.Raw)
				}
				manager.must("POST", base+"/appointments/"+str(r.Body, "id")+"/status", map[string]any{"status": "cancelled"}, 200)
			}
		})
	}
}

func TestManagedBookingOccupancyAndReplay(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	_, customerID, _ := e.newCustomer(w, "Alice")
	base := "/api/v1/instances/" + w.instanceID
	body := managedBody(customerID, slot10.Add(10*time.Minute), slot10.Add(50*time.Minute))
	w.worker.must("POST", base+"/appointments", body, 200)
	for _, candidate := range [][2]time.Time{
		{slot10, slot10.Add(20 * time.Minute)},
		{slot10.Add(20 * time.Minute), slot10.Add(30 * time.Minute)},
		{slot10, slot10.Add(time.Hour)},
		{slot10.Add(40 * time.Minute), slot10.Add(time.Hour)},
	} {
		if r := w.worker.do("POST", base+"/appointments", managedBody(customerID, candidate[0], candidate[1])); r.code() != "slot_taken" {
			t.Fatalf("overlap accepted: %s", r.Raw)
		}
	}
	w.worker.must("POST", base+"/appointments", managedBody(customerID, slot10, slot10.Add(10*time.Minute)), 200)
	w.worker.must("POST", base+"/appointments", managedBody(customerID, slot10.Add(50*time.Minute), slot10.Add(time.Hour)), 200)
	if r := w.worker.do("POST", base+"/appointments", managedBody(customerID, slot10, slot10)); r.code() != "invalid_interval" {
		t.Fatalf("zero duration: %s", r.Raw)
	}
	// Replays stay valid after the start passes and after cancellation.
	e.clock.Set(slot10.Add(2 * time.Hour))
	w.worker.must("POST", base+"/appointments", body, 200)
	if r := w.worker.do("POST", base+"/appointments", managedBody(customerID, slot10.Add(time.Hour), slot10.Add(90*time.Minute))); r.code() != "appointment_in_past" {
		t.Fatalf("past booking: %s", r.Raw)
	}
	w.worker.must("POST", base+"/appointments/"+body["id"].(string)+"/status", map[string]any{"status": "cancelled"}, 200)
	if r := w.worker.must("POST", base+"/appointments", body, 200); str(r.Body, "status") != "cancelled" {
		t.Fatalf("replay recreated booking: %s", r.Raw)
	}
}

func TestManagedCrossDayBookingPreservesCustomerGrid(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	cust, customerID, _ := e.newCustomer(w, "Alice")
	base := "/api/v1/instances/" + w.instanceID
	// June 1 23:10 through June 2 09:50 London.
	body := managedBody(customerID, slot10.Add(-10*time.Hour-50*time.Minute), slot10.Add(-10*time.Minute))
	w.worker.must("POST", base+"/appointments", body, 200)
	checkFirst := func(want time.Time) {
		t.Helper()
		r := cust.get(w.cust("/availability?date=2026-06-02"))
		slots := list(r.Body, "items")
		if len(slots) == 0 || slots[0].(map[string]any)["starts_at"] != want.Format(time.RFC3339) {
			t.Fatalf("grid: %s", r.Raw)
		}
	}
	checkFirst(slot10)
	policy := w.worker.get(base + "/policy").Body
	policy["buffer_minutes"] = 15
	w.worker.must("PUT", base+"/policy", policy, 200)
	checkFirst(slot10.Add(30 * time.Minute))
	// Manual occupancy contributes to the customer's future quota.
	policy["max_future_bookings"] = 1
	w.worker.must("PUT", base+"/policy", policy, 200)
	if r := cust.get(w.cust("/availability?date=2026-06-02")); len(list(r.Body, "items")) != 0 || str(r.Body, "empty_reason") != "customer_limit" {
		t.Fatalf("manual quota: %s", r.Raw)
	}
	e.clock.Set(slot10.Add(-time.Hour))
	if r := w.worker.get(base + "/appointments?view=today"); len(list(r.Body, "items")) != 1 {
		t.Fatalf("cross-day today: %s", r.Raw)
	}
	if r := w.worker.get(base + "/dashboard"); len(list(r.Body, "today")) != 1 || r.Body["remaining_today"] != float64(1) {
		t.Fatalf("cross-day dashboard: %s", r.Raw)
	}
	policy["max_future_bookings"] = nil
	w.worker.must("PUT", base+"/policy", policy, 200)
	w.worker.must("POST", base+"/appointments/"+body["id"].(string)+"/status", map[string]any{"status": "cancelled"}, 200)
	checkFirst(slot10.Add(-time.Hour))
}

func TestManagedRescheduleWhilePaused(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	cust, customerID, _ := e.newCustomer(w, "Alice")
	base := "/api/v1/instances/" + w.instanceID
	id := newUUID()
	cust.must("POST", w.cust("/appointments"), slotBody(id, slot10), 200)
	w.worker.must("PUT", base+"/status", map[string]any{"status": "paused"}, 200)
	policy := w.worker.get(base + "/policy").Body
	for _, key := range []string{"daily_customer_limit", "per_customer_daily_limit", "per_customer_weekly_limit", "max_future_bookings"} {
		policy[key] = 0
	}
	policy["buffer_minutes"] = 60
	policy["min_booking_notice_minutes"] = 1440
	w.worker.must("PUT", base+"/policy", policy, 200)
	// Off-grid, nonstandard duration, beyond the horizon, spanning two days.
	start := slot10.AddDate(1, 0, 0).Add(7 * time.Minute)
	end := start.Add(26*time.Hour + 13*time.Minute)
	w.worker.must("POST", base+"/blocks", map[string]any{"starts_at": start, "ends_at": end}, 200)
	w.worker.must("PUT", base+"/schedule", map[string]any{"cycle_length_days": 1, "anchor_date": "2026-01-01", "days": [][]any{{}}}, 200)
	w.worker.must("POST", base+"/appointments", managedBody(customerID, end, end.Add(time.Hour)), 200)
	request := map[string]any{"expected_starts_at": slot10, "expected_ends_at": slot10.Add(30 * time.Minute), "starts_at": start, "ends_at": end}
	endpoint := base + "/appointments/" + id + "/reschedule"
	w.worker.must("POST", endpoint, request, 200)
	w.admin.must("POST", endpoint, request, 200)
	for _, manager := range []*client{w.worker, w.admin} {
		if r := manager.do("POST", endpoint, map[string]any{"expected_starts_at": start, "expected_ends_at": end, "starts_at": e.clock.Now().Add(-time.Minute), "ends_at": e.clock.Now().Add(time.Minute)}); r.code() != "appointment_in_past" {
			t.Fatalf("past reschedule: %s", r.Raw)
		}
	}
	r := cust.get(w.cust("/appointments/" + id))
	if str(r.Body, "starts_at") != start.Format(time.RFC3339) {
		t.Fatalf("failed reschedule changed original: %s", r.Raw)
	}
}

func TestRescheduleDetectsEndOnlyChanges(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	cust, customerID, _ := e.newCustomer(w, "Alice")
	base := "/api/v1/instances/" + w.instanceID + "/appointments"
	created := w.worker.must("POST", base, managedBody(customerID, slot10, slot10.Add(30*time.Minute)), 200)
	id := str(created.Body, "id")
	endpoint := base + "/" + id + "/reschedule"
	extend := map[string]any{
		"expected_starts_at": slot10, "expected_ends_at": slot10.Add(30 * time.Minute),
		"starts_at": slot10, "ends_at": slot10.Add(time.Hour),
	}
	w.worker.must("POST", endpoint, extend, 200)
	// A retry of the successful extension remains idempotent.
	w.worker.must("POST", endpoint, extend, 200)

	// Another manager's draft still refers to the original 30-minute slot.
	for _, start := range []time.Time{slot10, slot10.Add(2 * time.Hour)} {
		stale := map[string]any{
			"expected_starts_at": slot10, "expected_ends_at": slot10.Add(30 * time.Minute),
			"starts_at": start, "ends_at": start.Add(45 * time.Minute),
		}
		r := w.admin.must("POST", endpoint, stale, 409)
		if r.code() != "appointment_changed" {
			t.Fatalf("stale duration accepted: %s", r.Raw)
		}
	}
	// Omitting the end precondition cannot bypass concurrency protection.
	w.admin.must("POST", endpoint, map[string]any{
		"expected_starts_at": slot10, "starts_at": slot10, "ends_at": slot10.Add(45 * time.Minute),
	}, 422)
	current := cust.get(w.cust("/appointments/" + id))
	if str(current.Body, "starts_at") != slot10.Format(time.RFC3339) || str(current.Body, "ends_at") != slot10.Add(time.Hour).Format(time.RFC3339) {
		t.Fatalf("stale draft changed appointment: %s", current.Raw)
	}
	// Reopening the appointment supplies the current interval and permits editing.
	w.admin.must("POST", endpoint, map[string]any{
		"expected_starts_at": slot10, "expected_ends_at": slot10.Add(time.Hour),
		"starts_at": slot10, "ends_at": slot10.Add(45 * time.Minute),
	}, 200)
}

func TestConfigurationChangesPreserveAppointments(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	cust, _, _ := e.newCustomer(w, "Alice")
	base := "/api/v1/instances/" + w.instanceID
	ids := []string{newUUID(), newUUID()}
	for i, id := range ids {
		cust.must("POST", w.cust("/appointments"), slotBody(id, slot10.Add(time.Duration(i)*30*time.Minute)), 200)
	}
	cycle := map[string]any{"cycle_length_days": 1, "anchor_date": "2026-01-01", "days": [][]map[string]int{{{"start": 660, "end": 720}}}}
	preview := w.worker.must("POST", base+"/schedule/preview", cycle, 200)
	if _, ok := preview.Body["conflicts"]; ok {
		t.Fatalf("obsolete conflict field: %s", preview.Raw)
	}
	w.worker.must("PUT", base+"/schedule", cycle, 200)
	if r := cust.do("POST", w.cust("/appointments"), slotBody(newUUID(), slot10.Add(-time.Hour))); r.code() != "slot_unavailable" {
		t.Fatalf("old schedule still accepted: %s", r.Raw)
	}
	w.worker.must("PUT", base+"/overrides/2026-06-02", map[string]any{"periods": []any{}}, 200)
	w.worker.must("PUT", base+"/overrides/2026-06-02", map[string]any{"periods": []map[string]int{{"start": 540, "end": 720}}}, 200)
	w.worker.must("DELETE", base+"/overrides/2026-06-02", nil, 204)
	for range 2 {
		w.worker.must("POST", base+"/blocks", map[string]any{"starts_at": slot10, "ends_at": slot10.Add(2 * time.Hour)}, 200)
	}
	policy := w.worker.get(base + "/policy").Body
	policy["buffer_minutes"] = 60
	policy["service_duration_minutes"] = 45
	saved := w.worker.must("PUT", base+"/policy", policy, 200)
	if _, ok := saved.Body["buffer_gaps"]; ok {
		t.Fatalf("obsolete buffer warnings: %s", saved.Raw)
	}
	preview = w.worker.must("POST", base+"/policy/preview", policy, 200)
	if _, ok := preview.Body["buffer_gaps"]; ok {
		t.Fatalf("obsolete preview warnings: %s", preview.Raw)
	}
	for i, id := range ids {
		r := cust.get(w.cust("/appointments/" + id))
		original := slotBody(id, slot10.Add(time.Duration(i)*30*time.Minute))
		if r.Body["starts_at"] != original["starts_at"] || r.Body["ends_at"] != original["ends_at"] || str(r.Body, "status") != "booked" {
			t.Fatalf("configuration changed booking: %s", r.Raw)
		}
	}
	if r := cust.do("POST", w.cust("/appointments"), slotBody(newUUID(), slot10.AddDate(0, 0, 1))); r.code() != "duration_changed" {
		t.Fatalf("new policy not enforced: %s", r.Raw)
	}
}

func TestConcurrentCustomerAndManagedBooking(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	cust, customerID, _ := e.newCustomer(w, "Alice")
	base := "/api/v1/instances/" + w.instanceID
	ready := make(chan struct{})
	results := make(chan result, 2)
	go func() { <-ready; results <- cust.do("POST", w.cust("/appointments"), slotBody(newUUID(), slot10)) }()
	go func() {
		<-ready
		results <- w.worker.do("POST", base+"/appointments", managedBody(customerID, slot10.Add(5*time.Minute), slot10.Add(25*time.Minute)))
	}()
	close(ready)
	wins := 0
	for range 2 {
		r := <-results
		if r.Status == 200 {
			wins++
		} else if r.code() != "slot_taken" {
			t.Fatalf("unexpected concurrent result: %s", r.Raw)
		}
	}
	if wins != 1 {
		t.Fatalf("want one winner, got %d", wins)
	}
}
