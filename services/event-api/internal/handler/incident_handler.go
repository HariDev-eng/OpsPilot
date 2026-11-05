package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
	"github.com/jackc/pgx/v5"
)

type IncidentHandler struct {
	service *service.IncidentService
}

func NewIncidentHandler(
	incidentService *service.IncidentService,
) *IncidentHandler {
	return &IncidentHandler{
		service: incidentService,
	}
}

func (h *IncidentHandler) Acknowledge(
	w http.ResponseWriter,
	r *http.Request,
) {
	incidentID, err := incidentIDFromPath(r)
	if err != nil {
		http.Error(w, "invalid incident id", http.StatusBadRequest)
		return
	}

	err = h.service.Acknowledge(
		r.Context(),
		incidentID,
	)
	if err != nil {
		http.Error(
			w,
			err.Error(),
			http.StatusConflict,
		)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *IncidentHandler) Resolve(
	w http.ResponseWriter,
	r *http.Request,
) {
	incidentID, err := incidentIDFromPath(r)
	if err != nil {
		http.Error(w, "invalid incident id", http.StatusBadRequest)
		return
	}

	err = h.service.Resolve(
		r.Context(),
		incidentID,
	)
	if err != nil {
		http.Error(
			w,
			err.Error(),
			http.StatusConflict,
		)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func incidentIDFromPath(r *http.Request) (int64, error) {
	path := strings.TrimPrefix(
		r.URL.Path,
		"/api/v1/incidents/",
	)

	parts := strings.Split(path, "/")

	return strconv.ParseInt(parts[0], 10, 64)
}

func (h *IncidentHandler) List(
	w http.ResponseWriter,
	r *http.Request,
) {
	limit := 50

	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
		limit = parsed
	}

	incidents, err := h.service.List(r.Context(), limit)
	if err != nil {
		http.Error(
			w,
			"failed to retrieve incidents",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(incidents)
}

func (h *IncidentHandler) GetByID(
	w http.ResponseWriter,
	r *http.Request,
) {
	incidentID, err := incidentIDFromPath(r)
	if err != nil || incidentID <= 0 {
		http.Error(w, "invalid incident id", http.StatusBadRequest)
		return
	}

	incident, err := h.service.GetByID(r.Context(), incidentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "incident not found", http.StatusNotFound)
			return
		}

		http.Error(
			w,
			"failed to retrieve incident",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(incident)
}

func (h *IncidentQueryHandler) GetDashboardSummary(
	w http.ResponseWriter,
	r *http.Request,
) {
	summary, err := h.service.GetDashboardSummary(r.Context())
	if err != nil {
		http.Error(
			w,
			"failed to retrieve dashboard summary",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(summary)
}
