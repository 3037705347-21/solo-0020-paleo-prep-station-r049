package httpapi

import (
	"net/http"

	"example.com/paleo-prep-station/internal/contracts"
)

func (api *API) registerStaff(w http.ResponseWriter, r *http.Request) {
	var request contracts.RegisterStaffRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := api.service.RegisterStaff(r.Context(), request)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Location", "/staff/"+value.ID)
	writeJSON(w, http.StatusCreated, value)
}

func (api *API) listStaff(w http.ResponseWriter, r *http.Request) {
	values, err := api.service.ListStaff(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (api *API) getStaff(w http.ResponseWriter, r *http.Request) {
	value, err := api.service.GetStaffWorkload(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (api *API) getWorkbench(w http.ResponseWriter, r *http.Request) {
	values, err := api.service.GetWorkbench(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}
