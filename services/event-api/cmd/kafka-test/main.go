package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	opskafka "github.com/HariDev-eng/opspilot/services/event-api/internal/kafka"
	kafkago "github.com/segmentio/kafka-go"
)

func main() {
	brokers := strings.Split(os.Getenv("KAFKA_BROKERS"), ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}

	if len(brokers) == 0 || brokers[0] == "" {
		log.Fatal("KAFKA_BROKERS is not configured")
	}

	topic := os.Getenv("KAFKA_TOPIC")
	if topic == "" {
		topic = "ops.events"
	}

	transport, err := opskafka.NewTransportFromEnv()
	if err != nil {
		log.Fatalf("Kafka transport error: %v", err)
	}

	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		GroupID:     "opspilot-verification-once",
		Dialer:      opskafka.NewDialer(transport),
		MinBytes:    1,
		MaxBytes:    10e6,
		StartOffset: kafkago.FirstOffset,
	})
	defer reader.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Printf("Waiting for messages from topic: %s\n", topic)

	msg, err := reader.FetchMessage(ctx)
	if err != nil {
		log.Fatalf("Failed to fetch Kafka message: %v", err)
	}

	fmt.Println("Kafka message received!")
	fmt.Printf("Topic:     %s\n", msg.Topic)
	fmt.Printf("Partition: %d\n", msg.Partition)
	fmt.Printf("Offset:    %d\n", msg.Offset)
	fmt.Printf("Key:       %s\n", string(msg.Key))
	fmt.Printf("Value:     %s\n", string(msg.Value))
}
