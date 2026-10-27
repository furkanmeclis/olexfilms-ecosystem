package handler

import (
	"net/http"
	"strings"

	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

type accountBody struct {
	Name    string `json:"name"`
	Surname string `json:"surname"`
	Phone   string `json:"phone"`
	Email   string `json:"email"`
}

func (b accountBody) input() orgusecase.AccountInput {
	return orgusecase.AccountInput{Name: b.Name, Surname: b.Surname, Phone: b.Phone, Email: b.Email}
}

// TenantCreateMember adds a staff member (phone-first account) to the
// active organization: POST /v1/tenant/members (members.write).
func (h *Handler) TenantCreateMember(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	var in struct {
		accountBody
		RoleSlugs      []string `json:"role_slugs"`
		WarehouseUUIDs []string `json:"warehouse_uuids"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	warehouses := make([]uuid.UUID, 0, len(in.WarehouseUUIDs))
	for _, raw := range in.WarehouseUUIDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "warehouse_uuids must be UUIDs")
			return
		}
		warehouses = append(warehouses, id)
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	out, err := h.svc.CreateTeamMember(r.Context(), scope.InternalID, actor, orgusecase.TeamMemberInput{
		AccountInput: in.input(), RoleSlugs: in.RoleSlugs, WarehouseUUIDs: warehouses,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// TenantCreateDealer opens a dealer with its owner directly under the
// active organization: POST /v1/tenant/dealers (organizations.write).
func (h *Handler) TenantCreateDealer(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	var in struct {
		Name       string      `json:"name"`
		Phone      string      `json:"phone"`
		City       string      `json:"city"`
		District   string      `json:"district"`
		Address    string      `json:"address"`
		CountryID  *int64      `json:"country_id"`
		ProvinceID *int64      `json:"province_id"`
		DistrictID *int64      `json:"district_id"`
		Owner      accountBody `json:"owner"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	out, err := h.svc.CreateDealer(r.Context(), scope.InternalID, orgusecase.DealerInput{
		OrganizationName: in.Name, Phone: in.Phone, City: in.City, District: in.District, Address: in.Address,
		Location: orgusecase.AddressInput{CountryID: in.CountryID, ProvinceID: in.ProvinceID, DistrictID: in.DistrictID},
		Owner:    in.Owner.input(),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}
