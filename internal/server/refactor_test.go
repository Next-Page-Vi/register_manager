package server

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"register_manager/internal/booking"
	"register_manager/internal/storage"
)

func TestNormalizedTimezoneAndPolicyStorage(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	settings := map[string]any{"name": "A", "timezone": "  Asia/Shanghai  ", "default_locale": "zh-Hans", "contact_kind": "phone", "contact_required": false, "brand_color": nil}
	if got := w.worker.must("PUT", "/api/v1/instances/"+w.instanceID, settings, 200); str(got.Body, "timezone") != "Asia/Shanghai" {
		t.Fatalf("timezone: %s", got.Raw)
	}
	created := w.admin.must("POST", "/api/v1/platform/instances", map[string]any{"name": "B", "slug": "beta", "timezone": " Asia/Tokyo ", "default_locale": "ja", "contact_kind": "text", "contact_required": false}, 200)
	if str(created.Body, "timezone") != "Asia/Tokyo" {
		t.Fatalf("create timezone: %s", created.Raw)
	}
	w.worker.get("/api/v1/instances/" + w.instanceID + "/availability")
	p := booking.DefaultPolicy()
	p.DailyCustomerLimit, p.PerCustomerDailyLimit, p.PerCustomerWeeklyLimit, p.MaxFutureBookings = new(4), new(2), new(7), new(9)
	err := e.db.WriteTx(context.Background(), func(tx bun.Tx) error {
		return booking.StorePolicy(context.Background(), tx, w.instanceID, p, e.clock.Now().UnixMilli())
	})
	if err != nil {
		t.Fatal(err)
	}
	row, err := storage.GetPolicy(context.Background(), e.db.Read(), w.instanceID)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(p)
	got, _ := json.Marshal(booking.PolicyFromRow(row))
	if string(got) != string(want) {
		t.Fatalf("mapping lost a field: %s != %s", got, want)
	}
	for _, field := range []string{"daily_customer_limit", "per_customer_daily_limit", "per_customer_weekly_limit", "max_future_bookings", "min_booking_notice_minutes", "min_cancellation_notice_minutes"} {
		if _, err := e.db.W.Exec("UPDATE booking_policies SET "+field+" = 525601 WHERE instance_id = ?", w.instanceID); err == nil {
			t.Fatalf("database accepted invalid %s", field)
		}
	}
}

func TestCursorAndManagedAppointmentContracts(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	_, customerID, _ := e.newCustomer(w, "Alice")
	base := "/api/v1/instances/" + w.instanceID
	for _, endpoint := range []string{base + "/availability", base + "/appointments", base + "/audit", "/api/v1/platform/audit"} {
		for _, query := range []string{"limit=0", "limit=201", "cursor="} {
			w.admin.must("GET", endpoint+"?"+query, nil, 422)
		}
		first := w.admin.get(endpoint + "?limit=1")
		if _, ok := first.Body["items"]; !ok {
			t.Fatalf("missing items: %s", first.Raw)
		}
		if _, ok := first.Body["next_cursor"]; !ok {
			t.Fatalf("missing cursor: %s", first.Raw)
		}
		if cursor, ok := first.Body["next_cursor"].(string); ok {
			second := w.admin.get(endpoint + "?limit=1&cursor=" + url.QueryEscape(cursor))
			if string(second.Raw) == string(first.Raw) {
				t.Fatal("cursor did not advance")
			}
		}
	}
	input := slotBody(newUUID(), slot10)
	input["customer_id"] = customerID
	created := w.worker.must("POST", base+"/appointments", input, 200)
	id := str(created.Body, "id")
	for _, response := range []result{created,
		w.worker.must("POST", base+"/appointments", input, 200),
		w.worker.must("POST", base+"/appointments/"+id+"/status", map[string]any{"status": "cancelled"}, 200),
		w.worker.must("POST", base+"/appointments/"+id+"/status", map[string]any{"status": "cancelled"}, 200),
		w.worker.must("POST", base+"/appointments", input, 200),
	} {
		if str(response.Body, "customer_name") != "Alice" {
			t.Fatalf("incomplete managed appointment: %s", response.Raw)
		}
	}
	notePath := base + "/customers/" + customerID + "/note"
	w.worker.must("PUT", notePath, map[string]any{"note": "keep me"}, 200)
	if response := w.worker.must("PUT", notePath, map[string]any{"note": nil}, 200); response.Body["internal_note"] != nil {
		t.Fatalf("note not cleared: %s", response.Raw)
	}
}

func TestFailedEntryAuditCanBeRetried(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	if _, err := e.db.W.Exec(`CREATE TRIGGER fail_entry BEFORE INSERT ON audit_log WHEN NEW.action = 'instance.super_admin_enter' BEGIN SELECT RAISE(ABORT, 'test audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	w.admin.must("GET", "/api/v1/instances/by-slug/alpha", nil, 500)
	if _, err := e.db.W.Exec(`DROP TRIGGER fail_entry`); err != nil {
		t.Fatal(err)
	}
	w.admin.get("/api/v1/instances/by-slug/alpha")
	w.admin.get("/api/v1/instances/by-slug/alpha")
	var count int
	if err := e.db.R.QueryRow(`SELECT count(*) FROM audit_log WHERE action = 'instance.super_admin_enter'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("entry audit count %d", count)
	}
	// Signing in starts a new account session, including its entry-audit scope.
	w.admin.must("POST", "/api/v1/account/login", map[string]any{"login": "root", "password": "root-password"}, 200)
	w.admin.get("/api/v1/instances/by-slug/alpha")
	if err := e.db.R.QueryRow(`SELECT count(*) FROM audit_log WHERE action = 'instance.super_admin_enter'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("new session did not record entry: %d", count)
	}
}

func TestOptionalValuesAreExplicit(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	customer, id, _ := e.newCustomer(w, "Alice")
	base := "/api/v1/instances/" + w.instanceID
	for _, endpoint := range []string{base + "/customers/" + id + "/note", "/api/v1/account/locale"} {
		field := "locale"
		if strings.HasSuffix(endpoint, "/note") {
			field = "note"
		}
		w.worker.must("PUT", endpoint, map[string]any{}, 422)
		w.worker.must("PUT", endpoint, map[string]any{field: ""}, 422)
		code := 204
		if field == "note" {
			code = 200
		}
		w.worker.must("PUT", endpoint, map[string]any{field: nil}, code)
	}
	customer.must("PUT", w.cust("/locale"), map[string]any{"locale": nil}, 204)
	for _, params := range []string{"status=", "status=unknown"} {
		w.worker.must("GET", base+"/customers?"+params, nil, 422)
	}
	for _, params := range []string{"customer_id=", "exclude_appointment_id=", "limit=", "cursor="} {
		w.worker.must("GET", base+"/availability?"+params, nil, 422)
	}
	for _, endpoint := range []string{base + "/availability", base + "/appointments?view=upcoming", base + "/audit"} {
		separator := "?"
		if strings.Contains(endpoint, "?") {
			separator = "&"
		}
		w.worker.must("GET", endpoint+separator+"cursor=invalid", nil, 422)
	}
}

func TestAllAppointmentViewsHonorCursorLimit(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	customer, _, _ := e.newCustomer(w, "Alice")
	for _, start := range []time.Time{slot10, slot10.Add(time.Hour)} {
		customer.must("POST", w.cust("/appointments"), slotBody(newUUID(), start), 200)
	}
	e.clock.Set(slot10.Add(-time.Hour))
	base := "/api/v1/instances/" + w.instanceID + "/appointments?limit=1&view="
	for _, view := range []string{"today", "upcoming", "history"} {
		if view == "history" {
			e.clock.Set(slot10.Add(2 * time.Hour))
		}
		first := w.worker.get(base + view)
		items := first.Body["items"].([]any)
		if len(items) != 1 || first.Body["next_cursor"] == nil {
			t.Fatalf("first %s page: %s", view, first.Raw)
		}
		second := w.worker.get(base + view + "&cursor=" + url.QueryEscape(first.Body["next_cursor"].(string)))
		next := second.Body["items"].([]any)
		if len(next) != 1 || second.Body["next_cursor"] != nil || items[0].(map[string]any)["id"] == next[0].(map[string]any)["id"] {
			t.Fatalf("second %s page: %s", view, second.Raw)
		}
	}
}
