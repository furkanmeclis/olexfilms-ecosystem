package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/svg"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// PartMap is GET /v1/measurement-part-maps/{body_type} (TEC-299): the raw
// (unfilled) NexPTG part map of a body type so the panel renders and
// colors it itself. The assets are embedded and immutable, so the answer
// is cacheable.
func (h *Handler) PartMap(w http.ResponseWriter, r *http.Request) {
	out, err := svg.PartMap(strings.TrimSpace(r.PathValue("body_type")))
	switch {
	case errors.Is(err, svg.ErrBodyTypeUnknown):
		response.NotFound(w, r, "Measurement body type not found")
		return
	case err != nil:
		response.InternalErr(w, r, err, "measurement part map failed")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	response.JSON(w, r, http.StatusOK, out)
}
