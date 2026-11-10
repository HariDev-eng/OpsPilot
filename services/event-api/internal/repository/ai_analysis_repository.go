package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AIAnalysisRepository struct {
	pool *pgxpool.Pool
}

func NewAIAnalysisRepository(
	pool *pgxpool.Pool,
) *AIAnalysisRepository {
	return &AIAnalysisRepository{pool: pool}
}

func (r *AIAnalysisRepository) Create(
	ctx context.Context,
	incidentID int64,
	provider string,
	modelName string,
) (*model.AIAnalysisRecord, error) {

	var record model.AIAnalysisRecord

	err := r.pool.QueryRow(ctx, `
        INSERT INTO ai_analyses (
            incident_id,
            status,
            provider,
            model,
            started_at
        )
        VALUES ($1, 'RUNNING', $2, $3, NOW())
        RETURNING
            id,
            incident_id,
            status,
            provider,
            model,
            started_at,
            created_at,
            updated_at
    `,
		incidentID,
		provider,
		modelName,
	).Scan(
		&record.ID,
		&record.IncidentID,
		&record.Status,
		&record.Provider,
		&record.Model,
		&record.StartedAt,
		&record.CreatedAt,
		&record.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("create AI analysis: %w", err)
	}

	return &record, nil
}

func (r *AIAnalysisRepository) Complete(
	ctx context.Context,
	analysisID int64,
	analysis any,
) error {

	data, err := json.Marshal(analysis)
	if err != nil {
		return fmt.Errorf("marshal analysis: %w", err)
	}

	result, err := r.pool.Exec(ctx, `
        UPDATE ai_analyses
        SET
            status = 'COMPLETED',
            analysis = $1::jsonb,
            error_message = NULL,
            completed_at = NOW(),
            updated_at = NOW()
        WHERE id = $2
          AND status = 'RUNNING'
    `, string(data), analysisID)

	if err != nil {
		return fmt.Errorf("complete AI analysis: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("AI analysis is not in RUNNING state")
	}

	return nil
}

func (r *AIAnalysisRepository) Fail(
	ctx context.Context,
	analysisID int64,
	message string,
) error {

	result, err := r.pool.Exec(ctx, `
        UPDATE ai_analyses
        SET
            status = 'FAILED',
            error_message = $1,
            completed_at = NOW(),
            updated_at = NOW()
        WHERE id = $2
          AND status = 'RUNNING'
    `, message, analysisID)

	if err != nil {
		return fmt.Errorf("fail AI analysis: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("AI analysis is not in RUNNING state")
	}

	return nil
}

func (r *AIAnalysisRepository) GetLatest(
	ctx context.Context,
	incidentID int64,
) (*model.AIAnalysisRecord, error) {

	var record model.AIAnalysisRecord

	err := r.pool.QueryRow(ctx, `
        SELECT
            id,
            incident_id,
            status,
            provider,
            model,
            analysis,
            error_message,
            started_at,
            completed_at,
            created_at,
            updated_at
        FROM ai_analyses
        WHERE incident_id = $1
        ORDER BY created_at DESC, id DESC
        LIMIT 1
    `, incidentID).Scan(
		&record.ID,
		&record.IncidentID,
		&record.Status,
		&record.Provider,
		&record.Model,
		&record.Analysis,
		&record.ErrorMessage,
		&record.StartedAt,
		&record.CompletedAt,
		&record.CreatedAt,
		&record.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &record, nil
}

func (r *AIAnalysisRepository) GetByID(
	ctx context.Context,
	analysisID int64,
) (*model.AIAnalysisRecord, error) {

	var record model.AIAnalysisRecord

	err := r.pool.QueryRow(ctx, `
        SELECT
            id,
            incident_id,
            status,
            provider,
            model,
            analysis,
            error_message,
            started_at,
            completed_at,
            created_at,
            updated_at
        FROM ai_analyses
        WHERE id = $1
    `, analysisID).Scan(
		&record.ID,
		&record.IncidentID,
		&record.Status,
		&record.Provider,
		&record.Model,
		&record.Analysis,
		&record.ErrorMessage,
		&record.StartedAt,
		&record.CompletedAt,
		&record.CreatedAt,
		&record.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &record, nil
}

func (r *AIAnalysisRepository) ListByIncident(
	ctx context.Context,
	incidentID int64,
	limit int,
) ([]model.AIAnalysisRecord, error) {

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := r.pool.Query(ctx, `
        SELECT
            id,
            incident_id,
            status,
            provider,
            model,
            analysis,
            error_message,
            started_at,
            completed_at,
            created_at,
            updated_at
        FROM ai_analyses
        WHERE incident_id = $1
        ORDER BY created_at DESC, id DESC
        LIMIT $2
    `, incidentID, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]model.AIAnalysisRecord, 0)

	for rows.Next() {
		var record model.AIAnalysisRecord

		if err := rows.Scan(
			&record.ID,
			&record.IncidentID,
			&record.Status,
			&record.Provider,
			&record.Model,
			&record.Analysis,
			&record.ErrorMessage,
			&record.StartedAt,
			&record.CompletedAt,
			&record.CreatedAt,
			&record.UpdatedAt,
		); err != nil {
			return nil, err
		}

		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

// Keep pgx.ErrNoRows available to the service layer.
var ErrAIAnalysisNotFound = pgx.ErrNoRows
