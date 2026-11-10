package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/ai"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/database"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/handler"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/kafka"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/publisher"
	redisStore "github.com/HariDev-eng/opspilot/services/event-api/internal/redis"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
)

const redisAddr = "localhost:6379"

func getRedisConnection() string {
	connection := os.Getenv("REDIS_URL")

	if connection == "" {
		connection = os.Getenv("REDIS_ADDR")
	}

	if connection == "" {
		connection = redisAddr
	}

	return connection
}

func getOperatorSecret() string {
	return os.Getenv("OPSPILOT_OPERATOR_SECRET")
}

func main() {
	pool, err := database.NewPool()
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	redisClient, err := redisStore.NewClient(getRedisConnection())
	if err != nil {
		log.Fatal(err)
	}
	defer redisClient.Close()

	eventRepo := repository.NewEventRepository(pool)
	eventService := service.NewEventService(eventRepo)
	eventHandler := handler.NewEventHandler(eventService)

	integrationRepo := repository.NewIntegrationRepository(pool)
	integrationService := service.NewIntegrationService(integrationRepo)
	integrationHandler := handler.NewIntegrationHandler(
		integrationService,
		getOperatorSecret(),
	)

	integrationEventHandler := handler.NewIntegrationEventHandler(
		eventService,
		integrationService,
	)

	incidentRepository := repository.NewIncidentRepository(pool)
	incidentService := service.NewIncidentService(incidentRepository, redisClient)
	incidentHandler := handler.NewIncidentHandler(incidentService)

	actionRepository := repository.NewActionRepository(pool)
	actionService := service.NewActionService(pool, actionRepository)
	actionHandler := handler.NewActionHandler(actionService)

	// AI repositories
	knowledgeRepository := repository.NewKnowledgeRepository(pool)
	aiAnalysisRepository := repository.NewAIAnalysisRepository(pool)

	// AI provider configuration
	ollamaURL := strings.TrimRight(
		os.Getenv("OLLAMA_BASE_URL"),
		"/",
	)

	ollamaEmbeddingModel := os.Getenv("OLLAMA_EMBEDDING_MODEL")
	ollamaLLMModel := os.Getenv("OLLAMA_LLM_MODEL")

	if ollamaURL == "" ||
		ollamaEmbeddingModel == "" ||
		ollamaLLMModel == "" {
		log.Fatal("AI provider configuration is incomplete")
	}

	// Existing Ollama providers
	embeddingProvider := ai.NewOllamaEmbeddingProvider(
		ollamaURL,
		ollamaEmbeddingModel,
	)

	llmProvider := ai.NewOllamaLLM(
		ollamaURL,
		ollamaLLMModel,
	)

	// Existing RAG context builder
	contextBuilder := ai.NewContextBuilder(
		incidentRepository,
		eventRepo,
		knowledgeRepository,
		embeddingProvider,
	)

	// AI service
	aiService := service.NewAIService(
		contextBuilder,
		llmProvider,
		aiAnalysisRepository,
		actionService,
		"ollama",
		ollamaLLMModel,
	)

	aiHandler := handler.NewAIHandler(aiService)

	incidentQueryService := service.NewIncidentQueryService(
		incidentRepository,
		eventRepo,
		actionRepository,
	)
	incidentQueryHandler := handler.NewIncidentQueryHandler(incidentQueryService)

	outboxRepo := repository.NewOutboxRepository(pool)

	kafkaBrokers := strings.Split(os.Getenv("KAFKA_BROKERS"), ",")
	for i := range kafkaBrokers {
		kafkaBrokers[i] = strings.TrimSpace(kafkaBrokers[i])
	}

	if len(kafkaBrokers) == 0 || kafkaBrokers[0] == "" {
		log.Fatal("KAFKA_BROKERS is required")
	}

	kafkaTopic := os.Getenv("KAFKA_TOPIC")
	if kafkaTopic == "" {
		kafkaTopic = "ops.events"
	}

	kafkaTransport, err := kafka.NewTransportFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	kafkaDialer := kafka.NewDialer(kafkaTransport)

	checkKafka := func(ctx context.Context) error {
		conn, err := kafkaDialer.DialContext(ctx, "tcp", kafkaBrokers[0])
		if err != nil {
			log.Printf("Kafka readiness check failed: %v", err)
			return err
		}

		if err := conn.Close(); err != nil {
			log.Printf("Kafka connection close failed: %v", err)
			return err
		}

		return nil
	}

	kafkaProducer := kafka.NewProducer(
		kafkaBrokers,
		kafkaTopic,
		kafkaTransport,
	)
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
	mux.HandleFunc("POST /api/v1/integrations", integrationHandler.Create)
	mux.HandleFunc("GET /api/v1/integrations", integrationHandler.List)
	mux.HandleFunc("DELETE /api/v1/integrations/{id}", integrationHandler.Revoke)
	mux.HandleFunc(
		"POST /api/v1/integrations/events",
		integrationEventHandler.CreateEvent,
	)
	mux.HandleFunc(
		"POST /api/v1/incidents/{id}/analyze",
		aiHandler.AnalyzeIncident,
	)

	mux.HandleFunc(
		"GET /api/v1/incidents/{id}/analysis",
		aiHandler.GetLatestAnalysis,
	)

	mux.HandleFunc(
		"GET /api/v1/incidents/{id}/analysis/history",
		aiHandler.GetAnalysisHistory,
	)

	mux.HandleFunc(
		"GET /api/v1/ai/status",
		aiHandler.Status,
	)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           corsMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("OpsPilot Event API running on :%s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	allowedOrigins := strings.Split(
		os.Getenv("CORS_ALLOWED_ORIGINS"),
		",",
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		for _, allowed := range allowedOrigins {
			if origin != "" && strings.TrimSpace(allowed) == origin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set(
					"Access-Control-Allow-Methods",
					"GET, POST, DELETE, OPTIONS",
				)
				w.Header().Set(
					"Access-Control-Allow-Headers",
					"Content-Type, Authorization",
				)
				break
			}
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
