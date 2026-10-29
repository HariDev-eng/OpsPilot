package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/segmentio/kafka-go"
)

type DLQProducer struct {
	writer *kafka.Writer
}

func NewDLQProducer(
	broker string,
	topic string,
) *DLQProducer {

	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:      []string{broker},
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: int(kafka.RequireAll),
	})

	return &DLQProducer{
		writer: writer,
	}
}

type DLQMessage struct {
	Event      model.Event `json:"event"`
	Error      string      `json:"error"`
	FailedAt   string      `json:"failed_at"`
	RetryCount int         `json:"retry_count"`
}

func (p *DLQProducer) Publish(
	ctx context.Context,
	event model.Event,
	errMessage string,
	retryCount int,
) error {

	payload := DLQMessage{
		Event:      event,
		Error:      errMessage,
		FailedAt:   time.Now().UTC().Format(time.RFC3339),
		RetryCount: retryCount,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf(
			"failed to marshal DLQ message: %w",
			err,
		)
	}

	message := kafka.Message{
		Key:   []byte(event.Service),
		Value: data,
	}

	if err := p.writer.WriteMessages(ctx, message); err != nil {
		return fmt.Errorf(
			"failed to publish message to DLQ: %w",
			err,
		)
	}

	return nil
}

func (p *DLQProducer) Close() error {
	return p.writer.Close()
}
