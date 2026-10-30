package repository

import (
	"context"
	"errors"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	IncidentStatusOpen         = "OPEN"
	IncidentStatusAcknowledged = "ACKNOWLEDGED"
	IncidentStatusResolved     = "RESOLVED"
)

type IncidentRepository struct {
	pool *pgxpool.Pool
}

func NewIncidentRepository(pool *pgxpool.Pool) *IncidentRepository {
	return &IncidentRepository{
		pool: pool,
	}
}

// CreateTx creates a new incident inside an existing transaction.
func (r *IncidentRepository) CreateTx(
	ctx context.Context,
	tx pgx.Tx,
	event model.Event,
	severity string,
) (int64, error) {

	var incidentID int64

	err := tx.QueryRow(
		ctx,
		`
		INSERT INTO incidents (
			event_id,
			service,
			event_type,
			severity,
			status
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
		`,
		event.EventID,
		event.Service,
		event.EventType,
		severity,
		IncidentStatusOpen,
	).Scan(&incidentID)

	if err != nil {
		return 0, err
	}

	return incidentID, nil
}

// AttachEventTx associates an event with an incident
// inside an existing transaction.
func (r *IncidentRepository) AttachEventTx(
	ctx context.Context,
	tx pgx.Tx,
	incidentID int64,
	eventID string,
) error {

	_, err := tx.Exec(
		ctx,
		`
		INSERT INTO incident_events (
			incident_id,
			event_id
		)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
		`,
		incidentID,
		eventID,
	)

	return err
}

// FindOpenByServiceTx finds the most recent OPEN incident
// for a service inside an existing transaction.
func (r *IncidentRepository) FindOpenByServiceTx(
	ctx context.Context,
	tx pgx.Tx,
	service string,
) (int64, error) {

	var incidentID int64

	err := tx.QueryRow(
		ctx,
		`
		SELECT id
		FROM incidents
		WHERE service = $1
		  AND status = $2
		ORDER BY created_at DESC
		LIMIT 1
		`,
		service,
		IncidentStatusOpen,
	).Scan(&incidentID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}

		return 0, err
	}

	return incidentID, nil
}

// Acknowledge transitions an incident:
//
// OPEN → ACKNOWLEDGED
func (r *IncidentRepository) Acknowledge(
	ctx context.Context,
	incidentID int64,
) error {

	result, err := r.pool.Exec(
		ctx,
		`
		UPDATE incidents
		SET
			status = $1,
			acknowledged_at = NOW()
		WHERE id = $2
		  AND status = $3
		`,
		IncidentStatusAcknowledged,
		incidentID,
		IncidentStatusOpen,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return errors.New(
			"incident cannot be acknowledged: incident does not exist or is not OPEN",
		)
	}

	return nil
}

// Resolve transitions an incident:
//
// ACKNOWLEDGED → RESOLVED
func (r *IncidentRepository) Resolve(
	ctx context.Context,
	incidentID int64,
) error {

	result, err := r.pool.Exec(
		ctx,
		`
		UPDATE incidents
		SET
			status = $1,
			resolved_at = NOW()
		WHERE id = $2
		  AND status = $3
		`,
		IncidentStatusResolved,
		incidentID,
		IncidentStatusAcknowledged,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return errors.New(
			"incident cannot be resolved: incident does not exist or is not ACKNOWLEDGED",
		)
	}

	return nil
}

func (r *IncidentRepository) EscalateSeverityTx(
	ctx context.Context,
	tx pgx.Tx,
	incidentID int64,
	severity string,
) error {
	_, err := tx.Exec(
		ctx,
		`
		UPDATE incidents
		SET severity = $1
		WHERE id = $2
		  AND (
			  severity = 'HIGH'
			  AND $1 = 'CRITICAL'
		  )
		`,
		severity,
		incidentID,
	)

	return err
}
