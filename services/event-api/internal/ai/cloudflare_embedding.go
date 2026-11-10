package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const cloudflareEmbeddingDimensions = 768

type CloudflareEmbeddingProvider struct {
	accountID  string
	apiToken   string
	model      string
	httpClient *http.Client
}

func NewCloudflareEmbeddingProvider(
	accountID string,
	apiToken string,
	model string,
) *CloudflareEmbeddingProvider {
	return &CloudflareEmbeddingProvider{
		accountID: accountID,
		apiToken:  apiToken,
		model:     model,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type cloudflareEmbeddingRequest struct {
	Text []string `json:"text"`
}

type cloudflareEmbeddingResponse struct {
	Result struct {
		Data  [][]float32 `json:"data"`
		Shape []int       `json:"shape"`
	} `json:"result"`

	Success bool `json:"success"`

	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *CloudflareEmbeddingProvider) Embed(
	ctx context.Context,
	input string,
) ([]float32, error) {

	requestBody := cloudflareEmbeddingRequest{
		Text: []string{input},
	}

	data, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf(
			"marshal embedding request: %w",
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
			"create embedding request: %w",
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
			"call Cloudflare embedding API: %w",
			err,
		)
	}
	defer resp.Body.Close()

	var result cloudflareEmbeddingResponse

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf(
			"decode embedding response (HTTP %d): %w",
			resp.StatusCode,
			err,
		)
	}

	if resp.StatusCode != http.StatusOK || !result.Success {
		if len(result.Errors) > 0 {
			return nil, fmt.Errorf(
				"Cloudflare embedding error (HTTP %d): %s",
				resp.StatusCode,
				result.Errors[0].Message,
			)
		}

		return nil, fmt.Errorf(
			"Cloudflare embedding API returned HTTP %d",
			resp.StatusCode,
		)
	}

	if len(result.Result.Data) != 1 {
		return nil, fmt.Errorf(
			"expected 1 embedding, received %d",
			len(result.Result.Data),
		)
	}

	embedding := result.Result.Data[0]

	if len(embedding) != cloudflareEmbeddingDimensions {
		return nil, fmt.Errorf(
			"unexpected embedding dimensions: expected %d, received %d",
			cloudflareEmbeddingDimensions,
			len(embedding),
		)
	}

	return embedding, nil
}
