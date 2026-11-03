package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/ai"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/database"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
)

func main() {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	pool, err := database.NewPool()
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	defer pool.Close()

	incidentRepository := repository.NewIncidentRepository(pool)
	eventRepository := repository.NewEventRepository(pool)

	knowledgeRepository := repository.NewKnowledgeRepository(pool)

	embeddingProvider := ai.NewOllamaEmbeddingProvider(
		"http://localhost:11434",
		"nomic-embed-text",
	)

	contextBuilder := ai.NewContextBuilder(
		incidentRepository,
		eventRepository,
		knowledgeRepository,
		embeddingProvider,
	)

	incidentID := int64(11)

	incidentContext, err := contextBuilder.Build(
		ctx,
		incidentID,
	)
	if err != nil {
		log.Fatalf("failed to build AI context: %v", err)
	}

	data, err := json.MarshalIndent(
		incidentContext,
		"",
		"  ",
	)
	if err != nil {
		log.Fatalf("failed to marshal AI context: %v", err)
	}

	log.Println("AI Context:")
	log.Println(string(data))
}
