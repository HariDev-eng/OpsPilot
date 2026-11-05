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

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx)

	eventQuery := `
		INSERT INTO events (
			event_id,
			event_type,
			service,
			timestamp,
			payload
		)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err = tx.Exec(
		ctx,
		eventQuery,
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

	outboxQuery := `
    	INSERT INTO outbox (
        	event_id,
         	event_type,
          	service,
           	timestamp,
            topic,
            payload
        )
        VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err = tx.Exec(
		ctx,
		outboxQuery,
		event.EventID,
		event.EventType,
		event.Service,
		event.Timestamp,
		"ops.events",
		event.Payload,
	)

	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}

func (r *EventRepository) GetByIncidentID(
	ctx context.Context,
	incidentID int64,
) ([]model.Event, error) {

	rows, err := r.pool.Query(ctx, `
		SELECT
			e.event_id,
			e.event_type,
			e.service,
			e.timestamp,
			e.payload,
			e.created_at
		FROM events e
		INNER JOIN incident_events ie
			ON ie.event_id = e.event_id
		WHERE ie.incident_id = $1
		ORDER BY e.timestamp ASC
	`, incidentID)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	events := make([]model.Event, 0)

	for rows.Next() {
		var event model.Event

		err := rows.Scan(
			&event.EventID,
			&event.EventType,
			&event.Service,
			&event.Timestamp,
			&event.Payload,
			&event.CreatedAt,
		)

		if err != nil {
			return nil, err
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}
