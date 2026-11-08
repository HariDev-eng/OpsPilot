package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/consumer"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/database"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/kafka"
	redisStore "github.com/HariDev-eng/opspilot/services/event-api/internal/redis"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"

	kafkago "github.com/segmentio/kafka-go"
)

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	pool, err := database.NewPool()
	if err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}
	defer pool.Close()

	redisConnection := envOrDefault(
		"REDIS_URL",
		envOrDefault("REDIS_ADDR", "localhost:6379"),
	)

	redisClient, err := redisStore.NewClient(redisConnection)
	if err != nil {
		log.Fatalf("Redis initialization failed: %v", err)
	}
	defer redisClient.Close()

	brokers := envOrDefault("KAFKA_BROKERS", "localhost:9092")
	kafkaTopic := envOrDefault("KAFKA_TOPIC", "ops.events")
	dlqTopic := envOrDefault("KAFKA_DLQ_TOPIC", "ops.events.dlq")
	groupID := envOrDefault("KAFKA_CONSUMER_GROUP", "incident-detection")

	// Local Kafka can run without SASL/TLS. If cloud credentials or a
	// custom CA are supplied, require the complete authenticated config.
	var transport *kafkago.Transport
	var dialer *kafkago.Dialer

	if os.Getenv("KAFKA_USERNAME") != "" ||
		os.Getenv("KAFKA_PASSWORD") != "" ||
		os.Getenv("KAFKA_CA_CERT") != "" ||
		os.Getenv("KAFKA_CA_CERT_PATH") != "" {
		transport, err = kafka.NewTransportFromEnv()
		if err != nil {
			log.Fatalf("Kafka transport initialization failed: %v", err)
		}
		dialer = kafka.NewDialer(transport)
	}

	incidentDetector := service.NewIncidentDetector(redisClient)
	processedRepository := repository.NewProcessedEventRepository(pool)
	incidentRepository := repository.NewIncidentRepository(pool)

	incidentProcessor := service.NewIncidentProcessor(
		pool,
		processedRepository,
		incidentRepository,
		incidentDetector,
		redisClient,
	)

	dlqProducer := kafka.NewDLQProducer(
		brokers,
		dlqTopic,
		transport,
	)
	defer dlqProducer.Close()

	incidentConsumer := consumer.NewIncidentConsumer(
		brokers,
		kafkaTopic,
		groupID,
		dlqProducer,
		incidentProcessor,
		dialer,
	)
	defer incidentConsumer.Close()

	log.Printf(
		"Starting OpsPilot incident consumer: brokers=%s topic=%s group=%s dlq=%s",
		brokers,
		kafkaTopic,
		groupID,
		dlqTopic,
	)

	incidentConsumer.Start(ctx)
	log.Println("OpsPilot incident consumer stopped")
}
