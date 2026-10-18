package booking

import (
	"context"
	"path/filepath"
	"testing"

	"register_manager/internal/storage"
)

func TestEmptyAppointmentProjection(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "test.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows := []appointmentRow{}
	q := appointmentQuery(db.Read(), "missing")
	if err := q.Scan(ctx, &rows); err != nil {
		t.Fatalf("%s: %v", q.String(), err)
	}
	if len(rows) != 0 {
		t.Fatalf("unexpected rows: %v", rows)
	}
}
