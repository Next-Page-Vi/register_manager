package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/uptrace/bun"
	"golang.org/x/time/rate"

	"register_manager/internal/apperr"
	"register_manager/internal/storage"
)

// Session keys.
const (
	keyAccountID   = "account_id"
	keyAuthVersion = "auth_version"
	keyCustomers   = "customers"
	keyAudited     = "audited_instances"
)

// Sessions holds the two independent session managers. Account and customer
// sessions use different cookies and tables, so one can never be read as the
// other.
type Sessions struct {
	Account  *scs.SessionManager
	Customer *scs.SessionManager
	db       *storage.DB
	log      *slog.Logger
}

// NewSessions creates the session managers.
func NewSessions(ctx context.Context, db *storage.DB, secureCookies bool, log *slog.Logger) *Sessions {
	acct := scs.New()
	acct.Store = storage.NewSessionStore(ctx, db, "account_sessions", log)
	acct.Lifetime = 30 * 24 * time.Hour
	acct.IdleTimeout = 7 * 24 * time.Hour
	acct.Cookie.Name = "rm_account"
	acct.Cookie.HttpOnly = true
	acct.Cookie.Secure = secureCookies
	acct.Cookie.SameSite = http.SameSiteLaxMode
	acct.Cookie.Path = "/"

	cust := scs.New()
	cust.Store = storage.NewSessionStore(ctx, db, "customer_sessions", log)
	cust.Lifetime = 180 * 24 * time.Hour
	cust.IdleTimeout = 60 * 24 * time.Hour
	cust.Cookie.Name = "rm_customer"
	cust.Cookie.HttpOnly = true
	cust.Cookie.Secure = secureCookies
	cust.Cookie.SameSite = http.SameSiteLaxMode
	cust.Cookie.Path = "/"

	return &Sessions{Account: acct, Customer: cust, db: db, log: log}
}

// Middleware loads both sessions and resolves the account principal,
// validating it against the database on every request. Customer identities
// are per instance and resolved by the customer API for the instance in the
// URL (see CustomerFor).
func (s *Sessions) Middleware(next http.Handler) http.Handler {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		q := s.db.Read()
		if id := s.Account.GetString(ctx, keyAccountID); id != "" {
			a, err := LoadAccount(ctx, q, id, s.Account.GetInt64(ctx, keyAuthVersion))
			switch {
			case err == nil:
				ctx = WithAccount(ctx, a)
			case errors.Is(err, apperr.ErrUnauthenticated):
				_ = s.Account.Destroy(ctx)
			default:
				s.log.Error("load account session", "err", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
	return s.Account.LoadAndSave(s.Customer.LoadAndSave(inner))
}

// LoginAccount starts a fresh account session.
func (s *Sessions) LoginAccount(ctx context.Context, id string, version int64) error {
	if err := s.Account.RenewToken(ctx); err != nil {
		return err
	}
	s.Account.Remove(ctx, keyAudited)
	s.Account.Put(ctx, keyAccountID, id)
	s.Account.Put(ctx, keyAuthVersion, version)
	return nil
}

// LogoutAccount ends the account session.
func (s *Sessions) LogoutAccount(ctx context.Context) error { return s.Account.Destroy(ctx) }

// customerIdentity is a customer signed in for one instance.
type customerIdentity struct {
	CustomerID  string `json:"c"`
	AuthVersion int64  `json:"v"`
}

// One browser has one customer session holding an identity per instance, so
// a person who is a customer of several instances can open each link in the
// same browser without signing out of the others. Identities are stored as
// JSON keyed by instance ID.
func (s *Sessions) customerIdentities(ctx context.Context) map[string]customerIdentity {
	m := map[string]customerIdentity{}
	if raw := s.Customer.GetString(ctx, keyCustomers); raw != "" {
		_ = json.Unmarshal([]byte(raw), &m)
	}
	return m
}

func (s *Sessions) putCustomerIdentities(ctx context.Context, m map[string]customerIdentity) error {
	if len(m) == 0 {
		return s.Customer.Destroy(ctx)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	s.Customer.Put(ctx, keyCustomers, string(b))
	return nil
}

// LoginCustomer signs the customer in for one instance. The session token is
// rotated; identities for other instances are kept.
func (s *Sessions) LoginCustomer(ctx context.Context, instanceID, id string, version int64) error {
	if err := s.Customer.RenewToken(ctx); err != nil {
		return err
	}
	m := s.customerIdentities(ctx)
	m[instanceID] = customerIdentity{CustomerID: id, AuthVersion: version}
	return s.putCustomerIdentities(ctx, m)
}

// CustomerFor returns the customer signed in for the instance, validated
// against the database. An identity that is no longer valid is removed from
// the session, and so are identities whose customer no longer exists (for
// example because their instance was deleted). Other instances' valid
// identities are unaffected.
func (s *Sessions) CustomerFor(ctx context.Context, q bun.IDB, instanceID string) (*Customer, error) {
	m := s.customerIdentities(ctx)
	changed := false
	var out *Customer
	for inst, ident := range m {
		if inst == instanceID {
			c, err := LoadCustomer(ctx, q, inst, ident.CustomerID, ident.AuthVersion)
			if errors.Is(err, apperr.ErrUnauthenticated) {
				delete(m, inst)
				changed = true
				continue
			}
			if err != nil {
				return nil, err
			}
			out = c
			continue
		}
		_, err := storage.GetCustomer(ctx, q, inst, ident.CustomerID)
		if errors.Is(err, sql.ErrNoRows) {
			delete(m, inst)
			changed = true
		} else if err != nil {
			return nil, err
		}
	}
	if changed {
		if err := s.putCustomerIdentities(ctx, m); err != nil {
			return nil, err
		}
	}
	if out == nil {
		return nil, apperr.ErrUnauthenticated
	}
	return out, nil
}

// LogoutCustomer signs the customer out of one instance only.
func (s *Sessions) LogoutCustomer(ctx context.Context, instanceID string) error {
	m := s.customerIdentities(ctx)
	if _, ok := m[instanceID]; !ok {
		return nil
	}
	delete(m, instanceID)
	return s.putCustomerIdentities(ctx, m)
}

// InstanceAudited checks without changing the session; mark only after commit.
func (s *Sessions) InstanceAudited(ctx context.Context, instanceID string) bool {
	ids, _ := s.Account.Get(ctx, keyAudited).([]string)
	return slices.Contains(ids, instanceID)
}
func (s *Sessions) MarkInstanceAudited(ctx context.Context, instanceID string) {
	ids, _ := s.Account.Get(ctx, keyAudited).([]string)
	s.Account.Put(ctx, keyAudited, append(slices.Clone(ids), instanceID))
}

// CSRFCookie is the double-submit cookie; its value must be echoed in the
// CSRFHeader on every unsafe API request.
const (
	CSRFCookie = "rm_csrf"
	CSRFHeader = "X-CSRF-Token"
)

// CSRF ensures every client has a CSRF cookie and verifies the header on
// unsafe requests under /api/.
func CSRF(secureCookies bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ""
			if c, err := r.Cookie(CSRFCookie); err == nil && len(c.Value) >= 32 {
				token = c.Value
			} else {
				token = NewToken(32)
				http.SetCookie(w, &http.Cookie{
					Name: CSRFCookie, Value: token, Path: "/",
					Secure: secureCookies, SameSite: http.SameSiteStrictMode,
					MaxAge: 400 * 24 * 3600,
				})
				// Make the new token visible to the check below.
				r.AddCookie(&http.Cookie{Name: CSRFCookie, Value: token})
			}
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
			default:
				if strings.HasPrefix(r.URL.Path, "/api/") {
					if h := r.Header.Get(CSRFHeader); h == "" || !EqualConstantTime(h, token) {
						w.Header().
							Set("Content-Type", "application/problem+json")
						w.WriteHeader(http.StatusForbidden)
						_, _ = w.Write([]byte(`{"status":403,"code":"csrf_failed"}`))
						return
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP middleware stores the client address in the context. When
// trustedHeader is set (the deployment's reverse proxy sets it), the address
// is taken from that header; for X-Forwarded-For the right-most entry, added
// by the trusted proxy, is used.
func ClientIP(trustedHeader string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ""
			if trustedHeader != "" {
				v := r.Header.Get(trustedHeader)
				if strings.EqualFold(trustedHeader, "X-Forwarded-For") {
					parts := strings.Split(v, ",")
					v = parts[len(parts)-1]
				}
				ip = strings.TrimSpace(v)
			}
			if ip == "" {
				host, _, err := net.SplitHostPort(r.RemoteAddr)
				if err != nil {
					host = r.RemoteAddr
				}
				ip = host
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestKey, ip)))
		})
	}
}

// ClientIPFrom returns the client address stored by ClientIP.
func ClientIPFrom(ctx context.Context) string {
	s, _ := ctx.Value(requestKey).(string)
	return s
}

// Limiter is an in-memory per-key token bucket limiter (single-node
// deployment).
type Limiter struct {
	mu      sync.Mutex
	every   rate.Limit
	burst   int
	buckets map[string]*bucket
}

type bucket struct {
	lim  *rate.Limiter
	seen time.Time
}

// NewLimiter allows `perMinute` events per minute with the given burst.
func NewLimiter(perMinute float64, burst int) *Limiter {
	return &Limiter{every: rate.Limit(perMinute / 60), burst: burst, buckets: map[string]*bucket{}}
}

// Allow consumes one event for key.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.buckets) > 10000 {
		for k, b := range l.buckets {
			if now.Sub(b.seen) > 30*time.Minute {
				delete(l.buckets, k)
			}
		}
	}
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{lim: rate.NewLimiter(l.every, l.burst)}
		l.buckets[key] = b
	}
	b.seen = now
	return b.lim.Allow()
}

// Limiters groups the rate limiters of sensitive endpoints.
type Limiters struct {
	Login         *Limiter
	PasswordReset *Limiter
	Register      *Limiter
	AccessToken   *Limiter
}

// NewLimiters returns the default limits.
func NewLimiters() *Limiters {
	return &Limiters{
		Login:         NewLimiter(6, 10),
		PasswordReset: NewLimiter(10, 10),
		Register:      NewLimiter(5, 5),
		AccessToken:   NewLimiter(20, 20),
	}
}

// Check returns ErrRateLimited when the limiter rejects the request.
func (l *Limiter) Check(ctx context.Context, extra string) error {
	if !l.Allow(ClientIPFrom(ctx) + "|" + extra) {
		return apperr.ErrRateLimited
	}
	return nil
}
