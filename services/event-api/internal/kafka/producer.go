package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/segmentio/kafka-go"
)

type Producer struct {
	writer *kafka.Writer
}

func NewProducer(broker string, topic string) *Producer {
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:      []string{broker},
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: int(kafka.RequireAll),
	})

	return &Producer{
		writer: writer,
	}
}

func (p *Producer) Publish(
	ctx context.Context,
	event model.Event,
) error {

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	message := kafka.Message{
		Key:   []byte(event.Service),
		Value: payload,
	}

	if err := p.writer.WriteMessages(ctx, message); err != nil {
		return fmt.Errorf("failed to publish event to kafka: %w", err)
	}

	return nil
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
