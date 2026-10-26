package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
)

type EventHandler struct {
	service *service.EventService
}

func NewEventHandler(eventService *service.EventService) *EventHandler {
	return &EventHandler{
		service: eventService,
	}
}

func (h *EventHandler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	var req model.CreateEventRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if err := validateCreateEventRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	event := model.Event{
		EventID:   req.EventID,
		EventType: req.EventType,
		Service:   req.Service,
		Timestamp: req.Timestamp,
		Payload:   req.Payload,
	}

	result, err := h.service.CreateEvent(r.Context(), event)
	if err != nil {
		http.Error(w, "failed to create event", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if !result.Created {
		w.WriteHeader(http.StatusOK)

		json.NewEncoder(w).Encode(map[string]string{
			"event_id": req.EventID,
			"status":   "already_received",
		})

		return
	}

	w.WriteHeader(http.StatusAccepted)

	json.NewEncoder(w).Encode(map[string]string{
		"event_id": req.EventID,
		"status":   "accepted",
	})
}

func validateCreateEventRequest(req model.CreateEventRequest) error {
	if strings.TrimSpace(req.EventID) == "" {
		return errors.New("event_id is required")
	}

	if strings.TrimSpace(req.EventType) == "" {
		return errors.New("event_type is required")
	}

	if strings.TrimSpace(req.Service) == "" {
		return errors.New("service is required")
	}

	if req.Timestamp.IsZero() {
		return errors.New("timestamp is required")
	}

	if len(req.Payload) == 0 {
		return errors.New("payload is required")
	}

	return nil
}
