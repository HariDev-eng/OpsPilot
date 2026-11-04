package ai

import "context"

type LLMProvider interface {
	Analyze(
		ctx context.Context,
		incidentContext IncidentContext,
	) (*IncidentAnalysis, error)
}
