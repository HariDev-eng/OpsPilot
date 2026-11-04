package model

import "time"

type ActionStatus string

const (
	ActionStatusProposed               ActionStatus = "PROPOSED"
	ActionStatusPendingApproval        ActionStatus = "PENDING_APPROVAL"
	ActionStatusApproved               ActionStatus = "APPROVED"
	ActionStatusRejected               ActionStatus = "REJECTED"
	ActionStatusExecuting              ActionStatus = "EXECUTING"
	ActionStatusExecuted               ActionStatus = "EXECUTED"
	ActionStatusFailed                 ActionStatus = "FAILED"
	ActionStatusReconciliationRequired ActionStatus = "RECONCILIATION_REQUIRED"
)

type ReconciliationDecision string

const (
	ReconciliationConfirmedSuccess ReconciliationDecision = "CONFIRMED_SUCCESS"

	ReconciliationNotPerformed ReconciliationDecision = "CONFIRMED_NOT_PERFORMED"

	ReconciliationUnknown ReconciliationDecision = "UNKNOWN"
)

type IncidentAction struct {
	ID               int64        `json:"id"`
	IncidentID       int64        `json:"incident_id"`
	Description      string       `json:"description"`
	ActionType       string       `json:"action_type"`
	Status           ActionStatus `json:"status"`
	RequiresApproval bool         `json:"requires_approval"`
	IdempotencyKey   string       `json:"idempotency_key"`

	ApprovedBy *string    `json:"approved_by,omitempty"`
	RejectedBy *string    `json:"rejected_by,omitempty"`
	ApprovedAt *time.Time `json:"approved_at,omitempty"`
	RejectedAt *time.Time `json:"rejected_at,omitempty"`

	// Server-approved operation configuration.
	OperationKey *string        `json:"operation_key,omitempty"`
	Parameters   map[string]any `json:"parameters"`

	// Execution lifecycle metadata.
	ExecutionStartedAt *time.Time `json:"execution_started_at,omitempty"`
	ExecutedAt         *time.Time `json:"executed_at,omitempty"`
	ExecutionError     *string    `json:"execution_error,omitempty"`
	ExecutionAttempts  int        `json:"execution_attempts"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ActionAuditLog struct {
	ID        int64          `json:"id"`
	ActionID  int64          `json:"action_id"`
	ActorID   string         `json:"actor_id"`
	Action    string         `json:"action"`
	Details   map[string]any `json:"details"`
	CreatedAt time.Time      `json:"created_at"`
}
