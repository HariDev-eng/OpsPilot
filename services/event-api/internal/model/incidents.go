package model

import "time"

type Incident struct {
	ID             int64      `json:"id"`
	EventID        string     `json:"event_id"`
	Service        string     `json:"service"`
	EventType      string     `json:"event_type"`
	Severity       string     `json:"severity"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
}
