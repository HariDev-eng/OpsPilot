package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
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
