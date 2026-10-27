package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboxMessage struct {
	ID        int64
	EventID   string
	EventType string
	Service   string
	Timestamp time.Time
	Topic     string
	Payload   []byte
	Attempts  int
}

type OutboxRepository struct {
	pool *pgxpool.Pool
}

func NewOutboxRepository(pool *pgxpool.Pool) *OutboxRepository {
	return &OutboxRepository{
		pool: pool,
	}
}

func (r *OutboxRepository) ClaimPending(
	ctx context.Context,
	limit int,
) ([]OutboxMessage, error) {

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}

	defer tx.Rollback(ctx)

	rows, err := tx.Query(
		ctx,
		`
		SELECT
			id,
			event_id,
			event_type,
			service,
			timestamp,
			topic,
			payload,
			attempts
		FROM outbox
		WHERE status = 'PENDING'
		  AND (
			next_retry_at IS NULL
			OR next_retry_at <= NOW()
		  )
		ORDER BY id
		FOR UPDATE SKIP LOCKED
		LIMIT $1
		`,
		limit,
	)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var messages []OutboxMessage

	for rows.Next() {
		var message OutboxMessage

		err := rows.Scan(
			&message.ID,
			&message.EventID,
			&message.EventType,
			&message.Service,
			&message.Timestamp,
			&message.Topic,
			&message.Payload,
			&message.Attempts,
		)

		if err != nil {
			return nil, err
		}

		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, message := range messages {
		_, err := tx.Exec(
			ctx,
			`
			UPDATE outbox
			SET
				status = 'PUBLISHING',
				attempts = attempts + 1
			WHERE id = $1
			`,
			message.ID,
		)

		if err != nil {
			return nil, err
		}

		message.Attempts++
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return messages, nil
}

func (r *OutboxRepository) MarkPublished(
	ctx context.Context,
	id int64,
) error {

	_, err := r.pool.Exec(
		ctx,
		`
		UPDATE outbox
		SET
			status = 'PUBLISHED',
			published_at = NOW()
		WHERE id = $1
		  AND status = 'PUBLISHING'
		`,
		id,
	)

	return err
}

func (r *OutboxRepository) MarkFailed(
	ctx context.Context,
	id int64,
	errMessage string,
	nextRetryAt time.Time,
) error {

	_, err := r.pool.Exec(
		ctx,
		`
		UPDATE outbox
		SET
			status = 'PENDING',
			last_error = $2,
			next_retry_at = $3
		WHERE id = $1
		  AND status = 'PUBLISHING'
		`,
		id,
		errMessage,
		nextRetryAt,
	)

	return err
}
