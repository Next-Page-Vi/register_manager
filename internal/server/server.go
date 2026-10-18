// Package server assembles services, middleware and routes into the HTTP
// handler.
package server

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"register_manager/internal/accounts"
	"register_manager/internal/api"
	"register_manager/internal/audit"
	"register_manager/internal/auth"
	"register_manager/internal/booking"
	"register_manager/internal/customers"
	"register_manager/internal/instances"
	"register_manager/internal/settings"
	"register_manager/internal/storage"
	"register_manager/internal/webui"
)

// Options configure the handler.
type Options struct {
	DB                 *storage.DB
	Log                *slog.Logger
	CookieSecure       bool
	TrustedProxyHeader string
	Frontend           fs.FS
	Now                func() time.Time
}

// New builds the HTTP handler. ctx bounds background goroutines such as
// session cleanup.
func New(ctx context.Context, o Options) (http.Handler, api.Deps) {
	if o.Now == nil {
		o.Now = time.Now
	}
	now := func() time.Time { return o.Now().UTC() }
	deps := api.Deps{
		DB:        o.DB,
		Audit:     &audit.Service{DB: o.DB},
		Accounts:  &accounts.Service{DB: o.DB, Now: now},
		Instances: &instances.Service{DB: o.DB, Now: now},
		Customers: &customers.Service{DB: o.DB, Now: now},
		Booking:   &booking.Service{DB: o.DB, Now: now},
		Settings:  &settings.Service{DB: o.DB, Now: now},
		Sessions:  auth.NewSessions(ctx, o.DB, o.CookieSecure, o.Log),
		Limiters:  auth.NewLimiters(),
		Log:       o.Log,
		Now:       now,
	}

	r := chi.NewRouter()
	r.Use(api.RequestLog(o.Log))
	r.Use(api.SecurityHeaders)
	r.Use(auth.ClientIP(o.TrustedProxyHeader))
	r.Use(http.NewCrossOriginProtection().Handler)
	r.Use(auth.CSRF(o.CookieSecure))
	r.Use(deps.Sessions.Middleware)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := o.DB.R.PingContext(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/i/{slug}/access", deps.AccessLinkHandler)
	api.Register(r, deps)
	if o.Frontend != nil {
		ui := webui.Handler(o.Frontend)
		r.NotFound(ui.ServeHTTP)
		r.MethodNotAllowed(ui.ServeHTTP)
	}
	return r, deps
}
