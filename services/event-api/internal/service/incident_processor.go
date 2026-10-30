package service

import (
	"context"
	"errors"
	"log"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

const incidentConsumerName = "incident-detection"

type IncidentProcessor struct {
	pool                *pgxpool.Pool
	processedRepository *repository.ProcessedEventRepository
	incidentRepository  *repository.IncidentRepository
	detector            *IncidentDetector
}

func NewIncidentProcessor(
	pool *pgxpool.Pool,
	processedRepository *repository.ProcessedEventRepository,
	incidentRepository *repository.IncidentRepository,
	detector *IncidentDetector,
) *IncidentProcessor {

	return &IncidentProcessor{
		pool:                pool,
		processedRepository: processedRepository,
		incidentRepository:  incidentRepository,
		detector:            detector,
	}
}

func (p *IncidentProcessor) Process(
	ctx context.Context,
	event model.Event,
) error {

	// 1. Detect using Redis.
	result, err := p.detector.Detect(ctx, event)
	if err != nil {
		return err
	}

	// 2. Start PostgreSQL transaction.
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx)

	// 3. Idempotency check.
	err = p.processedRepository.MarkProcessedTx(
		ctx,
		tx,
		event.EventID,
		incidentConsumerName,
	)

	if err != nil {
		if errors.Is(err, repository.ErrAlreadyProcessed) {
			log.Printf(
				"event already processed: event_id=%s",
				event.EventID,
			)

			return nil
		}

		return err
	}

	// 4. No threshold → processing completed.
	if !result.Triggered {

		if err := tx.Commit(ctx); err != nil {
			return err
		}

		log.Printf(
			"event processed, no incident: event_id=%s count=%d",
			event.EventID,
			result.Count,
		)

		return nil
	}

	// 5. Threshold exceeded.
	incidentID, err := p.incidentRepository.FindOpenByServiceTx(
		ctx,
		tx,
		event.Service,
	)

	if err != nil {
		return err
	}

	// 6. Create incident if necessary.
	if incidentID == 0 {
		incidentID, err = p.incidentRepository.CreateTx(
			ctx,
			tx,
			event,
			result.Severity,
		)
		if err != nil {
			return err
		}

		log.Printf(
			"new incident created: incident_id=%d service=%s severity=%s",
			incidentID,
			event.Service,
			result.Severity,
		)
	} else {
		err = p.incidentRepository.EscalateSeverityTx(
			ctx,
			tx,
			incidentID,
			result.Severity,
		)
		if err != nil {
			return err
		}
	}

	// 7. Attach event.
	if err := p.incidentRepository.AttachEventTx(
		ctx,
		tx,
		incidentID,
		event.EventID,
	); err != nil {
		return err
	}

	// 8. Commit everything.
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	log.Printf(
		"incident updated: incident_id=%d event_id=%s count=%d",
		incidentID,
		event.EventID,
		result.Count,
	)

	return nil
}
