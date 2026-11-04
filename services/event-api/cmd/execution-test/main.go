package main

import (
	"context"
	"log"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/database"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
)

// MockExecutor simulates an operational action.
// It does not communicate with external infrastructure.
type MockExecutor struct{}

func (m *MockExecutor) Execute(
	ctx context.Context,
	action *model.IncidentAction,
) (map[string]any, error) {

	log.Printf(
		"MOCK EXECUTOR: received operation %s",
		*action.OperationKey,
	)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Simulate a successful operation.
	return map[string]any{
		"message":   "Payment service restart simulated successfully",
		"service":   "payment-service",
		"simulated": true,
	}, nil
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

	mockExecutor := &MockExecutor{}

	executionService := service.NewExecutionService(
		actionRepository,
		mockExecutor,
	)

	const (
		actionID int64 = 1
		actorID        = "dev-operator"
	)

	log.Println("Starting mock execution...")

	action, err := executionService.Execute(
		ctx,
		actionID,
		actorID,
	)
	if err != nil {
		log.Fatalf("execution failed: %v", err)
	}

	log.Println("Execution completed successfully!")

	log.Printf(
		"Action ID=%d Operation=%s Status=%s Attempts=%d",
		action.ID,
		*action.OperationKey,
		action.Status,
		action.ExecutionAttempts,
	)
}
