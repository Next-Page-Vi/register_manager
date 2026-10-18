package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"register_manager/internal/accounts"
	"register_manager/internal/api"
	"register_manager/internal/auth"
	"register_manager/internal/storage"
)

// clock is a settable test clock.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

type env struct {
	t     *testing.T
	srv   *httptest.Server
	db    *storage.DB
	deps  api.Deps
	clock *clock
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "test.db"), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	c := &clock{t: time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)} // Monday
	h, deps := New(ctx, Options{DB: db, Log: log, CookieSecure: false, Now: c.Now})
	// Generous limits so tests are not rate limited.
	deps.Limiters.Login = auth.NewLimiter(1e6, 1e6)
	deps.Limiters.Register = auth.NewLimiter(1e6, 1e6)
	deps.Limiters.AccessToken = auth.NewLimiter(1e6, 1e6)
	deps.Limiters.PasswordReset = auth.NewLimiter(1e6, 1e6)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &env{t: t, srv: srv, db: db, deps: deps, clock: c}
}

// client is a browser-like client with its own cookie jar.
type client struct {
	e    *env
	http *http.Client
}

func (e *env) client() *client {
	jar, _ := cookiejar.New(nil)
	c := &client{e: e, http: &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
	// Load the CSRF cookie like a page load would.
	resp, err := c.http.Get(e.srv.URL + "/api/v1/account/me")
	if err != nil {
		e.t.Fatal(err)
	}
	resp.Body.Close()
	return c
}

func (c *client) csrf() string {
	u, _ := url.Parse(c.e.srv.URL)
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == auth.CSRFCookie {
			return ck.Value
		}
	}
	return ""
}

type result struct {
	Status int
	Body   map[string]any
	Raw    []byte
}

func (r result) code() string {
	s, _ := r.Body["code"].(string)
	return s
}

func (c *client) do(method, path string, body any) result {
	c.e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.e.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.e.srv.URL+path, rd)
	if err != nil {
		c.e.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(auth.CSRFHeader, c.csrf())
	resp, err := c.http.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	r := result{Status: resp.StatusCode, Raw: raw}
	if len(raw) > 0 && strings.Contains(resp.Header.Get("Content-Type"), "json") {
		_ = json.Unmarshal(raw, &r.Body)
	}
	return r
}

func (c *client) must(method, path string, body any, want int) result {
	c.e.t.Helper()
	r := c.do(method, path, body)
	if r.Status != want {
		c.e.t.Fatalf("%s %s: status %d, want %d: %s", method, path, r.Status, want, r.Raw)
	}
	return r
}

func (c *client) get(path string) result { c.e.t.Helper(); return c.must("GET", path, nil, 200) }

// openLink follows a customer access link and returns the redirect target.
func (c *client) openLink(path string) string {
	c.e.t.Helper()
	resp, err := c.http.Get(c.e.srv.URL + path)
	if err != nil {
		c.e.t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		c.e.t.Fatalf("access link status %d", resp.StatusCode)
	}
	return resp.Header.Get("Location")
}

// createSuperAdmin creates the built-in super admin through the CLI path
// (which sets a known password) and renames it, so tests read naturally.
func (e *env) createSuperAdmin(login, password string) {
	e.t.Helper()
	svc := &accounts.Service{DB: e.db, Now: e.clock.Now}
	if _, err := svc.CLIResetPassword(context.Background(), password); err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.db.W.Exec(`UPDATE accounts SET login = ? WHERE super_admin = 1`, login); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) login(login, password string) *client {
	e.t.Helper()
	c := e.client()
	c.must("POST", "/api/v1/account/login", map[string]any{"login": login, "password": password}, 200)
	return c
}

func str(m map[string]any, path ...string) string {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = mm[p]
	}
	s, _ := cur.(string)
	return s
}

func list(m map[string]any, key string) []any {
	l, _ := m[key].([]any)
	return l
}

// world is a configured instance with a bound operator.
type world struct {
	admin      *client
	worker     *client // the bound operator
	workerID   string
	instanceID string
	slug       string
	invite     string
}

// cust returns the customer API path for the world's instance.
func (w *world) cust(path string) string { return "/api/v1/customer/" + w.slug + path }

func (e *env) setupInstance(slug, workerLogin string) *world {
	e.t.Helper()
	admin := e.login("root", "root-password")
	acct := admin.must("POST", "/api/v1/platform/accounts", map[string]any{
		"login": workerLogin, "name": "Worker " + workerLogin, "password": "worker-password",
	}, 200)
	workerID := str(acct.Body, "id")
	inst := admin.must("POST", "/api/v1/platform/instances", map[string]any{
		"slug": slug, "name": "Instance " + slug,
		"timezone": "Europe/London", "default_locale": "en", "contact_kind": "phone", "contact_required": false,
	}, 200)
	id := str(inst.Body, "id")
	admin.must("PUT", "/api/v1/platform/instances/"+id+"/operators/"+workerID, nil, 200)
	worker := e.login(workerLogin, "worker-password")
	// Every day 09:00-12:00, 30-minute slots and services.
	worker.must("PUT", "/api/v1/instances/"+id+"/schedule", map[string]any{
		"cycle_length_days": 1, "anchor_date": "2026-01-01",
		"days": [][]map[string]int{{{"start": 540, "end": 720}}},
	}, 200)
	worker.must("PUT", "/api/v1/instances/"+id+"/policy", map[string]any{
		"daily_customer_limit": nil, "per_customer_daily_limit": nil, "per_customer_weekly_limit": nil, "max_future_bookings": nil,
		"slot_step_minutes": 30, "service_duration_minutes": 30, "buffer_minutes": 0,
		"booking_horizon_days": 14, "min_booking_notice_minutes": 0, "min_cancellation_notice_minutes": 60,
	}, 200)
	worker.must("PUT", "/api/v1/instances/"+id+"/status", map[string]any{"status": "active"}, 200)
	inv := worker.get("/api/v1/instances/" + id + "/invite")
	return &world{admin: admin, worker: worker, workerID: workerID, instanceID: id, slug: slug, invite: str(inv.Body, "code")}
}

// newCustomer registers, approves and signs in a customer, returning the
// customer client and the access path.
func (e *env) newCustomer(w *world, name string) (*client, string, string) {
	e.t.Helper()
	c := e.client()
	id, link := e.customerIn(w, c, name)
	return c, id, link
}

// customerIn registers and approves a customer of w using the browser c and
// opens the personal link there, returning the customer ID and link.
func (e *env) customerIn(w *world, c *client, name string) (string, string) {
	e.t.Helper()
	id := newUUID()
	r := c.must("POST", "/api/v1/public/instances/"+w.slug+"/registrations", map[string]any{
		"id": id, "invite": w.invite, "name": name,
	}, 200)
	if str(r.Body, "status") != "pending" {
		e.t.Fatalf("registration %s", r.Raw)
	}
	res := w.worker.must("POST", "/api/v1/instances/"+w.instanceID+"/customers/"+id+"/actions/approve", nil, 200)
	link := str(res.Body, "access_path")
	if loc := c.openLink(link); loc != "/i/"+w.slug+"/booking" {
		e.t.Fatalf("redirect to %s", loc)
	}
	return id, link
}
