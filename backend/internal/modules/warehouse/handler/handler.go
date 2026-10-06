// Package handler serves /v1/warehouse (TEC-201): warehouses, rooms and
// typed locations of the active organization.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the warehouse endpoints.
const (
	CodeWarehouseCodeTaken = "WAREHOUSE_CODE_TAKEN"
	CodeWarehouseInUse     = "WAREHOUSE_IN_USE"
)

// Handler serves the warehouse routes.
type Handler struct {
	svc *wh.Service
}

// New builds the handler.
func New(svc *wh.Service) *Handler { return &Handler{svc: svc} }

type listResponse[T any] struct {
	Items []T `json:"items"`
}

func caller(r *http.Request) wh.Caller {
	f, _ := scopefilter.From(r.Context())
	return wh.Caller{Org: orgctx.MustScope(r.Context()), Filter: f}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *wh.ValidationError
	var qe *apiquery.ValidationError
	switch {
	case errors.As(err, &qe):
		// TEC-375: list query parameters (sort, CSV enums, ranges).
		details := make([]response.Detail, 0, len(qe.Details))
		for _, d := range qe.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.Is(err, wh.ErrForbidden):
		response.Forbidden(w, r, "The warehouse module is available to the center and distributors only")
	case errors.Is(err, wh.ErrWarehouseNotFound):
		response.NotFound(w, r, "Warehouse not found")
	case errors.Is(err, wh.ErrRoomNotFound):
		response.NotFound(w, r, "Room not found")
	case errors.Is(err, wh.ErrLocationNotFound):
		response.NotFound(w, r, "Location not found")
	case errors.Is(err, wh.ErrCodeTaken):
		response.Conflict(w, r, CodeWarehouseCodeTaken, "This code is already used at this level")
	case errors.Is(err, wh.ErrInUse):
		response.Conflict(w, r, CodeWarehouseInUse, "Still in use: remove its children or stock first, or deactivate it")
	default:
		response.InternalErr(w, r, err, "warehouse request failed")
	}
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Resource not found")
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

type reorderBody struct {
	UUIDs []uuid.UUID `json:"uuids"`
}

// ---------------------------------------------------------------------------
// Warehouses.

type warehouseBody struct {
	Code    string  `json:"code"`
	Name    string  `json:"name"`
	Address *string `json:"address"`
}

type warehousePatchBody struct {
	Code    *string `json:"code"`
	Name    *string `json:"name"`
	Address *string `json:"address"`
	Active  *bool   `json:"active"`
}

// ListWarehouses (GET /v1/warehouse/warehouses?active).
func (h *Handler) ListWarehouses(w http.ResponseWriter, r *http.Request) {
	var active *bool
	if raw := r.URL.Query().Get("active"); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "active", Message: "must be true or false"}})
			return
		}
		active = &v
	}
	items, err := h.svc.ListWarehouses(r.Context(), caller(r), active)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, listResponse[wh.Warehouse]{Items: items})
}

// CreateWarehouse (POST /v1/warehouse/warehouses).
func (h *Handler) CreateWarehouse(w http.ResponseWriter, r *http.Request) {
	var b warehouseBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.CreateWarehouse(r.Context(), caller(r), wh.WarehouseInput{Code: b.Code, Name: b.Name, Address: b.Address})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// GetWarehouse (GET /v1/warehouse/warehouses/{uuid}).
func (h *Handler) GetWarehouse(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.GetWarehouse(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// UpdateWarehouse (PATCH /v1/warehouse/warehouses/{uuid}).
func (h *Handler) UpdateWarehouse(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b warehousePatchBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.UpdateWarehouse(r.Context(), caller(r), id, wh.WarehousePatch{
		Code: b.Code, Name: b.Name, Address: b.Address, Active: b.Active,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// DeleteWarehouse (DELETE /v1/warehouse/warehouses/{uuid}).
func (h *Handler) DeleteWarehouse(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteWarehouse(r.Context(), caller(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ReorderWarehouses (POST /v1/warehouse/warehouses/reorder).
func (h *Handler) ReorderWarehouses(w http.ResponseWriter, r *http.Request) {
	var b reorderBody
	if !decode(w, r, &b) {
		return
	}
	if err := h.svc.ReorderWarehouses(r.Context(), caller(r), b.UUIDs); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Rooms.

type roomBody struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type roomPatchBody struct {
	Code   *string `json:"code"`
	Name   *string `json:"name"`
	Active *bool   `json:"active"`
}

// ListRooms (GET /v1/warehouse/warehouses/{uuid}/rooms).
func (h *Handler) ListRooms(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ListRooms(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, listResponse[wh.Room]{Items: items})
}

// CreateRoom (POST /v1/warehouse/warehouses/{uuid}/rooms).
func (h *Handler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b roomBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.CreateRoom(r.Context(), caller(r), id, wh.RoomInput{Code: b.Code, Name: b.Name})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// GetRoom (GET /v1/warehouse/rooms/{uuid}).
func (h *Handler) GetRoom(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.GetRoom(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// UpdateRoom (PATCH /v1/warehouse/rooms/{uuid}).
func (h *Handler) UpdateRoom(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b roomPatchBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.UpdateRoom(r.Context(), caller(r), id, wh.RoomPatch{Code: b.Code, Name: b.Name, Active: b.Active})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// DeleteRoom (DELETE /v1/warehouse/rooms/{uuid}).
func (h *Handler) DeleteRoom(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteRoom(r.Context(), caller(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ReorderRooms (POST /v1/warehouse/rooms/reorder).
func (h *Handler) ReorderRooms(w http.ResponseWriter, r *http.Request) {
	var b reorderBody
	if !decode(w, r, &b) {
		return
	}
	if err := h.svc.ReorderRooms(r.Context(), caller(r), b.UUIDs); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Locations.

type locationBody struct {
	RoomUUID   uuid.UUID  `json:"room_uuid"`
	ParentUUID *uuid.UUID `json:"parent_uuid"`
	Type       string     `json:"type"`
	Code       string     `json:"code"`
	Name       string     `json:"name"`
}

type locationPatchBody struct {
	Code   *string `json:"code"`
	Name   *string `json:"name"`
	Active *bool   `json:"active"`
}

type generateLevelBody struct {
	Type   string   `json:"type"`
	Codes  []string `json:"codes"`
	From   *int     `json:"from"`
	To     *int     `json:"to"`
	Pad    int      `json:"pad"`
	Prefix string   `json:"prefix"`
}

type generateBody struct {
	RoomUUID   uuid.UUID           `json:"room_uuid"`
	ParentUUID *uuid.UUID          `json:"parent_uuid"`
	Levels     []generateLevelBody `json:"levels"`
}

// ListRoomLocations (GET /v1/warehouse/rooms/{uuid}/locations).
func (h *Handler) ListRoomLocations(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ListRoomLocations(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, listResponse[wh.Location]{Items: items})
}

// CreateLocation (POST /v1/warehouse/locations).
func (h *Handler) CreateLocation(w http.ResponseWriter, r *http.Request) {
	var b locationBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.CreateLocation(r.Context(), caller(r), wh.LocationInput{
		RoomUUID: b.RoomUUID, ParentUUID: b.ParentUUID, Type: b.Type, Code: b.Code, Name: b.Name,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// GetLocation (GET /v1/warehouse/locations/{uuid}).
func (h *Handler) GetLocation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.GetLocation(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// UpdateLocation (PATCH /v1/warehouse/locations/{uuid}).
func (h *Handler) UpdateLocation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b locationPatchBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.UpdateLocation(r.Context(), caller(r), id, wh.LocationPatch{Code: b.Code, Name: b.Name, Active: b.Active})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// DeleteLocation (DELETE /v1/warehouse/locations/{uuid}).
func (h *Handler) DeleteLocation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteLocation(r.Context(), caller(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ReorderLocations (POST /v1/warehouse/locations/reorder).
func (h *Handler) ReorderLocations(w http.ResponseWriter, r *http.Request) {
	var b reorderBody
	if !decode(w, r, &b) {
		return
	}
	if err := h.svc.ReorderLocations(r.Context(), caller(r), b.UUIDs); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GenerateLocations (POST /v1/warehouse/locations/generate).
func (h *Handler) GenerateLocations(w http.ResponseWriter, r *http.Request) {
	var b generateBody
	if !decode(w, r, &b) {
		return
	}
	in := wh.GenerateInput{RoomUUID: b.RoomUUID, ParentUUID: b.ParentUUID}
	for _, lv := range b.Levels {
		in.Levels = append(in.Levels, wh.GenerateLevel{
			Type: lv.Type, Codes: lv.Codes, From: lv.From, To: lv.To, Pad: lv.Pad, Prefix: lv.Prefix,
		})
	}
	out, err := h.svc.GenerateLocations(r.Context(), caller(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}
