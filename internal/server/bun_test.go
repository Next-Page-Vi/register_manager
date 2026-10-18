package server

import (
	"fmt"
	"testing"
)

func TestBunListsUseFixedQueryCount(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("query-count", "worker-count")
	paths := []string{
		"/api/v1/platform/accounts", "/api/v1/platform/instances",
		"/api/v1/instances/" + w.instanceID + "/customers",
		"/api/v1/instances/" + w.instanceID + "/appointments?view=upcoming",
		"/api/v1/instances/" + w.instanceID + "/statistics?from=2026-06-01&to=2026-06-30",
		"/api/v1/instances/" + w.instanceID + "/audit",
	}
	counts := func() []uint32 {
		out := make([]uint32, len(paths))
		for i, path := range paths {
			before := e.db.Read().DBStats().Queries
			w.admin.get(path)
			out[i] = e.db.Read().DBStats().Queries - before
			if out[i] == 0 {
				t.Fatalf("query instrumentation inactive for %s", path)
			}
		}
		return out
	}
	before := counts()
	for i := range 20 {
		id, accountID, customerID := newUUID(), newUUID(), newUUID()
		writes := []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO accounts (id,login,name,password_hash,created_at,updated_at) VALUES (?,?,'Operator','test',1,1)`, []any{accountID, fmt.Sprintf("extra-%d", i)}},
			{`INSERT INTO instances (id,slug,name,status,timezone,default_locale,registration_invite_code,created_at,updated_at) VALUES (?,?,'Calendar','active','UTC','en','test',1,1)`, []any{id, fmt.Sprintf("extra-%d", i)}},
			{`INSERT INTO instance_operators (instance_id,account_id,created_at) VALUES (?,?,1)`, []any{id, accountID}},
			{`INSERT INTO customers (id,instance_id,name,status,created_at,updated_at) VALUES (?,?,'Customer','pending',1,1)`, []any{customerID, w.instanceID}},
			{`INSERT INTO appointments (id,instance_id,customer_id,starts_at,ends_at,status,created_at,updated_at) VALUES (?,?,?,?,?,'cancelled',1,1)`, []any{newUUID(), w.instanceID, customerID, slot10.UnixMilli(), slot10.AddDate(0, 0, 1).UnixMilli()}},
			{`INSERT INTO audit_log (at,scope,instance_id,actor_kind,action) VALUES (1,'instance',?,'cli','test')`, []any{w.instanceID}},
		}
		for _, q := range writes {
			if _, err := e.db.W.Exec(q.sql, q.args...); err != nil {
				t.Fatal(err)
			}
		}
	}
	after := counts()
	for i, n := range after {
		if n != before[i] {
			t.Errorf("%s: queries grew from %d to %d", paths[i], before[i], n)
		}
	}
}

func TestRegistrationRetryComparesContactValues(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	w := e.setupInstance("retry-contact", "worker-contact")
	c := e.client()
	body := map[string]any{"id": newUUID(), "invite": w.invite, "name": "O'Reilly 中文", "contact": "+86 123"}
	path := "/api/v1/public/instances/" + w.slug + "/registrations"
	c.must("POST", path, body, 200)
	c.must("POST", path, body, 200)
	body["contact"] = nil
	c.must("POST", path, body, 409)
}

func TestAuditFailureRollsBackAccountCreation(t *testing.T) {
	e := newEnv(t)
	e.createSuperAdmin("root", "root-password")
	admin := e.login("root", "root-password")
	if _, err := e.db.W.Exec(`CREATE TRIGGER fail_create_audit BEFORE INSERT ON audit_log WHEN NEW.action = 'account.create' BEGIN SELECT RAISE(ABORT, 'test audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"login": "rollback-account", "name": "Rollback", "password": "test-password"}
	admin.must("POST", "/api/v1/platform/accounts", body, 500)
	var n int
	if err := e.db.R.QueryRow(`SELECT count(*) FROM accounts WHERE login = 'rollback-account'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("account survived a failed audit write")
	}
	if _, err := e.db.W.Exec(`DROP TRIGGER fail_create_audit`); err != nil {
		t.Fatal(err)
	}
	admin.must("POST", "/api/v1/platform/accounts", body, 200)
}
