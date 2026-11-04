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

// FailingExecutor deliberately simulates an infrastructure failure.
type FailingExecutor struct{}

func (f *FailingExecutor) Execute(
	ctx context.Context,
	action *model.IncidentAction,
) (map[string]any, error) {

	log.Printf(
		"MOCK EXECUTOR: executing %s",
		*action.OperationKey,
	)

	return nil, errors.New(
		"simulated payment service unavailable",
	)
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
		incidentID int64 = 11
		actorID          = "dev-operator"
	)

	// Generate a unique recommendation so this test creates
	// a fresh action rather than reusing an existing one.
	description := "Failure simulation " +
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

	// 1. Create a fresh AI recommendation.
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

	log.Printf(
		"Created action ID=%d Status=%s",
		actionID,
		actions[0].Status,
	)

	// 2. Assign the registered operation.
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

	log.Printf(
		"Operation assigned: %s",
		*assigned.OperationKey,
	)

	// 3. Approve the action.
	err = actionService.Approve(
		ctx,
		actionID,
		actorID,
	)
	if err != nil {
		log.Fatalf("approval failed: %v", err)
	}

	log.Println("Action approved")

	// 4. Execute using the deliberately failing executor.
	failingExecutor := &FailingExecutor{}

	executionService := service.NewExecutionService(
		actionRepository,
		failingExecutor,
	)

	log.Println("Starting failure simulation...")

	executionErr, err := executionService.Execute(
		ctx,
		actionID,
		actorID,
	)

	if executionErr == nil {
		log.Fatal("expected execution failure, but execution succeeded")
	}

	log.Printf(
		"EXPECTED EXECUTION ERROR: %v",
		executionErr,
	)

	// 5. Verify the persisted failure state.
	persisted, err := actionRepository.GetByID(
		ctx,
		actionID,
	)
	if err != nil {
		log.Fatalf("failed to retrieve action: %v", err)
	}

	if persisted.Status != model.ActionStatusFailed {
		log.Fatalf(
			"expected FAILED status, got %s",
			persisted.Status,
		)
	}

	if persisted.ExecutionAttempts != 1 {
		log.Fatalf(
			"expected 1 execution attempt, got %d",
			persisted.ExecutionAttempts,
		)
	}

	if persisted.ExecutionError == nil ||
		*persisted.ExecutionError != "simulated payment service unavailable" {
		log.Fatalf(
			"unexpected execution error: %v",
			persisted.ExecutionError,
		)
	}

	log.Println("FAILURE HANDLING VERIFIED!")

	log.Printf(
		"Action ID=%d Status=%s Attempts=%d Error=%s",
		persisted.ID,
		persisted.Status,
		persisted.ExecutionAttempts,
		*persisted.ExecutionError,
	)
}
