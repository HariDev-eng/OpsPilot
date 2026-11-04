package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
)

var (
	ErrActionPolicyMismatch = errors.New(
		"action configuration does not match registered operation policy",
	)

	ErrExecutorUnavailable = errors.New(
		"action executor is unavailable",
	)
)

// ActionExecutor defines the boundary between OpsPilot
// and external operational systems.
type ActionExecutor interface {
	Execute(
		ctx context.Context,
		action *model.IncidentAction,
	) (map[string]any, error)
}

type ExecutionService struct {
	actionRepository *repository.ActionRepository
	executor         ActionExecutor
}

func NewExecutionService(
	actionRepository *repository.ActionRepository,
	executor ActionExecutor,
) *ExecutionService {
	return &ExecutionService{
		actionRepository: actionRepository,
		executor:         executor,
	}
}

// Execute validates, claims and executes an approved operation.
//
// Investigation actions can execute from PROPOSED.
// Mitigation and destructive actions require APPROVED.
func (s *ExecutionService) Execute(
	ctx context.Context,
	actionID int64,
	actorID string,
) (*model.IncidentAction, error) {

	if actionID <= 0 {
		return nil, errors.New("invalid action ID")
	}

	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return nil, errors.New("actor ID cannot be empty")
	}

	if s.executor == nil {
		return nil, ErrExecutorUnavailable
	}

	// Step 1: Load the persisted action.
	action, err := s.actionRepository.GetByID(ctx, actionID)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to retrieve action: %w",
			err,
		)
	}

	if action.OperationKey == nil {
		return nil, ErrOperationNotAllowed
	}

	// Step 2: Revalidate against the server-owned registry.
	policy, err := LookupOperationPolicy(*action.OperationKey)
	if err != nil {
		return nil, err
	}

	if action.ActionType != policy.ActionType ||
		action.RequiresApproval != policy.RequiresApproval ||
		action.Description != policy.Description {
		return nil, ErrActionPolicyMismatch
	}

	if err := validateOperationParameters(
		policy.OperationKey,
		action.Parameters,
	); err != nil {
		return nil, err
	}

	// Step 3: Atomically claim execution.
	//
	// PostgreSQL determines whether this action is eligible.
	claimedAction, err := s.actionRepository.ClaimExecution(
		ctx,
		actionID,
		actorID,
		policy.OperationKey,
		policy.ActionType,
		policy.RequiresApproval,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to claim execution: %w",
			err,
		)
	}

	// Step 4: Execute the registered operation.
	result, executionErr := s.executor.Execute(
		ctx,
		claimedAction,
	)

	if executionErr != nil {
		markErr := s.actionRepository.MarkFailed(
			ctx,
			actionID,
			actorID,
			executionErr.Error(),
		)

		if markErr != nil {
			return nil, fmt.Errorf(
				"executor failed: %v; additionally failed to persist failure state: %w",
				executionErr,
				markErr,
			)
		}

		return nil, fmt.Errorf(
			"operation %s failed: %w",
			policy.OperationKey,
			executionErr,
		)
	}

	// Step 5: Persist successful completion.
	if err := s.actionRepository.MarkExecuted(
		ctx,
		actionID,
		actorID,
		result,
	); err != nil {
		// The external operation may already have succeeded.
		// Do not execute it again automatically.
		return nil, fmt.Errorf(
			"operation may have completed, but EXECUTED state could not be persisted; action %d may remain EXECUTING: %w",
			actionID,
			err,
		)
	}

	// Return the final persisted state.
	return s.actionRepository.GetByID(ctx, actionID)
}
