package publisher

import (
	"context"
	"log"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/kafka"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
)

type OutboxPublisher struct {
	repository *repository.OutboxRepository
	producer   *kafka.Producer
	batchSize  int
	interval   time.Duration
}

func NewOutboxPublisher(
	repository *repository.OutboxRepository,
	producer *kafka.Producer,
	batchSize int,
	interval time.Duration,
) *OutboxPublisher {

	return &OutboxPublisher{
		repository: repository,
		producer:   producer,
		batchSize:  batchSize,
		interval:   interval,
	}
}

func (p *OutboxPublisher) Start(ctx context.Context) {

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	log.Println("Outbox publisher started")

	p.publishBatch(ctx)

	for {
		select {

		case <-ticker.C:
			p.publishBatch(ctx)

		case <-ctx.Done():
			log.Println("Outbox publisher stopped")
			return
		}
	}
}

func (p *OutboxPublisher) publishBatch(ctx context.Context) {

	messages, err := p.repository.ClaimPending(
		ctx,
		p.batchSize,
	)

	if err != nil {
		log.Printf("failed to claim outbox messages: %v", err)
		return
	}

	if len(messages) == 0 {
		return
	}

	log.Printf(
		"claimed %d outbox messages",
		len(messages),
	)

	for _, message := range messages {

		err := p.publishMessage(ctx, message)

		if err != nil {
			log.Printf(
				"failed to publish event %s: %v",
				message.EventID,
				err,
			)

			continue
		}

		log.Printf(
			"published event %s",
			message.EventID,
		)
	}
}

func (p *OutboxPublisher) publishMessage(
	ctx context.Context,
	message repository.OutboxMessage,
) error {

	event := repositoryMessageToEvent(message)

	if err := p.producer.Publish(ctx, event); err != nil {

		nextRetry := time.Now().Add(
			retryDelay(message.Attempts),
		)

		markErr := p.repository.MarkFailed(
			ctx,
			message.ID,
			err.Error(),
			nextRetry,
		)

		if markErr != nil {
			return markErr
		}

		return err
	}

	if err := p.repository.MarkPublished(
		ctx,
		message.ID,
	); err != nil {
		return err
	}

	return nil
}

func repositoryMessageToEvent(
	message repository.OutboxMessage,
) model.Event {

	return model.Event{
		EventID:   message.EventID,
		EventType: message.EventType,
		Service:   message.Service,
		Timestamp: message.Timestamp,
		Payload:   message.Payload,
	}
}

func retryDelay(attempts int) time.Duration {

	switch attempts {

	case 1:
		return 2 * time.Second

	case 2:
		return 5 * time.Second

	case 3:
		return 10 * time.Second

	default:
		return 30 * time.Second
	}
}
