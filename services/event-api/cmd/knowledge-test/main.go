package main

import (
	"context"
	"log"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/ai"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/database"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
)

type knowledgeEntry struct {
	title   string
	content string
}

func main() {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		2*time.Minute,
	)
	defer cancel()

	pool, err := database.NewPool()
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	embeddingProvider := ai.NewOllamaEmbeddingProvider(
		"http://localhost:11434",
		"nomic-embed-text",
	)

	knowledgeRepository := repository.NewKnowledgeRepository(pool)

	entries := []knowledgeEntry{
		{
			title: "Redis Timeout Troubleshooting",
			content: `
Redis timeout errors can cause requests to fail or become slow.

Common symptoms include Redis connection timeouts,
increased request latency, cache failures, and HTTP 500 responses.

Investigation steps:
1. Check whether Redis is reachable.
2. Check Redis connection counts.
3. Check Redis latency.
4. Check application Redis timeout metrics.
5. Check recent Redis or application deployments.

Mitigation:
- Restore Redis connectivity if unavailable.
- Investigate connection exhaustion.
- Increase timeout values only when evidence supports it.
- Avoid blindly restarting Redis.
`,
		},
		{
			title: "Kafka Consumer Lag",
			content: `
Kafka consumer lag occurs when consumers cannot process
messages as quickly as producers publish them.

Common symptoms include increasing consumer lag,
delayed event processing, growing Kafka partitions,
and delayed downstream processing.

Investigation steps:
1. Check consumer group lag.
2. Identify affected partitions.
3. Check consumer processing latency.
4. Check consumer errors.
5. Check recent deployments.

Mitigation:
- Investigate slow consumers.
- Increase consumer capacity when appropriate.
- Check partition distribution.
- Avoid deleting consumer offsets as a first response.
`,
		},
		{
			title: "Payment Service Deployment Failure",
			content: `
Payment service deployment failures can cause HTTP 500 errors,
increased latency, failed transactions, and elevated error rates.

Investigation steps:
1. Check the latest payment-service deployment.
2. Compare error rates before and after deployment.
3. Inspect application logs.
4. Check database and Redis dependencies.
5. Compare the current version with the previous stable version.

Mitigation:
- Roll back the deployment when evidence supports the deployment
  as the cause.
- Verify database and Redis health before rollback.
- Do not restart unrelated infrastructure without evidence.
`,
		},
	}

	for _, entry := range entries {
		document := model.KnowledgeDocument{
			Title:      entry.title,
			SourceType: "runbook",
		}

		documentID, err := knowledgeRepository.CreateDocument(
			ctx,
			document,
		)
		if err != nil {
			log.Fatalf(
				"failed to create document %q: %v",
				entry.title,
				err,
			)
		}

		log.Printf(
			"Knowledge document created: id=%d title=%s",
			documentID,
			entry.title,
		)

		embedding, err := embeddingProvider.Embed(
			ctx,
			entry.content,
		)
		if err != nil {
			log.Fatalf(
				"failed to generate embedding for %q: %v",
				entry.title,
				err,
			)
		}

		log.Printf(
			"Embedding generated: title=%s dimensions=%d",
			entry.title,
			len(embedding),
		)

		chunk := model.KnowledgeChunk{
			DocumentID: documentID,
			ChunkIndex: 0,
			Content:    entry.content,
			Embedding:  embedding,
		}

		chunkID, err := knowledgeRepository.CreateChunk(
			ctx,
			chunk,
		)
		if err != nil {
			log.Fatalf(
				"failed to create chunk for %q: %v",
				entry.title,
				err,
			)
		}

		log.Printf(
			"Knowledge chunk created: id=%d document_id=%d",
			chunkID,
			documentID,
		)
	}
}
