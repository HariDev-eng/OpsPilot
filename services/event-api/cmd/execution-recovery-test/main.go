package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/ai"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/database"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
)

// NeverCalledExecutor helps verify that recovery prevents re-execution.
type NeverCalledExecutor struct {
	Calls int
}

func (e *NeverCalledExecutor) Execute(
	ctx context.Context,
	action *model.IncidentAction,
) (map[string]any, error) {
	e.Calls++
	return map[string]any{"result": "unexpected execution"}, nil
}

func main() {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
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

	const (
		incidentID    int64 = 11
		actorID             = "dev-operator"
		recoveryActor       = "opspilot-recovery-worker"
	)

	// 1. Create a fresh action.
	description := "Crash recovery test " +
		time.Now().UTC().Format(time.RFC3339Nano)

	analysis := &ai.IncidentAnalysis{
		RecommendedActions: []ai.RecommendedAction{
			{
				Description:      description,
				ActionType:       "MITIGATION",
				RequiresApproval: true,
			},
		},
	}

	actions, err := actionService.CreateFromAnalysis(
		ctx,
		incidentID,
		analysis,
	)
	if err != nil {
		log.Fatalf("failed to create action: %v", err)
	}

	if len(actions) != 1 {
		log.Fatalf("expected one action, got %d", len(actions))
	}

	actionID := actions[0].ID
	log.Printf("Created action ID=%d", actionID)

	// 2. Assign payment.restart.
	assigned, err := actionService.AssignOperation(
		ctx,
		actionID,
		actorID,
		service.OperationRestartPayment,
		map[string]any{},
	)
	if err != nil {
		log.Fatalf("operation assignment failed: %v", err)
	}

	log.Printf("Assigned operation: %s", *assigned.OperationKey)

	// 3. Approve the action.
	if err := actionService.Approve(ctx, actionID, actorID); err != nil {
		log.Fatalf("approval failed: %v", err)
	}

	log.Println("Action approved")

	// 4. Claim execution directly.
	// We intentionally do not call an executor or mark the action complete.
	claimed, err := actionRepository.ClaimExecution(
		ctx,
		actionID,
		actorID,
		service.OperationRestartPayment,
		"MITIGATION",
		true,
	)
	if err != nil {
		log.Fatalf("failed to claim execution: %v", err)
	}

	if claimed.Status != model.ActionStatusExecuting {
		log.Fatalf("expected EXECUTING, got %s", claimed.Status)
	}

	log.Println("Execution claimed successfully")

	// 5. Simulate a worker crash by making the execution timestamp stale.
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
		log.Fatalf("failed to simulate stale execution: %v", err)
	}

	if tag.RowsAffected() != 1 {
		log.Fatalf("expected to backdate one action, affected %d", tag.RowsAffected())
	}

	log.Println("Simulated worker crash: execution is now stale")

	// 6. Run recovery with a five-minute timeout.
	recovered, err := actionRepository.MarkStaleExecutionsForReconciliation(
		ctx,
		5*time.Minute,
		recoveryActor,
	)
	if err != nil {
		log.Fatalf("recovery failed: %v", err)
	}

	if len(recovered) != 1 {
		log.Fatalf("expected one recovered action, got %d", len(recovered))
	}

	if recovered[0].ID != actionID {
		log.Fatalf(
			"recovered wrong action: expected %d, got %d",
			actionID,
			recovered[0].ID,
		)
	}

	if recovered[0].Status != model.ActionStatusReconciliationRequired {
		log.Fatalf(
			"expected RECONCILIATION_REQUIRED, got %s",
			recovered[0].Status,
		)
	}

	log.Println("Stale execution recovered successfully")

	// 7. Verify the persisted action.
	persisted, err := actionRepository.GetByID(ctx, actionID)
	if err != nil {
		log.Fatalf("failed to retrieve action: %v", err)
	}

	if persisted.Status != model.ActionStatusReconciliationRequired {
		log.Fatalf("unexpected persisted status: %s", persisted.Status)
	}

	if persisted.ExecutionAttempts != 1 {
		log.Fatalf(
			"expected one execution attempt, got %d",
			persisted.ExecutionAttempts,
		)
	}

	// 8. Verify the audit record.
	var auditCount int

	err = pool.QueryRow(
		ctx,
		`
		SELECT COUNT(*)
		FROM action_audit_logs
		WHERE action_id = $1
		  AND actor_id = $2
		  AND action = 'EXECUTION_OUTCOME_UNKNOWN'
		`,
		actionID,
		recoveryActor,
	).Scan(&auditCount)

	if err != nil {
		log.Fatalf("failed to verify recovery audit: %v", err)
	}

	if auditCount != 1 {
		log.Fatalf("expected one recovery audit record, got %d", auditCount)
	}

	log.Println("Recovery audit record verified")

	// 9. Verify that running recovery again does not recover the same action.
	recoveredAgain, err := actionRepository.MarkStaleExecutionsForReconciliation(
		ctx,
		5*time.Minute,
		recoveryActor,
	)
	if err != nil {
		log.Fatalf("second recovery run failed: %v", err)
	}

	if len(recoveredAgain) != 0 {
		log.Fatalf(
			"expected zero additional recoveries, got %d",
			len(recoveredAgain),
		)
	}

	log.Println("Repeated recovery correctly skipped the action")

	// 10. Verify that execution cannot be claimed again.
	neverCalledExecutor := &NeverCalledExecutor{}

	executionService := service.NewExecutionService(
		actionRepository,
		neverCalledExecutor,
	)

	_, err = executionService.Execute(ctx, actionID, actorID)

	if !errors.Is(err, repository.ErrActionNotExecutable) {
		log.Fatalf("expected ErrActionNotExecutable, got %v", err)
	}

	if neverCalledExecutor.Calls != 0 {
		log.Fatalf(
			"executor must not run again; calls=%d",
			neverCalledExecutor.Calls,
		)
	}

	log.Println("Re-execution correctly rejected")
	log.Println("CRASH RECOVERY TEST PASSED")

	log.Printf(
		"Action ID=%d Status=%s Attempts=%d AuditRecords=%d",
		persisted.ID,
		persisted.Status,
		persisted.ExecutionAttempts,
		auditCount,
	)
}
