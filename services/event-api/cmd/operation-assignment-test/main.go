package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/database"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
)

func main() {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		20*time.Second,
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
		testActionID int64 = 1
		approvedID   int64 = 2
		actorID            = "dev-operator"
	)

	// 1. Assign the registered payment.restart operation.
	log.Println("TEST 1: Assign payment.restart")

	action, err := actionService.AssignOperation(
		ctx,
		testActionID,
		actorID,
		service.OperationRestartPayment,
		map[string]any{},
	)
	if err != nil {
		log.Fatalf("operation assignment failed: %v", err)
	}

	log.Printf(
		"SUCCESS: ID=%d Operation=%s Type=%s Status=%s Approval=%t",
		action.ID,
		*action.OperationKey,
		action.ActionType,
		action.Status,
		action.RequiresApproval,
	)

	// 2. Repeat the same assignment.
	// It should return the existing action without another audit entry.
	log.Println("TEST 2: Repeat identical assignment")

	repeated, err := actionService.AssignOperation(
		ctx,
		testActionID,
		actorID,
		service.OperationRestartPayment,
		map[string]any{},
	)
	if err != nil {
		log.Fatalf("idempotent assignment failed: %v", err)
	}

	if repeated.ID != action.ID {
		log.Fatalf(
			"idempotency failure: expected ID %d, got %d",
			action.ID,
			repeated.ID,
		)
	}

	log.Printf("SUCCESS: Same action returned, ID=%d", repeated.ID)

	// 3. Attempt to assign a different operation to the same action.
	log.Println("TEST 3: Attempt conflicting assignment")

	_, err = actionService.AssignOperation(
		ctx,
		testActionID,
		actorID,
		service.OperationInspectPaymentLogs,
		map[string]any{},
	)

	if !errors.Is(err, repository.ErrOperationAlreadyAssigned) {
		log.Fatalf(
			"expected ErrOperationAlreadyAssigned, got %v",
			err,
		)
	}

	log.Println("SUCCESS: Conflicting assignment rejected")

	// 4. Attempt to assign an operation to an already-approved action.
	log.Println("TEST 4: Assign operation to approved action")

	_, err = actionService.AssignOperation(
		ctx,
		approvedID,
		actorID,
		service.OperationRestartPayment,
		map[string]any{},
	)

	if !errors.Is(err, repository.ErrActionNotAssignable) {
		log.Fatalf(
			"expected ErrActionNotAssignable, got %v",
			err,
		)
	}

	log.Println("SUCCESS: Approved action cannot be assigned")

	// 5. Read the persisted action from PostgreSQL.
	log.Println("TEST 5: Verify persisted action")

	persisted, err := actionRepository.GetByID(
		ctx,
		testActionID,
	)
	if err != nil {
		log.Fatalf("failed to retrieve action: %v", err)
	}

	log.Printf(
		"FINAL: ID=%d Operation=%s Type=%s Status=%s Approval=%t Parameters=%v Attempts=%d",
		persisted.ID,
		*persisted.OperationKey,
		persisted.ActionType,
		persisted.Status,
		persisted.RequiresApproval,
		persisted.Parameters,
		persisted.ExecutionAttempts,
	)

	log.Println("All operation assignment tests passed!")
}
