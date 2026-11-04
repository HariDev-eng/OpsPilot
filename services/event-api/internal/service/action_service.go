package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/ai"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidActionType          = errors.New("unsupported action type")
	ErrInvalidApproval            = errors.New("invalid approval requirement for action type")
	ErrEmptyDescription           = errors.New("action description cannot be empty")
	ErrInvalidOperationParameters = errors.New(
		"operation does not accept the supplied parameters",
	)
	ErrInvalidReconciliationDecision = errors.New(
		"unsupported reconciliation decision",
	)
)

const (
	actionActorAI = "opspilot-ai"
)

type ActionService struct {
	pool             *pgxpool.Pool
	actionRepository *repository.ActionRepository
}

func NewActionService(
	pool *pgxpool.Pool,
	actionRepository *repository.ActionRepository,
) *ActionService {
	return &ActionService{
		pool:             pool,
		actionRepository: actionRepository,
	}
}

// CreateFromAnalysis converts LLM recommendations into persisted actions.
//
// All actions and their audit records are created in one transaction.
// If any operation fails, the entire transaction is rolled back.
func (s *ActionService) CreateFromAnalysis(
	ctx context.Context,
	incidentID int64,
	analysis *ai.IncidentAnalysis,
) ([]model.IncidentAction, error) {

	if analysis == nil {
		return nil, errors.New("incident analysis cannot be nil")
	}

	if incidentID <= 0 {
		return nil, errors.New("invalid incident ID")
	}

	if err := analysis.ValidateActions(); err != nil {
		return nil, fmt.Errorf(
			"invalid AI recommendations: %w",
			err,
		)
	}

	if len(analysis.RecommendedActions) == 0 {
		return []model.IncidentAction{}, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to begin action creation transaction: %w",
			err,
		)
	}

	defer tx.Rollback(ctx)

	var createdActions []model.IncidentAction

	for index, recommendation := range analysis.RecommendedActions {

		description := strings.TrimSpace(
			recommendation.Description,
		)

		if description == "" {
			return nil, ErrEmptyDescription
		}

		status, requiresApproval, err := determineActionPolicy(
			recommendation.ActionType,
		)

		if err != nil {
			return nil, err
		}

		// Reject inconsistent LLM approval classifications.
		if recommendation.RequiresApproval != requiresApproval {
			return nil, ErrInvalidApproval
		}

		idempotencyKey := generateActionIdempotencyKey(
			incidentID,
			index,
			recommendation.ActionType,
			description,
		)

		action := &model.IncidentAction{
			IncidentID:       incidentID,
			Description:      description,
			ActionType:       recommendation.ActionType,
			Status:           model.ActionStatus(status),
			RequiresApproval: requiresApproval,
			IdempotencyKey:   idempotencyKey,
		}

		persistedAction, created, err := s.actionRepository.CreateTx(
			ctx,
			tx,
			action,
		)

		if err != nil {
			return nil, fmt.Errorf(
				"failed to persist recommended action: %w",
				err,
			)
		}

		// Do not create another audit record when this is an
		// idempotent retry of an existing recommendation.
		if created {
			err = s.actionRepository.CreateAuditLogTx(
				ctx,
				tx,
				model.ActionAuditLog{
					ActionID: persistedAction.ID,
					ActorID:  actionActorAI,
					Action:   "RECOMMENDED",
					Details: map[string]any{
						"action_type":       persistedAction.ActionType,
						"initial_status":    persistedAction.Status,
						"requires_approval": persistedAction.RequiresApproval,
					},
				},
			)

			if err != nil {
				return nil, err
			}
		}

		createdActions = append(
			createdActions,
			*persistedAction,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf(
			"failed to commit recommended actions: %w",
			err,
		)
	}

	return createdActions, nil
}

// determineActionPolicy applies backend-owned approval rules.
func determineActionPolicy(
	actionType string,
) (string, bool, error) {

	switch actionType {
	case "INVESTIGATION":
		return repository.ActionStatusProposed, false, nil

	case "MITIGATION", "DESTRUCTIVE":
		return repository.ActionStatusPendingApproval, true, nil

	default:
		return "", false, ErrInvalidActionType
	}
}

// generateActionIdempotencyKey creates a deterministic key.
//
// The same incident, recommendation position, type and description
// produce the same key during retries.
func generateActionIdempotencyKey(
	incidentID int64,
	index int,
	actionType string,
	description string,
) string {

	input := fmt.Sprintf(
		"incident:%d:index:%d:type:%s:description:%s",
		incidentID,
		index,
		actionType,
		description,
	)

	hash := sha256.Sum256([]byte(input))

	return hex.EncodeToString(hash[:])
}

// Approve approves a pending action.
//
// Approval does not execute the action.
func (s *ActionService) Approve(
	ctx context.Context,
	actionID int64,
	actorID string,
) error {
	if actionID <= 0 {
		return errors.New("invalid action ID")
	}

	if strings.TrimSpace(actorID) == "" {
		return errors.New("actor ID cannot be empty")
	}

	return s.actionRepository.Approve(
		ctx,
		actionID,
		actorID,
	)
}

// Reject rejects a pending action.
func (s *ActionService) Reject(
	ctx context.Context,
	actionID int64,
	actorID string,
	reason string,
) error {
	if actionID <= 0 {
		return errors.New("invalid action ID")
	}

	if strings.TrimSpace(actorID) == "" {
		return errors.New("actor ID cannot be empty")
	}

	if strings.TrimSpace(reason) == "" {
		return errors.New("rejection reason cannot be empty")
	}

	return s.actionRepository.Reject(
		ctx,
		actionID,
		actorID,
		strings.TrimSpace(reason),
	)
}

// AssignOperation binds a server-registered operation to an action.
//
// The operation registry controls the action type, approval requirement,
// description and initial status. Client/LLM values cannot override them.
func (s *ActionService) AssignOperation(
	ctx context.Context,
	actionID int64,
	actorID string,
	operationKey string,
	parameters map[string]any,
) (*model.IncidentAction, error) {

	if actionID <= 0 {
		return nil, errors.New("invalid action ID")
	}

	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return nil, errors.New("actor ID cannot be empty")
	}

	operationKey = strings.TrimSpace(operationKey)
	if operationKey == "" {
		return nil, ErrOperationNotAllowed
	}

	policy, err := LookupOperationPolicy(operationKey)
	if err != nil {
		return nil, err
	}

	// Neither registered operation currently accepts parameters.
	// Add explicit schemas when we implement real executors.
	if err := validateOperationParameters(operationKey, parameters); err != nil {
		return nil, err
	}

	status, requiresApproval, err := determineActionPolicy(
		policy.ActionType,
	)
	if err != nil {
		return nil, err
	}

	// Protect against inconsistent server-side registry configuration.
	if requiresApproval != policy.RequiresApproval {
		return nil, ErrInvalidApproval
	}

	if parameters == nil {
		parameters = make(map[string]any)
	}

	return s.actionRepository.AssignOperation(
		ctx,
		actionID,
		actorID,
		policy.OperationKey,
		policy.Description,
		policy.ActionType,
		policy.RequiresApproval,
		model.ActionStatus(status),
		parameters,
	)
}

// validateOperationParameters rejects unrecognized configuration.
//
// Both registered operations currently have no configurable parameters.
func validateOperationParameters(
	operationKey string,
	parameters map[string]any,
) error {

	switch operationKey {
	case OperationInspectPaymentLogs, OperationRestartPayment:
		if len(parameters) != 0 {
			return ErrInvalidOperationParameters
		}
		return nil

	default:
		return ErrOperationNotAllowed
	}
}

// ResolveReconciliation validates and processes an operator's
// reconciliation decision.
func (s *ActionService) ResolveReconciliation(
	ctx context.Context,
	actionID int64,
	actorID string,
	decision model.ReconciliationDecision,
	reason string,
) (*model.IncidentAction, error) {

	if actionID <= 0 {
		return nil, errors.New("invalid action ID")
	}

	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return nil, errors.New("actor ID cannot be empty")
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, errors.New("reconciliation reason cannot be empty")
	}

	if len([]rune(reason)) > 2000 {
		return nil, errors.New(
			"reconciliation reason cannot exceed 2000 characters",
		)
	}

	switch decision {
	case model.ReconciliationConfirmedSuccess,
		model.ReconciliationNotPerformed,
		model.ReconciliationUnknown:
		// Valid decision.

	default:
		return nil, ErrInvalidReconciliationDecision
	}

	return s.actionRepository.ResolveReconciliation(
		ctx,
		actionID,
		actorID,
		decision,
		reason,
	)
}
