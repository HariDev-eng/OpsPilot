package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/ai"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/database"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/handler"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	incidentID    int64 = 11
	actorID             = "dev-operator"
	recoveryActor       = "opspilot-recovery-worker"
)

type reconciliationScenario struct {
	name             string
	actionType       string
	requiresApproval bool
	operationKey     string
	decision         model.ReconciliationDecision
	expectedStatus   model.ActionStatus
	expectedAudit    string
}

type SuccessfulTestExecutor struct {
	Calls int
}

func (e *SuccessfulTestExecutor) Execute(
	ctx context.Context,
	action *model.IncidentAction,
) (map[string]any, error) {
	e.Calls++

	return map[string]any{
		"result": "simulated successful execution",
	}, nil
}

func main() {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		90*time.Second,
	)
	defer cancel()

	pool, err := database.NewPool()
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()

	actionRepository := repository.NewActionRepository(pool)

	actionService := service.NewActionService(
		pool,
		actionRepository,
	)

	scenarios := []reconciliationScenario{
		{
			name:             "confirmed success",
			actionType:       "MITIGATION",
			requiresApproval: true,
			operationKey:     service.OperationRestartPayment,
			decision:         model.ReconciliationConfirmedSuccess,
			expectedStatus:   model.ActionStatusExecuted,
			expectedAudit:    "RECONCILIATION_RESOLVED",
		},
		{
			name:             "mitigation not performed",
			actionType:       "MITIGATION",
			requiresApproval: true,
			operationKey:     service.OperationRestartPayment,
			decision:         model.ReconciliationNotPerformed,
			expectedStatus:   model.ActionStatusApproved,
			expectedAudit:    "RECONCILIATION_RESOLVED",
		},
		{
			name:             "investigation not performed",
			actionType:       "INVESTIGATION",
			requiresApproval: false,
			operationKey:     "payment.inspect_logs",
			decision:         model.ReconciliationNotPerformed,
			expectedStatus:   model.ActionStatusProposed,
			expectedAudit:    "RECONCILIATION_RESOLVED",
		},
		{
			name:             "unknown outcome",
			actionType:       "MITIGATION",
			requiresApproval: true,
			operationKey:     service.OperationRestartPayment,
			decision:         model.ReconciliationUnknown,
			expectedStatus:   model.ActionStatusReconciliationRequired,
			expectedAudit:    "RECONCILIATION_REVIEWED",
		},
	}

	log.Println("Starting reconciliation integration tests")

	var unknownActionID int64
	var resolvedActionID int64
	var retryActionID int64

	for _, scenario := range scenarios {
		log.Printf("Testing: %s", scenario.name)

		actionID, err := createReconciliationRequiredAction(
			ctx,
			pool,
			actionRepository,
			actionService,
			scenario,
		)
		if err != nil {
			log.Fatalf(
				"failed to prepare scenario %q: %v",
				scenario.name,
				err,
			)
		}

		log.Printf("Action %d is awaiting reconciliation", actionID)

		resolved, err := actionService.ResolveReconciliation(
			ctx,
			actionID,
			actorID,
			scenario.decision,
			"Integration test: operator verified the simulated external outcome.",
		)
		if err != nil {
			log.Fatalf(
				"reconciliation failed for %q: %v",
				scenario.name,
				err,
			)
		}

		if resolved.Status != scenario.expectedStatus {
			log.Fatalf(
				"%s: expected status %s, got %s",
				scenario.name,
				scenario.expectedStatus,
				resolved.Status,
			)
		}

		// Verify the persisted database state.
		persisted, err := actionRepository.GetByID(ctx, actionID)
		if err != nil {
			log.Fatalf("failed to retrieve action %d: %v", actionID, err)
		}

		if persisted.Status != scenario.expectedStatus {
			log.Fatalf(
				"%s: persisted status mismatch: expected %s, got %s",
				scenario.name,
				scenario.expectedStatus,
				persisted.Status,
			)
		}

		if scenario.decision == model.ReconciliationConfirmedSuccess {
			if persisted.ExecutedAt == nil {
				log.Fatalf("confirmed success must set executed_at")
			}
		}

		if scenario.decision == model.ReconciliationNotPerformed ||
			scenario.decision == model.ReconciliationUnknown {
			if persisted.ExecutedAt != nil {
				log.Fatalf(
					"%s: executed_at should remain NULL",
					scenario.name,
				)
			}
		}

		// Verify the reconciliation audit record.
		var auditCount int

		err = pool.QueryRow(
			ctx,
			`
			SELECT COUNT(*)
			FROM action_audit_logs
			WHERE action_id = $1
			  AND actor_id = $2
			  AND action = $3
			  AND details->>'decision' = $4
			`,
			actionID,
			actorID,
			scenario.expectedAudit,
			string(scenario.decision),
		).Scan(&auditCount)

		if err != nil {
			log.Fatalf("audit query failed: %v", err)
		}

		if auditCount != 1 {
			log.Fatalf(
				"%s: expected one reconciliation audit, got %d",
				scenario.name,
				auditCount,
			)
		}

		log.Printf(
			"PASSED: %s | Action=%d | Status=%s | Audit=%s",
			scenario.name,
			actionID,
			persisted.Status,
			scenario.expectedAudit,
		)

		if scenario.decision == model.ReconciliationUnknown {
			unknownActionID = actionID
		} else if scenario.decision == model.ReconciliationConfirmedSuccess {
			resolvedActionID = actionID
		}

		if scenario.name == "mitigation not performed" {
			retryActionID = actionID
		}
	}

	// Test: Re-execution after confirmed non-execution.
	log.Println("Testing execution re-entry")

	retryAction, err := actionRepository.GetByID(ctx, retryActionID)
	if err != nil {
		log.Fatalf("failed to retrieve retry action: %v", err)
	}

	if retryAction.Status != model.ActionStatusApproved {
		log.Fatalf(
			"expected APPROVED before retry, got %s",
			retryAction.Status,
		)
	}

	if retryAction.ExecutionAttempts != 1 {
		log.Fatalf(
			"expected one previous attempt, got %d",
			retryAction.ExecutionAttempts,
		)
	}

	// Use a fake executor to avoid performing real operations.
	testExecutor := &SuccessfulTestExecutor{}

	executionService := service.NewExecutionService(
		actionRepository,
		testExecutor,
	)

	// Execute the approved action again.
	_, err = executionService.Execute(
		ctx,
		retryActionID,
		actorID,
	)
	if err != nil {
		log.Fatalf("second execution failed: %v", err)
	}

	if testExecutor.Calls != 1 {
		log.Fatalf(
			"expected exactly one executor call, got %d",
			testExecutor.Calls,
		)
	}

	// Verify the final database state.
	retriedAction, err := actionRepository.GetByID(ctx, retryActionID)
	if err != nil {
		log.Fatalf("failed to retrieve retried action: %v", err)
	}

	if retriedAction.Status != model.ActionStatusExecuted {
		log.Fatalf(
			"expected EXECUTED after retry, got %s",
			retriedAction.Status,
		)
	}

	if retriedAction.ExecutionAttempts != 2 {
		log.Fatalf(
			"expected two total execution attempts, got %d",
			retriedAction.ExecutionAttempts,
		)
	}

	// Verify two execution-start audit records.
	var executionAuditCount int

	err = pool.QueryRow(
		ctx,
		`
    SELECT COUNT(*)
    FROM action_audit_logs
    WHERE action_id = $1
      AND action = 'EXECUTION_STARTED'
    `,
		retryActionID,
	).Scan(&executionAuditCount)

	if err != nil {
		log.Fatalf("failed to verify execution audit: %v", err)
	}

	if executionAuditCount != 2 {
		log.Fatalf(
			"expected two execution-start audit records, got %d",
			executionAuditCount,
		)
	}

	log.Println("PASSED: execution re-entry works correctly")
	log.Printf(
		"Action=%d Status=%s Attempts=%d ExecutorCalls=%d",
		retryActionID,
		retriedAction.Status,
		retriedAction.ExecutionAttempts,
		testExecutor.Calls,
	)

	// Test 5: Invalid reconciliation decision.
	_, err = actionService.ResolveReconciliation(
		ctx,
		unknownActionID,
		actorID,
		model.ReconciliationDecision("INVALID_DECISION"),
		"Testing invalid decision",
	)

	if !errors.Is(err, service.ErrInvalidReconciliationDecision) {
		log.Fatalf(
			"expected ErrInvalidReconciliationDecision, got %v",
			err,
		)
	}

	log.Println("PASSED: invalid reconciliation decision rejected")

	// Test 6: An already-resolved action cannot be resolved again.
	_, err = actionService.ResolveReconciliation(
		ctx,
		resolvedActionID,
		actorID,
		model.ReconciliationConfirmedSuccess,
		"Testing duplicate reconciliation",
	)

	if !errors.Is(err, repository.ErrActionNotReconciliationRequired) {
		log.Fatalf(
			"expected ErrActionNotReconciliationRequired, got %v",
			err,
		)
	}

	// Test 7: Concurrent reconciliation.
	log.Println("Testing concurrent reconciliation")

	concurrentScenario := reconciliationScenario{
		name:             "concurrent reconciliation",
		actionType:       "MITIGATION",
		requiresApproval: true,
		operationKey:     service.OperationRestartPayment,
	}

	concurrentActionID, err := createReconciliationRequiredAction(
		ctx,
		pool,
		actionRepository,
		actionService,
		concurrentScenario,
	)
	if err != nil {
		log.Fatalf("failed to prepare concurrent action: %v", err)
	}

	type reconciliationResult struct {
		decision model.ReconciliationDecision
		err      error
	}

	decisions := []model.ReconciliationDecision{
		model.ReconciliationConfirmedSuccess,
		model.ReconciliationNotPerformed,
	}

	results := make(chan reconciliationResult, len(decisions))

	var wg sync.WaitGroup

	for _, decision := range decisions {
		wg.Add(1)

		go func(decision model.ReconciliationDecision) {
			defer wg.Done()

			_, err := actionService.ResolveReconciliation(
				ctx,
				concurrentActionID,
				actorID,
				decision,
				"Concurrent reconciliation integration test",
			)

			results <- reconciliationResult{
				decision: decision,
				err:      err,
			}
		}(decision)
	}

	wg.Wait()
	close(results)

	successCount := 0
	conflictCount := 0

	for result := range results {
		if result.err == nil {
			successCount++
			log.Printf("Concurrent decision succeeded: %s", result.decision)
			continue
		}

		if errors.Is(
			result.err,
			repository.ErrActionNotReconciliationRequired,
		) {
			conflictCount++
			log.Printf("Concurrent decision rejected: %s", result.decision)
			continue
		}

		log.Fatalf(
			"unexpected concurrent reconciliation error: %v",
			result.err,
		)
	}

	if successCount != 1 || conflictCount != 1 {
		log.Fatalf(
			"expected one success and one conflict; success=%d conflict=%d",
			successCount,
			conflictCount,
		)
	}

	// Verify final database state.
	concurrentAction, err := actionRepository.GetByID(
		ctx,
		concurrentActionID,
	)
	if err != nil {
		log.Fatalf("failed to retrieve concurrent action: %v", err)
	}

	if concurrentAction.Status != model.ActionStatusExecuted &&
		concurrentAction.Status != model.ActionStatusApproved {
		log.Fatalf(
			"unexpected concurrent final status: %s",
			concurrentAction.Status,
		)
	}

	// Exactly one reconciliation audit should exist.
	var concurrentAuditCount int

	err = pool.QueryRow(
		ctx,
		`
    SELECT COUNT(*)
    FROM action_audit_logs
    WHERE action_id = $1
      AND action IN (
          'RECONCILIATION_RESOLVED',
          'RECONCILIATION_REVIEWED'
      )
    `,
		concurrentActionID,
	).Scan(&concurrentAuditCount)

	if err != nil {
		log.Fatalf("failed to verify concurrent audit: %v", err)
	}

	if concurrentAuditCount != 1 {
		log.Fatalf(
			"expected exactly one reconciliation audit, got %d",
			concurrentAuditCount,
		)
	}

	log.Println("PASSED: concurrent reconciliation is serialized")

	log.Println("PASSED: repeated resolution of a completed action rejected")

	// Test 8: HTTP reconciliation endpoint.
	log.Println("Testing reconciliation HTTP endpoint")

	err = runHTTPTests(
		actionService,
		unknownActionID,
		resolvedActionID,
	)
	if err != nil {
		log.Fatalf("HTTP integration tests failed: %v", err)
	}

	log.Println("PASSED: all reconciliation HTTP tests")

	log.Println("----------------------------------------")
	log.Println("ALL RECONCILIATION TESTS PASSED")
	log.Println("----------------------------------------")
}

// createReconciliationRequiredAction prepares an action by:
//
// 1. Creating the recommendation.
// 2. Assigning an allowed operation.
// 3. Approving it when required.
// 4. Claiming execution.
// 5. Simulating a crashed worker.
// 6. Running stale execution recovery.
func createReconciliationRequiredAction(
	ctx context.Context,
	pool *pgxpool.Pool,
	actionRepository *repository.ActionRepository,
	actionService *service.ActionService,
	scenario reconciliationScenario,
) (int64, error) {
	description := fmt.Sprintf(
		"Reconciliation test: %s: %s",
		scenario.name,
		time.Now().UTC().Format(time.RFC3339Nano),
	)

	analysis := &ai.IncidentAnalysis{
		RecommendedActions: []ai.RecommendedAction{
			{
				Description:      description,
				ActionType:       scenario.actionType,
				RequiresApproval: scenario.requiresApproval,
			},
		},
	}

	actions, err := actionService.CreateFromAnalysis(
		ctx,
		incidentID,
		analysis,
	)
	if err != nil {
		return 0, fmt.Errorf("create action: %w", err)
	}

	if len(actions) != 1 {
		return 0, fmt.Errorf("expected one action, got %d", len(actions))
	}

	actionID := actions[0].ID

	assigned, err := actionService.AssignOperation(
		ctx,
		actionID,
		actorID,
		scenario.operationKey,
		map[string]any{},
	)
	if err != nil {
		return 0, fmt.Errorf("assign operation: %w", err)
	}

	if assigned.OperationKey == nil ||
		*assigned.OperationKey != scenario.operationKey {
		return 0, fmt.Errorf("operation assignment verification failed")
	}

	if scenario.requiresApproval {
		if err := actionService.Approve(
			ctx,
			actionID,
			actorID,
		); err != nil {
			return 0, fmt.Errorf("approve action: %w", err)
		}
	}

	claimed, err := actionRepository.ClaimExecution(
		ctx,
		actionID,
		actorID,
		scenario.operationKey,
		scenario.actionType,
		scenario.requiresApproval,
	)
	if err != nil {
		return 0, fmt.Errorf("claim execution: %w", err)
	}

	if claimed.Status != model.ActionStatusExecuting {
		return 0, fmt.Errorf(
			"expected EXECUTING, got %s",
			claimed.Status,
		)
	}

	// Simulate a worker crash.
	tag, err := pool.Exec(
		ctx,
		`
		UPDATE incident_actions
		SET execution_started_at = NOW() - INTERVAL '10 minutes'
		WHERE id = $1
		  AND status = 'EXECUTING'
		`,
		actionID,
	)
	if err != nil {
		return 0, fmt.Errorf("backdate execution: %w", err)
	}

	if tag.RowsAffected() != 1 {
		return 0, fmt.Errorf(
			"expected to backdate one action, affected %d",
			tag.RowsAffected(),
		)
	}

	// Recover stale execution.
	recovered, err := actionRepository.MarkStaleExecutionsForReconciliation(
		ctx,
		5*time.Minute,
		recoveryActor,
	)
	if err != nil {
		return 0, fmt.Errorf("recover stale execution: %w", err)
	}

	found := false

	for _, action := range recovered {
		if action.ID == actionID &&
			action.Status == model.ActionStatusReconciliationRequired {
			found = true
			break
		}
	}

	if !found {
		return 0, fmt.Errorf(
			"action %d was not found in recovery results (%d recovered)",
			actionID,
			len(recovered),
		)
	}

	// Verify the database state before reconciliation.
	persisted, err := actionRepository.GetByID(ctx, actionID)
	if err != nil {
		return 0, fmt.Errorf("verify recovered action: %w", err)
	}

	if persisted.Status != model.ActionStatusReconciliationRequired {
		return 0, fmt.Errorf(
			"expected RECONCILIATION_REQUIRED, got %s",
			persisted.Status,
		)
	}

	if persisted.ExecutionAttempts != 1 {
		return 0, fmt.Errorf(
			"expected one execution attempt, got %d",
			persisted.ExecutionAttempts,
		)
	}

	return actionID, nil
}

func runHTTPTests(
	actionService *service.ActionService,
	validActionID int64,
	resolvedActionID int64,
) error {
	mux := http.NewServeMux()

	actionHandler := handler.NewActionHandler(actionService)

	mux.HandleFunc(
		"POST /api/v1/actions/{id}/reconcile",
		actionHandler.Reconcile,
	)

	assertStatus := func(
		path string,
		body string,
		expectedStatus int,
	) error {
		request, err := http.NewRequest(
			http.MethodPost,
			path,
			strings.NewReader(body),
		)
		if err != nil {
			return err
		}

		request.Header.Set("Content-Type", "application/json")

		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)

		if recorder.Code != expectedStatus {
			return fmt.Errorf(
				"path=%s: expected HTTP %d, got %d; response=%s",
				path,
				expectedStatus,
				recorder.Code,
				recorder.Body.String(),
			)
		}

		return nil
	}

	basePath := "/api/v1/actions/"

	// 1. Valid reconciliation.
	err := assertStatus(
		basePath+fmt.Sprint(validActionID)+"/reconcile",
		`{
            "decision": "CONFIRMED_NOT_PERFORMED",
            "reason": "Verified using external system logs."
        }`,
		http.StatusOK,
	)
	if err != nil {
		return fmt.Errorf("valid request: %w", err)
	}

	log.Println("PASSED: valid reconciliation HTTP request")

	// 2. Invalid decision.
	err = assertStatus(
		basePath+fmt.Sprint(resolvedActionID)+"/reconcile",
		`{
            "decision": "INVALID",
            "reason": "Testing invalid decision."
        }`,
		http.StatusBadRequest,
	)
	if err != nil {
		return fmt.Errorf("invalid decision: %w", err)
	}

	log.Println("PASSED: invalid decision returns HTTP 400")

	// 3. Invalid action ID.
	err = assertStatus(
		basePath+"abc/reconcile",
		`{
            "decision": "CONFIRMED_SUCCESS",
            "reason": "Testing invalid action ID."
        }`,
		http.StatusBadRequest,
	)
	if err != nil {
		return fmt.Errorf("invalid action ID: %w", err)
	}

	log.Println("PASSED: invalid action ID returns HTTP 400")

	// 4. Nonexistent action ID.
	err = assertStatus(
		basePath+"9223372036854775807/reconcile",
		`{
            "decision": "CONFIRMED_SUCCESS",
            "reason": "Testing nonexistent action."
        }`,
		http.StatusNotFound,
	)
	if err != nil {
		return fmt.Errorf("nonexistent action: %w", err)
	}

	log.Println("PASSED: nonexistent action returns HTTP 404")

	// 5. Already-resolved action.
	err = assertStatus(
		basePath+fmt.Sprint(resolvedActionID)+"/reconcile",
		`{
            "decision": "CONFIRMED_SUCCESS",
            "reason": "Testing already-resolved action."
        }`,
		http.StatusConflict,
	)
	if err != nil {
		return fmt.Errorf("already-resolved action: %w", err)
	}

	log.Println("PASSED: already-resolved action returns HTTP 409")

	// 6. Missing reason.
	err = assertStatus(
		basePath+fmt.Sprint(validActionID)+"/reconcile",
		`{
            "decision": "CONFIRMED_SUCCESS",
            "reason": "   "
        }`,
		http.StatusBadRequest,
	)
	if err != nil {
		return fmt.Errorf("missing reason: %w", err)
	}

	log.Println("PASSED: missing reason returns HTTP 400")

	// 7. Unexpected JSON field.
	err = assertStatus(
		basePath+fmt.Sprint(validActionID)+"/reconcile",
		`{
            "decision": "CONFIRMED_SUCCESS",
            "reason": "Testing unknown fields.",
            "unexpected": true
        }`,
		http.StatusBadRequest,
	)
	if err != nil {
		return fmt.Errorf("unexpected JSON field: %w", err)
	}

	log.Println("PASSED: unexpected JSON field returns HTTP 400")

	return nil
}
