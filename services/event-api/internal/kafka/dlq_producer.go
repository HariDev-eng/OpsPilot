package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	transports ...*kafka.Transport,
) *DLQProducer {
	var transport *kafka.Transport
	if len(transports) > 0 {
		transport = transports[0]
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(strings.Split(broker, ",")...),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		Transport:    transport,
	}

	return &DLQProducer{writer: writer}
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

type RawDLQMessage struct {
	RawPayload        []byte `json:"raw_payload"`
	OriginalKey       []byte `json:"original_key"`
	OriginalTopic     string `json:"original_topic"`
	OriginalPartition int    `json:"original_partition"`
	OriginalOffset    int64  `json:"original_offset"`
	Error             string `json:"error"`
	FailedAt          string `json:"failed_at"`
	RetryCount        int    `json:"retry_count"`
}

func (p *DLQProducer) PublishRaw(
	ctx context.Context,
	rawPayload []byte,
	originalKey []byte,
	originalTopic string,
	originalPartition int,
	originalOffset int64,
	errMessage string,
) error {

	payload := RawDLQMessage{
		RawPayload:        rawPayload,
		OriginalKey:       originalKey,
		OriginalTopic:     originalTopic,
		OriginalPartition: originalPartition,
		OriginalOffset:    originalOffset,
		Error:             errMessage,
		FailedAt:          time.Now().UTC().Format(time.RFC3339),
		RetryCount:        0,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf(
			"failed to marshal raw DLQ message: %w",
			err,
		)
	}

	message := kafka.Message{
		Key:   originalKey,
		Value: data,
	}

	if err := p.writer.WriteMessages(ctx, message); err != nil {
		return fmt.Errorf(
			"failed to publish raw message to DLQ: %w",
			err,
		)
	}

	return nil
}
