package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
	"github.com/jackc/pgx/v5"
)

type AIHandler struct {
	service *service.AIService
}

func NewAIHandler(
	aiService *service.AIService,
) *AIHandler {
	return &AIHandler{
		service: aiService,
	}
}

func incidentIDFromAIPath(
	r *http.Request,
) (int64, error) {
	raw := r.PathValue("id")

	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid incident ID")
	}

	return id, nil
}

func writeAIJSON(
	w http.ResponseWriter,
	status int,
	data any,
) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *AIHandler) AnalyzeIncident(
	w http.ResponseWriter,
	r *http.Request,
) {
	incidentID, err := incidentIDFromAIPath(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	record, err := h.service.AnalyzeIncident(
		r.Context(),
		incidentID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) ||
			strings.Contains(err.Error(), "incident "+strconv.FormatInt(incidentID, 10)+" not found") {
			http.Error(w, "incident not found", http.StatusNotFound)
			return
		}

		http.Error(
			w,
			"incident analysis failed",
			http.StatusInternalServerError,
		)
		return
	}

	writeAIJSON(w, http.StatusCreated, record)
}

func (h *AIHandler) GetLatestAnalysis(
	w http.ResponseWriter,
	r *http.Request,
) {
	incidentID, err := incidentIDFromAIPath(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	record, err := h.service.GetLatestAnalysis(
		r.Context(),
		incidentID,
	)
	if err != nil {
		if errors.Is(err, repository.ErrAIAnalysisNotFound) {
			http.Error(
				w,
				"analysis not found",
				http.StatusNotFound,
			)
			return
		}

		http.Error(
			w,
			"failed to retrieve analysis",
			http.StatusInternalServerError,
		)
		return
	}

	writeAIJSON(w, http.StatusOK, record)
}

func (h *AIHandler) GetAnalysisHistory(
	w http.ResponseWriter,
	r *http.Request,
) {
	incidentID, err := incidentIDFromAIPath(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	limit := 20

	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > 100 {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
		limit = parsed
	}

	records, err := h.service.GetAnalysisHistory(
		r.Context(),
		incidentID,
		limit,
	)
	if err != nil {
		http.Error(
			w,
			"failed to retrieve analysis history",
			http.StatusInternalServerError,
		)
		return
	}

	writeAIJSON(w, http.StatusOK, map[string]any{
		"incident_id": incidentID,
		"analyses":    records,
		"count":       len(records),
	})
}

func (h *AIHandler) Status(
	w http.ResponseWriter,
	r *http.Request,
) {
	writeAIJSON(
		w,
		http.StatusOK,
		h.service.Status(),
	)
}
