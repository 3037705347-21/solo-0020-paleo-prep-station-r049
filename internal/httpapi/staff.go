package httpapi

import "net/http"

func (api *API) listStaff(w http.ResponseWriter, r *http.Request) {
	values, err := api.service.ListStaff(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}
