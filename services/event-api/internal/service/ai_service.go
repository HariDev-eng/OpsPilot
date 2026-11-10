package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/ai"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
)

type AIService struct {
	contextBuilder *ai.ContextBuilder
	llm            ai.LLMProvider
	analysisRepo   *repository.AIAnalysisRepository
	actionService  *ActionService

	provider string
	model    string
}

func NewAIService(
	contextBuilder *ai.ContextBuilder,
	llm ai.LLMProvider,
	analysisRepo *repository.AIAnalysisRepository,
	actionService *ActionService,
	provider string,
	modelName string,
) *AIService {
	return &AIService{
		contextBuilder: contextBuilder,
		llm:            llm,
		analysisRepo:   analysisRepo,
		actionService:  actionService,
		provider:       provider,
		model:          modelName,
	}
}

func (s *AIService) AnalyzeIncident(
	ctx context.Context,
	incidentID int64,
) (*model.AIAnalysisRecord, error) {

	if incidentID <= 0 {
		return nil, errors.New("invalid incident ID")
	}

	// Bound model execution time.
	analysisCtx, cancel := context.WithTimeout(
		ctx,
		150*time.Second,
	)
	defer cancel()

	// Persist a RUNNING analysis record first.
	record, err := s.analysisRepo.Create(
		analysisCtx,
		incidentID,
		s.provider,
		s.model,
	)
	if err != nil {
		return nil, err
	}

	// Build context using existing incident/event/knowledge repositories.
	incidentContext, err := s.contextBuilder.Build(
		analysisCtx,
		incidentID,
	)
	if err != nil {
		s.fail(record.ID, err)
		return nil, fmt.Errorf("build incident context: %w", err)
	}

	// Run existing LLM implementation.
	analysis, err := s.llm.Analyze(
		analysisCtx,
		*incidentContext,
	)
	if err != nil {
		s.fail(record.ID, err)
		return nil, fmt.Errorf("analyze incident: %w", err)
	}

	// Validate all evidence against the actual supplied context.
	if err := analysis.ValidateEvidence(*incidentContext); err != nil {
		s.fail(record.ID, err)
		return nil, fmt.Errorf("validate analysis evidence: %w", err)
	}

	if err := analysis.ValidateActions(); err != nil {
		s.fail(record.ID, err)
		return nil, fmt.Errorf("validate recommended actions: %w", err)
	}

	// Persist recommendations through the existing action workflow.
	actions, err := s.actionService.CreateFromAnalysis(
		analysisCtx,
		incidentID,
		analysis,
	)
	if err != nil {
		s.fail(record.ID, err)
		return nil, fmt.Errorf("persist recommended actions: %w", err)
	}

	// Store analysis and its generated action references together
	// in the analysis JSON document.
	result := struct {
		*ai.IncidentAnalysis
		Actions []model.IncidentAction `json:"actions"`
	}{
		IncidentAnalysis: analysis,
		Actions:          actions,
	}

	if err := s.analysisRepo.Complete(
		analysisCtx,
		record.ID,
		result,
	); err != nil {
		s.fail(record.ID, err)
		return nil, err
	}

	return s.analysisRepo.GetByID(
		analysisCtx,
		record.ID,
	)
}

func (s *AIService) fail(
	analysisID int64,
	cause error,
) {
	// Use a fresh context because the analysis context may have timed out.
	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	if err := s.analysisRepo.Fail(
		ctx,
		analysisID,
		cause.Error(),
	); err != nil {
		// The original failure remains the primary error.
		fmt.Printf(
			"failed to persist AI analysis failure: analysis_id=%d error=%v\n",
			analysisID,
			err,
		)
	}
}

func (s *AIService) GetLatestAnalysis(
	ctx context.Context,
	incidentID int64,
) (*model.AIAnalysisRecord, error) {
	return s.analysisRepo.GetLatest(ctx, incidentID)
}

func (s *AIService) GetAnalysisHistory(
	ctx context.Context,
	incidentID int64,
	limit int,
) ([]model.AIAnalysisRecord, error) {
	return s.analysisRepo.ListByIncident(
		ctx,
		incidentID,
		limit,
	)
}

func (s *AIService) Status() map[string]any {
	return map[string]any{
		"enabled":           s.llm != nil,
		"provider":          s.provider,
		"model":             s.model,
		"embedding_enabled": s.contextBuilder != nil,
	}
}
