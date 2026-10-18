package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"register_manager/internal/auth"
	"register_manager/internal/paging"
	"register_manager/internal/storage"
)

// BenchmarkLists exercises service queries and response mapping, without HTTP or password hashing.
func BenchmarkLists(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := storage.Open(ctx, filepath.Join(b.TempDir(), "bench.db"), nil)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	_, deps := New(ctx, Options{DB: db, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now }})
	tx, err := db.W.BeginTx(ctx, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO accounts (id,login,name,password_hash,super_admin,created_at,updated_at) VALUES ('admin','admin','Admin','test',1,1,1)`); err != nil {
		b.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO instances (id,slug,name,status,timezone,default_locale,registration_invite_code,created_at,updated_at) VALUES ('instance','instance','Calendar','active','UTC','en','test',1,1)`); err != nil {
		b.Fatal(err)
	}
	for i := range 200 {
		if _, err := tx.Exec(`INSERT INTO customers (id,instance_id,name,contact,status,created_at,updated_at) VALUES (?,'instance',?,?,'pending',?,?)`, fmt.Sprintf("c%03d", i), fmt.Sprintf("Customer %d", i), "contact@example.test", now.UnixMilli()+int64(i), now.UnixMilli()); err != nil {
			b.Fatal(err)
		}
	}
	for i := range 500 {
		start := now.Add(time.Duration(i+1) * time.Hour).UnixMilli()
		if _, err := tx.Exec(`INSERT INTO appointments (id,instance_id,customer_id,starts_at,ends_at,status,created_at,updated_at) VALUES (?,'instance',?,?,?,'booked',?,?)`, fmt.Sprintf("a%03d", i), fmt.Sprintf("c%03d", i%200), start, start+1800000, now.UnixMilli(), now.UnixMilli()); err != nil {
			b.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO audit_log (at,scope,instance_id,actor_kind,action) VALUES (?,'instance','instance','cli','benchmark')`, now.UnixMilli()); err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	actor := &auth.Account{ID: "admin", AuthVersion: 1, SuperAdmin: true}
	b.Run("Customers", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			rows, err := deps.Customers.List(ctx, actor, "instance", nil)
			if err != nil {
				b.Fatal(err)
			}
			if len(rows) != 200 {
				b.Fatal(len(rows))
			}
		}
	})
	b.Run("Appointments", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			rows, err := deps.Booking.ListAppointments(ctx, actor, "instance", "upcoming", paging.Input{Limit: new(50)})
			if err != nil {
				b.Fatal(err)
			}
			if len(rows.Items) != 50 {
				b.Fatal(len(rows.Items))
			}
		}
	})
	b.Run("Audit", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			rows, err := deps.Audit.List(ctx, actor, new("instance"), paging.Input{Limit: new(50)})
			if err != nil {
				b.Fatal(err)
			}
			if len(rows.Items) != 50 {
				b.Fatal(len(rows.Items))
			}
		}
	})
}
