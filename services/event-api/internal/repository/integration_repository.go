package repository

import (
	"context"
	"errors"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrIntegrationNotFound = errors.New("integration not found")

type IntegrationRepository struct {
	pool *pgxpool.Pool
}

func NewIntegrationRepository(pool *pgxpool.Pool) *IntegrationRepository {
	return &IntegrationRepository{
		pool: pool,
	}
}

// Create persists a new integration.

func (r *IntegrationRepository) Create(
	ctx context.Context,
	integration *model.Integration,
) error {
	query := `
		INSERT INTO integrations (id, name, service_name, key_prefix, key_hash)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at
	`

	return r.pool.QueryRow(
		ctx,
		query,
		integration.ID,
		integration.Name,
		integration.ServiceName,
		integration.KeyPrefix,
		integration.KeyHash,
	).Scan(&integration.CreatedAt)
}

// List returns integrations without exposing their key hashes.
func (r *IntegrationRepository) List(
	ctx context.Context,
) ([]model.Integration, error) {
	query := `
		SELECT
			id,
			name,
			service_name,
			key_prefix,
			created_at,
			last_used_at,
			revoked_at
		FROM integrations
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	integrations := make([]model.Integration, 0)

	for rows.Next() {
		var integration model.Integration

		err := rows.Scan(
			&integration.ID,
			&integration.Name,
			&integration.ServiceName,
			&integration.KeyPrefix,
			&integration.CreatedAt,
			&integration.LastUsedAt,
			&integration.RevokedAt,
		)
		if err != nil {
			return nil, err
		}

		integrations = append(integrations, integration)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return integrations, nil
}

// Revoke marks an integration as revoked.
// Repeated revocation requests are safe and preserve the original timestamp.
func (r *IntegrationRepository) Revoke(
	ctx context.Context,
	id string,
) error {
	query := `
		UPDATE integrations
		SET revoked_at = COALESCE(revoked_at, NOW())
		WHERE id = $1
		RETURNING id
	`

	var revokedID string

	err := r.pool.QueryRow(ctx, query, id).Scan(&revokedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrIntegrationNotFound
	}

	return err
}

// GetActiveByKeyHash finds an integration using its stored key hash.
func (r *IntegrationRepository) GetActiveByKeyHash(
	ctx context.Context,
	keyHash string,
) (model.Integration, error) {
	query := `
		SELECT
			id,
			name,
			service_name,
			key_prefix,
			key_hash,
			created_at,
			last_used_at,
			revoked_at
		FROM integrations
		WHERE key_hash = $1
		  AND revoked_at IS NULL
	`

	var integration model.Integration

	err := r.pool.QueryRow(ctx, query, keyHash).Scan(
		&integration.ID,
		&integration.Name,
		&integration.ServiceName,
		&integration.KeyPrefix,
		&integration.KeyHash,
		&integration.CreatedAt,
		&integration.LastUsedAt,
		&integration.RevokedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return model.Integration{}, ErrIntegrationNotFound
	}

	if err != nil {
		return model.Integration{}, err
	}

	return integration, nil
}

// UpdateLastUsed records successful API key usage.
func (r *IntegrationRepository) UpdateLastUsed(
	ctx context.Context,
	id string,
) error {
	query := `
		UPDATE integrations
		SET last_used_at = NOW()
		WHERE id = $1
		  AND revoked_at IS NULL
	`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrIntegrationNotFound
	}

	return nil
}
