package httpapi

import (
	"net/http"

	"example.com/paleo-prep-station/internal/contracts"
)

func (api *API) claimPlan(w http.ResponseWriter, r *http.Request) {
	var request contracts.ClaimPlanRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := api.service.ClaimPlan(r.Context(), r.PathValue("id"), request)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (api *API) reassignPlan(w http.ResponseWriter, r *http.Request) {
	var request contracts.ReassignPlanRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := api.service.ReassignPlan(r.Context(), r.PathValue("id"), request)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
