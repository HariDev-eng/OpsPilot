package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrOutboxClaimLost = errors.New("outbox claim lost or no longer valid")

type OutboxMessage struct {
	ID         int64
	EventID    string
	EventType  string
	Service    string
	Timestamp  time.Time
	Topic      string
	Payload    []byte
	Attempts   int
	ClaimToken string
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

	if limit <= 0 {
		return nil, fmt.Errorf("claim limit must be positive")
	}

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

	var messages []OutboxMessage

	for rows.Next() {
		var message OutboxMessage

		if err := rows.Scan(
			&message.ID,
			&message.EventID,
			&message.EventType,
			&message.Service,
			&message.Timestamp,
			&message.Topic,
			&message.Payload,
			&message.Attempts,
		); err != nil {
			rows.Close()
			return nil, err
		}

		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}

	rows.Close()

	for i := range messages {
		err := tx.QueryRow(
			ctx,
			`
			UPDATE outbox
			SET
				status = 'PUBLISHING',
				attempts = attempts + 1,
				claimed_at = NOW(),
				claim_token = gen_random_uuid()
			WHERE id = $1
			RETURNING claim_token::text, attempts
			`,
			messages[i].ID,
		).Scan(
			&messages[i].ClaimToken,
			&messages[i].Attempts,
		)

		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return messages, nil
}

func (r *OutboxRepository) MarkPublished(
	ctx context.Context,
	id int64,
	claimToken string,
) error {

	if claimToken == "" {
		return ErrOutboxClaimLost
	}

	result, err := r.pool.Exec(
		ctx,
		`
		UPDATE outbox
		SET
			status = 'PUBLISHED',
			published_at = NOW(),
			claimed_at = NULL,
			claim_token = NULL,
			last_error = NULL,
			next_retry_at = NULL
		WHERE id = $1
		  AND status = 'PUBLISHING'
		  AND claim_token = $2::uuid
		`,
		id,
		claimToken,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrOutboxClaimLost
	}

	return nil
}

func (r *OutboxRepository) MarkFailed(
	ctx context.Context,
	id int64,
	claimToken string,
	errMessage string,
	nextRetryAt time.Time,
) error {

	if claimToken == "" {
		return ErrOutboxClaimLost
	}

	result, err := r.pool.Exec(
		ctx,
		`
		UPDATE outbox
		SET
			status = 'PENDING',
			last_error = $3,
			next_retry_at = $4,
			claimed_at = NULL,
			claim_token = NULL
		WHERE id = $1
		  AND status = 'PUBLISHING'
		  AND claim_token = $2::uuid
		`,
		id,
		claimToken,
		errMessage,
		nextRetryAt,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrOutboxClaimLost
	}

	return nil
}

func (r *OutboxRepository) RecoverStalePublishing(
	ctx context.Context,
	lease time.Duration,
) (int64, error) {

	if lease <= 0 {
		return 0, fmt.Errorf("lease duration must be positive")
	}

	result, err := r.pool.Exec(
		ctx,
		`
		WITH stale AS (
			SELECT id
			FROM outbox
			WHERE status = 'PUBLISHING'
			  AND claimed_at IS NOT NULL
			  AND claimed_at < NOW() - ($1 * INTERVAL '1 second')
			ORDER BY claimed_at
			FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox AS o
		SET
			status = 'PENDING',
			claimed_at = NULL,
			claim_token = NULL,
			next_retry_at = NOW(),
			last_error = 'Recovered stale PUBLISHING record after lease expiry'
		FROM stale
		WHERE o.id = stale.id
		`,
		lease.Seconds(),
	)
	if err != nil {
		return 0, fmt.Errorf("recover stale outbox messages: %w", err)
	}

	return result.RowsAffected(), nil
}
