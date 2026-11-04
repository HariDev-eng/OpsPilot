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
		120*time.Second,
	)
	defer cancel()

	// PostgreSQL
	pool, err := database.NewPool()
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Repositories
	incidentRepository := repository.NewIncidentRepository(pool)
	eventRepository := repository.NewEventRepository(pool)
	knowledgeRepository := repository.NewKnowledgeRepository(pool)

	// Embedding provider for RAG
	embeddingProvider := ai.NewOllamaEmbeddingProvider(
		"http://localhost:11434",
		"nomic-embed-text",
	)

	// Context builder
	contextBuilder := ai.NewContextBuilder(
		incidentRepository,
		eventRepository,
		knowledgeRepository,
		embeddingProvider,
	)

	// Build AI context for Incident #11
	incidentID := int64(11)

	incidentContext, err := contextBuilder.Build(
		ctx,
		incidentID,
	)
	if err != nil {
		log.Fatalf("failed to build AI context: %v", err)
	}

	// Local LLM
	llm := ai.NewOllamaLLM(
		"http://localhost:11434",
		"llama3.2",
	)

	// Analyze incident
	analysis, err := llm.Analyze(
		ctx,
		*incidentContext,
	)
	if err != nil {
		log.Fatalf("failed to analyze incident: %v", err)
	}

	// Print structured analysis
	data, err := json.MarshalIndent(
		analysis,
		"",
		"  ",
	)
	if err != nil {
		log.Fatalf("failed to marshal analysis: %v", err)
	}

	log.Println("AI Incident Analysis:")
	log.Println(string(data))
}
