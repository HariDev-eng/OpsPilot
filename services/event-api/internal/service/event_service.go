package service

import (
	"context"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
)

type EventService struct {
	repo *repository.EventRepository
}

type CreateEventResult struct {
	Created bool
}

func NewEventService(
	repo *repository.EventRepository,
) *EventService {
	return &EventService{
		repo: repo,
	}
}

func (s *EventService) CreateEvent(
	ctx context.Context,
	event model.Event,
) (CreateEventResult, error) {

	err := s.repo.Create(ctx, event)

	if err == repository.ErrEventAlreadyExists {
		return CreateEventResult{
			Created: false,
		}, nil
	}

	if err != nil {
		return CreateEventResult{}, err
	}

	return CreateEventResult{
		Created: true,
	}, nil
}