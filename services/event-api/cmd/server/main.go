package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/database"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/handler"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/kafka"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/publisher"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
)

func main() {
	pool, err := database.NewPool()
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	// Event dependencies
	eventRepo := repository.NewEventRepository(pool)
	eventService := service.NewEventService(eventRepo)
	eventHandler := handler.NewEventHandler(eventService)

	// Outbox dependencies
	outboxRepo := repository.NewOutboxRepository(pool)

	kafkaProducer := kafka.NewProducer(
		"localhost:9092",
		"ops.events",
	)

	defer kafkaProducer.Close()

	outboxPublisher := publisher.NewOutboxPublisher(
		outboxRepo,
		kafkaProducer,
		10,
		2*time.Second,
	)

	// Start background publisher
	ctx := context.Background()

	go outboxPublisher.Start(ctx)

	// HTTP server
	mux := http.NewServeMux()

	mux.HandleFunc(
		"POST /api/v1/events",
		eventHandler.CreateEvent,
	)

	log.Println("OpsPilot Event API running on :8080")

	err = http.ListenAndServe(":8080", mux)
	if err != nil {
		log.Fatal(err)
	}
}
