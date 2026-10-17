package handler

import (
	"net/http"

	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Members serves the TEC-769 warehouse membership routes.
type Members struct {
	svc *wh.Members
}

// NewMembers builds the handler.
func NewMembers(svc *wh.Members) *Members { return &Members{svc: svc} }

type memberBody struct {
	UserUUID  string `json:"user_uuid"`
	IsDefault bool   `json:"is_default"`
}

type defaultBody struct {
	WarehouseUUID string `json:"warehouse_uuid"`
}

// List (GET /v1/warehouse/warehouses/{uuid}/members?q&sort&limit&offset).
func (h *Members) List(w http.ResponseWriter, r *http.Request) {
	f, err := wh.ParseMemberListFilter(r.URL.Query())
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.svc.List(r.Context(), caller(r), r.PathValue("uuid"), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// Add (POST /v1/warehouse/warehouses/{uuid}/members).
func (h *Members) Add(w http.ResponseWriter, r *http.Request) {
	var b memberBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.Add(r.Context(), entryCaller(r), r.PathValue("uuid"), wh.MemberInput{UserUUID: b.UserUUID, IsDefault: b.IsDefault})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Remove (DELETE /v1/warehouse/warehouses/{uuid}/members/{user_uuid}).
func (h *Members) Remove(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Remove(r.Context(), entryCaller(r), r.PathValue("uuid"), r.PathValue("user_uuid")); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Mine (GET /v1/warehouse/my-warehouses).
func (h *Members) Mine(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Mine(r.Context(), entryCaller(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// SetDefault (PUT /v1/warehouse/my-warehouses/default).
func (h *Members) SetDefault(w http.ResponseWriter, r *http.Request) {
	var b defaultBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.SetDefault(r.Context(), entryCaller(r), b.WarehouseUUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Candidates (GET /v1/warehouse/member-candidates?q): the member picker.
func (h *Members) Candidates(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Candidates(r.Context(), caller(r), r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, listResponse[wh.MemberUser]{Items: items})
}
