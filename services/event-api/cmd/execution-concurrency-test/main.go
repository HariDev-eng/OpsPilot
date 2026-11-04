package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/ai"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/database"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
)

type BlockingMockExecutor struct {
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (m *BlockingMockExecutor) Execute(
	ctx context.Context,
	action *model.IncidentAction,
) (map[string]any, error) {

	m.calls.Add(1)

	log.Printf(
		"MOCK EXECUTOR ENTERED: action=%d operation=%s",
		action.ID,
		*action.OperationKey,
	)

	// Notify the test that a worker successfully claimed execution.
	m.started <- struct{}{}

	// Keep the winning worker inside the executor until released.
	select {
	case <-m.release:
		return map[string]any{
			"message":   "Concurrent execution simulation successful",
			"simulated": true,
		}, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type ExecutionResult struct {
	worker string
	err    error
}

func main() {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		45*time.Second,
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

	actorID := "dev-operator"

	// 1. Create a fresh action.
	description := fmt.Sprintf(
		"Concurrency test %d",
		time.Now().UnixNano(),
	)

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
		11,
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
		"Created action ID=%d",
		actionID,
	)

	// 2. Assign the registered operation.
	_, err = actionService.AssignOperation(
		ctx,
		actionID,
		actorID,
		service.OperationRestartPayment,
		map[string]any{},
	)
	if err != nil {
		log.Fatalf("operation assignment failed: %v", err)
	}

	// 3. Approve the action.
	if err := actionService.Approve(
		ctx,
		actionID,
		actorID,
	); err != nil {
		log.Fatalf("approval failed: %v", err)
	}

	log.Println("Action approved. Starting concurrent workers...")

	// 4. Create a blocking mock executor.
	executor := &BlockingMockExecutor{
		started: make(chan struct{}, 2),
		release: make(chan struct{}),
	}

	executionService := service.NewExecutionService(
		actionRepository,
		executor,
	)

	results := make(chan ExecutionResult, 2)

	start := make(chan struct{})

	var wg sync.WaitGroup

	// 5. Launch two workers.
	for _, worker := range []string{"worker-A", "worker-B"} {
		wg.Add(1)

		go func(worker string) {
			defer wg.Done()

			<-start

			_, err := executionService.Execute(
				ctx,
				actionID,
				worker,
			)

			results <- ExecutionResult{
				worker: worker,
				err:    err,
			}
		}(worker)
	}

	// Release both workers simultaneously.
	close(start)

	// 6. Wait until one worker enters the executor.
	select {
	case <-executor.started:
		log.Println("One worker successfully claimed execution")

	case <-ctx.Done():
		log.Fatal("timed out waiting for execution claim")
	}

	// 7. The other worker must fail while the first is blocked.
	var losingResult ExecutionResult

	select {
	case losingResult = <-results:

	case <-ctx.Done():
		log.Fatal("timed out waiting for losing worker")
	}

	if !errors.Is(
		losingResult.err,
		repository.ErrActionNotExecutable,
	) {
		log.Fatalf(
			"expected second worker to be rejected, worker=%s error=%v",
			losingResult.worker,
			losingResult.err,
		)
	}

	log.Printf(
		"SUCCESS: %s was rejected because the action was already claimed",
		losingResult.worker,
	)

	// 8. Release the winning worker.
	close(executor.release)

	var winningResult ExecutionResult

	select {
	case winningResult = <-results:

	case <-ctx.Done():
		log.Fatal("timed out waiting for winning worker")
	}

	if winningResult.err != nil {
		log.Fatalf(
			"winning worker %s failed: %v",
			winningResult.worker,
			winningResult.err,
		)
	}

	wg.Wait()

	// 9. Verify executor invocation count.
	if executor.calls.Load() != 1 {
		log.Fatalf(
			"expected exactly one executor invocation, got %d",
			executor.calls.Load(),
		)
	}

	// 10. Verify persisted state.
	persisted, err := actionRepository.GetByID(
		ctx,
		actionID,
	)
	if err != nil {
		log.Fatalf("failed to retrieve action: %v", err)
	}

	if persisted.Status != model.ActionStatusExecuted {
		log.Fatalf(
			"expected EXECUTED, got %s",
			persisted.Status,
		)
	}

	if persisted.ExecutionAttempts != 1 {
		log.Fatalf(
			"expected one execution attempt, got %d",
			persisted.ExecutionAttempts,
		)
	}

	log.Println("--------------------------------")
	log.Println("CONCURRENCY TEST PASSED")
	log.Printf("Action ID: %d", actionID)
	log.Printf("Winning worker: %s", winningResult.worker)
	log.Printf("Rejected worker: %s", losingResult.worker)
	log.Printf("Executor invocations: %d", executor.calls.Load())
	log.Printf("Final status: %s", persisted.Status)
	log.Printf("Execution attempts: %d", persisted.ExecutionAttempts)
	log.Println("--------------------------------")
}
