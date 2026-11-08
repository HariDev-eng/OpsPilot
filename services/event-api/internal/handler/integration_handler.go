package handler

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
)

type IntegrationHandler struct {
	service     *service.IntegrationService
	operatorKey string
}

func NewIntegrationHandler(
	integrationService *service.IntegrationService,
	operatorKey string,
) *IntegrationHandler {
	return &IntegrationHandler{
		service:     integrationService,
		operatorKey: operatorKey,
	}
}

func (h *IntegrationHandler) authorized(r *http.Request) bool {
	if h.operatorKey == "" {
		return false
	}

	token := strings.TrimSpace(strings.TrimPrefix(
		r.Header.Get("Authorization"),
		"Bearer ",
	))

	if token == "" {
		return false
	}

	return subtle.ConstantTimeCompare(
		[]byte(token),
		[]byte(h.operatorKey),
	) == 1
}

func (h *IntegrationHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Name        string `json:"name"`
		ServiceName string `json:"service_name"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	result, err := h.service.Create(r.Context(), req.Name, req.ServiceName)
	if err != nil {
		if errors.Is(err, service.ErrInvalidIntegration) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		http.Error(w, "failed to create integration", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (h *IntegrationHandler) List(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	integrations, err := h.service.List(r.Context())
	if err != nil {
		http.Error(w, "failed to list integrations", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"integrations": integrations,
	})
}

func (h *IntegrationHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")

	if err := h.service.Revoke(r.Context(), id); err != nil {
		if errors.Is(err, repository.ErrIntegrationNotFound) {
			http.Error(w, "integration not found", http.StatusNotFound)
			return
		}

		http.Error(w, "failed to revoke integration", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"id":     id,
		"status": "revoked",
	})
}

func operatorSecret() string {
	return strings.TrimSpace(os.Getenv("OPSPILOT_OPERATOR_SECRET"))
}
