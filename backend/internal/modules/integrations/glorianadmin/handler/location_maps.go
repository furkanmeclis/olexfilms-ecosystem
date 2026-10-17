package handler

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/glorianadmin/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// CodeRemoteTaken is the 409 code of a remote location mapped twice.
const CodeRemoteTaken = "GLORIAN_REMOTE_LOCATION_TAKEN"

const (
	activityMapCreated = "integrations.glorian.location_map_created"
	activityMapUpdated = "integrations.glorian.location_map_updated"
	activityMapDeleted = "integrations.glorian.location_map_deleted"
)

func pathID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.NotFound(w, r, "Not found")
		return uuid.Nil, false
	}
	return id, true
}

// ListLocationMaps (GET .../location-maps?q=&sort=&limit=&offset=).
func (h *Handler) ListLocationMaps(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianView)
	if !ok {
		return
	}
	limit, offset, ok := pageParams(w, r)
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	srt, err := apiquery.ResolveSort(q.Sort, usecase.LocationMapsSortSpec)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.svc.ListLocationMaps(r.Context(), b, usecase.LocationMapFilter{
		Q: q.Q, SortKey: srt.Key, SortDesc: srt.Desc, Limit: limit, Offset: offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, int32(limit), int32(offset)))
}

// CreateLocationMap (POST .../location-maps).
func (h *Handler) CreateLocationMap(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianManage)
	if !ok {
		return
	}
	var in usecase.LocationMapInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.CreateLocationMap(r.Context(), b, in)
	if err != nil {
		writeMapError(w, r, err)
		return
	}
	h.record(r, activityMapCreated, &v.UUID, map[string]any{"remote_location_code": v.RemoteLocationCode})
	response.JSON(w, r, http.StatusCreated, v)
}

// UpdateLocationMap (PUT .../location-maps/{uuid}).
func (h *Handler) UpdateLocationMap(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianManage)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "uuid")
	if !ok {
		return
	}
	var in usecase.LocationMapInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.UpdateLocationMap(r.Context(), b, id, in)
	if err != nil {
		writeMapError(w, r, err)
		return
	}
	h.record(r, activityMapUpdated, &v.UUID, map[string]any{"remote_location_code": v.RemoteLocationCode})
	response.JSON(w, r, http.StatusOK, v)
}

// DeleteLocationMap (DELETE .../location-maps/{uuid}).
func (h *Handler) DeleteLocationMap(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianManage)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteLocationMap(r.Context(), b, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, activityMapDeleted, &id, nil)
	w.WriteHeader(http.StatusNoContent)
}

// MapWarehouses (GET .../location-maps/warehouses): picker options.
func (h *Handler) MapWarehouses(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianView)
	if !ok {
		return
	}
	items, err := h.svc.MapWarehouses(r.Context(), b)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// MapLocations (GET .../location-maps/warehouses/{uuid}/locations?q=).
func (h *Handler) MapLocations(w http.ResponseWriter, r *http.Request) {
	b, ok := h.brand(w, r, rbac.PermIntegrationsGlorianView)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "uuid")
	if !ok {
		return
	}
	items, err := h.svc.MapLocations(r.Context(), b, id, r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func writeMapError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, usecase.ErrRemoteTaken) {
		response.Conflict(w, r, CodeRemoteTaken, "This remote location is already mapped")
		return
	}
	writeError(w, r, err)
}
