package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAlreadyProcessed = errors.New("event already processed")

type ProcessedEventRepository struct {
	pool *pgxpool.Pool
}

func NewProcessedEventRepository(
	pool *pgxpool.Pool,
) *ProcessedEventRepository {
	return &ProcessedEventRepository{
		pool: pool,
	}
}

func (r *ProcessedEventRepository) MarkProcessedTx(
	ctx context.Context,
	tx pgx.Tx,
	eventID string,
	consumerName string,
) error {

	var inserted bool

	err := tx.QueryRow(
		ctx,
		`
		INSERT INTO processed_events (
			event_id,
			consumer_name
		)
		VALUES ($1, $2)
		ON CONFLICT (
			event_id,
			consumer_name
		)
		DO NOTHING
		RETURNING TRUE
		`,
		eventID,
		consumerName,
	).Scan(&inserted)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAlreadyProcessed
	}

	return err
}
