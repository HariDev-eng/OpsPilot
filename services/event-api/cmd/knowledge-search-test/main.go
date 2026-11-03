package main

import (
	"context"
	"log"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/ai"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/database"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
)

func main() {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	pool, err := database.NewPool()
	if err != nil {
		log.Fatalf(
			"failed to connect to database: %v",
			err,
		)
	}

	defer pool.Close()

	embeddingProvider := ai.NewOllamaEmbeddingProvider(
		"http://localhost:11434",
		"nomic-embed-text",
	)

	knowledgeRepository := repository.NewKnowledgeRepository(pool)

	query := `
Payment API requests are failing because the
database connection pool is exhausted and requests
are timing out.
`

	log.Printf("Query: %s", query)

	embedding, err := embeddingProvider.Embed(
		ctx,
		query,
	)
	if err != nil {
		log.Fatalf(
			"failed to generate query embedding: %v",
			err,
		)
	}

	log.Printf(
		"Query embedding dimensions: %d",
		len(embedding),
	)

	results, err := knowledgeRepository.SearchSimilarChunks(
		ctx,
		embedding,
		5,
	)
	if err != nil {
		log.Fatalf(
			"failed to search knowledge: %v",
			err,
		)
	}

	log.Printf(
		"Found %d similar chunks",
		len(results),
	)

	for i, result := range results {
		log.Printf(
			"\nResult #%d\nDocument ID: %d\nChunk ID: %d\nDistance: %.6f\nContent:\n%s\n",
			i+1,
			result.DocumentID,
			result.ID,
			result.Distance,
			result.Content,
		)
	}
}
