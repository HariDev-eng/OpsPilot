package model

import (
	"encoding/json"
	"time"
)

type Event struct {
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	Service       string          `json:"service"`
	Timestamp     time.Time       `json:"timestamp"`
	Payload       json.RawMessage `json:"payload"`
	CreatedAt     time.Time       `json:"created_at"`
	IntegrationID *string         `json:"integration_id,omitempty"`
}

type CreateEventRequest struct {
	EventID   string          `json:"event_id"`
	EventType string          `json:"event_type"`
	Service   string          `json:"service"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}
