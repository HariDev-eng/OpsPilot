package consumer

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
	"github.com/segmentio/kafka-go"

	kafkaProducer "github.com/HariDev-eng/opspilot/services/event-api/internal/kafka"
)

const maxRetries = 3

type IncidentConsumer struct {
	reader            *kafka.Reader
	dlqProducer       *kafkaProducer.DLQProducer
	failedOnce        map[string]bool
	incidentProcessor *service.IncidentProcessor
}

func NewIncidentConsumer(
	broker string,
	topic string,
	groupID string,
	dlqProducer *kafkaProducer.DLQProducer,
	incidentProcessor *service.IncidentProcessor,
) *IncidentConsumer {

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{broker},
		Topic:   topic,
		GroupID: groupID,

		MinBytes: 1,
		MaxBytes: 10e6,
	})

	return &IncidentConsumer{
		reader:            reader,
		dlqProducer:       dlqProducer,
		failedOnce:        make(map[string]bool),
		incidentProcessor: incidentProcessor,
	}
}

func (c *IncidentConsumer) Start(ctx context.Context) {

	log.Println("Incident consumer started")

	for {

		message, err := c.reader.FetchMessage(ctx)

		if err != nil {

			if ctx.Err() != nil {
				log.Println("Incident consumer stopped")
				return
			}

			log.Printf(
				"failed to fetch Kafka message: %v",
				err,
			)

			continue
		}

		var event model.Event

		if err := json.Unmarshal(
			message.Value,
			&event,
		); err != nil {

			log.Printf(
				"failed to deserialize Kafka message: %v",
				err,
			)

			continue
		}

		log.Printf(
			"received event: event_id=%s partition=%d offset=%d",
			event.EventID,
			message.Partition,
			message.Offset,
		)

		if err := c.processWithRetry(
			ctx,
			event,
		); err != nil {

			log.Printf(
				"event permanently failed: event_id=%s error=%v",
				event.EventID,
				err,
			)

			if err := c.sendToDLQ(
				ctx,
				event,
				err,
				maxRetries,
			); err != nil {

				log.Printf(
					"failed to send event %s to DLQ: %v",
					event.EventID,
					err,
				)

				// IMPORTANT:
				// Do NOT commit if DLQ publication failed.
				continue
			}
		}

		if err := c.reader.CommitMessages(
			ctx,
			message,
		); err != nil {

			log.Printf("failed to commit offset for event %s: %v",
				event.EventID,
				err,
			)

			continue
		}

		log.Printf(
			"event completed: event_id=%s partition=%d offset=%d",
			event.EventID,
			message.Partition,
			message.Offset,
		)
	}
}

func (c *IncidentConsumer) processWithRetry(
	ctx context.Context,
	event model.Event,
) error {

	for attempt := 1; attempt <= maxRetries; attempt++ {

		log.Printf(
			"processing event: event_id=%s attempt=%d",
			event.EventID,
			attempt,
		)

		err := c.processEvent(
			ctx,
			event,
		)

		if err == nil {
			return nil
		}

		log.Printf(
			"processing failed: event_id=%s attempt=%d error=%v",
			event.EventID,
			attempt,
			err,
		)

		if attempt == maxRetries {
			return err
		}

		delay := retryDelay(attempt)

		log.Printf(
			"retrying event %s in %v",
			event.EventID,
			delay,
		)

		timer := time.NewTimer(delay)

		select {

		case <-timer.C:

		case <-ctx.Done():

			timer.Stop()

			return ctx.Err()
		}
	}

	return nil
}

func retryDelay(attempt int) time.Duration {

	switch attempt {

	case 1:
		return 1 * time.Second

	case 2:
		return 2 * time.Second

	default:
		return 4 * time.Second
	}
}

func (c *IncidentConsumer) sendToDLQ(
	ctx context.Context,
	event model.Event,
	err error,
	retryCount int,
) error {

	return c.dlqProducer.Publish(
		ctx,
		event,
		err.Error(),
		retryCount,
	)
}

func (c *IncidentConsumer) processEvent(
	ctx context.Context,
	event model.Event,
) error {

	log.Printf(
		"processing event %s",
		event.EventID,
	)

	return c.incidentProcessor.Process(
		ctx,
		event,
	)
}

func (c *IncidentConsumer) Close() error {
	return c.reader.Close()
}
