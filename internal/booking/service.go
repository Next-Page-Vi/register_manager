package booking

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"register_manager/internal/apperr"
	"register_manager/internal/audit"
	"register_manager/internal/auth"
	"register_manager/internal/paging"
	"register_manager/internal/schedule"
	"register_manager/internal/storage"
	"register_manager/internal/validate"
)

// Service implements availability, booking transactions and schedule
// configuration.
type Service struct {
	DB  *storage.DB
	Now func() time.Time
}

func loadLocation(inst storage.Instance) (*time.Location, error) {
	loc, err := schedule.LoadLocation(inst.Timezone)
	if err != nil {
		return nil, fmt.Errorf("instance %s has invalid time zone %q: %w", inst.ID, inst.Timezone, err)
	}
	return loc, nil
}

func loadCycle(ctx context.Context, q bun.IDB, instanceID string) (schedule.Cycle, error) {
	row, err := storage.GetSchedule(ctx, q, instanceID)
	if err != nil {
		return schedule.Cycle{}, err
	}
	anchor, err := schedule.ParseDate(row.AnchorDate)
	if err != nil {
		return schedule.Cycle{}, err
	}
	return schedule.NewCycleFromJSON(anchor, int(row.CycleLengthDays), []byte(row.CycleDays))
}

func loadOverrides(ctx context.Context, q bun.IDB, instanceID string, from, to schedule.Date) (map[schedule.Date]schedule.DayPlan, error) {
	rows, err := listOverrides(ctx, q, instanceID, from.String(), to.String())
	if err != nil {
		return nil, err
	}
	out := make(map[schedule.Date]schedule.DayPlan, len(rows))
	for _, r := range rows {
		d, err := schedule.ParseDate(r.Date)
		if err != nil {
			return nil, err
		}
		var plan schedule.DayPlan
		if err := plan.UnmarshalJSON([]byte(r.Periods)); err != nil {
			return nil, err
		}
		out[d] = plan
	}
	return out, nil
}

func toAppts(rows []storage.Appointment) []Appt {
	out := make([]Appt, 0, len(rows))
	for _, r := range rows {
		out = append(out, Appt{
			ID: r.ID, CustomerID: r.CustomerID, Status: r.Status,
			Start: storage.FromMillis(r.StartsAt), End: storage.FromMillis(r.EndsAt),
		})
	}
	return out
}

// factsOptions customizes a facts snapshot.
type factsOptions struct {
	customerID *string
	excludeID  *string
	policy     *Policy         // proposed policy for previews
	cycle      *schedule.Cycle // proposed cycle for previews
	// extraDate widens the loaded range to include a specific date (for
	// checking a requested slot outside the horizon).
	extraDate *schedule.Date
}

// loadFacts reads everything the rules need for one instance.
func loadFacts(ctx context.Context, q bun.IDB, inst storage.Instance, now time.Time, o factsOptions) (*Facts, error) {
	loc, err := loadLocation(inst)
	if err != nil {
		return nil, err
	}
	var policy Policy
	if o.policy != nil {
		policy = *o.policy
	} else {
		row, err := storage.GetPolicy(ctx, q, inst.ID)
		if err != nil {
			return nil, err
		}
		policy = PolicyFromRow(row)
	}
	var cycle schedule.Cycle
	if o.cycle != nil {
		cycle = *o.cycle
	} else if cycle, err = loadCycle(ctx, q, inst.ID); err != nil {
		return nil, err
	}
	today := schedule.DateIn(now, loc)
	last := today.AddDays(policy.BookingHorizonDays)
	if o.extraDate != nil && *o.extraDate > last {
		last = *o.extraDate
	}
	from := today.WeekStart().AddDays(-1)
	to := last.WeekStart().AddDays(8)
	overrides, err := loadOverrides(ctx, q, inst.ID, from, to)
	if err != nil {
		return nil, err
	}
	pad := policy.Buffer() + 24*time.Hour
	fromT := schedule.DayStart(loc, from).Add(-pad).UnixMilli()
	toT := schedule.DayStart(loc, to).Add(pad).UnixMilli()
	blockRows, err := listBlocks(ctx, q, inst.ID, fromT, toT)
	if err != nil {
		return nil, err
	}
	blocks := make([]schedule.Interval, 0, len(blockRows))
	for _, b := range blockRows {
		blocks = append(blocks, schedule.Interval{Start: storage.FromMillis(b.StartsAt), End: storage.FromMillis(b.EndsAt)})
	}
	apptRows, err := listOccupying(ctx, q, inst.ID, fromT, toT)
	if err != nil {
		return nil, err
	}
	input := FactsInput{
		Loc: loc, Now: now, Policy: policy,
		Calendar:  schedule.Calendar{Cycle: cycle, Overrides: overrides},
		Blocks:    blocks,
		Appts:     toAppts(apptRows),
		ExcludeID: o.excludeID,
	}
	if o.customerID != nil {
		n, err := countFutureBooked(ctx, q, inst.ID, *o.customerID, now.UnixMilli(), o.excludeID)
		if err != nil {
			return nil, err
		}
		input.Customer = &CustomerContext{ID: *o.customerID, FutureBooked: int(n)}
	}
	return NewFacts(input)
}

// AvailabilityInput adds an optional instance-local date to normal pagination.
type AvailabilityInput struct {
	paging.Input
	Date *string
}

type DateRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func availabilityDate(raw *string, f *Facts) (*schedule.Date, error) {
	if raw == nil {
		return nil, nil
	}
	date, err := schedule.ParseDate(*raw)
	if err != nil || date < f.Today() || date > f.LastDate() {
		return nil, apperr.Invalid("invalid_date")
	}
	return &date, nil
}

// AvailabilityPage is a page of bookable slots.
type AvailabilityPage struct {
	paging.Page[Slot]
	DateRange   DateRange    `json:"date_range"`
	Timezone    string       `json:"timezone"`
	EmptyReason *EmptyReason `json:"empty_reason" enum:"paused,misconfigured,customer_limit,period_limit,full,no_slots_on_date"`
}

func availabilityCursor(cursor *string) (*time.Time, error) {
	if cursor == nil {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339Nano, *cursor)
	if err != nil {
		return nil, apperr.Invalid("invalid_cursor")
	}
	return &value, nil
}

func page(f *Facts, date *schedule.Date, after *time.Time, limit int) AvailabilityPage {
	count := limit + 1
	p := AvailabilityPage{
		DateRange: DateRange{From: f.Today().String(), To: f.LastDate().String()},
		Timezone:  f.input.Loc.String(),
		Page:      paging.New(f.slotsOn(date, after, &count), limit, func(slot Slot) string { return slot.StartsAt.Format(time.RFC3339Nano) }),
	}
	if len(p.Items) == 0 {
		reason := f.emptyReasonOn(date, after)
		p.EmptyReason = &reason
	}
	return p
}

// CustomerAvailability returns the customer's bookable slots. Availability
// is empty while the instance is paused.
func (s *Service) CustomerAvailability(ctx context.Context, cust *auth.Customer, input AvailabilityInput) (AvailabilityPage, error) {
	limit, err := paging.Limit(input.Limit, 20)
	if err != nil {
		return AvailabilityPage{}, err
	}
	after, err := availabilityCursor(input.Cursor)
	if err != nil {
		return AvailabilityPage{}, err
	}
	var p AvailabilityPage
	err = s.DB.ReadTx(ctx, func(tx bun.Tx) error {
		inst, err := storage.GetInstance(ctx, tx, cust.InstanceID)
		if err != nil {
			return err
		}
		f, err := loadFacts(ctx, tx, inst, s.Now(), factsOptions{customerID: &cust.ID})
		if err != nil {
			return err
		}
		date, err := availabilityDate(input.Date, f)
		if err != nil {
			return err
		}
		if inst.Status != "active" {
			p = AvailabilityPage{DateRange: DateRange{From: f.Today().String(), To: f.LastDate().String()}, Timezone: inst.Timezone, Page: paging.Page[Slot]{Items: []Slot{}}, EmptyReason: new(EmptyPaused)}
		} else {
			p = page(f, date, after, limit)
		}
		return nil
	})
	return p, err
}

// AdminAvailability returns slots for booking on behalf of a customer (or,
// without a customer, ignoring per-customer quotas), optionally excluding an
// appointment being rescheduled.
func (s *Service) AdminAvailability(ctx context.Context, actor *auth.Account, instanceID string, customerID, excludeID *string, input AvailabilityInput) (AvailabilityPage, error) {
	limit, err := paging.Limit(input.Limit, 20)
	if err != nil {
		return AvailabilityPage{}, err
	}
	after, err := availabilityCursor(input.Cursor)
	if err != nil {
		return AvailabilityPage{}, err
	}
	var p AvailabilityPage
	err = auth.InstanceRead(ctx, s.DB, actor, instanceID, func(tx bun.Tx, acc *auth.Access) error {
		if excludeID != nil && customerID == nil {
			a, err := getAppt(ctx, tx, instanceID, *excludeID)
			if err != nil {
				return err
			}
			customerID = &a.CustomerID
		}
		f, err := loadFacts(ctx, tx, acc.Instance, s.Now(), factsOptions{customerID: customerID, excludeID: excludeID})
		if err != nil {
			return err
		}
		date, err := availabilityDate(input.Date, f)
		if err != nil {
			return err
		}
		p = page(f, date, after, limit)
		return nil
	})
	return p, err
}

// Appointment is an appointment as returned by the API.
type Appointment struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customer_id"`
	StartsAt   time.Time `json:"starts_at"`
	EndsAt     time.Time `json:"ends_at"`
	Status     string    `json:"status" enum:"booked,cancelled,completed,no_show"`
}

// ManagedAppointment always includes the customer's current display name.
type ManagedAppointment struct {
	Appointment
	CustomerName string `json:"customer_name"`
}

func managedView(ctx context.Context, q bun.IDB, instanceID string, a Appointment) (ManagedAppointment, error) {
	customer, err := storage.GetCustomer(ctx, q, instanceID, a.CustomerID)
	if err != nil {
		return ManagedAppointment{}, err
	}
	return ManagedAppointment{Appointment: a, CustomerName: customer.Name}, nil
}

func apptView(a storage.Appointment) Appointment {
	return Appointment{
		ID: a.ID, CustomerID: a.CustomerID, Status: a.Status,
		StartsAt: storage.FromMillis(a.StartsAt), EndsAt: storage.FromMillis(a.EndsAt),
	}
}

func getAppt(ctx context.Context, q bun.IDB, instanceID, id string) (storage.Appointment, error) {
	a, err := loadAppointment(ctx, q, instanceID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return a, apperr.NotFound("appointment_not_found")
	}
	return a, err
}

// BookInput requests a new appointment. ID is generated by the client when
// the confirmation page is shown, and reused for every retry.
type BookInput struct {
	ID       string
	StartsAt time.Time
	EndsAt   time.Time
}

// checkCustomerBooking applies every current policy to self-service bookings.
func checkCustomerBooking(ctx context.Context, q bun.IDB, inst storage.Instance, customerID string, start, end, now time.Time) error {
	if inst.Status != "active" {
		return apperr.Conflict("instance_paused")
	}
	loc, err := loadLocation(inst)
	if err != nil {
		return err
	}
	d := schedule.DateIn(start, loc)
	f, err := loadFacts(ctx, q, inst, now, factsOptions{customerID: &customerID, extraDate: &d})
	if err != nil {
		return err
	}
	return f.Check(start, end)
}

// checkManagedInterval only protects actual service occupancy. It deliberately
// does not load schedules, blocks, quotas or the customer booking horizon.
func checkManagedInterval(ctx context.Context, q bun.IDB, instanceID string, start, end, now time.Time, excludeID *string) error {
	if start.Before(now) {
		return apperr.Conflict("appointment_in_past")
	}
	query := q.NewSelect().Model((*storage.Appointment)(nil)).
		Where("instance_id = ? AND status <> ?", instanceID, "cancelled").
		Where("ends_at > ? AND starts_at < ?", start.UnixMilli(), end.UnixMilli())
	if excludeID != nil {
		query.Where("id <> ?", *excludeID)
	}
	exists, err := query.Exists(ctx)
	if err != nil {
		return err
	}
	if exists {
		return apperr.Conflict("slot_taken")
	}
	return nil
}

// book runs shared creation and checks new requests after idempotent replay.
func (s *Service) book(ctx context.Context, tx bun.Tx, inst storage.Instance, customerID string, in BookInput, actor audit.Actor, check func(time.Time) error) (Appointment, error) {
	start, end := in.StartsAt.UTC(), in.EndsAt.UTC()
	// Idempotent replay: the same request returns the original appointment
	// in its current state, even if it was cancelled since.
	if existing, err := loadAppointment(ctx, tx, inst.ID, in.ID); err == nil {
		if existing.CustomerID == customerID && existing.StartsAt == start.UnixMilli() && existing.EndsAt == end.UnixMilli() {
			return apptView(existing), nil
		}
		return Appointment{}, apperr.Conflict("appointment_id_conflict")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Appointment{}, err
	}
	if exists, err := tx.NewSelect().
		Model((*storage.Appointment)(nil)).
		Where("id = ?", in.ID).Exists(ctx); err != nil {
		return Appointment{}, err
	} else if exists {
		return Appointment{}, apperr.Conflict("appointment_id_conflict")
	}
	c, err := storage.GetCustomer(ctx, tx, inst.ID, customerID)
	if errors.Is(err, sql.ErrNoRows) {
		return Appointment{}, apperr.NotFound("customer_not_found")
	}
	if err != nil {
		return Appointment{}, err
	}
	if c.Status != "approved" {
		return Appointment{}, apperr.Conflict("customer_not_approved")
	}
	now := s.Now()
	if err := check(now); err != nil {
		return Appointment{}, err
	}
	_, err = tx.NewInsert().
		Model(&storage.Appointment{
			ID:         in.ID,
			InstanceID: inst.ID,
			CustomerID: customerID,
			StartsAt:   start.UnixMilli(),
			EndsAt:     end.UnixMilli(),
			CreatedAt:  now.UnixMilli(),
			UpdatedAt:  now.UnixMilli(),
			Status:     "booked",
		}).
		Exec(ctx)
	if errors.Is(storage.MapError(err), storage.ErrOverlap) {
		return Appointment{}, apperr.Conflict("slot_taken")
	}
	if errors.Is(storage.MapError(err), storage.ErrUnique) {
		return Appointment{}, apperr.Conflict("appointment_id_conflict")
	}
	if err != nil {
		return Appointment{}, err
	}
	v := Appointment{ID: in.ID, CustomerID: customerID, StartsAt: start, EndsAt: end, Status: "booked"}
	return v, audit.Write(ctx, tx, audit.Entry{
		At: now, InstanceID: &inst.ID, Actor: actor, Action: "appointment.create",
		Target: audit.Record("appointment", in.ID), After: v,
	})
}

func normalizeBook(in BookInput) (BookInput, error) {
	id, err := validate.UUID(in.ID, "id")
	if err != nil {
		return in, err
	}
	in.ID = id
	if !in.StartsAt.Before(in.EndsAt) {
		return in, apperr.Invalid("invalid_interval")
	}
	return in, nil
}

// CustomerBook books an appointment for the authenticated customer.
func (s *Service) CustomerBook(ctx context.Context, cust *auth.Customer, in BookInput) (Appointment, error) {
	in, err := normalizeBook(in)
	if err != nil {
		return Appointment{}, err
	}
	var out Appointment
	err = s.DB.WriteTx(ctx, func(tx bun.Tx) error {
		if err := cust.Recheck(ctx, tx); err != nil {
			return err
		}
		inst, err := storage.GetInstance(ctx, tx, cust.InstanceID)
		if err != nil {
			return err
		}
		out, err = s.book(ctx, tx, inst, cust.ID, in, audit.CustomerActor(cust.ID), func(now time.Time) error {
			return checkCustomerBooking(ctx, tx, inst, cust.ID, in.StartsAt, in.EndsAt, now)
		})
		return err
	})
	return out, err
}

// AdminBook books on behalf of a customer.
func (s *Service) AdminBook(ctx context.Context, actor *auth.Account, instanceID, customerID string, in BookInput) (ManagedAppointment, error) {
	in, err := normalizeBook(in)
	if err != nil {
		return ManagedAppointment{}, err
	}
	var out ManagedAppointment
	err = auth.InstanceWrite(ctx, s.DB, actor, instanceID, func(tx bun.Tx, acc *auth.Access) error {
		v, err := s.book(ctx, tx, acc.Instance, customerID, in, audit.AccountActor(actor.ID), func(now time.Time) error {
			return checkManagedInterval(ctx, tx, instanceID, in.StartsAt, in.EndsAt, now, nil)
		})
		if err != nil {
			return err
		}
		out, err = managedView(ctx, tx, instanceID, v)
		return err
	})
	return out, err
}

func (s *Service) setStatus(ctx context.Context, tx bun.Tx, instanceID string, a storage.Appointment, status string, actor audit.Actor) (Appointment, error) {
	now := s.Now()
	if _, err := tx.NewUpdate().
		Model((*storage.Appointment)(nil)).
		Set("status = ?", status).
		Set("updated_at = ?", now.UnixMilli()).
		Where("instance_id = ?", instanceID).
		Where("id = ?", a.ID).
		Exec(ctx); err != nil {
		if errors.Is(storage.MapError(err), storage.ErrOverlap) {
			return Appointment{}, apperr.Conflict("slot_taken")
		}
		return Appointment{}, err
	}
	before := a.Status
	a.Status = status
	return apptView(a), audit.Write(ctx, tx, audit.Entry{
		At: now, InstanceID: &instanceID, Actor: actor, Action: "appointment." + status,
		Target: audit.Record("appointment", a.ID),
		Before: map[string]any{"status": before}, After: map[string]any{"status": status},
	})
}

// CustomerCancel cancels the customer's own appointment before the
// cancellation deadline. Cancelling twice succeeds. Allowed while paused.
func (s *Service) CustomerCancel(ctx context.Context, cust *auth.Customer, id string) (Appointment, error) {
	var out Appointment
	err := s.DB.WriteTx(ctx, func(tx bun.Tx) error {
		if err := cust.Recheck(ctx, tx); err != nil {
			return err
		}
		a, err := getAppt(ctx, tx, cust.InstanceID, id)
		if err != nil {
			return err
		}
		if a.CustomerID != cust.ID {
			return apperr.NotFound("appointment_not_found")
		}
		if a.Status == "cancelled" {
			out = apptView(a)
			return nil
		}
		if a.Status != "booked" {
			return apperr.Conflict("invalid_transition")
		}
		p, err := storage.GetPolicy(ctx, tx, cust.InstanceID)
		if err != nil {
			return err
		}
		deadline := storage.FromMillis(a.StartsAt).Add(-time.Duration(p.MinCancellationNoticeMinutes) * time.Minute)
		if s.Now().After(deadline) {
			return apperr.Conflict("cancellation_too_late").With("min_cancellation_notice_minutes", p.MinCancellationNoticeMinutes)
		}
		out, err = s.setStatus(ctx, tx, cust.InstanceID, a, "cancelled", audit.CustomerActor(cust.ID))
		return err
	})
	return out, err
}

// AdminSetStatus cancels or marks an appointment completed / no-show.
// Reaching the current status again succeeds without change.
func (s *Service) AdminSetStatus(ctx context.Context, actor *auth.Account, instanceID, id, status string) (ManagedAppointment, error) {
	var out ManagedAppointment
	err := auth.InstanceWrite(ctx, s.DB, actor, instanceID, func(tx bun.Tx, _ *auth.Access) error {
		a, err := getAppt(ctx, tx, instanceID, id)
		if err != nil {
			return err
		}
		if a.Status == status {
			out, err = managedView(ctx, tx, instanceID, apptView(a))
			return err
		}
		started := !s.Now().Before(storage.FromMillis(a.StartsAt))
		switch status {
		case "cancelled":
			if a.Status != "booked" {
				return apperr.Conflict("invalid_transition")
			}
		case "completed", "no_show":
			// Allowed from booked once started, and as a correction between
			// completed and no-show.
			if a.Status == "cancelled" || !started {
				return apperr.Conflict("invalid_transition")
			}
		default:
			return apperr.Invalid("invalid_status")
		}
		updated, err := s.setStatus(ctx, tx, instanceID, a, status, audit.AccountActor(actor.ID))
		if err != nil {
			return err
		}
		out, err = managedView(ctx, tx, instanceID, updated)
		return err
	})
	return out, err
}

// RescheduleInput moves an appointment. Both expected endpoints guard against
// concurrent changes, including duration-only edits; a completed retry succeeds.
type RescheduleInput struct {
	ExpectedStartsAt time.Time
	ExpectedEndsAt   time.Time
	StartsAt         time.Time
	EndsAt           time.Time
}

// Reschedule atomically moves a booked appointment; on any failure the
// original appointment is unchanged.
func (s *Service) Reschedule(ctx context.Context, actor *auth.Account, instanceID, id string, in RescheduleInput) (ManagedAppointment, error) {
	if !in.StartsAt.Before(in.EndsAt) {
		return ManagedAppointment{}, apperr.Invalid("invalid_interval")
	}
	var out ManagedAppointment
	err := auth.InstanceWrite(ctx, s.DB, actor, instanceID, func(tx bun.Tx, _ *auth.Access) error {
		a, err := getAppt(ctx, tx, instanceID, id)
		if err != nil {
			return err
		}
		start, end := in.StartsAt.UTC(), in.EndsAt.UTC()
		if a.StartsAt == start.UnixMilli() && a.EndsAt == end.UnixMilli() {
			out, err = managedView(ctx, tx, instanceID, apptView(a))
			return err
		}
		if a.StartsAt != in.ExpectedStartsAt.UnixMilli() || a.EndsAt != in.ExpectedEndsAt.UnixMilli() {
			return apperr.Conflict("appointment_changed")
		}
		if a.Status != "booked" {
			return apperr.Conflict("invalid_transition")
		}
		now := s.Now()
		if err := checkManagedInterval(ctx, tx, instanceID, start, end, now, &a.ID); err != nil {
			return err
		}
		_, err = tx.NewUpdate().
			Model((*storage.Appointment)(nil)).
			Set("starts_at = ?", start.UnixMilli()).
			Set("ends_at = ?", end.UnixMilli()).
			Set("updated_at = ?", now.UnixMilli()).
			Where("instance_id = ?", instanceID).
			Where("id = ?", id).
			Exec(ctx)
		if errors.Is(storage.MapError(err), storage.ErrOverlap) {
			return apperr.Conflict("slot_taken")
		}
		if err != nil {
			return err
		}
		before := apptView(a)
		a.StartsAt, a.EndsAt = start.UnixMilli(), end.UnixMilli()
		out, err = managedView(ctx, tx, instanceID, apptView(a))
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			At: now, InstanceID: &instanceID, Actor: audit.AccountActor(actor.ID), Action: "appointment.reschedule",
			Target: audit.Record("appointment", id), Before: before, After: out,
		})
	})
	return out, err
}

// CustomerAppointments returns the customer's upcoming and recent
// appointments.
func (s *Service) CustomerAppointments(ctx context.Context, cust *auth.Customer) (upcoming, recent []Appointment, err error) {
	err = s.DB.ReadTx(ctx, func(tx bun.Tx) error {
		up, err := customerUpcoming(ctx, tx, cust.InstanceID, cust.ID, s.Now().UnixMilli())
		if err != nil {
			return err
		}
		all, err := storage.CustomerAppointments(ctx, tx, cust.InstanceID, cust.ID, 20)
		if err != nil {
			return err
		}
		upcoming = make([]Appointment, 0, len(up))
		isUp := map[string]bool{}
		for _, a := range up {
			upcoming = append(upcoming, apptView(a))
			isUp[a.ID] = true
		}
		recent = make([]Appointment, 0, len(all))
		for _, a := range all {
			if !isUp[a.ID] {
				recent = append(recent, apptView(a))
			}
		}
		return nil
	})
	return upcoming, recent, err
}

// CustomerAppointment returns one of the customer's appointments.
func (s *Service) CustomerAppointment(ctx context.Context, cust *auth.Customer, id string) (Appointment, error) {
	a, err := getAppt(ctx, s.DB.Read(), cust.InstanceID, id)
	if err != nil {
		return Appointment{}, err
	}
	if a.CustomerID != cust.ID {
		return Appointment{}, apperr.NotFound("appointment_not_found")
	}
	return apptView(a), nil
}

func listRow(a storage.Appointment, customerName string) ManagedAppointment {
	return ManagedAppointment{Appointment: apptView(a), CustomerName: customerName}
}

// todayRange returns the instants bounding the instance-local day of now.
func todayRange(loc *time.Location, now time.Time) (from, to int64) {
	today := schedule.DateIn(now, loc)
	return schedule.DayStart(loc, today).UnixMilli(), schedule.DayStart(loc, today.AddDays(1)).UnixMilli()
}

// AppointmentList is a page of appointments for managers.
type AppointmentList = paging.Page[ManagedAppointment]

func encodeCursor(ms int64, id string) string { return strconv.FormatInt(ms, 10) + "_" + id }

func decodeCursor(c string) (int64, string, error) {
	ms, id, ok := strings.Cut(c, "_")
	if !ok || id == "" {
		return 0, "", apperr.Invalid("invalid_cursor")
	}
	v, err := strconv.ParseInt(ms, 10, 64)
	if err != nil {
		return 0, "", apperr.Invalid("invalid_cursor")
	}
	return v, id, nil
}

// cursorPage makes a page from rows fetched with limit+1: the extra row only
// tells that there is a next page, whose cursor is the last row shown.
func cursorPage(rows []appointmentRow, limit int) AppointmentList {
	items := make([]ManagedAppointment, 0, len(rows))
	for _, row := range rows {
		items = append(items, listRow(row.Appointment, row.CustomerName))
	}
	return paging.New(items, limit, func(a ManagedAppointment) string { return encodeCursor(a.StartsAt.UnixMilli(), a.ID) })
}

// ListAppointments returns appointments for a manager view: "today" (local
// day), "upcoming" (from now, ascending) or "history" (before now,
// descending), paginated by cursor.
func (s *Service) ListAppointments(ctx context.Context, actor *auth.Account, instanceID, view string, input paging.Input) (AppointmentList, error) {
	limit, err := paging.Limit(input.Limit, 50)
	if err != nil {
		return AppointmentList{}, err
	}
	var out AppointmentList
	err = auth.InstanceRead(ctx, s.DB, actor, instanceID, func(tx bun.Tx, acc *auth.Access) error {
		now := s.Now()
		q := appointmentQuery(tx, instanceID).
			Limit(int64(limit + 1))
		switch view {
		case "today":
			loc, err := loadLocation(acc.Instance)
			if err != nil {
				return err
			}
			from, until := todayRange(loc, now)
			q.Where("appointments.ends_at > ? AND appointments.starts_at < ?", from, until)
		case "upcoming":
			q.Where("appointments.starts_at >= ?", now.UnixMilli())
		case "history":
			q.Where("appointments.starts_at < ?", now.UnixMilli())
		default:
			return apperr.Invalid("invalid_view")
		}
		if view == "history" {
			q.Order("appointments.starts_at DESC", "appointments.id DESC")
		} else {
			q.Order("appointments.starts_at", "appointments.id")
		}
		if input.Cursor != nil {
			at, id, err := decodeCursor(*input.Cursor)
			if err != nil {
				return err
			}
			if view == "history" {
				q.Where("(appointments.starts_at, appointments.id) < (?, ?)", at, id)
			} else {
				q.Where("(appointments.starts_at, appointments.id) > (?, ?)", at, id)
			}
		}
		rows := []appointmentRow{}
		if err := q.Scan(ctx, &rows); err != nil {
			return err
		}
		out = cursorPage(rows, limit)
		return nil
	})
	return out, err
}

// Dashboard is the manager home summary.
type Dashboard struct {
	Today            []ManagedAppointment `json:"today"`
	Next             *ManagedAppointment  `json:"next"`
	RemainingToday   int                  `json:"remaining_today"`
	PendingCustomers int                  `json:"pending_customers"`
}

// Dashboard summarizes today for the instance.
func (s *Service) Dashboard(ctx context.Context, actor *auth.Account, instanceID string) (Dashboard, error) {
	var d Dashboard
	err := auth.InstanceRead(ctx, s.DB, actor, instanceID, func(tx bun.Tx, acc *auth.Access) error {
		loc, err := loadLocation(acc.Instance)
		if err != nil {
			return err
		}
		now := s.Now()
		from, to := todayRange(loc, now)
		rows, err := appointmentsBetween(ctx, tx, instanceID, from, to)
		if err != nil {
			return err
		}
		d.Today = make([]ManagedAppointment, 0, len(rows))
		for _, r := range rows {
			d.Today = append(d.Today, listRow(r.Appointment, r.CustomerName))
			if r.Appointment.Status == "booked" && r.Appointment.EndsAt > now.UnixMilli() {
				d.RemainingToday++
			}
		}
		next, err := nextBooked(ctx, tx, instanceID, now.UnixMilli())
		switch {
		case err == nil:
			v := listRow(next.Appointment, next.CustomerName)
			d.Next = &v
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
		n, err := tx.NewSelect().
			Model((*storage.Customer)(nil)).
			Where("instance_id = ? AND status = ?", instanceID, "pending").Count(ctx)
		if err != nil {
			return err
		}
		d.PendingCustomers = int(n)
		return nil
	})
	return d, err
}
