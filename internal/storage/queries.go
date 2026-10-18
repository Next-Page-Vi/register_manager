package storage

import (
	"context"

	"github.com/uptrace/bun"
)

func GetAccount(ctx context.Context, db bun.IDB, id string) (Account, error) {
	var row Account
	err := db.NewSelect().
		Model(&row).
		Where("id = ?", id).
		Scan(ctx)
	return row, err
}

func GetAccountByLogin(ctx context.Context, db bun.IDB, login string) (Account, error) {
	var row Account
	err := db.NewSelect().
		Model(&row).
		Where("login = ?", login).
		Scan(ctx)
	return row, err
}

func GetSuperAdmin(ctx context.Context, db bun.IDB) (Account, error) {
	var row Account
	err := db.NewSelect().
		Model(&row).
		Where("super_admin = 1").
		Scan(ctx)
	return row, err
}

func GetInstance(ctx context.Context, db bun.IDB, id string) (Instance, error) {
	var row Instance
	err := db.NewSelect().
		Model(&row).
		Where("id = ?", id).
		Scan(ctx)
	return row, err
}

func GetInstanceBySlug(ctx context.Context, db bun.IDB, slug string) (Instance, error) {
	var row Instance
	err := db.NewSelect().
		Model(&row).
		Where("slug = ?", slug).
		Scan(ctx)
	return row, err
}

func GetCustomer(ctx context.Context, db bun.IDB, instanceID, id string) (Customer, error) {
	var row Customer
	err := db.NewSelect().
		Model(&row).
		Where("instance_id = ? AND id = ?", instanceID, id).
		Scan(ctx)
	return row, err
}

func GetCustomerAnyInstance(ctx context.Context, db bun.IDB, id string) (Customer, error) {
	var row Customer
	err := db.NewSelect().
		Model(&row).
		Where("id = ?", id).
		Scan(ctx)
	return row, err
}

func GetCustomerByTokenDigest(ctx context.Context, db bun.IDB, digest string) (Customer, error) {
	var row Customer
	err := db.NewSelect().
		Model(&row).
		Where("access_token_digest = ?", digest).
		Scan(ctx)
	return row, err
}

func GetSchedule(ctx context.Context, db bun.IDB, instanceID string) (Schedule, error) {
	var row Schedule
	err := db.NewSelect().
		Model(&row).
		Where("instance_id = ?", instanceID).
		Scan(ctx)
	return row, err
}

func GetPolicy(ctx context.Context, db bun.IDB, instanceID string) (BookingPolicy, error) {
	var row BookingPolicy
	err := db.NewSelect().
		Model(&row).
		Where("instance_id = ?", instanceID).
		Scan(ctx)
	return row, err
}

func GetPlatformSettings(ctx context.Context, db bun.IDB) (PlatformSetting, error) {
	var row PlatformSetting
	err := db.NewSelect().
		Model(&row).
		Where("id = 1").
		Scan(ctx)
	return row, err
}

func CustomerAppointments(ctx context.Context, db bun.IDB, instanceID, customerID string, limit int) ([]Appointment, error) {
	rows := []Appointment{}
	err := db.NewSelect().
		Model(&rows).
		Where("instance_id = ? AND customer_id = ?", instanceID, customerID).
		Order("starts_at DESC").
		Limit(int64(limit)).
		Scan(ctx)
	return rows, err
}

func StoreSchedule(ctx context.Context, db bun.IDB, row *Schedule) error {
	_, err := db.NewInsert().
		Model(row).
		On("CONFLICT (instance_id) DO UPDATE").
		Set("cycle_length_days = EXCLUDED.cycle_length_days").
		Set("anchor_date = EXCLUDED.anchor_date").
		Set("cycle_days = EXCLUDED.cycle_days").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return MapError(err)
}
