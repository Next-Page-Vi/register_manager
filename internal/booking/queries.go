package booking

import (
	"context"

	"github.com/uptrace/bun"

	"register_manager/internal/storage"
)

// appointmentRow is the shared projection for managed appointment lists.
type appointmentRow struct {
	bun.BaseModel `bun:"table:appointments,alias:appointments"`
	storage.Appointment
	CustomerName string
}

func appointmentQuery(db bun.IDB, instanceID string) *bun.SelectQuery {
	return db.NewSelect().
		Model((*appointmentRow)(nil)).
		ColumnExpr("appointments.*").
		ColumnExpr("c.name AS customer_name").
		Join("JOIN customers AS c ON c.instance_id = appointments.instance_id AND c.id = appointments.customer_id").
		Where("appointments.instance_id = ?", instanceID)
}

func loadAppointment(ctx context.Context, db bun.IDB, instanceID, id string) (storage.Appointment, error) {
	var row storage.Appointment
	err := db.NewSelect().
		Model(&row).
		Where("instance_id = ? AND id = ?", instanceID, id).
		Scan(ctx)
	return row, err
}

func listOccupying(ctx context.Context, db bun.IDB, instanceID string, from, to int64) ([]storage.Appointment, error) {
	rows := []storage.Appointment{}
	err := db.NewSelect().
		Model(&rows).
		Where("instance_id = ? AND status <> ?", instanceID, "cancelled").
		Where("ends_at > ? AND starts_at < ?", from, to).
		Order("starts_at").
		Scan(ctx)
	return rows, err
}

func countFutureBooked(ctx context.Context, db bun.IDB, instanceID, customerID string, now int64, excludeID *string) (int64, error) {
	q := db.NewSelect().
		Model((*storage.Appointment)(nil)).
		Where("instance_id = ? AND customer_id = ? AND status = ? AND starts_at > ?", instanceID, customerID, "booked", now)
	if excludeID != nil {
		q.Where("id <> ?", *excludeID)
	}
	return q.Count(ctx)
}

func listBlocks(ctx context.Context, db bun.IDB, instanceID string, from, to int64) ([]storage.BlockedPeriod, error) {
	rows := []storage.BlockedPeriod{}
	err := db.NewSelect().
		Model(&rows).
		Where("instance_id = ? AND ends_at > ? AND starts_at < ?", instanceID, from, to).
		Order("starts_at").
		Scan(ctx)
	return rows, err
}

func loadBlock(ctx context.Context, db bun.IDB, instanceID, id string) (storage.BlockedPeriod, error) {
	var row storage.BlockedPeriod
	err := db.NewSelect().
		Model(&row).
		Where("instance_id = ? AND id = ?", instanceID, id).
		Scan(ctx)
	return row, err
}

func listOverrides(ctx context.Context, db bun.IDB, instanceID, from, to string) ([]storage.ScheduleOverride, error) {
	rows := []storage.ScheduleOverride{}
	err := db.NewSelect().
		Model(&rows).
		Where("instance_id = ? AND date >= ? AND date <= ?", instanceID, from, to).
		Order("date").
		Scan(ctx)
	return rows, err
}

func loadOverride(ctx context.Context, db bun.IDB, instanceID, date string) (storage.ScheduleOverride, error) {
	var row storage.ScheduleOverride
	err := db.NewSelect().
		Model(&row).
		Where("instance_id = ? AND date = ?", instanceID, date).
		Scan(ctx)
	return row, err
}

func customerUpcoming(ctx context.Context, db bun.IDB, instanceID, customerID string, now int64) ([]storage.Appointment, error) {
	rows := []storage.Appointment{}
	err := db.NewSelect().
		Model(&rows).
		Where("instance_id = ? AND customer_id = ? AND status = ? AND ends_at > ?", instanceID, customerID, "booked", now).
		Order("starts_at").
		Scan(ctx)
	return rows, err
}

func appointmentsBetween(ctx context.Context, db bun.IDB, instanceID string, from, to int64) ([]appointmentRow, error) {
	rows := []appointmentRow{}
	err := appointmentQuery(db, instanceID).
		Where("appointments.ends_at > ? AND appointments.starts_at < ?", from, to).
		Order("appointments.starts_at", "appointments.id").
		Scan(ctx, &rows)
	return rows, err
}

func nextBooked(ctx context.Context, db bun.IDB, instanceID string, now int64) (appointmentRow, error) {
	var row appointmentRow
	err := appointmentQuery(db, instanceID).
		Where("appointments.status = ? AND appointments.starts_at >= ?", "booked", now).
		Order("appointments.starts_at", "appointments.id").
		Limit(1).
		Scan(ctx, &row)
	return row, err
}
