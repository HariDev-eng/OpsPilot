package service

import (
	"context"
	"log"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/redis"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
)

type IncidentService struct {
	repository  *repository.IncidentRepository
	redisClient *redis.Client
}

func NewIncidentService(
	repository *repository.IncidentRepository,
	redisClient *redis.Client,
) *IncidentService {
	return &IncidentService{
		repository:  repository,
		redisClient: redisClient,
	}
}

// Acknowledge transitions an incident:
//
// # OPEN → ACKNOWLEDGED
//
// PostgreSQL is updated first because it is the source of truth.
// Redis is then updated as a best-effort projection.
func (s *IncidentService) Acknowledge(
	ctx context.Context,
	incidentID int64,
) error {

	if err := s.repository.Acknowledge(ctx, incidentID); err != nil {
		return err
	}

	incident, err := s.repository.GetByID(ctx, incidentID)
	if err != nil {
		return err
	}

	s.projectIncident(ctx, incident)

	return nil
}

// Resolve transitions an incident:
//
// # ACKNOWLEDGED → RESOLVED
//
// PostgreSQL is updated first because it is the source of truth.
// Redis is then updated as a best-effort projection.
func (s *IncidentService) Resolve(
	ctx context.Context,
	incidentID int64,
) error {

	if err := s.repository.Resolve(ctx, incidentID); err != nil {
		return err
	}

	incident, err := s.repository.GetByID(ctx, incidentID)
	if err != nil {
		return err
	}

	s.projectIncident(ctx, incident)

	return nil
}

// projectIncident projects the authoritative PostgreSQL incident
// state into Redis.
//
// Redis is a derived projection and is therefore not allowed to
// determine whether the business operation succeeded.
func (s *IncidentService) projectIncident(
	ctx context.Context,
	incident *model.Incident,
) {
	state := redis.IncidentState{
		IncidentID: incident.ID,
		Service:    incident.Service,
		Severity:   incident.Severity,
		Status:     incident.Status,
	}

	if err := s.redisClient.SetIncidentState(ctx, state); err != nil {
		log.Printf(
			"failed to update incident state projection: incident_id=%d error=%v",
			incident.ID,
			err,
		)
	}
}
