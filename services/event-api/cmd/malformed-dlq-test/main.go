package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	eventConsumer "github.com/HariDev-eng/opspilot/services/event-api/internal/consumer"
	kafkaProducer "github.com/HariDev-eng/opspilot/services/event-api/internal/kafka"
	"github.com/segmentio/kafka-go"
)

const (
	broker      = "localhost:9092"
	sourceTopic = "ops.events.malformed-test"
	dlqTopic    = "ops.events.dlq"
)

func main() {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		45*time.Second,
	)
	defer cancel()

	testKey := fmt.Sprintf(
		"malformed-test-%d",
		time.Now().UnixNano(),
	)

	groupID := fmt.Sprintf(
		"malformed-dlq-test-%d",
		time.Now().UnixNano(),
	)

	// --------------------------------------------------
	// Start actual consumer with a dedicated group
	// --------------------------------------------------

	dlqProducer := kafkaProducer.NewDLQProducer(
		broker,
		dlqTopic,
	)
	defer dlqProducer.Close()

	incidentConsumer := eventConsumer.NewIncidentConsumer(
		broker,
		sourceTopic,
		groupID,
		dlqProducer,
		nil, // Malformed messages never reach the processor.
	)

	consumerDone := make(chan struct{})

	go func() {
		defer close(consumerDone)
		incidentConsumer.Start(ctx)
	}()

	defer func() {
		cancel()
		<-consumerDone
		incidentConsumer.Close()
	}()

	// --------------------------------------------------
	// Publish malformed message to source topic
	// --------------------------------------------------

	rawPayload := []byte(`{"event_id":`)

	writer := &kafka.Writer{
		Addr:         kafka.TCP(broker),
		Topic:        sourceTopic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
	}

	defer writer.Close()

	fmt.Println("1. Publishing malformed Kafka message")

	err := writer.WriteMessages(
		ctx,
		kafka.Message{
			Key:   []byte(testKey),
			Value: rawPayload,
		},
	)

	if err != nil {
		log.Fatalf("failed to publish malformed message: %v", err)
	}

	fmt.Println("   Malformed message published")
	fmt.Printf("   Test key: %s\n", testKey)

	// --------------------------------------------------
	// Read DLQ records
	// --------------------------------------------------

	fmt.Println("2. Waiting for raw DLQ message")

	dlqGroupID := fmt.Sprintf(
		"malformed-dlq-reader-%d",
		time.Now().UnixNano(),
	)

	dlqReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{broker},
		Topic:       dlqTopic,
		GroupID:     dlqGroupID,
		StartOffset: kafka.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
	})

	defer dlqReader.Close()

	var matched bool

	for {
		message, err := dlqReader.FetchMessage(ctx)
		if err != nil {
			log.Fatalf("failed reading DLQ: %v", err)
		}

		var dlqMessage kafkaProducer.RawDLQMessage

		if err := json.Unmarshal(
			message.Value,
			&dlqMessage,
		); err != nil {
			continue
		}

		// Ignore unrelated historical DLQ records.
		if !bytes.Equal(dlqMessage.OriginalKey, []byte(testKey)) {
			continue
		}

		fmt.Println("3. Found matching raw DLQ message")

		// --------------------------------------------------
		// Verify original payload
		// --------------------------------------------------

		if !bytes.Equal(dlqMessage.RawPayload, rawPayload) {
			log.Fatal("FAIL: original payload was changed")
		}

		fmt.Println("   PASS: original payload preserved")

		// --------------------------------------------------
		// Verify Kafka metadata
		// --------------------------------------------------

		if dlqMessage.OriginalTopic != sourceTopic {
			log.Fatalf(
				"FAIL: unexpected original topic: %s",
				dlqMessage.OriginalTopic,
			)
		}

		if !bytes.Equal(dlqMessage.OriginalKey, []byte(testKey)) {
			log.Fatal("FAIL: original Kafka key was changed")
		}

		if dlqMessage.OriginalPartition != 0 {
			log.Fatalf(
				"FAIL: unexpected partition: %d",
				dlqMessage.OriginalPartition,
			)
		}

		if dlqMessage.OriginalOffset < 0 {
			log.Fatal("FAIL: invalid original offset")
		}

		if dlqMessage.Error == "" {
			log.Fatal("FAIL: missing deserialization error")
		}

		fmt.Println("   PASS: original Kafka key preserved")
		fmt.Println("   PASS: original topic preserved")
		fmt.Println("   PASS: original partition preserved")
		fmt.Println("   PASS: original offset preserved")
		fmt.Println("   PASS: deserialization error preserved")

		fmt.Printf(
			"\nOriginal offset: %d\n",
			dlqMessage.OriginalOffset,
		)

		matched = true
		break
	}

	if !matched {
		log.Fatal("FAIL: matching DLQ message not found")
	}

	fmt.Println("\nMALFORMED MESSAGE DLQ TEST PASSED")
	fmt.Printf("Consumer group: %s\n", groupID)
}
