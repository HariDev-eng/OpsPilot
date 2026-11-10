package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/ai"
	incidentConsumer "github.com/HariDev-eng/opspilot/services/event-api/internal/consumer"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/database"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/handler"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/kafka"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/publisher"
	redisStore "github.com/HariDev-eng/opspilot/services/event-api/internal/redis"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
)

const redisAddr = "localhost:6379"

const (
	defaultLLMModel       = "@cf/google/gemma-4-26b-a4b-it"
	defaultEmbeddingModel = "@cf/baai/bge-base-en-v1.5"
)

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

func getEnvOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))

	if value == "" {
		return fallback
	}

	return value
}

func main() {
	// Database
	pool, err := database.NewPool()
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	// Redis
	redisClient, err := redisStore.NewClient(getRedisConnection())
	if err != nil {
		log.Fatal(err)
	}
	defer redisClient.Close()

	// Event API
	eventRepo := repository.NewEventRepository(pool)
	eventService := service.NewEventService(eventRepo)
	eventHandler := handler.NewEventHandler(eventService)

	// Integrations
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

	// Incidents
	incidentRepository := repository.NewIncidentRepository(pool)
	incidentService := service.NewIncidentService(
		incidentRepository,
		redisClient,
	)
	incidentHandler := handler.NewIncidentHandler(incidentService)

	// Actions
	actionRepository := repository.NewActionRepository(pool)
	actionService := service.NewActionService(
		pool,
		actionRepository,
	)
	actionHandler := handler.NewActionHandler(actionService)

	// AI repositories
	knowledgeRepository := repository.NewKnowledgeRepository(pool)
	aiAnalysisRepository := repository.NewAIAnalysisRepository(pool)

	// Cloudflare Workers AI configuration
	accountID := strings.TrimSpace(
		os.Getenv("CLOUDFLARE_ACCOUNT_ID"),
	)
	apiToken := strings.TrimSpace(
		os.Getenv("CLOUDFLARE_API_TOKEN"),
	)

	if accountID == "" || apiToken == "" {
		log.Fatal(
			"CLOUDFLARE_ACCOUNT_ID and CLOUDFLARE_API_TOKEN are required",
		)
	}

	llmModel := getEnvOrDefault(
		"CLOUDFLARE_LLM_MODEL",
		defaultLLMModel,
	)

	embeddingModel := getEnvOrDefault(
		"CLOUDFLARE_EMBEDDING_MODEL",
		defaultEmbeddingModel,
	)

	log.Printf(
		"Initializing Cloudflare AI: llm=%s embedding=%s",
		llmModel,
		embeddingModel,
	)

	// Cloudflare Gemma LLM
	llmProvider := ai.NewCloudflareLLM(
		accountID,
		apiToken,
		llmModel,
	)

	// Cloudflare embedding provider
	embeddingProvider := ai.NewCloudflareEmbeddingProvider(
		accountID,
		apiToken,
		embeddingModel,
	)

	// RAG context builder
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
		"cloudflare",
		llmModel,
	)

	aiHandler := handler.NewAIHandler(aiService)

	// Incident queries
	incidentQueryService := service.NewIncidentQueryService(
		incidentRepository,
		eventRepo,
		actionRepository,
	)

	incidentQueryHandler := handler.NewIncidentQueryHandler(
		incidentQueryService,
	)

	// Kafka configuration
	outboxRepo := repository.NewOutboxRepository(pool)

	kafkaBrokers := strings.Split(
		os.Getenv("KAFKA_BROKERS"),
		",",
	)

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

	// Incident detection consumer
	dlqTopic := getEnvOrDefault(
		"KAFKA_DLQ_TOPIC",
		"ops.events.dlq",
	)

	consumerGroup := getEnvOrDefault(
		"KAFKA_CONSUMER_GROUP",
		"incident-detection",
	)

	incidentDetector := service.NewIncidentDetector(redisClient)

	processedRepository := repository.NewProcessedEventRepository(pool)

	incidentProcessor := service.NewIncidentProcessor(
		pool,
		processedRepository,
		incidentRepository,
		incidentDetector,
		redisClient,
	)

	dlqProducer := kafka.NewDLQProducer(
		strings.Join(kafkaBrokers, ","),
		dlqTopic,
		kafkaTransport,
	)
	defer dlqProducer.Close()

	kafkaIncidentConsumer := incidentConsumer.NewIncidentConsumer(
		strings.Join(kafkaBrokers, ","),
		kafkaTopic,
		consumerGroup,
		dlqProducer,
		incidentProcessor,
		kafkaDialer,
	)
	defer kafkaIncidentConsumer.Close()

	checkKafka := func(ctx context.Context) error {
		conn, err := kafkaDialer.DialContext(
			ctx,
			"tcp",
			kafkaBrokers[0],
		)
		if err != nil {
			log.Printf(
				"Kafka readiness check failed: %v",
				err,
			)
			return err
		}

		if err := conn.Close(); err != nil {
			log.Printf(
				"Kafka connection close failed: %v",
				err,
			)
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

	// Outbox publisher
	outboxPublisher := publisher.NewOutboxPublisher(
		outboxRepo,
		kafkaProducer,
		10,
		2*time.Second,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go outboxPublisher.Start(ctx)

	go kafkaIncidentConsumer.Start(ctx)

	log.Println("Incident detection consumer launched")

	// Health
	healthHandler := handler.NewHealthHandler(
		pool,
		redisClient,
		checkKafka,
	)

	// Routes
	mux := http.NewServeMux()

	mux.HandleFunc(
		"GET /health/live",
		healthHandler.Live,
	)
	mux.HandleFunc(
		"GET /health/ready",
		healthHandler.Ready,
	)
	mux.HandleFunc(
		"GET /api/v1/health",
		healthHandler.Ready,
	)

	// Events
	mux.HandleFunc(
		"POST /api/v1/events",
		eventHandler.CreateEvent,
	)

	// Incident operations
	mux.HandleFunc(
		"POST /api/v1/incidents/{id}/acknowledge",
		incidentHandler.Acknowledge,
	)
	mux.HandleFunc(
		"POST /api/v1/incidents/{id}/resolve",
		incidentHandler.Resolve,
	)
	mux.HandleFunc(
		"GET /api/v1/incidents",
		incidentHandler.List,
	)
	mux.HandleFunc(
		"GET /api/v1/incidents/{id}",
		incidentHandler.GetByID,
	)

	// Incident events and actions
	mux.HandleFunc(
		"GET /api/v1/incidents/{id}/events",
		incidentQueryHandler.GetEvents,
	)
	mux.HandleFunc(
		"GET /api/v1/incidents/{id}/actions",
		incidentQueryHandler.GetActions,
	)
	mux.HandleFunc(
		"GET /api/v1/dashboard/summary",
		incidentQueryHandler.GetDashboardSummary,
	)

	// Action operations
	mux.HandleFunc(
		"POST /api/v1/actions/{id}/approve",
		actionHandler.Approve,
	)
	mux.HandleFunc(
		"POST /api/v1/actions/{id}/reject",
		actionHandler.Reject,
	)
	mux.HandleFunc(
		"POST /api/v1/actions/{id}/reconcile",
		actionHandler.Reconcile,
	)

	// Integration management
	mux.HandleFunc(
		"POST /api/v1/integrations",
		integrationHandler.Create,
	)
	mux.HandleFunc(
		"GET /api/v1/integrations",
		integrationHandler.List,
	)
	mux.HandleFunc(
		"DELETE /api/v1/integrations/{id}",
		integrationHandler.Revoke,
	)

	mux.HandleFunc(
		"POST /api/v1/integrations/events",
		integrationEventHandler.CreateEvent,
	)

	// AI analysis
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

	// HTTP server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           corsMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf(
		"OpsPilot Event API running on :%s",
		port,
	)

	log.Printf(
		"AI provider: Cloudflare, model: %s",
		llmModel,
	)

	if err := server.ListenAndServe(); err != nil &&
		err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	allowedOrigins := strings.Split(
		os.Getenv("CORS_ALLOWED_ORIGINS"),
		",",
	)

	return http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		origin := r.Header.Get("Origin")

		for _, allowed := range allowedOrigins {
			if origin != "" &&
				strings.TrimSpace(allowed) == origin {

				w.Header().Set(
					"Access-Control-Allow-Origin",
					origin,
				)
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
