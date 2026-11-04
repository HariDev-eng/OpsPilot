package main

import (
	"context"
	"log"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/ai"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/database"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
)

func main() {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
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

	// Controlled mitigation recommendation.
	// This is persisted for approval testing only.
	analysis := &ai.IncidentAnalysis{
		RecommendedActions: []ai.RecommendedAction{
			{
				Description:      "Scale payment service replicas after operator verification",
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
		log.Fatalf("failed to create test action: %v", err)
	}

	for _, action := range actions {
		log.Printf(
			"Action ID=%d Type=%s Status=%s RequiresApproval=%t",
			action.ID,
			action.ActionType,
			action.Status,
			action.RequiresApproval,
		)
	}
}
