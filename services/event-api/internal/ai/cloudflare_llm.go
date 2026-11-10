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
	Messages            []cloudflareMessage `json:"messages"`
	MaxCompletionTokens int                 `json:"max_completion_tokens"`
	Temperature         float64             `json:"temperature"`
	Stream              bool                `json:"stream"`
	ChatTemplateKwargs  map[string]bool     `json:"chat_template_kwargs,omitempty"`
}

type cloudflareLLMResponse struct {
	Result struct {
		// Legacy Workers AI text-generation response.
		Response string `json:"response"`

		// OpenAI-compatible response used by newer Workers AI models.
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				Reasoning string `json:"reasoning"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`

		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
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
		MaxCompletionTokens: 2048,
		Temperature:         0.1,
		Stream:              false,
		// Gemma 4 enables thinking by default. Disable it for this
		// synchronous structured-output request so the model returns
		// the requested JSON in its message content.
		ChatTemplateKwargs: map[string]bool{
			"enable_thinking": false,
		},
	}

	data, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal Cloudflare request: %w", err)
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
		return nil, fmt.Errorf("create Cloudflare request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call Cloudflare Workers AI: %w", err)
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
		return nil, fmt.Errorf("Cloudflare AI returned HTTP %d", resp.StatusCode)
	}

	// Prefer the legacy response field when present, then fall back to
	// the OpenAI-compatible choices shape used by newer Workers AI models.
	rawJSON := strings.TrimSpace(result.Result.Response)
	finishReason := ""

	if rawJSON == "" && len(result.Result.Choices) > 0 {
		rawJSON = strings.TrimSpace(result.Result.Choices[0].Message.Content)
		finishReason = result.Result.Choices[0].FinishReason
	}

	if rawJSON == "" {
		return nil, fmt.Errorf(
			"Cloudflare returned empty model content (finish_reason=%q, prompt_tokens=%d, completion_tokens=%d)",
			finishReason,
			result.Result.Usage.PromptTokens,
			result.Result.Usage.CompletionTokens,
		)
	}

	var analysis IncidentAnalysis
	if err := json.Unmarshal([]byte(rawJSON), &analysis); err != nil {
		return nil, fmt.Errorf("parse incident analysis JSON: %w", err)
	}

	if err := analysis.Validate(); err != nil {
		return nil, fmt.Errorf("invalid incident analysis: %w", err)
	}

	if err := analysis.ValidateEvidence(incidentContext); err != nil {
		return nil, fmt.Errorf("invalid evidence references: %w", err)
	}

	if err := analysis.ValidateActions(); err != nil {
		return nil, fmt.Errorf("invalid recommended actions: %w", err)
	}

	return &analysis, nil
}
