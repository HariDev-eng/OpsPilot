package service

import (
	"context"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
)

type IncidentService struct {
	repository *repository.IncidentRepository
}

func NewIncidentService(
	incidentRepository *repository.IncidentRepository,
) *IncidentService {
	return &IncidentService{
		repository: incidentRepository,
	}
}

func (s *IncidentService) Acknowledge(
	ctx context.Context,
	incidentID int64,
) error {
	return s.repository.Acknowledge(ctx, incidentID)
}

func (s *IncidentService) Resolve(
	ctx context.Context,
	incidentID int64,
) error {
	return s.repository.Resolve(ctx, incidentID)
}
