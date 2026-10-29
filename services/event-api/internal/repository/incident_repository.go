package repository

import (
	"context"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/jackc/pgx/v5"
)

type IncidentRepository struct {
}

func NewIncidentRepository() *IncidentRepository {
	return &IncidentRepository{}
}

func (r *IncidentRepository) CreateTx(
	ctx context.Context,
	tx pgx.Tx,
	event model.Event,
) error {

	_, err := tx.Exec(
		ctx,
		`
		INSERT INTO incidents (
			event_id,
			service,
			event_type,
			severity
		)
		VALUES ($1, $2, $3, $4)
		`,
		event.EventID,
		event.Service,
		event.EventType,
		"MEDIUM",
	)

	return err
}
