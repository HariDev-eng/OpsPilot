package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
)

type IntegrationEventHandler struct {
	eventService       *service.EventService
	integrationService *service.IntegrationService
}

func NewIntegrationEventHandler(
	eventService *service.EventService,
	integrationService *service.IntegrationService,
) *IntegrationEventHandler {
	return &IntegrationEventHandler{
		eventService:       eventService,
		integrationService: integrationService,
	}
}

func (h *IntegrationEventHandler) CreateEvent(
	w http.ResponseWriter,
	r *http.Request,
) {
	// Require the exact Bearer authentication scheme.
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	apiKey := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if apiKey == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	integration, err := h.integrationService.AuthenticateAPIKey(
		r.Context(),
		apiKey,
	)
	if err != nil {
		if errors.Is(err, repository.ErrIntegrationNotFound) {
			http.Error(w, "invalid or revoked integration key", http.StatusUnauthorized)
			return
		}

		http.Error(w, "authentication failed", http.StatusInternalServerError)
		return
	}

	// Limit incoming request size to 1 MiB.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req model.CreateEventRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Reject multiple JSON values in one request.
	if err := decoder.Decode(&struct{}{}); err != nil {
		if !errors.Is(err, context.Canceled) && err.Error() != "EOF" {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
	} else {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if err := validateCreateEventRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	event := model.Event{
		EventID:       req.EventID,
		EventType:     req.EventType,
		Service:       req.Service,
		Timestamp:     req.Timestamp,
		Payload:       req.Payload,
		IntegrationID: &integration.ID,
	}

	result, err := h.eventService.CreateEvent(r.Context(), event)
	if err != nil {
		http.Error(w, "failed to create event", http.StatusInternalServerError)
		return
	}

	if !result.Created {
		writeJSON(w, http.StatusOK, map[string]string{
			"event_id": req.EventID,
			"status":   "already_received",
		})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"event_id": req.EventID,
		"status":   "accepted",
	})
}
