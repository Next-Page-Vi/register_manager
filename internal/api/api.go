// Package api exposes the services as a REST/JSON API described by OpenAPI
// 3.1 (via Huma), plus the customer access-link entry point.
package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"register_manager/internal/accounts"
	"register_manager/internal/apperr"
	"register_manager/internal/audit"
	"register_manager/internal/auth"
	"register_manager/internal/booking"
	"register_manager/internal/customers"
	"register_manager/internal/instances"
	"register_manager/internal/settings"
	"register_manager/internal/storage"
)

// Deps are the services the API needs.
type Deps struct {
	DB        *storage.DB
	Audit     *audit.Service
	Accounts  *accounts.Service
	Instances *instances.Service
	Customers *customers.Service
	Booking   *booking.Service
	Settings  *settings.Service
	Sessions  *auth.Sessions
	Limiters  *auth.Limiters
	Log       *slog.Logger
	Now       func() time.Time
}

type server struct{ Deps }

var log = slog.Default()

func init() {
	// Services always return empty slices rather than nil.
	huma.DefaultArrayNullable = false
	// Render every Huma-generated error (validation, parsing, unexpected)
	// as an apperr.Error with a stable code.
	huma.NewError = newError
	huma.NewErrorWithContext = func(_ huma.Context, status int, msg string, errs ...error) huma.StatusError {
		return newError(status, msg, errs...)
	}
}

func newError(status int, msg string, errs ...error) huma.StatusError {
	code := "error"
	switch {
	case status == http.StatusUnprocessableEntity || status == http.StatusBadRequest:
		code = "validation_failed"
	case status == http.StatusNotFound:
		code = "not_found"
	case status == http.StatusRequestEntityTooLarge:
		code = "request_too_large"
	case status == http.StatusUnsupportedMediaType:
		code = "unsupported_media_type"
	case status == http.StatusMethodNotAllowed:
		code = "method_not_allowed"
	case status >= 500:
		code = "internal_error"
		var nonNil []any
		for _, e := range errs {
			if e != nil {
				nonNil = append(nonNil, "err", e)
			}
		}
		log.Error("internal error: "+msg, nonNil...)
		return &apperr.Error{Status: status, Code: code}
	}
	var fields []apperr.FieldError
	for _, e := range errs {
		var d huma.ErrorDetailer
		if errors.As(e, &d) {
			det := d.ErrorDetail()
			fields = append(fields, apperr.FieldError{Location: det.Location, Code: "invalid", Message: det.Message})
		}
	}
	ae := &apperr.Error{Status: status, Code: code}
	if len(fields) > 0 {
		ae.Details = map[string]any{"fields": fields}
	}
	return ae
}

// Config returns the Huma configuration.
func Config() huma.Config {
	cfg := huma.DefaultConfig("register_manager API", "1.0.0")
	cfg.Info.Description = "Self-hosted multi-instance booking system. Errors are RFC 9457 problem details with a stable `code`."
	cfg.Info.License = &huma.License{Name: "MIT", Identifier: "MIT"}
	cfg.OpenAPIPath = "/api/openapi"
	cfg.DocsPath = ""
	cfg.SchemasPath = ""
	cfg.CreateHooks = nil
	return cfg
}

// Register adds all operations to router.
func Register(r chi.Router, d Deps) huma.API {
	if d.Log != nil {
		log = d.Log
	}
	a := humachi.New(r, Config())
	s := &server{Deps: d}
	s.registerAccount(a)
	s.registerPlatform(a)
	s.registerInstance(a)
	s.registerPublic(a)
	s.registerCustomer(a)
	return a
}

// OpenAPI returns the API description without needing any service.
func OpenAPI() *huma.OpenAPI {
	return Register(chi.NewRouter(), Deps{}).OpenAPI()
}

func op(method, path, id, tag, summary string) huma.Operation {
	status := http.StatusOK
	return huma.Operation{
		OperationID: id, Method: method, Path: path, Tags: []string{tag},
		Summary: summary, DefaultStatus: status,
	}
}

func noContent(o huma.Operation) huma.Operation {
	o.DefaultStatus = http.StatusNoContent
	return o
}

// Empty is a response without a body.
type Empty struct{}

// out is a response whose body is a named type, so the schema keeps that
// type's name. (Anonymous bodies take their schema name from the wrapper
// type, so those responses keep their own wrapper types.)
type out[T any] struct{ Body T }

// body turns a service result into a response.
func body[T any](v T, err error) (*out[T], error) {
	if err != nil {
		return nil, err
	}
	return &out[T]{Body: v}, nil
}

// as builds a handler that first resolves the caller with who (account,
// superAdmin or signedIn) and then runs fn.
func as[I, O any](who func(context.Context) (*auth.Account, error), fn func(context.Context, *auth.Account, *I) (*O, error)) func(context.Context, *I) (*O, error) {
	return func(ctx context.Context, in *I) (*O, error) {
		a, err := who(ctx)
		if err != nil {
			return nil, err
		}
		return fn(ctx, a, in)
	}
}

// AccessLinkHandler handles /i/{slug}/access?t=TOKEN: it validates the
// token, starts a customer session and redirects to a URL without the token.
// Failures redirect to a frontend page; the server renders no text.
func (d Deps) AccessLinkHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	slug := chi.URLParam(r, "slug")
	ctx := r.Context()
	fail := func(reason string) {
		http.Redirect(w, r, "/i/"+slug+"/link-invalid?reason="+reason, http.StatusSeeOther)
	}
	if err := d.Limiters.AccessToken.Check(ctx, "access"); err != nil {
		fail("rate_limited")
		return
	}
	token := r.URL.Query().Get("t")
	if token == "" || len(token) > 200 {
		fail("link_invalid")
		return
	}
	c, err := d.Customers.AuthenticateLink(ctx, slug, token)
	if err != nil {
		if apperr.Code(err) == "" {
			d.Log.Error("access link", "err", err)
			fail("error")
			return
		}
		fail("link_invalid")
		return
	}
	if err := d.Sessions.LoginCustomer(ctx, c.InstanceID, c.ID, c.AuthVersion); err != nil {
		d.Log.Error("start customer session", "err", err)
		fail("error")
		return
	}
	http.Redirect(w, r, "/i/"+slug+"/booking", http.StatusSeeOther)
}

// SecurityHeaders sets headers common to every response.
func SecurityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data: blob:; font-src 'self'; connect-src 'self'; object-src 'none'; " +
		"base-uri 'none'; frame-ancestors 'none'; form-action 'self'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		}
		next.ServeHTTP(w, r)
	})
}

// RequestLog logs requests by path only; query strings may hold tokens and
// are never logged.
func RequestLog(l *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			defer func() {
				if p := recover(); p != nil {
					l.Error("panic", "path", r.URL.Path, "panic", p)
					if !sw.wrote {
						http.Error(w, "internal error", http.StatusInternalServerError)
					}
				}
				if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/i/") {
					l.Info("request", "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", time.Since(start).Milliseconds())
				}
			}()
			next.ServeHTTP(sw, r)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wrote {
		w.status = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

var errPasswordChangeRequired = apperr.Forbidden("password_change_required")

// account returns the authenticated account principal. Until the super admin
// has replaced its generated initial password, only signedIn endpoints (me,
// sign-out, language, change password) are available.
func account(ctx context.Context) (*auth.Account, error) {
	a, err := auth.AccountFrom(ctx)
	if err != nil {
		return nil, err
	}
	if a.MustChangePassword {
		return nil, errPasswordChangeRequired
	}
	return a, nil
}

// signedIn returns the authenticated account even if it must still change
// its password.
func signedIn(ctx context.Context) (*auth.Account, error) { return auth.AccountFrom(ctx) }

func superAdmin(ctx context.Context) (*auth.Account, error) {
	a, err := account(ctx)
	if err != nil {
		return nil, err
	}
	if !a.SuperAdmin {
		return nil, apperr.ErrForbidden
	}
	return a, nil
}

// customerFor returns the customer signed in for the instance with this
// slug. A browser may hold identities for several instances; each request
// uses only the one for the instance in its URL.
func (s *server) customerFor(ctx context.Context, slug string) (*auth.Customer, storage.Instance, error) {
	q := s.DB.Read()
	inst, err := s.Instances.BySlug(ctx, slug)
	if err != nil {
		return nil, inst, err
	}
	c, err := s.Sessions.CustomerFor(ctx, q, inst.ID)
	return c, inst, err
}
