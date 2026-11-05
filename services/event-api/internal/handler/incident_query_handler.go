package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
	"github.com/jackc/pgx/v5"
)

type IncidentQueryHandler struct {
	service *service.IncidentQueryService
}

func NewIncidentQueryHandler(
	queryService *service.IncidentQueryService,
) *IncidentQueryHandler {
	return &IncidentQueryHandler{
		service: queryService,
	}
}

func (h *IncidentQueryHandler) GetEvents(
	w http.ResponseWriter,
	r *http.Request,
) {
	incidentID, err := incidentIDFromPath(r)
	if err != nil || incidentID <= 0 {
		http.Error(w, "invalid incident id", http.StatusBadRequest)
		return
	}

	events, err := h.service.GetEvents(r.Context(), incidentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "incident not found", http.StatusNotFound)
			return
		}

		http.Error(
			w,
			"failed to retrieve incident events",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(events)
}

func (h *IncidentQueryHandler) GetActions(
	w http.ResponseWriter,
	r *http.Request,
) {
	incidentID, err := incidentIDFromPath(r)
	if err != nil || incidentID <= 0 {
		http.Error(w, "invalid incident id", http.StatusBadRequest)
		return
	}

	actions, err := h.service.GetActions(r.Context(), incidentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "incident not found", http.StatusNotFound)
			return
		}

		http.Error(
			w,
			"failed to retrieve incident actions",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(actions)
}
