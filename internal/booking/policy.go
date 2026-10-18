package booking

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/uptrace/bun"

	"register_manager/internal/apperr"
	"register_manager/internal/storage"
)

// policyFields owns the numeric constraints used by services and OpenAPI.
// The frontend derives its input constraints from that same schema.
var policyFields = []struct {
	name     string
	min, max int
	value    func(*Policy) *int
}{
	{"slot_step_minutes", 1, 1440, func(p *Policy) *int { return &p.SlotStepMinutes }},
	{"service_duration_minutes", 1, 1440, func(p *Policy) *int { return &p.ServiceDurationMinutes }},
	{"buffer_minutes", 0, 1440, func(p *Policy) *int { return &p.BufferMinutes }},
	{"daily_customer_limit", 0, 10000, func(p *Policy) *int { return p.DailyCustomerLimit }},
	{"per_customer_daily_limit", 0, 10000, func(p *Policy) *int { return p.PerCustomerDailyLimit }},
	{"per_customer_weekly_limit", 0, 10000, func(p *Policy) *int { return p.PerCustomerWeeklyLimit }},
	{"max_future_bookings", 0, 10000, func(p *Policy) *int { return p.MaxFutureBookings }},
	{"booking_horizon_days", 0, 366, func(p *Policy) *int { return &p.BookingHorizonDays }},
	{"min_booking_notice_minutes", 0, 525600, func(p *Policy) *int { return &p.MinBookingNoticeMinutes }},
	{"min_cancellation_notice_minutes", 0, 525600, func(p *Policy) *int { return &p.MinCancellationNoticeMinutes }},
}

func ValidatePolicy(p Policy) error {
	for _, field := range policyFields {
		if value := field.value(&p); value != nil && (*value < field.min || *value > field.max) {
			return apperr.Invalid("invalid_policy").With("field", field.name)
		}
	}
	return nil
}

func (Policy) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	for _, field := range policyFields {
		min, max := float64(field.min), float64(field.max)
		schema.Properties[field.name].Minimum = &min
		schema.Properties[field.name].Maximum = &max
	}
	return schema
}

// StorePolicy is the complete mapping shared by initialization and updates.
// The caller owns the transaction, authorization and audit record.
func StorePolicy(ctx context.Context, q bun.IDB, instanceID string, p Policy, at int64) error {
	if err := ValidatePolicy(p); err != nil {
		return err
	}
	_, writeErr := q.NewInsert().
		Model(&storage.BookingPolicy{
			InstanceID:                   instanceID,
			UpdatedAt:                    at,
			SlotStepMinutes:              int64(p.SlotStepMinutes),
			ServiceDurationMinutes:       int64(p.ServiceDurationMinutes),
			BufferMinutes:                int64(p.BufferMinutes),
			BookingHorizonDays:           int64(p.BookingHorizonDays),
			DailyCustomerLimit:           p.DailyCustomerLimit,
			PerCustomerDailyLimit:        p.PerCustomerDailyLimit,
			PerCustomerWeeklyLimit:       p.PerCustomerWeeklyLimit,
			MaxFutureBookings:            p.MaxFutureBookings,
			MinBookingNoticeMinutes:      int64(p.MinBookingNoticeMinutes),
			MinCancellationNoticeMinutes: int64(p.MinCancellationNoticeMinutes),
		}).
		On("CONFLICT (instance_id) DO UPDATE").
		Set("updated_at = EXCLUDED.updated_at").
		Set("slot_step_minutes = EXCLUDED.slot_step_minutes").
		Set("service_duration_minutes = EXCLUDED.service_duration_minutes").
		Set("buffer_minutes = EXCLUDED.buffer_minutes").
		Set("booking_horizon_days = EXCLUDED.booking_horizon_days").
		Set("daily_customer_limit = EXCLUDED.daily_customer_limit").
		Set("per_customer_daily_limit = EXCLUDED.per_customer_daily_limit").
		Set("per_customer_weekly_limit = EXCLUDED.per_customer_weekly_limit").
		Set("max_future_bookings = EXCLUDED.max_future_bookings").
		Set("min_booking_notice_minutes = EXCLUDED.min_booking_notice_minutes").
		Set("min_cancellation_notice_minutes = EXCLUDED.min_cancellation_notice_minutes").
		Exec(ctx)
	return storage.MapError(writeErr)
}

// PolicyFromRow converts a stored policy.
func PolicyFromRow(p storage.BookingPolicy) Policy {
	return Policy{
		SlotStepMinutes:              int(p.SlotStepMinutes),
		ServiceDurationMinutes:       int(p.ServiceDurationMinutes),
		BufferMinutes:                int(p.BufferMinutes),
		DailyCustomerLimit:           p.DailyCustomerLimit,
		PerCustomerDailyLimit:        p.PerCustomerDailyLimit,
		PerCustomerWeeklyLimit:       p.PerCustomerWeeklyLimit,
		MaxFutureBookings:            p.MaxFutureBookings,
		BookingHorizonDays:           int(p.BookingHorizonDays),
		MinBookingNoticeMinutes:      int(p.MinBookingNoticeMinutes),
		MinCancellationNoticeMinutes: int(p.MinCancellationNoticeMinutes),
	}
}
