package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/consumer"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/database"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/kafka"
	redisStore "github.com/HariDev-eng/opspilot/services/event-api/internal/redis"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
)

func main() {

	// --------------------------------------------------
	// Context
	// --------------------------------------------------

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown when Ctrl+C is pressed.
	signalChan := make(chan os.Signal, 1)

	signal.Notify(
		signalChan,
		os.Interrupt,
		syscall.SIGTERM,
	)

	go func() {
		<-signalChan

		log.Println("Shutdown signal received")

		cancel()
	}()

	// --------------------------------------------------
	// Database
	// --------------------------------------------------

	pool, err := database.NewPool()
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	redisClient, err := redisStore.NewClient("localhost:6379")
	if err != nil {
		log.Fatal(err)
	}
	defer redisClient.Close()

	incidentDetector := service.NewIncidentDetector(
		redisClient,
	)

	// --------------------------------------------------
	// Processed Event Repository
	// --------------------------------------------------

	processedRepository := repository.NewProcessedEventRepository(
		pool,
	)

	// --------------------------------------------------
	// Incident Repository
	// --------------------------------------------------

	incidentRepository := repository.NewIncidentRepository(pool)

	// --------------------------------------------------
	// Incident Processor
	// --------------------------------------------------

	incidentProcessor := service.NewIncidentProcessor(
		pool,
		processedRepository,
		incidentRepository,
		incidentDetector,
	)

	// --------------------------------------------------
	// DLQ Producer
	// --------------------------------------------------

	dlqProducer := kafka.NewDLQProducer(
		"localhost:9092",
		"ops.events.dlq",
	)

	defer dlqProducer.Close()

	// --------------------------------------------------
	// Kafka Consumer
	// --------------------------------------------------

	incidentConsumer := consumer.NewIncidentConsumer(
		"localhost:9092",
		"ops.events",
		"incident-detection",
		dlqProducer,
		incidentProcessor,
	)

	defer incidentConsumer.Close()

	// --------------------------------------------------
	// Start Consumer
	// --------------------------------------------------

	log.Println("Starting OpsPilot incident detection consumer")

	incidentConsumer.Start(ctx)

	log.Println("OpsPilot incident detection consumer stopped")
}
