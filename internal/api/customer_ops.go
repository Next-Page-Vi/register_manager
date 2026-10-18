package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"register_manager/internal/apperr"
	"register_manager/internal/booking"
	"register_manager/internal/customers"
	"register_manager/internal/instances"
)

type slugPath struct {
	Slug string `path:"slug"`
}

type registerIn struct {
	Slug string `path:"slug"`
	Body struct {
		ID      string  `json:"id" doc:"Client-generated UUID; reuse it when retrying"`
		Invite  string  `json:"invite" maxLength:"100"`
		Name    string  `json:"name" maxLength:"100"`
		Contact *string `json:"contact,omitempty" nullable:"true" maxLength:"200"`
		Locale  *string `json:"locale,omitempty" nullable:"true" maxLength:"35"`
	}
}

type customerMeOut struct {
	Body struct {
		Customer *customers.CustomerMe `json:"customer"`
	}
}

type customerAppointmentsOut struct {
	Body struct {
		Upcoming []booking.Appointment `json:"upcoming"`
		Recent   []booking.Appointment `json:"recent"`
	}
}

type customerSlugIn struct {
	Slug string `path:"slug"`
}

type customerLocaleIn struct {
	Slug string `path:"slug"`
	Body struct {
		Locale *string `json:"locale" required:"true" nullable:"true" maxLength:"35"`
	}
}

type customerAppointmentPath struct {
	Slug          string `path:"slug"`
	AppointmentID string `path:"appointment_id"`
}

type customerBookIn struct {
	Slug string `path:"slug"`
	Body struct {
		ID       string    `json:"id" doc:"Client-generated UUID created when the confirmation is shown; reuse it when retrying"`
		StartsAt time.Time `json:"starts_at"`
		EndsAt   time.Time `json:"ends_at"`
	}
}

type customerAvailabilityIn struct {
	Slug string `path:"slug"`
	AvailabilityParams
}

func (s *server) registerPublic(a huma.API) {
	const tag = "public"
	huma.Register(a, op(http.MethodGet, "/api/v1/public/instances/{slug}", "getPublicInstance", tag, "Public instance information"),
		func(ctx context.Context, in *slugPath) (*out[instances.PublicInstance], error) {
			return body(s.Instances.Public(ctx, in.Slug))
		})
	huma.Register(a, op(http.MethodPost, "/api/v1/public/instances/{slug}/registrations", "register", tag, "Apply for registration"),
		func(ctx context.Context, in *registerIn) (*out[customers.Registration], error) {
			if err := s.Limiters.Register.Check(ctx, in.Slug); err != nil {
				return nil, err
			}
			b := in.Body
			return body(s.Customers.Register(ctx, in.Slug, customers.RegisterInput{
				ID: b.ID, Invite: b.Invite, Name: b.Name, Contact: b.Contact, Locale: b.Locale,
			}))
		})
}

// registerCustomer adds the customer API. Every path names the instance by
// slug: one browser may be signed in to several instances, and each request
// uses only the identity for the instance in its URL.
func (s *server) registerCustomer(a huma.API) {
	const tag = "customer"
	const base = "/api/v1/customer/{slug}"
	huma.Register(a, op(http.MethodGet, base+"/me", "getCustomerMe", tag, "Current customer of the instance; null when not signed in to it"),
		func(ctx context.Context, in *customerSlugIn) (*customerMeOut, error) {
			out := &customerMeOut{}
			c, _, err := s.customerFor(ctx, in.Slug)
			if apperr.Code(err) == "instance_not_found" {
				return nil, err
			}
			if errors.Is(err, apperr.ErrUnauthenticated) {
				return out, nil
			}
			if err != nil {
				return nil, err
			}
			me, err := s.Customers.Me(ctx, c)
			if err != nil {
				return nil, err
			}
			out.Body.Customer = &me
			return out, nil
		})

	huma.Register(a, noContent(op(http.MethodPut, base+"/locale", "setCustomerLocale", tag, "Set own interface language")),
		func(ctx context.Context, in *customerLocaleIn) (*Empty, error) {
			c, _, err := s.customerFor(ctx, in.Slug)
			if err != nil {
				return nil, err
			}
			return nil, s.Customers.SetLocale(ctx, c, in.Body.Locale)
		})

	huma.Register(a, noContent(op(http.MethodPost, base+"/logout", "customerLogout", tag, "Sign out of this instance only")),
		func(ctx context.Context, in *customerSlugIn) (*Empty, error) {
			_, inst, err := s.customerFor(ctx, in.Slug)
			if apperr.Code(err) == "instance_not_found" {
				return nil, nil
			}
			return nil, s.Sessions.LogoutCustomer(ctx, inst.ID)
		})

	huma.Register(a, op(http.MethodGet, base+"/appointments", "listMyAppointments", tag, "Own upcoming and recent appointments"),
		func(ctx context.Context, in *customerSlugIn) (*customerAppointmentsOut, error) {
			c, _, err := s.customerFor(ctx, in.Slug)
			if err != nil {
				return nil, err
			}
			up, recent, err := s.Booking.CustomerAppointments(ctx, c)
			if err != nil {
				return nil, err
			}
			out := &customerAppointmentsOut{}
			out.Body.Upcoming, out.Body.Recent = up, recent
			return out, nil
		})

	huma.Register(a, op(http.MethodGet, base+"/appointments/{appointment_id}", "getMyAppointment", tag, "One own appointment"),
		func(ctx context.Context, in *customerAppointmentPath) (*out[booking.Appointment], error) {
			c, _, err := s.customerFor(ctx, in.Slug)
			if err != nil {
				return nil, err
			}
			return body(s.Booking.CustomerAppointment(ctx, c, in.AppointmentID))
		})

	huma.Register(a, op(http.MethodPost, base+"/appointments", "book", tag, "Book an appointment"),
		func(ctx context.Context, in *customerBookIn) (*out[booking.Appointment], error) {
			c, _, err := s.customerFor(ctx, in.Slug)
			if err != nil {
				return nil, err
			}
			return body(s.Booking.CustomerBook(ctx, c, booking.BookInput{ID: in.Body.ID, StartsAt: in.Body.StartsAt, EndsAt: in.Body.EndsAt}))
		})

	huma.Register(a, op(http.MethodPost, base+"/appointments/{appointment_id}/cancel", "cancelMyAppointment", tag, "Cancel an own appointment"),
		func(ctx context.Context, in *customerAppointmentPath) (*out[booking.Appointment], error) {
			c, _, err := s.customerFor(ctx, in.Slug)
			if err != nil {
				return nil, err
			}
			return body(s.Booking.CustomerCancel(ctx, c, in.AppointmentID))
		})

	huma.Register(a, op(http.MethodGet, base+"/availability", "myAvailability", tag, "Bookable slots, nearest first"),
		func(ctx context.Context, in *customerAvailabilityIn) (*out[booking.AvailabilityPage], error) {
			c, _, err := s.customerFor(ctx, in.Slug)
			if err != nil {
				return nil, err
			}
			return body(s.Booking.CustomerAvailability(ctx, c, in.Availability()))
		})
}
