package customers

import (
	"context"

	"github.com/uptrace/bun"

	"register_manager/internal/auth"
	"register_manager/internal/instances"
	"register_manager/internal/storage"
)

type CustomerMe struct {
	ID                           string                   `json:"id"`
	Name                         string                   `json:"name"`
	Locale                       *string                  `json:"locale"`
	Instance                     instances.PublicInstance `json:"instance"`
	ServiceDurationMinutes       int                      `json:"service_duration_minutes"`
	MinCancellationNoticeMinutes int                      `json:"min_cancellation_notice_minutes"`
}

func (s *Service) Me(ctx context.Context, customer *auth.Customer) (CustomerMe, error) {
	var out CustomerMe
	err := s.DB.ReadTx(ctx, func(tx bun.Tx) error {
		inst, err := storage.GetInstance(ctx, tx, customer.InstanceID)
		if err != nil {
			return err
		}
		policy, err := storage.GetPolicy(ctx, tx, customer.InstanceID)
		if err != nil {
			return err
		}
		out = CustomerMe{ID: customer.ID, Name: customer.Name, Locale: customer.Locale, Instance: instances.PublicView(inst),
			ServiceDurationMinutes: int(policy.ServiceDurationMinutes), MinCancellationNoticeMinutes: int(policy.MinCancellationNoticeMinutes)}
		return nil
	})
	return out, err
}
