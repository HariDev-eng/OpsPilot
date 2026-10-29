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
}

func NewIncidentProcessor(
	pool *pgxpool.Pool,
	processedRepository *repository.ProcessedEventRepository,
	incidentRepository *repository.IncidentRepository,
) *IncidentProcessor {

	return &IncidentProcessor{
		pool:                pool,
		processedRepository: processedRepository,
		incidentRepository:  incidentRepository,
	}
}

func (p *IncidentProcessor) Process(
	ctx context.Context,
	event model.Event,
) error {

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx)

	err = p.processedRepository.MarkProcessedTx(
		ctx,
		tx,
		event.EventID,
		incidentConsumerName,
	)

	if err != nil {

		if errors.Is(
			err,
			repository.ErrAlreadyProcessed,
		) {
			log.Printf(
				"event already processed: event_id=%s",
				event.EventID,
			)

			return nil
		}

		return err
	}

	if err := p.incidentRepository.CreateTx(
		ctx,
		tx,
		event,
	); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	log.Printf(
		"incident created: event_id=%s service=%s",
		event.EventID,
		event.Service,
	)

	return nil
}
