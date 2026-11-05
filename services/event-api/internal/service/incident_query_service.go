package service

import (
	"context"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
)

type IncidentQueryService struct {
	incidents *repository.IncidentRepository
	events    *repository.EventRepository
	actions   *repository.ActionRepository
}

func NewIncidentQueryService(
	incidents *repository.IncidentRepository,
	events *repository.EventRepository,
	actions *repository.ActionRepository,
) *IncidentQueryService {
	return &IncidentQueryService{
		incidents: incidents,
		events:    events,
		actions:   actions,
	}
}

func (s *IncidentQueryService) GetEvents(
	ctx context.Context,
	incidentID int64,
) ([]model.Event, error) {
	_, err := s.incidents.GetByID(ctx, incidentID)
	if err != nil {
		return nil, err
	}

	events, err := s.events.GetByIncidentID(ctx, incidentID)
	if err != nil {
		return nil, err
	}

	return events, nil
}

func (s *IncidentQueryService) GetActions(
	ctx context.Context,
	incidentID int64,
) ([]model.IncidentAction, error) {
	_, err := s.incidents.GetByID(ctx, incidentID)
	if err != nil {
		return nil, err
	}

	actions, err := s.actions.ListByIncident(ctx, incidentID)
	if err != nil {
		return nil, err
	}

	return actions, nil
}

func (s *IncidentQueryService) GetDashboardSummary(
	ctx context.Context,
) (*model.DashboardSummary, error) {
	return s.incidents.GetDashboardSummary(ctx)
}
