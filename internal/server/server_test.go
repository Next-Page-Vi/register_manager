package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"register_manager/internal/accounts"
	"register_manager/internal/storage"
)

func newUUID() string { return uuid.NewString() }

func slotBody(id string, start time.Time) map[string]any {
	return map[string]any{
		"id": id, "starts_at": start.Format(time.RFC3339), "ends_at": start.Add(30 * time.Minute).Format(time.RFC3339),
	}
}

// June 2 2026 10:00 London (BST, UTC+1).
var slot10 = time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)

func TestEndToEndBookingFlow(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	cust, custID, _ := e.newCustomer(w, "Alice")

	me := cust.get(w.cust("/me"))
	if str(me.Body, "customer", "id") != custID || str(me.Body, "customer", "instance", "slug") != "alpha" {
		t.Fatalf("customer me: %s", me.Raw)
	}

	av := cust.get(w.cust("/availability?limit=3"))
	slots := list(av.Body, "items")
	if len(slots) != 3 {
		t.Fatalf("availability: %s", av.Raw)
	}
	// Now is 09:00 London on June 1; the first slot is 09:00 the same day.
	if first := slots[0].(map[string]any)["starts_at"]; first != "2026-06-01T08:00:00Z" {
		t.Errorf("first slot %v", first)
	}

	apptID := newUUID()
	r := cust.must("POST", w.cust("/appointments"), slotBody(apptID, slot10), 200)
	if str(r.Body, "status") != "booked" {
		t.Fatalf("book: %s", r.Raw)
	}
	// Network retry with the same ID returns the same appointment.
	r2 := cust.must("POST", w.cust("/appointments"), slotBody(apptID, slot10), 200)
	if str(r2.Body, "id") != apptID {
		t.Fatalf("replay: %s", r2.Raw)
	}
	// Same ID, different content is refused.
	if r := cust.do("POST", w.cust("/appointments"), slotBody(apptID, slot10.Add(time.Hour))); r.Status != 409 || r.code() != "appointment_id_conflict" {
		t.Fatalf("same id different content: %d %s", r.Status, r.Raw)
	}
	// The slot is no longer offered.
	for _, s := range list(cust.get(w.cust("/availability?limit=50")).Body, "items") {
		if s.(map[string]any)["starts_at"] == slot10.Format(time.RFC3339) {
			t.Fatal("booked slot still offered")
		}
	}

	mine := cust.get(w.cust("/appointments"))
	if len(list(mine.Body, "upcoming")) != 1 {
		t.Fatalf("my appointments: %s", mine.Raw)
	}

	cust.must("POST", w.cust("/appointments/")+apptID+"/cancel", nil, 200)
	cust.must("POST", w.cust("/appointments/")+apptID+"/cancel", nil, 200) // idempotent
	// Replaying the original create after cancellation does not rebook.
	r3 := cust.must("POST", w.cust("/appointments"), slotBody(apptID, slot10), 200)
	if str(r3.Body, "status") != "cancelled" {
		t.Fatalf("replay after cancel: %s", r3.Raw)
	}
	// The worker sees the cancelled appointment in upcoming.
	up := w.worker.get("/api/v1/instances/" + w.instanceID + "/appointments?view=upcoming")
	if len(list(up.Body, "items")) != 1 {
		t.Fatalf("upcoming: %s", up.Raw)
	}
	// Audit contains the booking.
	audit := w.worker.get("/api/v1/instances/" + w.instanceID + "/audit")
	if !strings.Contains(string(audit.Raw), "appointment.create") {
		t.Errorf("audit missing create: %s", audit.Raw)
	}
}

func TestConcurrentBookingSameSlot(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	const n = 8
	clients := make([]*client, n)
	for i := range clients {
		clients[i], _, _ = e.newCustomer(w, fmt.Sprintf("Customer %d", i))
	}
	var wg sync.WaitGroup
	statuses := make([]int, n)
	codes := make([]string, n)
	for i := range clients {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := clients[i].do("POST", w.cust("/appointments"), slotBody(newUUID(), slot10))
			statuses[i], codes[i] = r.Status, r.code()
		}(i)
	}
	wg.Wait()
	ok := 0
	for i, s := range statuses {
		switch {
		case s == 200:
			ok++
		case s == 409 && codes[i] == "slot_taken":
		default:
			t.Errorf("client %d: %d %s", i, s, codes[i])
		}
	}
	if ok != 1 {
		t.Fatalf("%d bookings succeeded for one slot", ok)
	}
}

func TestConcurrentQuota(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	w.worker.must("PUT", "/api/v1/instances/"+w.instanceID+"/policy", map[string]any{
		"daily_customer_limit": nil, "per_customer_weekly_limit": nil, "max_future_bookings": nil,
		"slot_step_minutes": 30, "service_duration_minutes": 30, "buffer_minutes": 0,
		"booking_horizon_days": 14, "min_booking_notice_minutes": 0, "min_cancellation_notice_minutes": 0,
		"per_customer_daily_limit": 1,
	}, 200)
	cust, _, _ := e.newCustomer(w, "Alice")
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := range 5 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := cust.do("POST", w.cust("/appointments"), slotBody(newUUID(), slot10.Add(time.Duration(i)*30*time.Minute)))
			if r.Status == 200 {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("daily limit 1 but %d bookings succeeded", ok)
	}
}

func TestInstanceIsolation(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	a := e.setupInstance("alpha", "worker-a")
	b := e.setupInstance("beta", "worker-b")
	custA, custAID, _ := e.newCustomer(a, "Alice")
	_, custBID, _ := e.newCustomer(b, "Bob")

	// Worker A cannot see or change instance B.
	for _, p := range []string{"", "/customers", "/appointments", "/schedule", "/policy", "/invite", "/audit", "/customers/" + custBID} {
		if r := a.worker.do("GET", "/api/v1/instances/"+b.instanceID+p, nil); r.Status != 404 {
			t.Errorf("GET B%s as worker A: %d", p, r.Status)
		}
	}
	if r := a.worker.do("POST", "/api/v1/instances/"+b.instanceID+"/customers/"+custBID+"/actions/disable", nil); r.Status != 404 {
		t.Errorf("disable B customer as A: %d", r.Status)
	}
	// Using A's instance path with B's customer ID is not found either.
	if r := a.worker.do("GET", "/api/v1/instances/"+a.instanceID+"/customers/"+custBID, nil); r.Status != 404 {
		t.Errorf("B customer under A path: %d", r.Status)
	}
	// Admin booking in A for B's customer fails.
	if r := a.worker.do("POST", "/api/v1/instances/"+a.instanceID+"/appointments", map[string]any{
		"id": newUUID(), "customer_id": custBID, "starts_at": slot10.Format(time.RFC3339), "ends_at": slot10.Add(30 * time.Minute).Format(time.RFC3339),
	}); r.Status != 404 {
		t.Errorf("cross-instance admin booking: %d %s", r.Status, r.Raw)
	}
	// Same time in different instances both succeed.
	custA.must("POST", a.cust("/appointments"), slotBody(newUUID(), slot10), 200)

	// A's customer session is not a customer of B.
	if me := custA.get(b.cust("/me")); me.Body["customer"] != nil {
		t.Errorf("A's customer is signed in to B: %s", me.Raw)
	}
	if r := custA.do("GET", b.cust("/appointments"), nil); r.Status != 401 {
		t.Errorf("A's customer reads B's appointments: %d", r.Status)
	}
	b.worker.must("POST", "/api/v1/instances/"+b.instanceID+"/appointments", map[string]any{
		"id": newUUID(), "customer_id": custBID, "starts_at": slot10.Format(time.RFC3339), "ends_at": slot10.Add(30 * time.Minute).Format(time.RFC3339),
	}, 200)

	// The composite foreign key rejects a cross-instance reference directly.
	err := e.db.WriteTx(context.Background(), func(tx bun.Tx) error {
		_, err := tx.Tx.Exec(`INSERT INTO appointments (id, instance_id, customer_id, starts_at, ends_at, status, created_at, updated_at)
			VALUES (?, ?, ?, 1, 2, 'booked', 0, 0)`, newUUID(), a.instanceID, custBID)
		return err
	})
	if !errors.Is(err, storage.ErrForeignKey) {
		t.Errorf("cross-instance FK: %v", err)
	}
	_ = custAID
}

func TestTriggerRejectsOverlapWhenBusinessChecksAreBypassed(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	_, custID, _ := e.newCustomer(w, "Alice")
	insert := func(start, end int64) error {
		return e.db.WriteTx(context.Background(), func(tx bun.Tx) error {
			_, err := tx.Tx.Exec(`INSERT INTO appointments (id, instance_id, customer_id, starts_at, ends_at, status, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, 'booked', 0, 0)`, newUUID(), w.instanceID, custID, start, end)
			return err
		})
	}
	if err := insert(1000, 2000); err != nil {
		t.Fatal(err)
	}
	if err := insert(1500, 2500); !errors.Is(err, storage.ErrOverlap) {
		t.Errorf("overlap: %v", err)
	}
	if err := insert(2000, 3000); err != nil {
		t.Errorf("adjacent should be allowed: %v", err)
	}
	// Cancelled appointments do not occupy time.
	err := e.db.WriteTx(context.Background(), func(tx bun.Tx) error {
		_, err := tx.Tx.Exec(`UPDATE appointments SET status = 'cancelled' WHERE starts_at = 1000`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := insert(900, 1900); err != nil {
		t.Errorf("after cancel: %v", err)
	}
}

func TestAccountSecurity(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	root := e.login("root", "root-password")
	acct := root.must("POST", "/api/v1/platform/accounts", map[string]any{"login": "bob", "name": "Bob", "password": "bob-password"}, 200)
	bobID := str(acct.Body, "id")
	bob := e.login("bob", "bob-password")

	// An account without a role sees no instances and cannot manage accounts.
	me := bob.get("/api/v1/account/me")
	if len(me.Body["account"].(map[string]any)["instances"].([]any)) != 0 {
		t.Errorf("unassigned account sees instances")
	}
	if r := bob.do("GET", "/api/v1/platform/accounts", nil); r.Status != 403 {
		t.Errorf("non-admin lists accounts: %d", r.Status)
	}
	if r := bob.do("POST", "/api/v1/platform/accounts", map[string]any{"login": "x", "name": "x", "password": "xxxxxxxx"}); r.Status != 403 {
		t.Errorf("non-admin creates account: %d", r.Status)
	}

	// Password reset ends existing sessions.
	root.must("POST", "/api/v1/platform/accounts/"+bobID+"/password", map[string]any{"password": "new-password"}, 204)
	if me := bob.get("/api/v1/account/me"); me.Body["account"] != nil {
		t.Error("session survived password reset")
	}
	if r := e.client().do("POST", "/api/v1/account/login", map[string]any{"login": "bob", "password": "bob-password"}); r.Status != 401 {
		t.Errorf("old password: %d", r.Status)
	}
	bob = e.login("bob", "new-password")

	// Disabling ends sessions and blocks login.
	root.must("PUT", "/api/v1/platform/accounts/"+bobID+"/disabled", map[string]any{"disabled": true}, 204)
	if me := bob.get("/api/v1/account/me"); me.Body["account"] != nil {
		t.Error("session survived disable")
	}
	if r := e.client().do("POST", "/api/v1/account/login", map[string]any{"login": "bob", "password": "new-password"}); r.Status != 401 {
		t.Errorf("disabled login: %d", r.Status)
	}

	// The super admin is not managed through account management: it cannot
	// be edited, disabled or have its password reset there, and there is no
	// way to make another super admin.
	rootID := str(root.get("/api/v1/account/me").Body, "account", "id")
	for _, r := range []result{
		root.do("PUT", "/api/v1/platform/accounts/"+rootID+"/disabled", map[string]any{"disabled": true}),
		root.do("PUT", "/api/v1/platform/accounts/"+rootID, map[string]any{"login": "x", "name": "x"}),
		root.do("POST", "/api/v1/platform/accounts/"+rootID+"/password", map[string]any{"password": "another-password"}),
	} {
		if r.code() != "super_admin_protected" {
			t.Errorf("managing the super admin: %d %s", r.Status, r.Raw)
		}
	}
	if r := root.do("PUT", "/api/v1/platform/accounts/"+bobID+"/super-admin", map[string]any{"enabled": true}); r.Status != 404 && r.Status != 405 {
		t.Errorf("super admin grant endpoint exists: %d", r.Status)
	}
	// The database refuses a second super admin and a disabled one.
	if _, err := e.db.W.Exec(`UPDATE accounts SET super_admin = 1 WHERE id = ?`, bobID); err == nil {
		t.Error("second super admin accepted")
	}
	if _, err := e.db.W.Exec(`UPDATE accounts SET disabled_at = 1 WHERE id = ?`, rootID); err == nil {
		t.Error("disabled super admin accepted")
	}
}

func TestCLIRecoversLockedOutAdmin(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	old := e.login("root", "root-password")
	// The CLI needs no login name and tells it, even after a rename.
	login, err := e.deps.Accounts.CLIResetPassword(context.Background(), "recovered-pw")
	if err != nil {
		t.Fatal(err)
	}
	if login != "root" {
		t.Errorf("CLI reported login %q", login)
	}
	if me := old.get("/api/v1/account/me"); me.Body["account"] != nil {
		t.Error("super admin session survived the CLI reset")
	}
	me := e.login("root", "recovered-pw").get("/api/v1/account/me")
	if str(me.Body, "account", "login") != "root" || me.Body["account"].(map[string]any)["must_change_password"] != false {
		t.Errorf("after CLI reset: %s", me.Raw)
	}
	var n int
	_ = e.db.R.QueryRow(`SELECT count(*) FROM accounts WHERE super_admin = 1`).Scan(&n)
	if n != 1 {
		t.Errorf("%d super admins", n)
	}
}

func TestConcurrentSuperAdminBootstrap(t *testing.T) {
	e := newEnv(t)
	credentials := make(chan *accounts.BootstrapCredentials, 5)
	var wg sync.WaitGroup
	for range cap(credentials) {
		wg.Go(func() {
			result, err := e.deps.Accounts.EnsureSuperAdmin(t.Context())
			if err != nil {
				t.Errorf("bootstrap: %v", err)
			}
			credentials <- result
		})
	}
	wg.Wait()
	close(credentials)
	var created *accounts.BootstrapCredentials
	for result := range credentials {
		if result != nil {
			if created != nil {
				t.Fatal("more than one caller received initial credentials")
			}
			created = result
		}
	}
	if created == nil {
		t.Fatal("no caller received initial credentials")
	}
	e.login(created.Login, created.Password)
	var creations int
	if err := e.db.R.QueryRow(`SELECT count(*) FROM audit_log WHERE action = 'account.create_super_admin'`).Scan(&creations); err != nil {
		t.Fatal(err)
	}
	if creations != 1 {
		t.Fatalf("super admin created %d times", creations)
	}
}

func TestSuperAdminBootstrap(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	credentials, err := e.deps.Accounts.EnsureSuperAdmin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if credentials == nil || credentials.Login != "admin" || len(credentials.Password) < 20 {
		t.Fatal("bootstrap did not return valid credentials")
	}
	password := credentials.Password
	// Later starts neither create another nor show a password again, also
	// when several run at once.
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if credentials, err := e.deps.Accounts.EnsureSuperAdmin(ctx); err != nil || credentials != nil {
				t.Errorf("second start: credentials returned=%v err=%v", credentials != nil, err)
			}
		}()
	}
	wg.Wait()
	var n int
	_ = e.db.R.QueryRow(`SELECT count(*) FROM accounts WHERE super_admin = 1`).Scan(&n)
	if n != 1 {
		t.Errorf("%d super admins", n)
	}

	// Until the generated password is changed, only me, the password change,
	// language and sign-out work.
	c := e.login("admin", password)
	me := c.get("/api/v1/account/me")
	if me.Body["account"].(map[string]any)["must_change_password"] != true {
		t.Fatalf("must change flag missing: %s", me.Raw)
	}
	for _, p := range []string{"/api/v1/platform/accounts", "/api/v1/platform/instances"} {
		if r := c.do("GET", p, nil); r.code() != "password_change_required" {
			t.Errorf("GET %s before password change: %d %s", p, r.Status, r.Raw)
		}
	}
	if r := c.do("PUT", "/api/v1/account/profile", map[string]any{"login": "boss", "name": "Boss"}); r.code() != "password_change_required" {
		t.Errorf("profile before password change: %d %s", r.Status, r.Raw)
	}
	c.must("PUT", "/api/v1/account/locale", map[string]any{"locale": "ja"}, 204)
	if r := c.do("POST", "/api/v1/account/password", map[string]any{"current_password": "wrong-password", "new_password": "chosen-password"}); r.code() != "current_password_invalid" {
		t.Errorf("wrong current password: %d %s", r.Status, r.Raw)
	}
	r := c.must("POST", "/api/v1/account/password", map[string]any{"current_password": password, "new_password": "chosen-password"}, 200)
	if r.Body["must_change_password"] != false {
		t.Errorf("flag kept after change: %s", r.Raw)
	}
	c.must("GET", "/api/v1/platform/accounts", nil, 200)
}

func TestMyAccount(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	// Every account, operators included, edits its own profile and password.
	for _, tc := range []struct {
		c              *client
		login, current string
	}{{w.admin, "root", "root-password"}, {w.worker, "worker1", "worker-password"}} {
		other := e.login(tc.login, tc.current)
		tc.c.must("PUT", "/api/v1/account/profile", map[string]any{"login": tc.login + "-new", "name": "New Name"}, 204)
		tc.c.must("POST", "/api/v1/account/password", map[string]any{"current_password": tc.current, "new_password": "brand-new-password"}, 200)
		// This browser stays signed in; other sessions end.
		if me := tc.c.get("/api/v1/account/me"); str(me.Body, "account", "login") != tc.login+"-new" || str(me.Body, "account", "name") != "New Name" {
			t.Errorf("%s after change: %s", tc.login, me.Raw)
		}
		if me := other.get("/api/v1/account/me"); me.Body["account"] != nil {
			t.Errorf("%s: other session survived the password change", tc.login)
		}
		e.login(tc.login+"-new", "brand-new-password")
	}
	// A login name in use is refused.
	if r := w.worker.do("PUT", "/api/v1/account/profile", map[string]any{"login": "root-new", "name": "x"}); r.code() != "login_taken" {
		t.Errorf("taken login: %d %s", r.Status, r.Raw)
	}
}

func TestCustomerCredentialLifecycle(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	cust, custID, link := e.newCustomer(w, "Alice")
	base := "/api/v1/instances/" + w.instanceID + "/customers/" + custID + "/actions/"

	// Regenerating the link ends the session and kills the old link.
	res := w.worker.must("POST", base+"regenerate_link", nil, 200)
	newLink := str(res.Body, "access_path")
	if me := cust.get(w.cust("/me")); me.Body["customer"] != nil {
		t.Error("session survived link regeneration")
	}
	fresh := e.client()
	if loc := fresh.openLink(link); !strings.Contains(loc, "link-invalid") {
		t.Errorf("old link still works: %s", loc)
	}
	if loc := fresh.openLink(newLink); loc != "/i/alpha/booking" {
		t.Errorf("new link: %s", loc)
	}

	// Disabling ends sessions; restoring issues another link.
	w.worker.must("POST", base+"disable", nil, 200)
	if me := fresh.get(w.cust("/me")); me.Body["customer"] != nil {
		t.Error("session survived disable")
	}
	if loc := e.client().openLink(newLink); !strings.Contains(loc, "link-invalid") {
		t.Error("link works after disable")
	}
	res = w.worker.must("POST", base+"restore", nil, 200)
	if l := str(res.Body, "access_path"); l == "" || l == newLink {
		t.Error("restore must issue a new link")
	}

	// A link for instance alpha does not work under another slug.
	e.setupInstance("beta", "worker2")
	restored := str(res.Body, "access_path")
	wrong := strings.Replace(restored, "/i/alpha/", "/i/beta/", 1)
	if loc := e.client().openLink(wrong); !strings.Contains(loc, "link-invalid") {
		t.Error("link accepted under another instance")
	}

	// Rotating the invite code does not affect customer links.
	w.worker.must("POST", "/api/v1/instances/"+w.instanceID+"/invite/rotate", nil, 200)
	if loc := e.client().openLink(restored); loc != "/i/alpha/booking" {
		t.Errorf("invite rotation broke link: %s", loc)
	}
	// ...but the old invite no longer registers.
	if r := e.client().do("POST", "/api/v1/public/instances/alpha/registrations", map[string]any{
		"id": newUUID(), "invite": w.invite, "name": "Eve",
	}); r.code() != "invite_invalid" {
		t.Errorf("old invite: %d %s", r.Status, r.Raw)
	}
	// Tokens never reach the audit log.
	var cnt int
	tok := restored[strings.Index(restored, "t=")+2:]
	if err := e.db.R.QueryRow(`SELECT count(*) FROM audit_log WHERE coalesce(before_json,'') || coalesce(after_json,'') LIKE ?`, "%"+tok+"%").Scan(&cnt); err != nil || cnt != 0 {
		t.Errorf("token in audit log: %d %v", cnt, err)
	}
}

func TestPausedInstance(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	cust, _, _ := e.newCustomer(w, "Alice")
	apptID := newUUID()
	cust.must("POST", w.cust("/appointments"), slotBody(apptID, slot10), 200)

	w.worker.must("PUT", "/api/v1/instances/"+w.instanceID+"/status", map[string]any{"status": "paused"}, 200)
	if r := cust.do("POST", w.cust("/appointments"), slotBody(newUUID(), slot10.Add(time.Hour))); r.code() != "instance_paused" {
		t.Errorf("book while paused: %d %s", r.Status, r.Raw)
	}
	if r := e.client().do("POST", "/api/v1/public/instances/alpha/registrations", map[string]any{
		"id": newUUID(), "invite": w.invite, "name": "Eve",
	}); r.code() != "instance_paused" {
		t.Errorf("register while paused: %d %s", r.Status, r.Raw)
	}
	if av := cust.get(w.cust("/availability")); len(list(av.Body, "items")) != 0 {
		t.Error("availability while paused")
	}
	// Existing customers can still view and cancel.
	if len(list(cust.get(w.cust("/appointments")).Body, "upcoming")) != 1 {
		t.Error("cannot view while paused")
	}
	cust.must("POST", w.cust("/appointments/")+apptID+"/cancel", nil, 200)
}

func TestCancellationDeadline(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	cust, _, _ := e.newCustomer(w, "Alice")
	apptID := newUUID()
	cust.must("POST", w.cust("/appointments"), slotBody(apptID, slot10), 200)
	e.clock.Set(slot10.Add(-30 * time.Minute)) // 60-minute notice required
	if r := cust.do("POST", w.cust("/appointments/")+apptID+"/cancel", nil); r.code() != "cancellation_too_late" {
		t.Errorf("late cancel: %d %s", r.Status, r.Raw)
	}
	// Managers can still cancel.
	w.worker.must("POST", "/api/v1/instances/"+w.instanceID+"/appointments/"+apptID+"/status", map[string]any{"status": "cancelled"}, 200)
}

func TestRescheduleIsAtomic(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	cust, _, _ := e.newCustomer(w, "Alice")
	other, _, _ := e.newCustomer(w, "Bob")
	id := newUUID()
	cust.must("POST", w.cust("/appointments"), slotBody(id, slot10), 200)
	other.must("POST", w.cust("/appointments"), slotBody(newUUID(), slot10.Add(time.Hour)), 200)
	base := "/api/v1/instances/" + w.instanceID + "/appointments/" + id + "/reschedule"
	body := func(expected, to time.Time) map[string]any {
		return map[string]any{
			"expected_starts_at": expected.Format(time.RFC3339),
			"expected_ends_at":   expected.Add(30 * time.Minute).Format(time.RFC3339),
			"starts_at":          to.Format(time.RFC3339), "ends_at": to.Add(30 * time.Minute).Format(time.RFC3339),
		}
	}
	// Target taken: refused, original kept.
	if r := w.worker.do("POST", base, body(slot10, slot10.Add(time.Hour))); r.code() != "slot_taken" {
		t.Fatalf("reschedule onto taken slot: %d %s", r.Status, r.Raw)
	}
	// Moving onto an adjacent slot that overlaps its own old time is allowed.
	ok := w.worker.must("POST", base, body(slot10, slot10.Add(30*time.Minute)), 200)
	if str(ok.Body, "starts_at") != slot10.Add(30*time.Minute).Format(time.RFC3339) || str(ok.Body, "customer_name") != "Alice" {
		t.Fatalf("reschedule: %s", ok.Raw)
	}
	// Retrying the same reschedule succeeds; a stale expectation fails.
	replayed := w.worker.must("POST", base, body(slot10, slot10.Add(30*time.Minute)), 200)
	if str(replayed.Body, "customer_name") != "Alice" {
		t.Fatalf("incomplete reschedule replay: %s", replayed.Raw)
	}
	if r := w.worker.do("POST", base, body(slot10, slot10.Add(90*time.Minute))); r.code() != "appointment_changed" {
		t.Errorf("stale reschedule: %d %s", r.Status, r.Raw)
	}
}

func TestCSRFAndRegistrationDedup(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")

	// Missing CSRF header is rejected.
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/account/login", strings.NewReader(`{"login":"root","password":"root-password"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Errorf("no csrf: %d", resp.StatusCode)
	}

	// Submitting the same registration twice creates one application.
	c := e.client()
	id := newUUID()
	body := map[string]any{"id": id, "invite": w.invite, "name": "Alice", "contact": "123"}
	c.must("POST", "/api/v1/public/instances/alpha/registrations", body, 200)
	c.must("POST", "/api/v1/public/instances/alpha/registrations", body, 200)
	l := w.worker.get("/api/v1/instances/" + w.instanceID + "/customers?status=pending")
	if len(list(l.Body, "customers")) != 1 {
		t.Errorf("duplicate registrations: %s", l.Raw)
	}
	// Customers cannot use account endpoints and vice versa.
	cust, _, _ := e.newCustomer(w, "Bob")
	if r := cust.do("GET", "/api/v1/instances/"+w.instanceID+"/customers", nil); r.Status != 401 {
		t.Errorf("customer on manager API: %d", r.Status)
	}
	if r := w.worker.do("GET", w.cust("/appointments"), nil); r.Status != 401 {
		t.Errorf("worker on customer API: %d", r.Status)
	}
}

func TestOperatorBindings(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	root := w.admin
	beta := root.must("POST", "/api/v1/platform/instances", map[string]any{
		"slug": "beta", "name": "Beta", "timezone": "UTC", "default_locale": "en", "contact_kind": "phone", "contact_required": false,
	}, 200)
	betaID := str(beta.Body, "id")
	if ops := list(beta.Body, "operators"); len(ops) != 0 {
		t.Errorf("new instance has operators: %s", beta.Raw)
	}
	acct := root.must("POST", "/api/v1/platform/accounts", map[string]any{"login": "op2", "name": "Op2", "password": "op2-password"}, 200)
	op2ID := str(acct.Body, "id")
	op2 := e.login("op2", "op2-password")

	// One operator can be bound to many instances, one instance can have many
	// operators.
	if r := op2.do("GET", "/api/v1/instances/"+w.instanceID, nil); r.Status != 404 {
		t.Errorf("unbound operator: %d", r.Status)
	}
	root.must("PUT", "/api/v1/platform/instances/"+w.instanceID+"/operators/"+op2ID, nil, 200)
	root.must("PUT", "/api/v1/platform/instances/"+w.instanceID+"/operators/"+op2ID, nil, 200) // idempotent
	r := root.must("PUT", "/api/v1/platform/instances/"+betaID+"/operators/"+op2ID, nil, 200)
	if len(list(r.Body, "operators")) != 1 {
		t.Errorf("operators %s", r.Raw)
	}
	if r := op2.get("/api/v1/instances/" + w.instanceID); str(r.Body, "role") != "operator" {
		t.Errorf("role %s", r.Raw)
	}
	if n := len(op2.get("/api/v1/account/me").Body["account"].(map[string]any)["instances"].([]any)); n != 2 {
		t.Errorf("operator sees %d instances", n)
	}
	accts := list(root.get("/api/v1/platform/accounts").Body, "accounts")
	for _, a := range accts {
		m := a.(map[string]any)
		if m["id"] == op2ID && len(m["instance_ids"].([]any)) != 2 {
			t.Errorf("account list bindings: %v", m)
		}
	}

	// Removing one binding leaves the other.
	root.must("DELETE", "/api/v1/platform/instances/"+w.instanceID+"/operators/"+op2ID, nil, 200)
	if r := op2.do("GET", "/api/v1/instances/"+w.instanceID, nil); r.Status != 404 {
		t.Errorf("operator after unbind: %d", r.Status)
	}
	op2.must("GET", "/api/v1/instances/"+betaID, nil, 200)

	// The super admin and disabled accounts cannot be bound.
	rootID := str(root.get("/api/v1/account/me").Body, "account", "id")
	if r := root.do("PUT", "/api/v1/platform/instances/"+betaID+"/operators/"+rootID, nil); r.code() != "cannot_bind_super_admin" {
		t.Errorf("bind super admin: %d %s", r.Status, r.Raw)
	}
	root.must("PUT", "/api/v1/platform/accounts/"+op2ID+"/disabled", map[string]any{"disabled": true}, 204)
	if r := root.do("PUT", "/api/v1/platform/instances/"+w.instanceID+"/operators/"+op2ID, nil); r.code() != "account_disabled" {
		t.Errorf("bind disabled account: %d %s", r.Status, r.Raw)
	}

	// Operators cannot manage accounts, instances or bindings, including
	// binding themselves elsewhere.
	op := w.worker
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/v1/platform/accounts"},
		{"POST", "/api/v1/platform/accounts"},
		{"GET", "/api/v1/platform/instances"},
		{"POST", "/api/v1/platform/instances"},
		{"PUT", "/api/v1/platform/instances/" + betaID + "/operators/" + w.workerID},
		{"DELETE", "/api/v1/platform/instances/" + w.instanceID + "/operators/" + w.workerID},
		{"GET", "/api/v1/platform/instances/" + w.instanceID + "/deletion-preview"},
		{"DELETE", "/api/v1/platform/instances/" + w.instanceID},
	} {
		if r := op.do(c.method, c.path, map[string]any{}); r.Status != 403 && r.Status != 422 {
			t.Errorf("operator %s %s: %d %s", c.method, c.path, r.Status, r.Raw)
		}
	}
	if r := op.do("GET", "/api/v1/instances/"+betaID, nil); r.Status != 404 {
		t.Errorf("operator reaches unbound instance: %d", r.Status)
	}
	// Super admin enters any instance and the entry is audited once.
	root.get("/api/v1/instances/by-slug/" + w.slug)
	root.get("/api/v1/instances/by-slug/" + w.slug)
	var n int
	_ = e.db.R.QueryRow(`SELECT count(*) FROM audit_log WHERE action = 'instance.super_admin_enter'`).Scan(&n)
	if n != 1 {
		t.Errorf("super admin entry audited %d times", n)
	}
}

func TestTimezoneChangeNeedsConfirmation(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	cust, _, _ := e.newCustomer(w, "Alice")
	cust.must("POST", w.cust("/appointments"), slotBody(newUUID(), slot10), 200)
	body := map[string]any{"name": "A", "timezone": "Asia/Tokyo", "brand_color": nil, "default_locale": "ja", "contact_kind": "phone", "contact_required": false}
	if r := w.worker.do("PUT", "/api/v1/instances/"+w.instanceID, body); r.code() != "timezone_change_needs_confirmation" {
		t.Errorf("tz change: %d %s", r.Status, r.Raw)
	}
	body["confirm_timezone_change"] = true
	w.worker.must("PUT", "/api/v1/instances/"+w.instanceID, body, 200)
}

func TestInstanceBrandColor(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	cust, _, _ := e.newCustomer(w, "Alice")
	path := "/api/v1/instances/" + w.instanceID
	settings := func(color any) map[string]any {
		b := map[string]any{"name": "Instance alpha", "timezone": "Europe/London", "default_locale": "en", "contact_kind": "phone", "contact_required": false}
		b["brand_color"] = color
		return b
	}

	// Stored in lowercase and shown on the public and customer views.
	if r := w.worker.must("PUT", path, settings("#1D4ED8"), 200); str(r.Body, "brand_color") != "#1d4ed8" {
		t.Errorf("patch: %s", r.Raw)
	}
	if r := cust.get(w.cust("/me")); str(r.Body, "customer", "instance", "brand_color") != "#1d4ed8" {
		t.Errorf("customer me: %s", r.Raw)
	}
	if r := e.client().get("/api/v1/public/instances/alpha"); str(r.Body, "brand_color") != "#1d4ed8" {
		t.Errorf("public: %s", r.Raw)
	}

	for _, bad := range []string{"blue", "#12345", "#12345g", "1d4ed8"} {
		if r := w.worker.do("PUT", path, settings(bad)); r.code() != "invalid_color" {
			t.Errorf("%q: %d %s", bad, r.Status, r.Raw)
		}
	}

	// Settings replace the whole record: omitting the colour clears it.
	if r := w.worker.must("PUT", path, settings(nil), 200); r.Body["brand_color"] != nil {
		t.Errorf("cleared: %s", r.Raw)
	}
	if r := e.client().get("/api/v1/public/instances/alpha"); r.Body["brand_color"] != nil {
		t.Errorf("public after clear: %s", r.Raw)
	}
}

func TestCustomerIdentitiesPerInstance(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	a := e.setupInstance("alpha", "worker-a")
	b := e.setupInstance("beta", "worker-b")
	// One person, one browser, a customer of both instances.
	c := e.client()
	idA, _ := e.customerIn(a, c, "Alice at A")
	idB, _ := e.customerIn(b, c, "Alice at B")
	if str(c.get(a.cust("/me")).Body, "customer", "id") != idA || str(c.get(b.cust("/me")).Body, "customer", "id") != idB {
		t.Fatal("opening the second link signed the browser out of the first instance")
	}
	c.must("POST", a.cust("/appointments"), slotBody(newUUID(), slot10), 200)
	c.must("POST", b.cust("/appointments"), slotBody(newUUID(), slot10), 200)

	// Disabling the customer in A ends only A's identity.
	a.worker.must("POST", "/api/v1/instances/"+a.instanceID+"/customers/"+idA+"/actions/disable", nil, 200)
	if me := c.get(a.cust("/me")); me.Body["customer"] != nil {
		t.Error("A identity survived disable")
	}
	if str(c.get(b.cust("/me")).Body, "customer", "id") != idB {
		t.Error("B identity lost when A's customer was disabled")
	}

	// Signing out of B keeps a new A identity.
	res := a.worker.must("POST", "/api/v1/instances/"+a.instanceID+"/customers/"+idA+"/actions/restore", nil, 200)
	c.openLink(str(res.Body, "access_path"))
	c.must("POST", b.cust("/logout"), nil, 204)
	if me := c.get(b.cust("/me")); me.Body["customer"] != nil {
		t.Error("B identity survived sign-out")
	}
	if str(c.get(a.cust("/me")).Body, "customer", "id") != idA {
		t.Error("signing out of B signed the browser out of A")
	}
	if r := c.do("GET", "/api/v1/customer/no-such-instance/me", nil); r.code() != "instance_not_found" {
		t.Errorf("unknown instance: %d %s", r.Status, r.Raw)
	}
}

func TestCreateInstanceCopiesSettings(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	settings := map[string]any{"name": "Instance alpha", "timezone": "Europe/London", "default_locale": "en",
		"contact_kind": "phone", "contact_required": false, "brand_color": "#0f766e"}
	w.worker.must("PUT", "/api/v1/instances/"+w.instanceID, settings, 200)
	w.worker.must("PUT", "/api/v1/instances/"+w.instanceID+"/policy", map[string]any{
		"daily_customer_limit": nil, "per_customer_daily_limit": nil, "max_future_bookings": nil,
		"slot_step_minutes": 30, "service_duration_minutes": 30, "buffer_minutes": 5, "per_customer_weekly_limit": 2,
		"booking_horizon_days": 14, "min_booking_notice_minutes": 0, "min_cancellation_notice_minutes": 60,
	}, 200)
	e.newCustomer(w, "Alice")

	r := w.admin.must("POST", "/api/v1/platform/instances", map[string]any{
		"slug": "alpha-two", "name": "Alpha two", "timezone": "Europe/London", "default_locale": "en",
		"contact_kind": "phone", "contact_required": false, "copy_from": w.instanceID,
	}, 200)
	id := str(r.Body, "id")
	if str(r.Body, "brand_color") != "#0f766e" || len(list(r.Body, "operators")) != 0 || str(r.Body, "status") != "paused" {
		t.Errorf("copied instance: %s", r.Raw)
	}
	base := "/api/v1/instances/" + id
	srcBase := "/api/v1/instances/" + w.instanceID
	if p, src := w.admin.get(base+"/policy"), w.admin.get(srcBase+"/policy"); string(p.Raw) != string(src.Raw) {
		t.Errorf("policy not copied: %s, source %s", p.Raw, src.Raw)
	}
	if s, src := w.admin.get(base+"/schedule"), w.admin.get(srcBase+"/schedule"); string(s.Raw) != string(src.Raw) {
		t.Errorf("schedule not copied: %s, source %s", s.Raw, src.Raw)
	}
	if c := w.admin.get(base + "/customers"); len(list(c.Body, "customers")) != 0 {
		t.Errorf("customers copied: %s", c.Raw)
	}
	if inv := w.admin.get(base + "/invite"); str(inv.Body, "code") == w.invite {
		t.Error("invite code copied")
	}
	if r := w.admin.do("POST", "/api/v1/platform/instances", map[string]any{
		"slug": "alpha-two", "name": "Again", "timezone": "UTC", "default_locale": "en", "contact_kind": "phone", "contact_required": false,
	}); r.code() != "slug_taken" {
		t.Errorf("duplicate slug: %d %s", r.Status, r.Raw)
	}
}

func TestInstanceDeletion(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	other := e.setupInstance("beta", "worker2")
	root := w.admin
	// A distinctive name to look for in the database file afterwards.
	const marker = "Zebulon Quackenbush"
	cust, custID, link := e.newCustomer(w, marker)
	cust.must("POST", w.cust("/appointments"), slotBody(newUUID(), slot10), 200)
	w.worker.must("PUT", "/api/v1/instances/"+w.instanceID+"/customers/"+custID+"/note", map[string]any{"note": marker + " note"}, 200)
	otherCust, otherID, _ := e.newCustomer(other, "Bob")

	p := root.get("/api/v1/platform/instances/" + w.instanceID + "/deletion-preview")
	if p.Body["customers"] != float64(1) || p.Body["future_appointments"] != float64(1) || p.Body["appointments"] != float64(1) {
		t.Errorf("preview %s", p.Raw)
	}
	if r := root.do("DELETE", "/api/v1/platform/instances/"+w.instanceID, map[string]any{"confirm_name": "alpha"}); r.code() != "confirm_name_mismatch" {
		t.Errorf("wrong confirmation: %d %s", r.Status, r.Raw)
	}
	// Future appointments do not block deletion.
	r := root.must("DELETE", "/api/v1/platform/instances/"+w.instanceID, map[string]any{"confirm_name": "Instance alpha"}, 200)
	if r.Body["customers"] != float64(1) {
		t.Errorf("deleted counts %s", r.Raw)
	}

	// Nothing of the instance is left.
	for _, q := range []string{
		`SELECT count(*) FROM instances WHERE id = ?`,
		`SELECT count(*) FROM instance_operators WHERE instance_id = ?`,
		`SELECT count(*) FROM customers WHERE instance_id = ?`,
		`SELECT count(*) FROM appointments WHERE instance_id = ?`,
		`SELECT count(*) FROM schedules WHERE instance_id = ?`,
		`SELECT count(*) FROM schedule_overrides WHERE instance_id = ?`,
		`SELECT count(*) FROM blocked_periods WHERE instance_id = ?`,
		`SELECT count(*) FROM booking_policies WHERE instance_id = ?`,
		`SELECT count(*) FROM audit_log WHERE instance_id = ?1 OR (target_id = ?1 AND action <> 'instance.deleted')`,
	} {
		var n int
		if err := e.db.R.QueryRow(q, w.instanceID).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s: %d left (err %v)", q, n, err)
		}
	}
	var before, after string
	if err := e.db.R.QueryRow(`SELECT before_json, after_json FROM audit_log WHERE action = 'instance.deleted' AND target_id = ?`, w.instanceID).Scan(&before, &after); err != nil {
		t.Fatalf("deletion audit: %v", err)
	}
	if !strings.Contains(before, "Instance alpha") || strings.Contains(before+after, marker) {
		t.Errorf("deletion audit: %s %s", before, after)
	}
	// secure_delete and the checkpoint leave no trace in the files.
	for _, path := range []string{e.db.Path, e.db.Path + "-wal"} {
		if b, err := os.ReadFile(path); err == nil && bytes.Contains(b, []byte(marker)) {
			t.Errorf("%s still contains the deleted customer's name", filepath.Base(path))
		}
	}

	// Other instances are unaffected.
	if str(otherCust.get(other.cust("/me")).Body, "customer", "id") != otherID {
		t.Error("other instance's customer lost")
	}
	other.worker.must("GET", "/api/v1/instances/"+other.instanceID+"/appointments", nil, 200)

	// The old link and the old customer session no longer work, and the slug
	// can be reused at once without accepting them.
	if loc := e.client().openLink(link); !strings.Contains(loc, "link-invalid") {
		t.Errorf("old link redirects to %s", loc)
	}
	if r := cust.do("GET", w.cust("/me"), nil); r.code() != "instance_not_found" {
		t.Errorf("old session after deletion: %d %s", r.Status, r.Raw)
	}
	root.must("POST", "/api/v1/platform/instances", map[string]any{
		"slug": "alpha", "name": "New alpha", "timezone": "UTC", "default_locale": "en", "contact_kind": "phone", "contact_required": false,
	}, 200)
	if loc := e.client().openLink(link); !strings.Contains(loc, "link-invalid") {
		t.Errorf("old link on the reused slug redirects to %s", loc)
	}
	if me := cust.get(w.cust("/me")); me.Body["customer"] != nil {
		t.Errorf("old session signed in to the new instance: %s", me.Raw)
	}
	if r := e.client().do("POST", "/api/v1/public/instances/alpha/registrations", map[string]any{
		"id": newUUID(), "invite": w.invite, "name": "Late",
	}); r.code() != "invite_invalid" {
		t.Errorf("old invite on the reused slug: %d %s", r.Status, r.Raw)
	}
}

// cycleBody is a cycle of n days, each 09:00-12:00.
func cycleBody(n int) map[string]any {
	days := make([][]map[string]int, n)
	for i := range days {
		days[i] = []map[string]int{{"start": 540, "end": 720}}
	}
	return map[string]any{"cycle_length_days": n, "anchor_date": "2026-01-01", "days": days}
}

func TestCycleLengthLimit(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	root := w.admin
	sched := "/api/v1/instances/" + w.instanceID + "/schedule"

	// The default limit is 30 days, shown to operators with the schedule.
	if s := root.get("/api/v1/platform/settings"); s.Body["max_cycle_days"] != float64(30) || len(list(s.Body, "over_limit")) != 0 {
		t.Errorf("default settings %s", s.Raw)
	}
	if s := w.worker.get(sched); s.Body["max_cycle_days"] != float64(30) {
		t.Errorf("schedule %s", s.Raw)
	}
	// Only the super admin reads and changes the settings.
	for _, r := range []result{
		w.worker.do("GET", "/api/v1/platform/settings", nil),
		w.worker.do("PUT", "/api/v1/platform/settings", map[string]any{"max_cycle_days": 60}),
	} {
		if r.Status != 403 {
			t.Errorf("operator settings access: %d %s", r.Status, r.Raw)
		}
	}

	// Saving or previewing a longer cycle is refused with the limit.
	for _, p := range []string{sched, sched + "/preview"} {
		method := "PUT"
		if p != sched {
			method = "POST"
		}
		r := w.worker.do(method, p, cycleBody(31))
		if r.code() != "cycle_length_over_limit" || r.Body["params"].(map[string]any)["max"] != float64(30) {
			t.Errorf("%s 31 days: %d %s", method, r.Status, r.Raw)
		}
	}
	w.worker.must("PUT", sched, cycleBody(30), 200)

	// A 40-day cycle saved under a higher limit keeps working after the
	// limit is lowered; the settings list it, and saving needs a shorter one.
	root.must("PUT", "/api/v1/platform/settings", map[string]any{"max_cycle_days": 60}, 200)
	w.worker.must("PUT", sched, cycleBody(40), 200)
	r := root.must("PUT", "/api/v1/platform/settings", map[string]any{"max_cycle_days": 30}, 200)
	over := list(r.Body, "over_limit")
	if len(over) != 1 || over[0].(map[string]any)["cycle_length_days"] != float64(40) || over[0].(map[string]any)["slug"] != "alpha" {
		t.Errorf("over limit %s", r.Raw)
	}
	cust, _, _ := e.newCustomer(w, "Alice")
	if av := cust.get(w.cust("/availability?limit=3")); len(list(av.Body, "items")) == 0 {
		t.Errorf("no availability with an over-limit cycle: %s", av.Raw)
	}
	if s := w.worker.get(sched); s.Body["cycle_length_days"] != float64(40) || s.Body["max_cycle_days"] != float64(30) {
		t.Errorf("over-limit schedule %s", s.Raw)
	}
	if r := w.worker.do("PUT", sched, cycleBody(40)); r.code() != "cycle_length_over_limit" {
		t.Errorf("resave over limit: %d %s", r.Status, r.Raw)
	}
	w.worker.must("PUT", sched, cycleBody(30), 200)
	if s := root.get("/api/v1/platform/settings"); len(list(s.Body, "over_limit")) != 0 {
		t.Errorf("still over limit %s", s.Raw)
	}

	if r := root.do("PUT", "/api/v1/platform/settings", map[string]any{"max_cycle_days": 0}); r.Status != 422 {
		t.Errorf("limit 0: %d %s", r.Status, r.Raw)
	}
	var n int
	_ = e.db.R.QueryRow(`SELECT count(*) FROM audit_log WHERE action = 'platform.settings'`).Scan(&n)
	if n != 2 {
		t.Errorf("%d settings audit entries", n)
	}
}

func TestBackupWhileRunning(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	e.setupInstance("alpha", "worker1")
	path := t.TempDir() + "/backup.db"
	if err := e.db.Backup(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	restored, err := storage.Open(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var n int
	if err := restored.R.QueryRow(`SELECT count(*) FROM instances`).Scan(&n); err != nil || n != 1 {
		t.Errorf("restored instances: %d %v", n, err)
	}
}

// The dashboard's next appointment is the first booked one, however many
// cancelled appointments come before it.
func TestDashboardNextSkipsCancelled(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("alpha", "worker1")
	_, custID, _ := e.newCustomer(w, "Alice")
	base := "/api/v1/instances/" + w.instanceID
	var ids []string
	// Six 30-minute slots a day from 09:00 London (08:00 UTC), from June 2.
	for i := range 22 {
		start := time.Date(2026, 6, 2+i/6, 8, 30*(i%6), 0, 0, time.UTC)
		body := slotBody(newUUID(), start)
		body["customer_id"] = custID
		r := w.worker.must("POST", base+"/appointments", body, 200)
		ids = append(ids, str(r.Body, "id"))
	}
	for _, id := range ids[:21] {
		w.worker.must("POST", base+"/appointments/"+id+"/status", map[string]any{"status": "cancelled"}, 200)
	}
	d := w.worker.get(base + "/dashboard")
	if got := str(d.Body, "next", "id"); got != ids[21] {
		t.Fatalf("next = %q, want %q: %s", got, ids[21], d.Raw)
	}
}
