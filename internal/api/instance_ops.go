package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"register_manager/internal/audit"
	"register_manager/internal/auth"
	"register_manager/internal/booking"
	"register_manager/internal/customers"
	"register_manager/internal/instances"
	"register_manager/internal/paging"
	"register_manager/internal/schedule"
)

type instancePath struct {
	InstanceID string `path:"instance_id"`
}

type statisticsIn struct {
	InstanceID string                `path:"instance_id"`
	From       string                `query:"from" required:"true" minLength:"10" maxLength:"10" doc:"First instance-local date, YYYY-MM-DD (inclusive)"`
	To         string                `query:"to" required:"true" minLength:"10" maxLength:"10" doc:"Last instance-local date, YYYY-MM-DD (inclusive); at most 366 days"`
	Basis      optionalParam[string] `query:"basis" enum:"starts_at,created_at" doc:"Defaults to starts_at when omitted"`
}

type settingsIn struct {
	InstanceID string `path:"instance_id"`
	Body       struct {
		Name                  string  `json:"name" maxLength:"100"`
		Timezone              string  `json:"timezone" maxLength:"64"`
		DefaultLocale         string  `json:"default_locale" maxLength:"35"`
		ContactKind           string  `json:"contact_kind" enum:"phone,email,text"`
		ContactRequired       bool    `json:"contact_required"`
		BrandColor            *string `json:"brand_color" required:"true" nullable:"true" maxLength:"7" doc:"\"#rrggbb\"; null means the standard look."`
		ConfirmTimezoneChange bool    `json:"confirm_timezone_change,omitempty"`
	}
}

type statusIn struct {
	InstanceID string `path:"instance_id"`
	Body       struct {
		Status string `json:"status" enum:"active,paused"`
	}
}

type customersIn struct {
	InstanceID string                `path:"instance_id"`
	Status     optionalParam[string] `query:"status" enum:"pending,approved,rejected,disabled"`
}

type customersOut struct {
	Body struct {
		Customers []customers.Customer `json:"customers"`
	}
}

type customerPath struct {
	InstanceID string `path:"instance_id"`
	CustomerID string `path:"customer_id"`
}

type customerActionIn struct {
	InstanceID string `path:"instance_id"`
	CustomerID string `path:"customer_id"`
	Action     string `path:"action" enum:"approve,reject,disable,restore,regenerate_link"`
}

type noteIn struct {
	InstanceID string `path:"instance_id"`
	CustomerID string `path:"customer_id"`
	Body       struct {
		Note *string `json:"note" required:"true" nullable:"true" maxLength:"2000"`
	}
}

type appointmentsIn struct {
	InstanceID string `path:"instance_id"`
	View       string `query:"view" enum:"today,upcoming,history" default:"today"`
	PageParams
}

type adminBookIn struct {
	InstanceID string `path:"instance_id"`
	Body       struct {
		ID         string    `json:"id" doc:"Client-generated UUID; reuse it when retrying"`
		CustomerID string    `json:"customer_id"`
		StartsAt   time.Time `json:"starts_at"`
		EndsAt     time.Time `json:"ends_at"`
	}
}

type appointmentStatusIn struct {
	InstanceID    string `path:"instance_id"`
	AppointmentID string `path:"appointment_id"`
	Body          struct {
		Status string `json:"status" enum:"cancelled,completed,no_show"`
	}
}

type rescheduleIn struct {
	InstanceID    string `path:"instance_id"`
	AppointmentID string `path:"appointment_id"`
	Body          struct {
		ExpectedStartsAt time.Time `json:"expected_starts_at"`
		ExpectedEndsAt   time.Time `json:"expected_ends_at"`
		StartsAt         time.Time `json:"starts_at"`
		EndsAt           time.Time `json:"ends_at"`
	}
}

type adminAvailabilityIn struct {
	InstanceID           string                `path:"instance_id"`
	CustomerID           optionalParam[string] `query:"customer_id"`
	ExcludeAppointmentID optionalParam[string] `query:"exclude_appointment_id"`
	AvailabilityParams
}

type cycleIn struct {
	InstanceID string `path:"instance_id"`
	Body       booking.CycleView
}

type overridesIn struct {
	InstanceID string `path:"instance_id"`
	From       string `query:"from" doc:"YYYY-MM-DD"`
	To         string `query:"to" doc:"YYYY-MM-DD"`
}

type overridesOut struct {
	Body struct {
		Overrides []booking.OverrideView `json:"overrides"`
	}
}

type overridePath struct {
	InstanceID string `path:"instance_id"`
	Date       string `path:"date" doc:"YYYY-MM-DD"`
}

type putOverrideIn struct {
	InstanceID string `path:"instance_id"`
	Date       string `path:"date" doc:"YYYY-MM-DD"`
	Body       struct {
		Periods []schedule.Period `json:"periods" doc:"Empty list means a day off"`
	}
}

type blocksOut struct {
	Body struct {
		Blocks []booking.BlockView `json:"blocks"`
	}
}

type createBlockIn struct {
	InstanceID string `path:"instance_id"`
	Body       struct {
		StartsAt time.Time `json:"starts_at"`
		EndsAt   time.Time `json:"ends_at"`
		Reason   *string   `json:"reason,omitempty" nullable:"true" maxLength:"200"`
	}
}

type blockPath struct {
	InstanceID string `path:"instance_id"`
	BlockID    string `path:"block_id"`
}

type policyIn struct {
	InstanceID string `path:"instance_id"`
	Body       booking.Policy
}

type instanceAuditIn struct {
	InstanceID string `path:"instance_id"`
	PageParams
}

func (s *server) registerInstance(a huma.API) {
	const tag = "instance"
	const base = "/api/v1/instances/{instance_id}"

	huma.Register(a, op(http.MethodGet, "/api/v1/instances/by-slug/{slug}", "getInstanceBySlug", tag, "Resolve an instance by slug for managers"),
		as(account, func(ctx context.Context, actor *auth.Account, in *struct {
			Slug string `path:"slug"`
		}) (*out[instances.InstanceAccess], error) {
			access, err := s.Instances.AccessBySlug(ctx, actor, in.Slug)
			if err != nil {
				return nil, err
			}
			if access.Role == auth.RoleSuper && !s.Sessions.InstanceAudited(ctx, access.Instance.ID) {
				if err := s.Instances.RecordSuperAdminEntry(ctx, actor, access.Instance.ID); err != nil {
					return nil, err
				}
				s.Sessions.MarkInstanceAudited(ctx, access.Instance.ID)
			}
			return body(access, nil)
		}))
	huma.Register(a, op(http.MethodGet, base, "getInstance", tag, "Get instance settings"),
		as(account, func(ctx context.Context, actor *auth.Account, in *instancePath) (*out[instances.InstanceAccess], error) {
			return body(s.Instances.Access(ctx, actor, in.InstanceID))
		}))

	huma.Register(a, op(http.MethodPut, base, "updateInstance", tag, "Update instance settings"),
		as(account, func(ctx context.Context, actor *auth.Account, in *settingsIn) (*out[instances.Instance], error) {
			b := in.Body
			return body(s.Instances.UpdateSettings(ctx, actor, in.InstanceID, instances.SettingsInput{
				Name: b.Name, Timezone: b.Timezone, DefaultLocale: b.DefaultLocale,
				ContactKind: b.ContactKind, ContactRequired: b.ContactRequired, BrandColor: b.BrandColor,
				ConfirmTimezoneChange: b.ConfirmTimezoneChange,
			}))
		}))

	huma.Register(a, op(http.MethodPut, base+"/status", "setInstanceStatus", tag, "Activate or pause the instance"),
		as(account, func(ctx context.Context, actor *auth.Account, in *statusIn) (*out[instances.Instance], error) {
			return body(s.Instances.SetStatus(ctx, actor, in.InstanceID, in.Body.Status))
		}))

	huma.Register(a, op(http.MethodGet, base+"/invite", "getInvite", tag, "Get the registration invite code"),
		as(account, func(ctx context.Context, actor *auth.Account, in *instancePath) (*out[instances.Invite], error) {
			return body(s.Instances.Invite(ctx, actor, in.InstanceID))
		}))
	huma.Register(a, op(http.MethodPost, base+"/invite/rotate", "rotateInvite", tag, "Replace the invite code"),
		as(account, func(ctx context.Context, actor *auth.Account, in *instancePath) (*out[instances.Invite], error) {
			return body(s.Instances.RotateInvite(ctx, actor, in.InstanceID))
		}))
	huma.Register(a, op(http.MethodGet, base+"/dashboard", "getDashboard", tag, "Today's summary"),
		as(account, func(ctx context.Context, actor *auth.Account, in *instancePath) (*out[booking.Dashboard], error) {
			return body(s.Booking.Dashboard(ctx, actor, in.InstanceID))
		}))

	huma.Register(a, op(http.MethodGet, base+"/statistics", "getStatistics", tag, "Appointment statistics using current statuses"),
		as(account, func(ctx context.Context, actor *auth.Account, in *statisticsIn) (*out[booking.Statistics], error) {
			basis := "starts_at"
			if in.Basis.value != nil {
				basis = *in.Basis.value
			}
			return body(s.Booking.Statistics(ctx, actor, in.InstanceID, booking.StatisticsInput{From: in.From, To: in.To, Basis: basis}))
		}))

	huma.Register(a, op(http.MethodGet, base+"/customers", "listCustomers", tag, "List customers"),
		as(account, func(ctx context.Context, actor *auth.Account, in *customersIn) (*customersOut, error) {
			list, err := s.Customers.List(ctx, actor, in.InstanceID, in.Status.value)
			if err != nil {
				return nil, err
			}
			o := &customersOut{}
			o.Body.Customers = list
			return o, nil
		}))

	huma.Register(a, op(http.MethodGet, base+"/customers/{customer_id}", "getCustomer", tag, "Customer with appointment history"),
		as(account, func(ctx context.Context, actor *auth.Account, in *customerPath) (*out[customers.CustomerDetail], error) {
			return body(s.Customers.Get(ctx, actor, in.InstanceID, in.CustomerID))
		}))

	huma.Register(a, op(http.MethodPost, base+"/customers/{customer_id}/actions/{action}", "customerAction", tag, "Approve, reject, disable, restore or regenerate the link"),
		as(account, func(ctx context.Context, actor *auth.Account, in *customerActionIn) (*out[customers.ActionResult], error) {
			return body(s.Customers.Transition(ctx, actor, in.InstanceID, in.CustomerID, customers.Action(in.Action)))
		}))

	huma.Register(a, op(http.MethodPut, base+"/customers/{customer_id}/note", "setCustomerNote", tag, "Set the internal note"),
		as(account, func(ctx context.Context, actor *auth.Account, in *noteIn) (*out[customers.Customer], error) {
			return body(s.Customers.SetNote(ctx, actor, in.InstanceID, in.CustomerID, in.Body.Note))
		}))

	huma.Register(a, op(http.MethodGet, base+"/appointments", "listAppointments", tag, "List appointments"),
		as(account, func(ctx context.Context, actor *auth.Account, in *appointmentsIn) (*out[booking.AppointmentList], error) {
			return body(s.Booking.ListAppointments(ctx, actor, in.InstanceID, in.View, in.Page()))
		}))

	huma.Register(a, op(http.MethodPost, base+"/appointments", "adminBook", tag, "Book on behalf of a customer"),
		as(account, func(ctx context.Context, actor *auth.Account, in *adminBookIn) (*out[booking.ManagedAppointment], error) {
			b := in.Body
			return body(s.Booking.AdminBook(ctx, actor, in.InstanceID, b.CustomerID, booking.BookInput{ID: b.ID, StartsAt: b.StartsAt, EndsAt: b.EndsAt}))
		}))

	huma.Register(a, op(http.MethodPost, base+"/appointments/{appointment_id}/status", "setAppointmentStatus", tag, "Cancel or mark completed / no-show"),
		as(account, func(ctx context.Context, actor *auth.Account, in *appointmentStatusIn) (*out[booking.ManagedAppointment], error) {
			return body(s.Booking.AdminSetStatus(ctx, actor, in.InstanceID, in.AppointmentID, in.Body.Status))
		}))

	huma.Register(a, op(http.MethodPost, base+"/appointments/{appointment_id}/reschedule", "rescheduleAppointment", tag, "Move an appointment atomically"),
		as(account, func(ctx context.Context, actor *auth.Account, in *rescheduleIn) (*out[booking.ManagedAppointment], error) {
			b := in.Body
			return body(s.Booking.Reschedule(ctx, actor, in.InstanceID, in.AppointmentID, booking.RescheduleInput{
				ExpectedStartsAt: b.ExpectedStartsAt, ExpectedEndsAt: b.ExpectedEndsAt,
				StartsAt: b.StartsAt, EndsAt: b.EndsAt,
			}))
		}))

	huma.Register(a, op(http.MethodGet, base+"/availability", "adminAvailability", tag, "Bookable slots"),
		as(account, func(ctx context.Context, actor *auth.Account, in *adminAvailabilityIn) (*out[booking.AvailabilityPage], error) {
			return body(s.Booking.AdminAvailability(ctx, actor, in.InstanceID, in.CustomerID.value, in.ExcludeAppointmentID.value, in.Availability()))
		}))

	huma.Register(a, op(http.MethodGet, base+"/schedule", "getSchedule", tag, "Get the repeating schedule"),
		as(account, func(ctx context.Context, actor *auth.Account, in *instancePath) (*out[booking.CycleState], error) {
			return body(s.Booking.GetCycle(ctx, actor, in.InstanceID))
		}))

	huma.Register(a, op(http.MethodPut, base+"/schedule", "saveSchedule", tag, "Replace the repeating schedule"),
		as(account, func(ctx context.Context, actor *auth.Account, in *cycleIn) (*out[booking.CycleState], error) {
			return body(s.Booking.SaveCycle(ctx, actor, in.InstanceID, in.Body))
		}))

	huma.Register(a, op(http.MethodPost, base+"/schedule/preview", "previewSchedule", tag, "Preview a proposed schedule"),
		as(account, func(ctx context.Context, actor *auth.Account, in *cycleIn) (*out[booking.Preview], error) {
			return body(s.Booking.PreviewCycle(ctx, actor, in.InstanceID, in.Body))
		}))

	huma.Register(a, op(http.MethodGet, base+"/overrides", "listOverrides", tag, "List date overrides"),
		as(account, func(ctx context.Context, actor *auth.Account, in *overridesIn) (*overridesOut, error) {
			list, err := s.Booking.ListOverrides(ctx, actor, in.InstanceID, in.From, in.To)
			if err != nil {
				return nil, err
			}
			o := &overridesOut{}
			o.Body.Overrides = list
			return o, nil
		}))

	huma.Register(a, op(http.MethodPut, base+"/overrides/{date}", "putOverride", tag, "Set a date override"),
		as(account, func(ctx context.Context, actor *auth.Account, in *putOverrideIn) (*out[booking.OverrideView], error) {
			return body(s.Booking.PutOverride(ctx, actor, in.InstanceID, in.Date, in.Body.Periods))
		}))

	huma.Register(a, noContent(op(http.MethodDelete, base+"/overrides/{date}", "deleteOverride", tag, "Remove a date override")),
		as(account, func(ctx context.Context, actor *auth.Account, in *overridePath) (*Empty, error) {
			return nil, s.Booking.DeleteOverride(ctx, actor, in.InstanceID, in.Date)
		}))

	huma.Register(a, op(http.MethodGet, base+"/blocks", "listBlocks", tag, "List temporary blocks"),
		as(account, func(ctx context.Context, actor *auth.Account, in *instancePath) (*blocksOut, error) {
			list, err := s.Booking.ListBlocks(ctx, actor, in.InstanceID)
			if err != nil {
				return nil, err
			}
			o := &blocksOut{}
			o.Body.Blocks = list
			return o, nil
		}))

	huma.Register(a, op(http.MethodPost, base+"/blocks", "createBlock", tag, "Add a temporary block"),
		as(account, func(ctx context.Context, actor *auth.Account, in *createBlockIn) (*out[booking.BlockView], error) {
			return body(s.Booking.CreateBlock(ctx, actor, in.InstanceID, in.Body.StartsAt, in.Body.EndsAt, in.Body.Reason))
		}))

	huma.Register(a, noContent(op(http.MethodDelete, base+"/blocks/{block_id}", "deleteBlock", tag, "Remove a block")),
		as(account, func(ctx context.Context, actor *auth.Account, in *blockPath) (*Empty, error) {
			return nil, s.Booking.DeleteBlock(ctx, actor, in.InstanceID, in.BlockID)
		}))

	huma.Register(a, op(http.MethodGet, base+"/policy", "getPolicy", tag, "Get booking rules"),
		as(account, func(ctx context.Context, actor *auth.Account, in *instancePath) (*out[booking.Policy], error) {
			return body(s.Booking.GetPolicy(ctx, actor, in.InstanceID))
		}))

	huma.Register(a, op(http.MethodPut, base+"/policy", "savePolicy", tag, "Save booking rules"),
		as(account, func(ctx context.Context, actor *auth.Account, in *policyIn) (*out[booking.PolicySaved], error) {
			return body(s.Booking.SavePolicy(ctx, actor, in.InstanceID, in.Body))
		}))

	huma.Register(a, op(http.MethodPost, base+"/policy/preview", "previewPolicy", tag, "Preview proposed booking rules"),
		as(account, func(ctx context.Context, actor *auth.Account, in *policyIn) (*out[booking.Preview], error) {
			return body(s.Booking.PreviewPolicy(ctx, actor, in.InstanceID, in.Body))
		}))

	huma.Register(a, op(http.MethodGet, base+"/audit", "listInstanceAudit", tag, "Instance audit log"),
		as(account, func(ctx context.Context, actor *auth.Account, in *instanceAuditIn) (*out[paging.Page[audit.LogEntry]], error) {
			return body(s.Audit.List(ctx, actor, &in.InstanceID, in.Page()))
		}))
}
