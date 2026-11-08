package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	kafkago "github.com/segmentio/kafka-go"
)

type Producer struct {
	writer *kafkago.Writer
}

func NewProducer(
	brokers []string,
	topic string,
	transport *kafkago.Transport,
) *Producer {
	writer := &kafkago.Writer{
		Addr:         kafkago.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafkago.Hash{},
		RequiredAcks: kafkago.RequireAll,
		Transport:    transport,
	}

	return &Producer{writer: writer}
}

func (p *Producer) Publish(
	ctx context.Context,
	event model.Event,
) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	message := kafkago.Message{
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
