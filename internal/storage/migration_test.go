package storage

import (
	"slices"
	"testing"
)

func TestInitialMigrationIncludesStatisticsIndex(t *testing.T) {
	d := open(t)
	var version int64
	if err := d.R.QueryRow(`SELECT max(version_id) FROM goose_db_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Fatalf("initial schema version = %d, want 1", version)
	}

	rows, err := d.R.Query(`SELECT name FROM pragma_index_info('appointments_instance_created') ORDER BY seqno`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(columns, []string{"instance_id", "created_at"}) {
		t.Fatalf("statistics index columns = %v, want [instance_id created_at]", columns)
	}
}
