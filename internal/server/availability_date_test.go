package server

import (
	"net/url"
	"testing"
	"time"
)

func TestAvailabilityDateHTTP(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("dates", "date-worker")
	customer, customerID, _ := e.newCustomer(w, "Date customer")
	adminBase := "/api/v1/instances/" + w.instanceID
	endpoints := []struct {
		c    *client
		path string
	}{
		{customer, w.cust("/availability")},
		{w.worker, adminBase + "/availability?customer_id=" + customerID},
	}
	for _, endpoint := range endpoints {
		u, _ := url.Parse(endpoint.path)
		request := func(date *string, cursor *string) result {
			q := u.Query()
			q.Set("limit", "2")
			if date != nil {
				q.Set("date", *date)
			}
			if cursor != nil {
				q.Set("cursor", *cursor)
			}
			return endpoint.c.do("GET", u.Path+"?"+q.Encode(), nil)
		}
		nearest := request(nil, nil)
		if nearest.Status != 200 || str(nearest.Body, "date_range", "from") != "2026-06-01" || str(nearest.Body, "date_range", "to") != "2026-06-15" {
			t.Fatalf("range: %s", nearest.Raw)
		}
		if first := list(nearest.Body, "items")[0].(map[string]any)["starts_at"]; first != "2026-06-01T08:00:00Z" {
			t.Fatalf("nearest compatibility: %v", first)
		}
		date := "2026-06-02"
		var cursor *string
		seen := map[string]bool{}
		for {
			response := request(&date, cursor)
			if response.Status != 200 {
				t.Fatalf("date: %s", response.Raw)
			}
			for _, raw := range list(response.Body, "items") {
				start := str(raw.(map[string]any), "starts_at")
				if seen[start] {
					t.Fatalf("duplicate %s", start)
				}
				seen[start] = true
				instant, _ := time.Parse(time.RFC3339, start)
				london, _ := time.LoadLocation("Europe/London")
				if instant.In(london).Format("2006-01-02") != date {
					t.Fatalf("pagination spilled: %s", start)
				}
			}
			if response.Body["next_cursor"] == nil {
				break
			}
			next := str(response.Body, "next_cursor")
			cursor = &next
		}
		if len(seen) != 6 {
			t.Fatalf("got %d slots", len(seen))
		}
		for _, bad := range []string{"", "2026-02-30", "bad", "2026-05-31", "2026-06-16"} {
			response := request(&bad, nil)
			if response.Status != 422 || response.code() != "invalid_date" {
				t.Fatalf("invalid date %q: %s", bad, response.Raw)
			}
		}
	}
	// A local day with no working time is not a broken calendar.
	w.worker.must("PUT", adminBase+"/overrides/2026-06-02", map[string]any{"periods": []any{}}, 200)
	for _, endpoint := range []struct {
		c    *client
		path string
	}{{customer, w.cust("/availability?date=2026-06-02")}, {w.worker, adminBase + "/availability?date=2026-06-02"}} {
		response := endpoint.c.get(endpoint.path)
		if str(response.Body, "empty_reason") != "no_slots_on_date" || len(list(response.Body, "items")) != 0 {
			t.Fatalf("rest day: %s", response.Raw)
		}
	}
	// Midnight is instance-local, even though UTC is still on the previous date.
	e.clock.Set(time.Date(2026, 6, 1, 23, 30, 0, 0, time.UTC))
	response := customer.get(w.cust("/availability"))
	if str(response.Body, "date_range", "from") != "2026-06-02" {
		t.Fatalf("local date: %s", response.Raw)
	}
	w.worker.must("PUT", adminBase+"/status", map[string]any{"status": "paused"}, 200)
	response = customer.get(w.cust("/availability?date=2026-06-02"))
	if str(response.Body, "empty_reason") != "paused" || str(response.Body, "date_range", "from") != "2026-06-02" {
		t.Fatalf("paused range: %s", response.Raw)
	}
}
