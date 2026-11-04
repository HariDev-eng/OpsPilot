package ai

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type IncidentAnalysis struct {
	Summary            string              `json:"summary"`
	LikelyCause        string              `json:"likely_cause"`
	CauseStatus        string              `json:"cause_status"`
	Confidence         float64             `json:"confidence"`
	Evidence           []EvidenceReference `json:"evidence"`
	RecommendedActions []RecommendedAction `json:"recommended_actions"`
	Risks              []string            `json:"risks"`
}

type EvidenceReference struct {
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
	Claim      string `json:"claim"`
}

type RecommendedAction struct {
	Description      string `json:"description"`
	ActionType       string `json:"action_type"`
	RequiresApproval bool   `json:"requires_approval"`
}

func (a *IncidentAnalysis) Validate() error {
	if strings.TrimSpace(a.Summary) == "" {
		return fmt.Errorf("summary is required")
	}

	if strings.TrimSpace(a.LikelyCause) == "" {
		return fmt.Errorf("likely_cause is required")
	}

	switch a.CauseStatus {
	case "CONFIRMED", "UNCONFIRMED", "UNKNOWN":
	default:
		return fmt.Errorf("invalid cause_status: %q", a.CauseStatus)
	}

	if math.IsNaN(a.Confidence) ||
		math.IsInf(a.Confidence, 0) ||
		a.Confidence < 0 ||
		a.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}

	if len(a.Evidence) == 0 {
		return fmt.Errorf("at least one evidence item is required")
	}

	return nil
}

func (a *IncidentAnalysis) ValidateEvidence(
	incidentContext IncidentContext,
) error {
	eventIDs := make(map[string]struct{}, len(incidentContext.Events))

	for _, event := range incidentContext.Events {
		eventIDs[event.EventID] = struct{}{}
	}

	chunkIDs := make(map[string]struct{}, len(incidentContext.Knowledge))

	for _, knowledge := range incidentContext.Knowledge {
		chunkIDs[strconv.FormatInt(knowledge.ChunkID, 10)] = struct{}{}
	}

	seen := make(map[string]struct{})

	for i, evidence := range a.Evidence {
		if strings.TrimSpace(evidence.SourceID) == "" {
			return fmt.Errorf("evidence[%d]: source_id is required", i)
		}

		if strings.TrimSpace(evidence.Claim) == "" {
			return fmt.Errorf("evidence[%d]: claim is required", i)
		}

		key := evidence.SourceType + ":" + evidence.SourceID

		if _, exists := seen[key]; exists {
			return fmt.Errorf("evidence[%d]: duplicate source reference %q", i, key)
		}
		seen[key] = struct{}{}

		switch evidence.SourceType {
		case "EVENT":
			if _, exists := eventIDs[evidence.SourceID]; !exists {
				return fmt.Errorf(
					"evidence[%d]: event %q was not supplied in incident context",
					i,
					evidence.SourceID,
				)
			}

		case "KNOWLEDGE":
			if _, exists := chunkIDs[evidence.SourceID]; !exists {
				return fmt.Errorf(
					"evidence[%d]: knowledge chunk %q was not supplied in incident context",
					i,
					evidence.SourceID,
				)
			}

		default:
			return fmt.Errorf(
				"evidence[%d]: unsupported source_type %q",
				i,
				evidence.SourceType,
			)
		}
	}

	return nil
}

func (a *IncidentAnalysis) ValidateActions() error {
	for i, action := range a.RecommendedActions {
		if strings.TrimSpace(action.Description) == "" {
			return fmt.Errorf(
				"action[%d]: description is required",
				i,
			)
		}

		switch action.ActionType {
		case "INVESTIGATION":
			if action.RequiresApproval {
				return fmt.Errorf(
					"action[%d]: investigation does not require approval",
					i,
				)
			}

		case "MITIGATION", "DESTRUCTIVE":
			if !action.RequiresApproval {
				return fmt.Errorf(
					"action[%d]: %s action requires approval",
					i,
					action.ActionType,
				)
			}

		default:
			return fmt.Errorf(
				"action[%d]: unsupported action type %q",
				i,
				action.ActionType,
			)
		}
	}

	return nil
}
