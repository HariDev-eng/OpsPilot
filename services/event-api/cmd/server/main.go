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
	redisStore "github.com/HariDev-eng/opspilot/services/event-api/internal/redis"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"

	kafkago "github.com/segmentio/kafka-go"
)

const (
	kafkaBroker = "localhost:9092"
	kafkaTopic  = "ops.events"
	redisAddr   = "localhost:6379"
)

func checkKafka(ctx context.Context) error {
	conn, err := kafkago.DialContext(ctx, "tcp", kafkaBroker)
	if err != nil {
		return err
	}
	return conn.Close()
}

func main() {
	pool, err := database.NewPool()
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	redisClient, err := redisStore.NewClient(redisAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer redisClient.Close()

	eventRepo := repository.NewEventRepository(pool)
	eventService := service.NewEventService(eventRepo)
	eventHandler := handler.NewEventHandler(eventService)

	incidentRepository := repository.NewIncidentRepository(pool)
	incidentService := service.NewIncidentService(incidentRepository, redisClient)
	incidentHandler := handler.NewIncidentHandler(incidentService)

	actionRepository := repository.NewActionRepository(pool)
	actionService := service.NewActionService(pool, actionRepository)
	actionHandler := handler.NewActionHandler(actionService)

	incidentQueryService := service.NewIncidentQueryService(
		incidentRepository,
		eventRepo,
		actionRepository,
	)
	incidentQueryHandler := handler.NewIncidentQueryHandler(incidentQueryService)

	outboxRepo := repository.NewOutboxRepository(pool)
	kafkaProducer := kafka.NewProducer(kafkaBroker, kafkaTopic)
	defer kafkaProducer.Close()

	outboxPublisher := publisher.NewOutboxPublisher(
		outboxRepo,
		kafkaProducer,
		10,
		2*time.Second,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go outboxPublisher.Start(ctx)

	healthHandler := handler.NewHealthHandler(pool, redisClient, checkKafka)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", healthHandler.Live)
	mux.HandleFunc("GET /health/ready", healthHandler.Ready)
	mux.HandleFunc("GET /api/v1/health", healthHandler.Ready)

	mux.HandleFunc("POST /api/v1/events", eventHandler.CreateEvent)
	mux.HandleFunc("POST /api/v1/incidents/{id}/acknowledge", incidentHandler.Acknowledge)
	mux.HandleFunc("POST /api/v1/incidents/{id}/resolve", incidentHandler.Resolve)
	mux.HandleFunc("POST /api/v1/actions/{id}/approve", actionHandler.Approve)
	mux.HandleFunc("POST /api/v1/actions/{id}/reject", actionHandler.Reject)
	mux.HandleFunc("POST /api/v1/actions/{id}/reconcile", actionHandler.Reconcile)
	mux.HandleFunc("GET /api/v1/incidents", incidentHandler.List)
	mux.HandleFunc("GET /api/v1/incidents/{id}", incidentHandler.GetByID)
	mux.HandleFunc("GET /api/v1/incidents/{id}/events", incidentQueryHandler.GetEvents)
	mux.HandleFunc("GET /api/v1/incidents/{id}/actions", incidentQueryHandler.GetActions)
	mux.HandleFunc("GET /api/v1/dashboard/summary", incidentQueryHandler.GetDashboardSummary)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("OpsPilot Event API running on :8080")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
