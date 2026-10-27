package main

import (
	"context"
	"log"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/kafka"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
)

func main() {

	producer := kafka.NewProducer(
		"localhost:9092",
		"ops.events",
	)

	defer producer.Close()

	event := model.Event{
		EventID:   "go-kafka-test-001",
		EventType: "payment.failed",
		Service:   "payment-service",
		Timestamp: time.Now(),
		Payload: []byte(`{
			"payment_id": "pay-001",
			"error_code": "TIMEOUT"
		}`),
	}

	err := producer.Publish(
		context.Background(),
		event,
	)

	if err != nil {
		log.Fatal(err)
	}

	log.Println("Event published successfully")
}
