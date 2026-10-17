package handler

import (
	"mime"
	"net/http"
	"strings"
)

// OrderSlip downloads the current order slip after the orders.read scope check.
func (h *Handler) OrderSlip(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	data, name, err := h.svc.OrderSlip(r.Context(), caller(r), id, strings.TrimSpace(r.URL.Query().Get("lang")), strings.TrimSpace(r.URL.Query().Get("tz")))
	if err != nil {
		writePickingError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		return
	}
}
