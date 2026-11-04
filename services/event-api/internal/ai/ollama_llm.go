package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type OllamaLLM struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

func NewOllamaLLM(
	baseURL string,
	model string,
) *OllamaLLM {
	return &OllamaLLM{
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

type ollamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Format string `json:"format,omitempty"`
	Stream bool   `json:"stream"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func (o *OllamaLLM) Analyze(
	ctx context.Context,
	incidentContext IncidentContext,
) (*IncidentAnalysis, error) {

	prompt := buildIncidentAnalysisPrompt(incidentContext)

	requestBody := ollamaGenerateRequest{
		Model:  o.model,
		Prompt: prompt,
		Format: "json",
		Stream: false,
	}

	data, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		o.baseURL+"/api/generate",
		bytes.NewReader(data),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Ollama request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"Ollama returned HTTP %d",
			resp.StatusCode,
		)
	}

	var ollamaResponse ollamaGenerateResponse

	if err := json.NewDecoder(resp.Body).Decode(&ollamaResponse); err != nil {
		return nil, fmt.Errorf(
			"failed to decode Ollama response: %w",
			err,
		)
	}

	rawJSON := strings.TrimSpace(ollamaResponse.Response)

	var analysis IncidentAnalysis

	if err := json.Unmarshal(
		[]byte(rawJSON),
		&analysis,
	); err != nil {
		return nil, fmt.Errorf(
			"failed to parse LLM analysis JSON: %w; response=%s",
			err,
			rawJSON,
		)
	}

	if err := analysis.Validate(); err != nil {
		return nil, fmt.Errorf(
			"invalid incident analysis: %w",
			err,
		)
	}

	if err := analysis.ValidateEvidence(incidentContext); err != nil {
		return nil, fmt.Errorf(
			"invalid evidence references: %w",
			err,
		)
	}

	if err := analysis.ValidateActions(); err != nil {
		return nil, fmt.Errorf(
			"invalid recommended actions: %w",
			err,
		)
	}

	return &analysis, nil
}

func buildIncidentAnalysisPrompt(
	ctx IncidentContext,
) string {

	var builder strings.Builder

	builder.WriteString(`
You are an incident intelligence assistant for a production
incident management system.

Your task is to analyze the supplied incident using ONLY the
incident data, observed events, and retrieved knowledge provided
below.


Rules:
1. Do not invent evidence, events, metrics, or infrastructure changes.
2. Clearly distinguish observed facts from possible explanations.
3. If evidence is insufficient, explicitly state that the cause is unconfirmed.
4. Retrieved knowledge describes general troubleshooting relationships.
   It is NOT proof that a specific event occurred.
5. Never claim that a deployment, rollback, configuration change,
   infrastructure change, or external action occurred unless explicitly
   present in the supplied incident evidence.
6. Every factual claim must be traceable to the supplied incident data.
7. Label explanations that are not directly supported by incident data
   as hypotheses.
8. Recommended actions must be grounded in the supplied evidence
   or retrieved knowledge.
9. Do not perform or claim to have performed any action.
10. Return ONLY valid JSON matching the required structure.
11. Confidence must be a number between 0 and 1.
12. Confidence represents your subjective assessment of the available
    evidence, NOT a statistically calibrated probability.
13. Avoid repeating the same information across summary, evidence,
    and risks.
14. Every evidence item must reference an actual source supplied below.
15. For event evidence:
	- source_type must be EVENT.
    - source_id must exactly match a supplied event_id.
16. For retrieved knowledge:
    - source_type must be KNOWLEDGE.
    - source_id must exactly match a supplied chunk_id.
17. Never invent source IDs.
18. Do not use retrieved knowledge as proof that a specific event occurred.
19. If no source supports a claim, do not include it as evidence.
20. Classify every recommended action as INVESTIGATION, MITIGATION,
    or DESTRUCTIVE.
21. INVESTIGATION actions only inspect or collect information.
22. MITIGATION actions change system state to reduce incident impact.
23. DESTRUCTIVE actions include rollback, deletion, data modification,
    or other potentially high-impact operations.
24. MITIGATION and DESTRUCTIVE actions must always require human approval.
25. INVESTIGATION actions do not require approval.
26. Never recommend automatically executing an action.
27. Prefer investigation before mitigation when the root cause is unconfirmed.
28. Never recommend rollback unless there is evidence of a deployment
    and a clear reason to investigate rollback.


Required JSON structure:

{
  "summary": "short incident summary",
  "likely_cause": "most likely explanation",
  "cause_status": "UNCONFIRMED",
  "confidence": 0.0,
  "evidence": [
    {
      "source_type": "EVENT",
      "source_id": "event-id",
      "claim": "observed event evidence"
    }
  ],
  "recommended_actions": [
    {
      "description": "Inspect payment service application logs",
      "action_type": "INVESTIGATION",
      "requires_approval": false
    }
  ]
  "risks": [
    "potential operational risks"
  ]
}
`)

	builder.WriteString("\nINCIDENT:\n")

	builder.WriteString(
		fmt.Sprintf(
			"ID: %d\nService: %s\nSeverity: %s\nStatus: %s\nCreated At: %s\n",
			ctx.Incident.ID,
			ctx.Incident.Service,
			ctx.Incident.Severity,
			ctx.Incident.Status,
			ctx.Incident.CreatedAt.Format(time.RFC3339),
		),
	)

	builder.WriteString("\nEVENTS:\n")

	for _, event := range ctx.Events {
		payload, err := json.Marshal(event.Payload)
		if err != nil {
			payload = []byte(`{"error":"failed to serialize payload"}`)
		}

		builder.WriteString(
			fmt.Sprintf(
				"- event_id=%s type=%s service=%s timestamp=%s payload=%s\n",
				event.EventID,
				event.EventType,
				event.Service,
				event.Timestamp.Format(time.RFC3339),
				payload,
			),
		)
	}

	builder.WriteString("\nRETRIEVED KNOWLEDGE:\n")

	for _, knowledge := range ctx.Knowledge {
		builder.WriteString(
			fmt.Sprintf(
				"- document_id=%d chunk_id=%d distance=%.4f\n%s\n",
				knowledge.DocumentID,
				knowledge.ChunkID,
				knowledge.Distance,
				knowledge.Content,
			),
		)
	}

	return builder.String()
}
