package ai

import "time"

type IncidentContext struct {
	Incident  IncidentSummary   `json:"incident"`
	Events    []EventSummary    `json:"events"`
	Knowledge []KnowledgeResult `json:"knowledge"`
}

type IncidentSummary struct {
	ID        int64     `json:"id"`
	Service   string    `json:"service"`
	Severity  string    `json:"severity"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type EventSummary struct {
	EventID   string      `json:"event_id"`
	EventType string      `json:"event_type"`
	Service   string      `json:"service"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

type KnowledgeResult struct {
	DocumentID int64   `json:"document_id"`
	ChunkID    int64   `json:"chunk_id"`
	Distance   float64 `json:"distance"`
	Content    string  `json:"content"`
}
