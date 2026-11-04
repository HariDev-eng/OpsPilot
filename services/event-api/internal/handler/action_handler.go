package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
	"github.com/jackc/pgx/v5"
)

type ActionHandler struct {
	service *service.ActionService
}

func NewActionHandler(
	actionService *service.ActionService,
) *ActionHandler {
	return &ActionHandler{
		service: actionService,
	}
}

// Temporary development identity.
// Replace this with authenticated identity from middleware
// before exposing the API to real users.
const developmentActorID = "dev-operator"

type actionResponse struct {
	ActionID int64  `json:"action_id"`
	Status   string `json:"status"`
	Message  string `json:"message"`
}

type rejectActionRequest struct {
	Reason string `json:"reason"`
}

type reconcileActionRequest struct {
	Decision model.ReconciliationDecision `json:"decision"`
	Reason   string                       `json:"reason"`
}

// Approve handles:
//
// POST /api/v1/actions/{id}/approve
func (h *ActionHandler) Approve(
	w http.ResponseWriter,
	r *http.Request,
) {
	actionID, err := actionIDFromPath(r)
	if err != nil {
		http.Error(
			w,
			"invalid action id",
			http.StatusBadRequest,
		)
		return
	}

	err = h.service.Approve(
		r.Context(),
		actionID,
		developmentActorID,
	)

	if err != nil {
		http.Error(
			w,
			err.Error(),
			http.StatusConflict,
		)
		return
	}

	writeActionResponse(
		w,
		http.StatusOK,
		actionResponse{
			ActionID: actionID,
			Status:   "APPROVED",
			Message:  "Action approved successfully",
		},
	)
}

// Reject handles:
//
// POST /api/v1/actions/{id}/reject
func (h *ActionHandler) Reject(
	w http.ResponseWriter,
	r *http.Request,
) {
	actionID, err := actionIDFromPath(r)
	if err != nil {
		http.Error(
			w,
			"invalid action id",
			http.StatusBadRequest,
		)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var request rejectActionRequest

	if err := decoder.Decode(&request); err != nil {
		http.Error(
			w,
			"invalid rejection request body",
			http.StatusBadRequest,
		)
		return
	}

	// Reject additional JSON values after the first object.
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		http.Error(
			w,
			"request must contain only one JSON object",
			http.StatusBadRequest,
		)
		return
	}
	reason := strings.TrimSpace(request.Reason)

	if reason == "" {
		http.Error(
			w,
			"rejection reason is required",
			http.StatusBadRequest,
		)
		return
	}

	if !utf8.ValidString(reason) || utf8.RuneCountInString(reason) > 2000 {
		http.Error(
			w,
			"rejection reason must be valid UTF-8 and at most 2000 characters",
			http.StatusBadRequest,
		)
		return
	}

	err = h.service.Reject(
		r.Context(),
		actionID,
		developmentActorID,
		reason,
	)

	if err != nil {
		http.Error(
			w,
			err.Error(),
			http.StatusConflict,
		)
		return
	}

	writeActionResponse(
		w,
		http.StatusOK,
		actionResponse{
			ActionID: actionID,
			Status:   "REJECTED",
			Message:  "Action rejected successfully",
		},
	)
}

func actionIDFromPath(r *http.Request) (int64, error) {
	actionID, err := strconv.ParseInt(
		r.PathValue("id"),
		10,
		64,
	)

	if err != nil || actionID <= 0 {
		return 0, strconv.ErrSyntax
	}

	return actionID, nil
}

func writeActionResponse(
	w http.ResponseWriter,
	statusCode int,
	response actionResponse,
) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	_ = json.NewEncoder(w).Encode(response)
}

// Reconcile handles:
//
// POST /api/v1/actions/{id}/reconcile
func (h *ActionHandler) Reconcile(
	w http.ResponseWriter,
	r *http.Request,
) {
	actionID, err := actionIDFromPath(r)
	if err != nil {
		http.Error(w, "invalid action id", http.StatusBadRequest)
		return
	}

	// Limit request size.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid or oversized request body", http.StatusBadRequest)
		return
	}

	if !utf8.Valid(body) {
		http.Error(w, "request body must contain valid UTF-8", http.StatusBadRequest)
		return
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()

	var request reconcileActionRequest

	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid reconciliation request body", http.StatusBadRequest)
		return
	}

	// Reject additional JSON values after the first object.
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		http.Error(w, "request must contain only one JSON object", http.StatusBadRequest)
		return
	}

	request.Reason = strings.TrimSpace(request.Reason)

	if request.Reason == "" {
		http.Error(w, "reconciliation reason is required", http.StatusBadRequest)
		return
	}

	if utf8.RuneCountInString(request.Reason) > 2000 {
		http.Error(w, "reconciliation reason cannot exceed 2000 characters", http.StatusBadRequest)
		return
	}

	switch request.Decision {
	case model.ReconciliationConfirmedSuccess,
		model.ReconciliationNotPerformed,
		model.ReconciliationUnknown:
		// Valid decision.
	default:
		http.Error(w, "unsupported reconciliation decision", http.StatusBadRequest)
		return
	}

	action, err := h.service.ResolveReconciliation(
		r.Context(),
		actionID,
		developmentActorID,
		request.Decision,
		request.Reason,
	)

	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			http.Error(w, "action not found", http.StatusNotFound)

		case errors.Is(err, repository.ErrActionNotReconciliationRequired):
			http.Error(w, "action is not awaiting reconciliation", http.StatusConflict)

		default:
			http.Error(w, "failed to reconcile action", http.StatusInternalServerError)
		}

		return
	}

	writeActionResponse(
		w,
		http.StatusOK,
		actionResponse{
			ActionID: action.ID,
			Status:   string(action.Status),
			Message:  "Action reconciliation recorded successfully",
		},
	)
}
