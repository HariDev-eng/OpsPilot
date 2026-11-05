package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrActionNotFoundOrNotPending = errors.New(
		"action does not exist or is not pending approval",
	)

	ErrIdempotencyConflict = errors.New(
		"idempotency key already belongs to a different action",
	)
	ErrActionNotAssignable = errors.New(
		"action is not eligible for operation assignment",
	)

	ErrOperationAlreadyAssigned = errors.New(
		"action already has a different operation assignment",
	)
	ErrActionNotExecutable = errors.New(
		"action does not exist or is not eligible for execution",
	)
	ErrActionNotReconciliationRequired = errors.New(
		"action is not awaiting reconciliation",
	)
)

const (
	ActionStatusProposed               = "PROPOSED"
	ActionStatusPendingApproval        = "PENDING_APPROVAL"
	ActionStatusApproved               = "APPROVED"
	ActionStatusRejected               = "REJECTED"
	ActionStatusExecuting              = "EXECUTING"
	ActionStatusExecuted               = "EXECUTED"
	ActionStatusFailed                 = "FAILED"
	ActionStatusReconciliationRequired = "RECONCILIATION_REQUIRED"
)

type ActionRepository struct {
	pool *pgxpool.Pool
}

func NewActionRepository(pool *pgxpool.Pool) *ActionRepository {
	return &ActionRepository{
		pool: pool,
	}
}

const actionColumns = `
	id,
	incident_id,
	description,
	action_type,
	status,
	requires_approval,
	idempotency_key,
	approved_by,
	rejected_by,
	approved_at,
	rejected_at,
	operation_key,
	parameters,
	execution_started_at,
	executed_at,
	execution_error,
	execution_attempts,
	created_at,
	updated_at
`

func scanIncidentAction(row pgx.Row) (*model.IncidentAction, error) {
	var action model.IncidentAction
	var parametersJSON []byte

	err := row.Scan(
		&action.ID,
		&action.IncidentID,
		&action.Description,
		&action.ActionType,
		&action.Status,
		&action.RequiresApproval,
		&action.IdempotencyKey,
		&action.ApprovedBy,
		&action.RejectedBy,
		&action.ApprovedAt,
		&action.RejectedAt,
		&action.OperationKey,
		&parametersJSON,
		&action.ExecutionStartedAt,
		&action.ExecutedAt,
		&action.ExecutionError,
		&action.ExecutionAttempts,
		&action.CreatedAt,
		&action.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if len(parametersJSON) > 0 {
		if err := json.Unmarshal(parametersJSON, &action.Parameters); err != nil {
			return nil, fmt.Errorf(
				"failed to decode action parameters: %w",
				err,
			)
		}
	}

	if action.Parameters == nil {
		action.Parameters = make(map[string]any)
	}

	return &action, nil
}

// CreateTx creates an action inside an existing transaction.
//
// If the idempotency key already exists with identical action data,
// the existing action is returned instead of creating a duplicate.
func (r *ActionRepository) CreateTx(
	ctx context.Context,
	tx pgx.Tx,
	action *model.IncidentAction,
) (*model.IncidentAction, bool, error) {

	query := `
		INSERT INTO incident_actions (
			incident_id,
			description,
			action_type,
			status,
			requires_approval,
			idempotency_key
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING ` + actionColumns

	created, err := scanIncidentAction(
		tx.QueryRow(
			ctx,
			query,
			action.IncidentID,
			action.Description,
			action.ActionType,
			action.Status,
			action.RequiresApproval,
			action.IdempotencyKey,
		),
	)

	if err == nil {
		return created, true, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf(
			"failed to create incident action: %w",
			err,
		)
	}

	// The idempotency key already exists.
	existing, err := scanIncidentAction(
		tx.QueryRow(
			ctx,
			`
			SELECT `+actionColumns+`
			FROM incident_actions
			WHERE idempotency_key = $1
			`,
			action.IdempotencyKey,
		),
	)

	if err != nil {
		return nil, false, fmt.Errorf(
			"failed to retrieve existing action: %w",
			err,
		)
	}

	// Never silently accept a key reused for different action data.
	if existing.IncidentID != action.IncidentID ||
		existing.Description != action.Description ||
		existing.ActionType != action.ActionType ||
		existing.RequiresApproval != action.RequiresApproval {

		return nil, false, ErrIdempotencyConflict
	}

	return existing, false, nil
}

// CreateAuditLogTx inserts an audit record inside an existing transaction.
func (r *ActionRepository) CreateAuditLogTx(
	ctx context.Context,
	tx pgx.Tx,
	audit model.ActionAuditLog,
) error {

	details, err := json.Marshal(audit.Details)
	if err != nil {
		return fmt.Errorf(
			"failed to encode audit details: %w",
			err,
		)
	}

	_, err = tx.Exec(
		ctx,
		`
		INSERT INTO action_audit_logs (
			action_id,
			actor_id,
			action,
			details
		)
		VALUES ($1, $2, $3, $4::jsonb)
		`,
		audit.ActionID,
		audit.ActorID,
		audit.Action,
		details,
	)

	if err != nil {
		return fmt.Errorf(
			"failed to create action audit log: %w",
			err,
		)
	}

	return nil
}

// GetByID retrieves an action by its ID.
func (r *ActionRepository) GetByID(
	ctx context.Context,
	actionID int64,
) (*model.IncidentAction, error) {

	action, err := scanIncidentAction(
		r.pool.QueryRow(
			ctx,
			`
			SELECT `+actionColumns+`
			FROM incident_actions
			WHERE id = $1
			`,
			actionID,
		),
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}

		return nil, fmt.Errorf(
			"failed to retrieve incident action: %w",
			err,
		)
	}

	return action, nil
}

// ListByIncident retrieves all actions associated with an incident.
func (r *ActionRepository) ListByIncident(
	ctx context.Context,
	incidentID int64,
) ([]model.IncidentAction, error) {

	rows, err := r.pool.Query(
		ctx,
		`
		SELECT `+actionColumns+`
		FROM incident_actions
		WHERE incident_id = $1
		ORDER BY created_at DESC
		`,
		incidentID,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"failed to list incident actions: %w",
			err,
		)
	}

	defer rows.Close()

	actions := make([]model.IncidentAction, 0)

	for rows.Next() {
		action, err := scanIncidentAction(rows)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to scan incident action: %w",
				err,
			)
		}

		actions = append(actions, *action)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"failed while reading incident actions: %w",
			err,
		)
	}

	return actions, nil
}

// Approve transitions:
//
// PENDING_APPROVAL → APPROVED
//
// The state transition and audit record are atomic.
func (r *ActionRepository) Approve(
	ctx context.Context,
	actionID int64,
	actorID string,
) error {

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf(
			"failed to begin approval transaction: %w",
			err,
		)
	}

	defer tx.Rollback(ctx)

	action, err := scanIncidentAction(
		tx.QueryRow(
			ctx,
			`
			UPDATE incident_actions
			SET
				status = $1,
				approved_by = $2,
				approved_at = NOW(),
				updated_at = NOW()
			WHERE id = $3
			  AND status = $4
			  AND requires_approval = TRUE
			RETURNING `+actionColumns,
			ActionStatusApproved,
			actorID,
			actionID,
			ActionStatusPendingApproval,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrActionNotFoundOrNotPending
	}

	if err != nil {
		return fmt.Errorf(
			"failed to approve action: %w",
			err,
		)
	}

	err = r.CreateAuditLogTx(
		ctx,
		tx,
		model.ActionAuditLog{
			ActionID: action.ID,
			ActorID:  actorID,
			Action:   "APPROVED",
			Details: map[string]any{
				"previous_status": ActionStatusPendingApproval,
				"new_status":      ActionStatusApproved,
			},
		},
	)

	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(
			"failed to commit approval transaction: %w",
			err,
		)
	}

	return nil
}

// Reject transitions:
//
// PENDING_APPROVAL → REJECTED
//
// The state transition and audit record are atomic.
func (r *ActionRepository) Reject(
	ctx context.Context,
	actionID int64,
	actorID string,
	reason string,
) error {

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf(
			"failed to begin rejection transaction: %w",
			err,
		)
	}

	defer tx.Rollback(ctx)

	action, err := scanIncidentAction(
		tx.QueryRow(
			ctx,
			`
			UPDATE incident_actions
			SET
				status = $1,
				rejected_by = $2,
				rejected_at = NOW(),
				updated_at = NOW()
			WHERE id = $3
			  AND status = $4
			  AND requires_approval = TRUE
			RETURNING `+actionColumns,
			ActionStatusRejected,
			actorID,
			actionID,
			ActionStatusPendingApproval,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrActionNotFoundOrNotPending
	}

	if err != nil {
		return fmt.Errorf(
			"failed to reject action: %w",
			err,
		)
	}

	err = r.CreateAuditLogTx(
		ctx,
		tx,
		model.ActionAuditLog{
			ActionID: action.ID,
			ActorID:  actorID,
			Action:   "REJECTED",
			Details: map[string]any{
				"previous_status": ActionStatusPendingApproval,
				"new_status":      ActionStatusRejected,
				"reason":          reason,
			},
		},
	)

	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(
			"failed to commit rejection transaction: %w",
			err,
		)
	}

	return nil
}

// AssignOperation binds a registered backend operation to an action.
//
// Assignment and its audit record are committed atomically.
// Repeating an identical assignment is idempotent.
func (r *ActionRepository) AssignOperation(
	ctx context.Context,
	actionID int64,
	actorID string,
	operationKey string,
	description string,
	actionType string,
	requiresApproval bool,
	status model.ActionStatus,
	parameters map[string]any,
) (*model.IncidentAction, error) {

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to begin operation assignment transaction: %w",
			err,
		)
	}
	defer tx.Rollback(ctx)

	// Lock the action so concurrent assignment requests cannot
	// assign different operations to the same action.
	existing, err := scanIncidentAction(
		tx.QueryRow(
			ctx,
			`
			SELECT `+actionColumns+`
			FROM incident_actions
			WHERE id = $1
			FOR UPDATE
			`,
			actionID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf(
			"failed to retrieve action for assignment: %w",
			err,
		)
	}

	parameterJSON, err := json.Marshal(parameters)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to encode operation parameters: %w",
			err,
		)
	}

	// An identical retry must not create another audit record.
	if existing.OperationKey != nil {
		existingParameters, marshalErr := json.Marshal(
			existing.Parameters,
		)
		if marshalErr != nil {
			return nil, marshalErr
		}

		sameAssignment :=
			*existing.OperationKey == operationKey &&
				existing.Description == description &&
				existing.ActionType == actionType &&
				existing.RequiresApproval == requiresApproval &&
				existing.Status == status &&
				bytes.Equal(existingParameters, parameterJSON)

		if !sameAssignment {
			return nil, ErrOperationAlreadyAssigned
		}

		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf(
				"failed to commit idempotent assignment: %w",
				err,
			)
		}

		return existing, nil
	}

	// Only unassigned recommendations can receive an operation.
	if existing.Status != model.ActionStatusProposed &&
		existing.Status != model.ActionStatusPendingApproval {
		return nil, ErrActionNotAssignable
	}

	assigned, err := scanIncidentAction(
		tx.QueryRow(
			ctx,
			`
			UPDATE incident_actions
			SET
				operation_key = $1,
				description = $2,
				action_type = $3,
				requires_approval = $4,
				status = $5,
				parameters = $6::jsonb,
				updated_at = NOW()
			WHERE id = $7
			RETURNING `+actionColumns,
			operationKey,
			description,
			actionType,
			requiresApproval,
			status,
			parameterJSON,
			actionID,
		),
	)

	if err != nil {
		return nil, fmt.Errorf(
			"failed to assign operation: %w",
			err,
		)
	}

	err = r.CreateAuditLogTx(
		ctx,
		tx,
		model.ActionAuditLog{
			ActionID: assigned.ID,
			ActorID:  actorID,
			Action:   "OPERATION_ASSIGNED",
			Details: map[string]any{
				"operation_key":     operationKey,
				"action_type":       actionType,
				"requires_approval": requiresApproval,
				"initial_status":    existing.Status,
				"new_status":        status,
			},
		},
	)

	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf(
			"failed to commit operation assignment: %w",
			err,
		)
	}

	return assigned, nil
}

// ClaimExecution atomically claims an eligible action.
//
// Investigation: PROPOSED -> EXECUTING
// Mitigation/destructive: APPROVED -> EXECUTING
//
// Only one concurrent worker can successfully claim the action.
func (r *ActionRepository) ClaimExecution(
	ctx context.Context,
	actionID int64,
	actorID string,
	operationKey string,
	actionType string,
	requiresApproval bool,
) (*model.IncidentAction, error) {

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to begin execution claim: %w",
			err,
		)
	}
	defer tx.Rollback(ctx)

	action, err := scanIncidentAction(
		tx.QueryRow(
			ctx,
			`
			UPDATE incident_actions
			SET
				status = $1,
				execution_started_at = NOW(),
				execution_attempts = execution_attempts + 1,
				execution_error = NULL,
				updated_at = NOW()
			WHERE id = $2
			  AND operation_key = $3
			  AND action_type = $4
			  AND requires_approval = $5
			  AND (
				(
					requires_approval = TRUE
					AND status = 'APPROVED'
				)
				OR
				(
					requires_approval = FALSE
					AND status = 'PROPOSED'
				)
			  )
			RETURNING `+actionColumns,
			ActionStatusExecuting,
			actionID,
			operationKey,
			actionType,
			requiresApproval,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrActionNotExecutable
	}
	if err != nil {
		return nil, fmt.Errorf(
			"failed to claim action execution: %w",
			err,
		)
	}

	err = r.CreateAuditLogTx(
		ctx,
		tx,
		model.ActionAuditLog{
			ActionID: action.ID,
			ActorID:  actorID,
			Action:   "EXECUTION_STARTED",
			Details: map[string]any{
				"operation_key": action.OperationKey,
				"attempt":       action.ExecutionAttempts,
				"previous_status": func() string {
					if requiresApproval {
						return ActionStatusApproved
					}
					return ActionStatusProposed
				}(),
				"new_status": ActionStatusExecuting,
			},
		},
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf(
			"failed to commit execution claim: %w",
			err,
		)
	}

	return action, nil
}

// MarkExecuted records successful execution.
//
// EXECUTING -> EXECUTED
//
// The state update and audit record are atomic.
func (r *ActionRepository) MarkExecuted(
	ctx context.Context,
	actionID int64,
	actorID string,
	result map[string]any,
) error {

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf(
			"failed to begin execution completion transaction: %w",
			err,
		)
	}
	defer tx.Rollback(ctx)

	action, err := scanIncidentAction(
		tx.QueryRow(
			ctx,
			`
			UPDATE incident_actions
			SET
				status = $1,
				executed_at = NOW(),
				execution_error = NULL,
				updated_at = NOW()
			WHERE id = $2
			  AND status = $3
			RETURNING `+actionColumns,
			ActionStatusExecuted,
			actionID,
			ActionStatusExecuting,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrActionNotExecutable
	}
	if err != nil {
		return fmt.Errorf(
			"failed to mark action executed: %w",
			err,
		)
	}

	if result == nil {
		result = map[string]any{}
	}

	err = r.CreateAuditLogTx(
		ctx,
		tx,
		model.ActionAuditLog{
			ActionID: action.ID,
			ActorID:  actorID,
			Action:   "EXECUTED",
			Details: map[string]any{
				"previous_status": ActionStatusExecuting,
				"new_status":      ActionStatusExecuted,
				"result":          result,
			},
		},
	)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(
			"failed to commit execution completion: %w",
			err,
		)
	}

	return nil
}

// MarkFailed records an execution failure.
//
// EXECUTING -> FAILED
//
// The state update and audit record are atomic.
func (r *ActionRepository) MarkFailed(
	ctx context.Context,
	actionID int64,
	actorID string,
	executionError string,
) error {

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf(
			"failed to begin execution failure transaction: %w",
			err,
		)
	}
	defer tx.Rollback(ctx)

	action, err := scanIncidentAction(
		tx.QueryRow(
			ctx,
			`
			UPDATE incident_actions
			SET
				status = $1,
				execution_error = $2,
				updated_at = NOW()
			WHERE id = $3
			  AND status = $4
			RETURNING `+actionColumns,
			ActionStatusFailed,
			executionError,
			actionID,
			ActionStatusExecuting,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrActionNotExecutable
	}
	if err != nil {
		return fmt.Errorf(
			"failed to mark action failed: %w",
			err,
		)
	}

	err = r.CreateAuditLogTx(
		ctx,
		tx,
		model.ActionAuditLog{
			ActionID: action.ID,
			ActorID:  actorID,
			Action:   "EXECUTION_FAILED",
			Details: map[string]any{
				"previous_status": ActionStatusExecuting,
				"new_status":      ActionStatusFailed,
				"error":           executionError,
			},
		},
	)
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(
			"failed to commit execution failure: %w",
			err,
		)
	}

	return nil
}

const staleExecutionBatchSize = 20

// MarkStaleExecutionsForReconciliation moves stale EXECUTING actions
// into RECONCILIATION_REQUIRED.
//
// It never retries an operation. The external outcome must be verified
// before anyone decides whether the action can be executed again.
func (r *ActionRepository) MarkStaleExecutionsForReconciliation(
	ctx context.Context,
	olderThan time.Duration,
	actorID string,
) ([]*model.IncidentAction, error) {

	if olderThan <= 0 {
		return nil, fmt.Errorf("olderThan must be greater than zero")
	}

	if actorID == "" {
		return nil, fmt.Errorf("actorID cannot be empty")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to begin stale execution recovery: %w",
			err,
		)
	}
	defer tx.Rollback(ctx)

	// Lock eligible rows so concurrent recovery workers cannot
	// process the same actions.
	rows, err := tx.Query(
		ctx,
		`
		SELECT id
		FROM incident_actions
		WHERE status = $1
  AND execution_started_at IS NOT NULL
  AND execution_started_at <
      NOW() - ($2 * INTERVAL '1 microsecond')
		ORDER BY execution_started_at ASC
		LIMIT $3
		FOR UPDATE SKIP LOCKED
        `,
		ActionStatusExecuting,
		olderThan.Microseconds(),
		staleExecutionBatchSize,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to find stale executions: %w",
			err,
		)
	}

	var actionIDs []int64

	for rows.Next() {
		var actionID int64

		if err := rows.Scan(&actionID); err != nil {
			rows.Close()
			return nil, fmt.Errorf(
				"failed to scan stale action ID: %w",
				err,
			)
		}

		actionIDs = append(actionIDs, actionID)
	}

	rowsErr := rows.Err()
	rows.Close()

	if rowsErr != nil {
		return nil, fmt.Errorf(
			"failed while reading stale executions: %w",
			rowsErr,
		)
	}

	recovered := make([]*model.IncidentAction, 0, len(actionIDs))

	for _, actionID := range actionIDs {
		action, err := scanIncidentAction(
			tx.QueryRow(
				ctx,
				`
                UPDATE incident_actions
                SET
                    status = $1,
                    updated_at = NOW()
                WHERE id = $2
                  AND status = $3
                RETURNING `+actionColumns,
				ActionStatusReconciliationRequired,
				actionID,
				ActionStatusExecuting,
			),
		)

		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}

		if err != nil {
			return nil, fmt.Errorf(
				"failed to recover action %d: %w",
				actionID,
				err,
			)
		}

		err = r.CreateAuditLogTx(
			ctx,
			tx,
			model.ActionAuditLog{
				ActionID: action.ID,
				ActorID:  actorID,
				Action:   "EXECUTION_OUTCOME_UNKNOWN",
				Details: map[string]any{
					"previous_status": ActionStatusExecuting,
					"new_status":      ActionStatusReconciliationRequired,
					"reason":          "execution exceeded recovery timeout",
					"timeout":         olderThan.String(),
				},
			},
		)
		if err != nil {
			return nil, err
		}

		recovered = append(recovered, action)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf(
			"failed to commit stale execution recovery: %w",
			err,
		)
	}

	return recovered, nil
}

// ResolveReconciliation resolves an action whose execution outcome is uncertain.
//
// RECONCILIATION_REQUIRED -> EXECUTED
// RECONCILIATION_REQUIRED -> APPROVED (approval-required operation)
// RECONCILIATION_REQUIRED -> PROPOSED (no-approval operation)
// RECONCILIATION_REQUIRED -> RECONCILIATION_REQUIRED (still unknown)
//
// The state transition and audit record are atomic.
func (r *ActionRepository) ResolveReconciliation(
	ctx context.Context,
	actionID int64,
	actorID string,
	decision model.ReconciliationDecision,
	reason string,
) (*model.IncidentAction, error) {

	if actionID <= 0 {
		return nil, fmt.Errorf("invalid action ID")
	}

	if actorID == "" {
		return nil, fmt.Errorf("actor ID cannot be empty")
	}

	if reason == "" {
		return nil, fmt.Errorf("reconciliation reason cannot be empty")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to begin reconciliation transaction: %w",
			err,
		)
	}
	defer tx.Rollback(ctx)

	// Lock the action so two operators cannot resolve it concurrently.
	existing, err := scanIncidentAction(
		tx.QueryRow(
			ctx,
			`
            SELECT `+actionColumns+`
            FROM incident_actions
            WHERE id = $1
            FOR UPDATE
            `,
			actionID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}

	if err != nil {
		return nil, fmt.Errorf(
			"failed to retrieve action for reconciliation: %w",
			err,
		)
	}

	if existing.Status != model.ActionStatusReconciliationRequired {
		return nil, ErrActionNotReconciliationRequired
	}

	previousStatus := existing.Status
	newStatus := previousStatus

	switch decision {
	case model.ReconciliationConfirmedSuccess:
		newStatus = model.ActionStatusExecuted

	case model.ReconciliationNotPerformed:
		// Preserve the original approval policy.
		if existing.RequiresApproval {
			newStatus = model.ActionStatusApproved
		} else {
			newStatus = model.ActionStatusProposed
		}

	case model.ReconciliationUnknown:
		// Keep the action in reconciliation.
		newStatus = model.ActionStatusReconciliationRequired

	default:
		return nil, fmt.Errorf(
			"unsupported reconciliation decision: %s",
			decision,
		)
	}

	resolved, err := scanIncidentAction(
		tx.QueryRow(
			ctx,
			`
            UPDATE incident_actions
            SET
                status = $1,
                executed_at = CASE
                    WHEN $1 = $2 THEN NOW()
                    ELSE executed_at
                END,
                execution_error = CASE
                    WHEN $1 = $2 THEN NULL
                    ELSE execution_error
                END,
                updated_at = NOW()
            WHERE id = $3
              AND status = $4
            RETURNING `+actionColumns,
			newStatus,
			model.ActionStatusExecuted,
			actionID,
			model.ActionStatusReconciliationRequired,
		),
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrActionNotReconciliationRequired
		}

		return nil, fmt.Errorf(
			"failed to update reconciled action: %w",
			err,
		)
	}

	auditAction := "RECONCILIATION_RESOLVED"

	if decision == model.ReconciliationUnknown {
		auditAction = "RECONCILIATION_REVIEWED"
	}

	err = r.CreateAuditLogTx(
		ctx,
		tx,
		model.ActionAuditLog{
			ActionID: resolved.ID,
			ActorID:  actorID,
			Action:   auditAction,
			Details: map[string]any{
				"decision":        decision,
				"previous_status": previousStatus,
				"new_status":      newStatus,
				"reason":          reason,
				"operation_key":   resolved.OperationKey,
			},
		},
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf(
			"failed to commit reconciliation transaction: %w",
			err,
		)
	}

	return resolved, nil
}
