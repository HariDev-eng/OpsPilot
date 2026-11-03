package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
)

type ContextBuilder struct {
	incidentRepository  *repository.IncidentRepository
	eventRepository     *repository.EventRepository
	knowledgeRepository *repository.KnowledgeRepository
	embeddingProvider   EmbeddingProvider
}

func NewContextBuilder(
	incidentRepository *repository.IncidentRepository,
	eventRepository *repository.EventRepository,
	knowledgeRepository *repository.KnowledgeRepository,
	embeddingProvider EmbeddingProvider,
) *ContextBuilder {
	return &ContextBuilder{
		incidentRepository:  incidentRepository,
		eventRepository:     eventRepository,
		knowledgeRepository: knowledgeRepository,
		embeddingProvider:   embeddingProvider,
	}
}

func (b *ContextBuilder) Build(
	ctx context.Context,
	incidentID int64,
) (*IncidentContext, error) {

	// 1. Fetch incident
	incident, err := b.incidentRepository.GetByID(
		ctx,
		incidentID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get incident: %w",
			err,
		)
	}

	if incident == nil {
		return nil, fmt.Errorf(
			"incident %d not found",
			incidentID,
		)
	}

	// 2. Fetch related events
	events, err := b.eventRepository.GetByIncidentID(
		ctx,
		incidentID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get incident events: %w",
			err,
		)
	}

	// 3. Build incident context
	result := &IncidentContext{
		Incident: IncidentSummary{
			ID:        incident.ID,
			Service:   incident.Service,
			Severity:  incident.Severity,
			Status:    incident.Status,
			CreatedAt: incident.CreatedAt,
		},
		Events:    make([]EventSummary, 0, len(events)),
		Knowledge: make([]KnowledgeResult, 0),
	}

	for _, event := range events {
		result.Events = append(
			result.Events,
			EventSummary{
				EventID:   event.EventID,
				EventType: event.EventType,
				Service:   event.Service,
				Timestamp: event.Timestamp,
				Payload:   event.Payload,
			},
		)
	}

	// 4. Build semantic retrieval query
	retrievalQuery := buildRetrievalQuery(
		result.Incident,
		result.Events,
	)

	// 5. Generate query embedding
	embedding, err := b.embeddingProvider.Embed(
		ctx,
		retrievalQuery,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to generate retrieval embedding: %w",
			err,
		)
	}

	// 6. Retrieve relevant knowledge
	results, err := b.knowledgeRepository.SearchSimilarChunks(
		ctx,
		embedding,
		3,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to search knowledge base: %w",
			err,
		)
	}

	// 7. Convert into AI context
	for _, knowledge := range results {
		result.Knowledge = append(
			result.Knowledge,
			KnowledgeResult{
				DocumentID: knowledge.DocumentID,
				ChunkID:    knowledge.ID,
				Distance:   knowledge.Distance,
				Content:    knowledge.Content,
			},
		)
	}

	return result, nil
}

func buildRetrievalQuery(
	incident IncidentSummary,
	events []EventSummary,
) string {
	var builder strings.Builder

	fmt.Fprintf(
		&builder,
		"Service: %s\n",
		incident.Service,
	)

	fmt.Fprintf(
		&builder,
		"Severity: %s\n",
		incident.Severity,
	)

	fmt.Fprintf(
		&builder,
		"Status: %s\n",
		incident.Status,
	)

	builder.WriteString("\nEvents:\n")

	for _, event := range events {
		fmt.Fprintf(
			&builder,
			"- %s: %s\n",
			event.EventType,
			formatPayload(event.Payload),
		)
	}

	return builder.String()
}

func formatPayload(payload interface{}) string {
	switch value := payload.(type) {
	case string:
		return value
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprintf("%v", value)
		}

		return string(data)
	}
}
