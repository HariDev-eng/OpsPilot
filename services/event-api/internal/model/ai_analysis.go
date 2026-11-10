package model

import (
	"encoding/json"
	"time"
)

type AIAnalysisStatus string

const (
	AIAnalysisPending   AIAnalysisStatus = "PENDING"
	AIAnalysisRunning   AIAnalysisStatus = "RUNNING"
	AIAnalysisCompleted AIAnalysisStatus = "COMPLETED"
	AIAnalysisFailed    AIAnalysisStatus = "FAILED"
)

type AIAnalysisRecord struct {
	ID         int64            `json:"id"`
	IncidentID int64            `json:"incident_id"`
	Status     AIAnalysisStatus `json:"status"`

	Provider string `json:"provider"`
	Model    string `json:"model"`

	Analysis     json.RawMessage `json:"analysis,omitempty"`
	ErrorMessage *string         `json:"error_message,omitempty"`

	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
