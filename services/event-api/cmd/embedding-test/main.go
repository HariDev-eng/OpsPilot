package main

import (
	"context"
	"log"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/ai"
)

func main() {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	provider := ai.NewOllamaEmbeddingProvider(
		"http://localhost:11434",
		"nomic-embed-text",
	)

	text := "Payment service database connection pool exhausted"

	embedding, err := provider.Embed(ctx, text)
	if err != nil {
		log.Fatalf("failed to generate embedding: %v", err)
	}

	log.Printf("Embedding generated successfully")
	log.Printf("Dimensions: %d", len(embedding))

	if len(embedding) > 5 {
		log.Printf(
			"First 5 values: %v",
			embedding[:5],
		)
	}
}
