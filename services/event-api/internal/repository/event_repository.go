package repository

import (
	"context"
	"errors"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrEventAlreadyExists = errors.New("event already exists")

type EventRepository struct {
	pool *pgxpool.Pool
}

func NewEventRepository(pool *pgxpool.Pool) *EventRepository {
	return &EventRepository{
		pool: pool,
	}
}

func (r *EventRepository) Create(
	ctx context.Context,
	event model.Event,
) error {

	query := `
		INSERT INTO events (
			event_id,
			event_type,
			service,
			timestamp,
			payload
		)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := r.pool.Exec(
		ctx,
		query,
		event.EventID,
		event.EventType,
		event.Service,
		event.Timestamp,
		event.Payload,
	)

	if err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrEventAlreadyExists
		}

		return err
	}

	return nil
}