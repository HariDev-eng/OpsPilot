package ai

import "testing"

func validIncidentContext() IncidentContext {
	return IncidentContext{
		Events: []EventSummary{
			{
				EventID: "payment-event-1",
			},
		},
		Knowledge: []KnowledgeResult{
			{
				ChunkID: 4,
			},
		},
	}
}

func TestValidateEvidence(t *testing.T) {
	tests := []struct {
		name     string
		evidence []EvidenceReference
		wantErr  bool
	}{
		{
			name: "valid event reference",
			evidence: []EvidenceReference{
				{
					SourceType: "EVENT",
					SourceID:   "payment-event-1",
					Claim:      "Payment request timed out",
				},
			},
		},
		{
			name: "valid knowledge reference",
			evidence: []EvidenceReference{
				{
					SourceType: "KNOWLEDGE",
					SourceID:   "4",
					Claim:      "Connection pool exhaustion can cause timeouts",
				},
			},
		},
		{
			name: "nonexistent event",
			evidence: []EvidenceReference{
				{
					SourceType: "EVENT",
					SourceID:   "deployment-123",
					Claim:      "Deployment occurred",
				},
			},
			wantErr: true,
		},
		{
			name: "nonexistent knowledge chunk",
			evidence: []EvidenceReference{
				{
					SourceType: "KNOWLEDGE",
					SourceID:   "99",
					Claim:      "Some knowledge",
				},
			},
			wantErr: true,
		},
		{
			name: "unsupported source type",
			evidence: []EvidenceReference{
				{
					SourceType: "DEPLOYMENT",
					SourceID:   "123",
					Claim:      "Deployment occurred",
				},
			},
			wantErr: true,
		},
		{
			name: "empty claim",
			evidence: []EvidenceReference{
				{
					SourceType: "EVENT",
					SourceID:   "payment-event-1",
					Claim:      "   ",
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate reference",
			evidence: []EvidenceReference{
				{
					SourceType: "EVENT",
					SourceID:   "payment-event-1",
					Claim:      "First claim",
				},
				{
					SourceType: "EVENT",
					SourceID:   "payment-event-1",
					Claim:      "Duplicate claim",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := IncidentAnalysis{
				Evidence: tt.evidence,
			}

			err := analysis.ValidateEvidence(validIncidentContext())

			if (err != nil) != tt.wantErr {
				t.Fatalf(
					"ValidateEvidence() error = %v, wantErr = %v",
					err,
					tt.wantErr,
				)
			}
		})
	}
}

func TestValidateActions(t *testing.T) {
	tests := []struct {
		name    string
		actions []RecommendedAction
		wantErr bool
	}{
		{
			name: "investigation without approval",
			actions: []RecommendedAction{
				{
					Description:      "Inspect application logs",
					ActionType:       "INVESTIGATION",
					RequiresApproval: false,
				},
			},
		},
		{
			name: "mitigation with approval",
			actions: []RecommendedAction{
				{
					Description:      "Restart payment service",
					ActionType:       "MITIGATION",
					RequiresApproval: true,
				},
			},
		},
		{
			name: "destructive with approval",
			actions: []RecommendedAction{
				{
					Description:      "Rollback deployment",
					ActionType:       "DESTRUCTIVE",
					RequiresApproval: true,
				},
			},
		},
		{
			name: "destructive without approval",
			actions: []RecommendedAction{
				{
					Description:      "Rollback deployment",
					ActionType:       "DESTRUCTIVE",
					RequiresApproval: false,
				},
			},
			wantErr: true,
		},
		{
			name: "mitigation without approval",
			actions: []RecommendedAction{
				{
					Description:      "Restart payment service",
					ActionType:       "MITIGATION",
					RequiresApproval: false,
				},
			},
			wantErr: true,
		},
		{
			name: "unsupported action type",
			actions: []RecommendedAction{
				{
					Description: "Delete database",
					ActionType:  "DELETE_DATABASE",
				},
			},
			wantErr: true,
		},
		{
			name: "empty description",
			actions: []RecommendedAction{
				{
					ActionType: "INVESTIGATION",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := IncidentAnalysis{
				RecommendedActions: tt.actions,
			}

			err := analysis.ValidateActions()

			if (err != nil) != tt.wantErr {
				t.Fatalf(
					"ValidateActions() error = %v, wantErr = %v",
					err,
					tt.wantErr,
				)
			}
		})
	}
}
