package consumer

import (
	"context"
	"encoding/json"
	"log"
	"time"

	kafkaProducer "github.com/HariDev-eng/opspilot/services/event-api/internal/kafka"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
	"github.com/segmentio/kafka-go"
)

const (
	maxRetries      = 3
	initialDLQDelay = time.Second
	maxDLQDelay     = 30 * time.Second
)

type IncidentConsumer struct {
	reader            *kafka.Reader
	dlqProducer       *kafkaProducer.DLQProducer
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
		incidentProcessor: incidentProcessor,
	}
}

// Start continuously consumes and processes Kafka messages.
func (c *IncidentConsumer) Start(ctx context.Context) {

	log.Println("Incident consumer started")

	defer log.Println("Incident consumer stopped")

	for {

		message, err := c.reader.FetchMessage(ctx)

		if err != nil {

			if ctx.Err() != nil {
				return
			}

			log.Printf("failed to fetch Kafka message: %v", err)

			// Avoid a tight loop if Kafka is temporarily unavailable.
			if !c.wait(ctx, time.Second) {
				return
			}

			continue
		}

		// --------------------------------------------------
		// Deserialize Kafka message
		// --------------------------------------------------

		var event model.Event

		if err := json.Unmarshal(message.Value, &event); err != nil {

			log.Printf(
				"failed to unmarshal event: topic=%s partition=%d offset=%d error=%v",
				message.Topic,
				message.Partition,
				message.Offset,
				err,
			)

			// Preserve malformed messages in the raw DLQ.
			dlqErr := c.publishToDLQWithRetry(
				ctx,
				func() error {
					return c.dlqProducer.PublishRaw(
						ctx,
						message.Value,
						message.Key,
						message.Topic,
						message.Partition,
						message.Offset,
						err.Error(),
					)
				},
			)

			if dlqErr != nil {
				log.Printf(
					"malformed message DLQ handling stopped: topic=%s partition=%d offset=%d error=%v",
					message.Topic,
					message.Partition,
					message.Offset,
					dlqErr,
				)

				// Do not process later messages in this partition.
				return
			}

			// Commit only after successful DLQ publication.
			if commitErr := c.reader.CommitMessages(ctx, message); commitErr != nil {

				log.Printf(
					"failed to commit malformed message after DLQ publication: topic=%s partition=%d offset=%d error=%v",
					message.Topic,
					message.Partition,
					message.Offset,
					commitErr,
				)

				continue
			}

			log.Printf(
				"malformed message sent to DLQ and committed: topic=%s partition=%d offset=%d",
				message.Topic,
				message.Partition,
				message.Offset,
			)

			continue
		}

		// --------------------------------------------------
		// Process valid event
		// --------------------------------------------------

		log.Printf(
			"received event: event_id=%s partition=%d offset=%d",
			event.EventID,
			message.Partition,
			message.Offset,
		)

		processErr := c.processWithRetry(ctx, event)

		if processErr != nil {

			if ctx.Err() != nil {
				return
			}

			log.Printf(
				"event permanently failed: event_id=%s error=%v",
				event.EventID,
				processErr,
			)

			// Publish failed event to structured DLQ.
			dlqErr := c.publishToDLQWithRetry(
				ctx,
				func() error {
					return c.sendToDLQ(
						ctx,
						event,
						processErr,
						maxRetries,
					)
				},
			)

			if dlqErr != nil {

				log.Printf(
					"event DLQ handling stopped: event_id=%s error=%v",
					event.EventID,
					dlqErr,
				)

				// Never commit an event that wasn't processed
				// or successfully published to the DLQ.
				return
			}
		}

		// --------------------------------------------------
		// Commit Kafka offset
		// --------------------------------------------------

		if ctx.Err() != nil {
			return
		}

		if err := c.reader.CommitMessages(ctx, message); err != nil {

			log.Printf(
				"failed to commit offset: event_id=%s partition=%d offset=%d error=%v",
				event.EventID,
				message.Partition,
				message.Offset,
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

// --------------------------------------------------
// Processing Retry
// --------------------------------------------------

func (c *IncidentConsumer) processWithRetry(
	ctx context.Context,
	event model.Event,
) error {

	for attempt := 1; attempt <= maxRetries; attempt++ {

		if err := ctx.Err(); err != nil {
			return err
		}

		log.Printf(
			"processing event: event_id=%s attempt=%d",
			event.EventID,
			attempt,
		)

		err := c.processEvent(ctx, event)

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

		if !c.wait(ctx, retryDelay(attempt)) {
			return ctx.Err()
		}
	}

	return nil
}

func retryDelay(attempt int) time.Duration {

	switch attempt {

	case 1:
		return time.Second

	case 2:
		return 2 * time.Second

	default:
		return 4 * time.Second
	}
}

// --------------------------------------------------
// DLQ Retry
// --------------------------------------------------

// publishToDLQWithRetry retries indefinitely until the
// publication succeeds or the consumer context is cancelled.
func (c *IncidentConsumer) publishToDLQWithRetry(
	ctx context.Context,
	publish func() error,
) error {

	delay := initialDLQDelay

	for attempt := 1; ; attempt++ {

		if err := ctx.Err(); err != nil {
			return err
		}

		if err := publish(); err == nil {
			return nil
		} else {

			log.Printf(
				"DLQ publication failed: attempt=%d error=%v",
				attempt,
				err,
			)
		}

		if !c.wait(ctx, delay) {
			return ctx.Err()
		}

		delay *= 2

		if delay > maxDLQDelay {
			delay = maxDLQDelay
		}
	}
}

// --------------------------------------------------
// DLQ Publishing
// --------------------------------------------------

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

// --------------------------------------------------
// Event Processing
// --------------------------------------------------

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

// --------------------------------------------------
// Context-aware Waiting
// --------------------------------------------------

func (c *IncidentConsumer) wait(
	ctx context.Context,
	duration time.Duration,
) bool {

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {

	case <-timer.C:
		return true

	case <-ctx.Done():
		return false
	}
}

// --------------------------------------------------
// Close
// --------------------------------------------------

func (c *IncidentConsumer) Close() error {
	return c.reader.Close()
}
