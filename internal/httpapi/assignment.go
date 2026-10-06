package httpapi

import (
	"net/http"

	"example.com/paleo-prep-station/internal/contracts"
)

func (api *API) claimAssignment(w http.ResponseWriter, r *http.Request) {
	var request contracts.ClaimAssignmentRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := api.service.ClaimAssignment(r.Context(), r.PathValue("id"), request)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Location", "/plans/"+value.PlanID+"/assignments")
	writeJSON(w, http.StatusCreated, value)
}

func (api *API) transferAssignment(w http.ResponseWriter, r *http.Request) {
	var request contracts.TransferAssignmentRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := api.service.TransferAssignment(r.Context(), r.PathValue("id"), request)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Location", "/plans/"+value.PlanID+"/assignments")
	writeJSON(w, http.StatusCreated, value)
}

func (api *API) listAssignments(w http.ResponseWriter, r *http.Request) {
	values, err := api.service.ListAssignments(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}
