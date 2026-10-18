// Package audit writes audit log entries inside the same transaction as the
// change they describe.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"register_manager/internal/storage"
)

// Actor is one identity, never a combination of account and customer IDs.
type Actor interface{ auditActor() }
type accountActor struct{ id string }
type customerActor struct{ id string }
type cliActor struct{}

func (accountActor) auditActor()    {}
func (customerActor) auditActor()   {}
func (cliActor) auditActor()        {}
func AccountActor(id string) Actor  { return accountActor{id: id} }
func CustomerActor(id string) Actor { return customerActor{id: id} }

var CLI Actor = cliActor{}

// Target identifies a record or a singleton resource (without a record ID).
type Target struct {
	kind string
	id   *string
}

func Record(kind, id string) *Target { return &Target{kind: kind, id: &id} }
func Resource(kind string) *Target   { return &Target{kind: kind} }

// Entry is one audit record; a nil InstanceID denotes platform scope.
type Entry struct {
	At         time.Time
	InstanceID *string
	Actor      Actor
	Action     string
	Target     *Target
	Before     any
	After      any
}

func nullJSON(v any) (*string, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	value := string(b)
	return &value, nil
}

// Write inserts e using q, which must be bound to the write transaction.
func Write(ctx context.Context, q bun.IDB, e Entry) error {
	before, err := nullJSON(e.Before)
	if err != nil {
		return err
	}
	after, err := nullJSON(e.After)
	if err != nil {
		return err
	}
	scope := "instance"
	if e.InstanceID == nil {
		scope = "platform"
	}
	var kind string
	var accountID, customerID, targetType, targetID *string
	switch actor := e.Actor.(type) {
	case accountActor:
		kind, accountID = "account", &actor.id
	case customerActor:
		kind, customerID = "customer", &actor.id
	case cliActor:
		kind = "cli"
	default:
		return errors.New("audit actor is required")
	}
	if (accountID != nil && *accountID == "") || (customerID != nil && *customerID == "") || (e.InstanceID != nil && *e.InstanceID == "") {
		return errors.New("audit identity is empty")
	}
	if e.Target != nil {
		if e.Target.kind == "" || (e.Target.id != nil && *e.Target.id == "") {
			return errors.New("audit target is empty")
		}
		targetType, targetID = &e.Target.kind, e.Target.id
	}
	_, writeErr := q.NewInsert().
		Model(&storage.AuditLog{
			At:              e.At.UnixMilli(),
			Scope:           scope,
			InstanceID:      e.InstanceID,
			ActorKind:       kind,
			ActorAccountID:  accountID,
			ActorCustomerID: customerID,
			Action:          e.Action,
			TargetType:      targetType,
			TargetID:        targetID,
			BeforeJson:      before,
			AfterJson:       after,
		}).
		ExcludeColumn("id").
		Exec(ctx)
	return storage.MapError(writeErr)
}
