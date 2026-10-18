package audit

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/uptrace/bun"

	"register_manager/internal/apperr"
	"register_manager/internal/auth"
	"register_manager/internal/paging"
	"register_manager/internal/storage"
)

type Service struct{ DB *storage.DB }

type LogEntry struct {
	ID         int64   `json:"id"`
	At         int64   `json:"at" doc:"UTC Unix milliseconds"`
	Action     string  `json:"action"`
	ActorKind  string  `json:"actor_kind" enum:"account,customer,cli"`
	ActorName  *string `json:"actor_name"`
	TargetType *string `json:"target_type"`
	TargetID   *string `json:"target_id"`
	Before     any     `json:"before"`
	After      any     `json:"after"`
}

func logView(row storage.AuditLog, name *string) (LogEntry, error) {
	v := LogEntry{ID: row.ID, At: row.At, Action: row.Action, ActorKind: row.ActorKind,
		ActorName: name, TargetType: row.TargetType, TargetID: row.TargetID}
	for _, part := range []struct {
		stored *string
		value  *any
	}{{row.BeforeJson, &v.Before}, {row.AfterJson, &v.After}} {
		if part.stored != nil {
			if err := json.Unmarshal([]byte(*part.stored), part.value); err != nil {
				return LogEntry{}, err
			}
		}
	}
	return v, nil
}

func (s *Service) List(ctx context.Context, actor *auth.Account, instanceID *string, input paging.Input) (paging.Page[LogEntry], error) {
	var out paging.Page[LogEntry]
	limit, err := paging.Limit(input.Limit, 50)
	if err != nil {
		return out, err
	}
	var before *int64
	if input.Cursor != nil {
		id, err := strconv.ParseInt(*input.Cursor, 10, 64)
		if err != nil || id <= 0 {
			return out, apperr.Invalid("invalid_cursor")
		}
		before = &id
	}
	read := func(tx bun.Tx, _ *auth.Access) error {
		if instanceID == nil {
			if err := actor.RecheckSuperAdmin(ctx, tx); err != nil {
				return err
			}
		}
		rows, err := listEntries(ctx, tx, instanceID, before, limit+1)
		if err != nil {
			return err
		}
		entries := make([]LogEntry, 0, len(rows))
		for _, row := range rows {
			name := row.ActorName
			if name == nil {
				name = row.ActorCustomerName
			}
			v, err := logView(row.AuditLog, name)
			if err != nil {
				return err
			}
			entries = append(entries, v)
		}
		out = paging.New(entries, limit, func(v LogEntry) string { return strconv.FormatInt(v.ID, 10) })
		return nil
	}
	if instanceID == nil {
		err = s.DB.ReadTx(ctx, func(tx bun.Tx) error { return read(tx, nil) })
	} else {
		err = auth.InstanceRead(ctx, s.DB, actor, *instanceID, read)
	}
	return out, err
}

type auditRow struct {
	bun.BaseModel `bun:"table:audit_log,alias:audit_log"`
	storage.AuditLog
	ActorName         *string
	ActorCustomerName *string
}

func listEntries(ctx context.Context, db bun.IDB, instanceID *string, before *int64, limit int) ([]auditRow, error) {
	rows := []auditRow{}
	q := db.NewSelect().
		Model(&rows).
		ColumnExpr("audit_log.*").
		ColumnExpr("a.name AS actor_name").
		ColumnExpr("c.name AS actor_customer_name").
		Join("LEFT JOIN accounts AS a ON a.id = audit_log.actor_account_id").
		Join("LEFT JOIN customers AS c ON c.id = audit_log.actor_customer_id").
		Order("audit_log.id DESC").
		Limit(int64(limit))
	if instanceID == nil {
		q.Where("audit_log.scope = ?", "platform")
	} else {
		q.Where("audit_log.instance_id = ?", *instanceID)
	}
	if before != nil {
		q.Where("audit_log.id < ?", *before)
	}
	err := q.Scan(ctx)
	return rows, err
}
