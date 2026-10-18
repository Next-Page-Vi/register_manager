package storage

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"

	"register_manager/db"
)

func TestStatisticsIndexUpgradePreservesExistingData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade.db")
	d, err := Open(ctx, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, d.W, migrations)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 1); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO instances (id,slug,name,status,timezone,default_locale,registration_invite_code,created_at,updated_at) VALUES ('i','old','Existing','paused','UTC','en','invite',1,1)`,
		`INSERT INTO customers (id,instance_id,name,status,created_at,updated_at) VALUES ('c','i','Existing customer','pending',1,1)`,
		`INSERT INTO appointments (id,instance_id,customer_id,starts_at,ends_at,status,created_at,updated_at) VALUES ('a','i','c',10,20,'cancelled',1,1)`,
	} {
		if _, err := d.W.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(ctx, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var count int
	if err := d.R.QueryRow(`SELECT count(*) FROM appointments WHERE id='a' AND customer_id='c' AND status='cancelled' AND starts_at=10 AND ends_at=20 AND created_at=1 AND updated_at=1`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("existing appointment changed: %d %v", count, err)
	}
	if err := d.R.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name='appointments_instance_created' AND type='index'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("missing statistics index: %d %v", count, err)
	}
	backups, err := filepath.Glob(path + ".pre-migrate-v1-*.bak")
	if err != nil || len(backups) != 1 {
		t.Fatalf("missing pre-migration backup: %v %v", backups, err)
	}
}
