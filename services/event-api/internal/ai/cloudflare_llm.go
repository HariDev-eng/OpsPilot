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

type CloudflareLLM struct {
	accountID  string
	apiToken   string
	model      string
	httpClient *http.Client
}

func NewCloudflareLLM(
	accountID string,
	apiToken string,
	model string,
) *CloudflareLLM {
	return &CloudflareLLM{
		accountID: accountID,
		apiToken:  apiToken,
		model:     model,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

type cloudflareMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type cloudflareLLMRequest struct {
	Messages       []cloudflareMessage `json:"messages"`
	MaxTokens      int                 `json:"max_tokens"`
	Temperature    float64             `json:"temperature"`
	Stream         bool                `json:"stream"`
	ResponseFormat map[string]string   `json:"response_format,omitempty"`
}

type cloudflareLLMResponse struct {
	Result struct {
		Response string `json:"response"`
	} `json:"result"`

	Success bool `json:"success"`

	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *CloudflareLLM) Analyze(
	ctx context.Context,
	incidentContext IncidentContext,
) (*IncidentAnalysis, error) {

	prompt := buildIncidentAnalysisPrompt(incidentContext)

	requestBody := cloudflareLLMRequest{
		Messages: []cloudflareMessage{
			{
				Role: "system",
				Content: "You are an incident intelligence assistant. " +
					"Analyze only the supplied evidence. " +
					"Return valid JSON matching the requested structure.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens:   2048,
		Temperature: 0.1,
		Stream:      false,
		ResponseFormat: map[string]string{
			"type": "json_object",
		},
	}

	data, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf(
			"marshal Cloudflare request: %w",
			err,
		)
	}

	url := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/accounts/%s/ai/run/%s",
		c.accountID,
		c.model,
	)

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url,
		bytes.NewReader(data),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create Cloudflare request: %w",
			err,
		)
	}

	req.Header.Set(
		"Authorization",
		"Bearer "+c.apiToken,
	)
	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf(
			"call Cloudflare Workers AI: %w",
			err,
		)
	}
	defer resp.Body.Close()

	var result cloudflareLLMResponse

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf(
			"decode Cloudflare response (HTTP %d): %w",
			resp.StatusCode,
			err,
		)
	}

	if resp.StatusCode != http.StatusOK || !result.Success {
		if len(result.Errors) > 0 {
			return nil, fmt.Errorf(
				"Cloudflare AI error (HTTP %d): %s",
				resp.StatusCode,
				result.Errors[0].Message,
			)
		}

		return nil, fmt.Errorf(
			"Cloudflare AI returned HTTP %d",
			resp.StatusCode,
		)
	}

	rawJSON := strings.TrimSpace(result.Result.Response)

	if rawJSON == "" {
		return nil, fmt.Errorf(
			"Cloudflare returned an empty model response",
		)
	}

	// Parse model-generated JSON.
	var analysis IncidentAnalysis

	if err := json.Unmarshal(
		[]byte(rawJSON),
		&analysis,
	); err != nil {
		return nil, fmt.Errorf(
			"parse incident analysis JSON: %w",
			err,
		)
	}

	// Validate required fields and confidence.
	if err := analysis.Validate(); err != nil {
		return nil, fmt.Errorf(
			"invalid incident analysis: %w",
			err,
		)
	}

	// Verify every evidence reference against supplied context.
	if err := analysis.ValidateEvidence(incidentContext); err != nil {
		return nil, fmt.Errorf(
			"invalid evidence references: %w",
			err,
		)
	}

	// Validate action categories and approval requirements.
	if err := analysis.ValidateActions(); err != nil {
		return nil, fmt.Errorf(
			"invalid recommended actions: %w",
			err,
		)
	}

	return &analysis, nil
}
