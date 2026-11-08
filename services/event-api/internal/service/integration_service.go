package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
)

var (
	ErrInvalidIntegration = errors.New("integration name and service name are required")
)

type IntegrationService struct {
	repo *repository.IntegrationRepository
}

type CreateIntegrationResult struct {
	Integration model.Integration `json:"integration"`
	APIKey      string            `json:"api_key"`
}

func NewIntegrationService(
	repo *repository.IntegrationRepository,
) *IntegrationService {
	return &IntegrationService{repo: repo}
}

func generateUUID() (string, error) {
	var id [16]byte

	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}

	// UUID version 4 and RFC 4122 variant.
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		id[0:4],
		id[4:6],
		id[6:8],
		id[8:10],
		id[10:16],
	), nil
}

func generateAPIKey() (string, error) {
	randomBytes := make([]byte, 32)

	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	return "opsp_live_" + hex.EncodeToString(randomBytes), nil
}

func hashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}

func (s *IntegrationService) Create(
	ctx context.Context,
	name string,
	serviceName string,
) (CreateIntegrationResult, error) {
	name = strings.TrimSpace(name)
	serviceName = strings.TrimSpace(serviceName)

	if name == "" || serviceName == "" {
		return CreateIntegrationResult{}, ErrInvalidIntegration
	}

	id, err := generateUUID()
	if err != nil {
		return CreateIntegrationResult{}, err
	}

	apiKey, err := generateAPIKey()
	if err != nil {
		return CreateIntegrationResult{}, err
	}

	const keyPrefixLength = 18
	keyPrefix := apiKey[:keyPrefixLength]

	integration := model.Integration{
		ID:          id,
		Name:        name,
		ServiceName: serviceName,
		KeyPrefix:   keyPrefix,
		KeyHash:     hashAPIKey(apiKey),
	}

	if err := s.repo.Create(ctx, &integration); err != nil {
		return CreateIntegrationResult{}, err
	}

	// Return the raw key only in this creation response.
	integration.KeyHash = ""

	return CreateIntegrationResult{
		Integration: integration,
		APIKey:      apiKey,
	}, nil
}

func (s *IntegrationService) List(
	ctx context.Context,
) ([]model.Integration, error) {
	return s.repo.List(ctx)
}

func (s *IntegrationService) Revoke(
	ctx context.Context,
	id string,
) error {
	id = strings.TrimSpace(id)

	if id == "" {
		return repository.ErrIntegrationNotFound
	}

	return s.repo.Revoke(ctx, id)
}

func (s *IntegrationService) AuthenticateAPIKey(
	ctx context.Context,
	apiKey string,
) (model.Integration, error) {
	apiKey = strings.TrimSpace(apiKey)

	if !strings.HasPrefix(apiKey, "opsp_live_") {
		return model.Integration{}, repository.ErrIntegrationNotFound
	}

	keyHash := hashAPIKey(apiKey)

	integration, err := s.repo.GetActiveByKeyHash(ctx, keyHash)
	if err != nil {
		return model.Integration{}, err
	}

	if err := s.repo.UpdateLastUsed(ctx, integration.ID); err != nil {
		return model.Integration{}, err
	}

	now := time.Now().UTC()
	integration.LastUsedAt = &now

	// Never expose the stored hash through this returned model.
	integration.KeyHash = ""

	return integration, nil
}
