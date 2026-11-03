package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type OllamaEmbeddingProvider struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

func NewOllamaEmbeddingProvider(
	baseURL string,
	model string,
) *OllamaEmbeddingProvider {
	return &OllamaEmbeddingProvider{
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type ollamaEmbedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type ollamaEmbedResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float32 `json:"embeddings"`
}

func (o *OllamaEmbeddingProvider) Embed(
	ctx context.Context,
	text string,
) ([]float32, error) {

	requestBody := ollamaEmbedRequest{
		Model: o.model,
		Input: text,
	}

	data, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to marshal embedding request: %w",
			err,
		)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		o.baseURL+"/api/embed",
		bytes.NewReader(data),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create embedding request: %w",
			err,
		)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to call Ollama: %w",
			err,
		)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"Ollama returned status %d",
			resp.StatusCode,
		)
	}

	var result ollamaEmbedResponse

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf(
			"failed to decode Ollama response: %w",
			err,
		)
	}

	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf(
			"Ollama returned no embeddings",
		)
	}

	if len(result.Embeddings) != 1 {
		return nil, fmt.Errorf(
			"expected 1 embedding, got %d",
			len(result.Embeddings),
		)
	}

	return result.Embeddings[0], nil
}
