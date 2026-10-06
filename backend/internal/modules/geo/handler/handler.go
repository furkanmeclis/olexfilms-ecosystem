// Package handler serves the TEC-84 geography endpoints: the country >
// province > district pickers, distributor territories (K5) and licence
// plate formats.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Handler serves geo endpoints.
type Handler struct {
	svc      *geo.Service
	q        *db.Queries
	activity *activity.Recorder
}

// New creates the handler. rec may be nil.
func New(svc *geo.Service, q *db.Queries, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, q: q, activity: rec}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var conflict *geo.ConflictError
	switch {
	case errors.As(err, &conflict):
		details := make([]response.Detail, 0, len(conflict.Conflicts))
		for _, c := range conflict.Conflicts {
			details = append(details, response.Detail{
				Field: "territory:" + c.UUID.String(),
				Message: c.Level + " territory of " + c.OrganizationName +
					" (" + c.OrganizationUUID.String() + ")",
				Code: "territory_conflict",
			})
		}
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeTerritoryConflict,
			"The area or an overlapping area already belongs to a distributor", details)
	case errors.Is(err, geo.ErrInvalidPlate):
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, response.CodeInvalidPlate,
			"The plate does not match the format of its country",
			[]response.Detail{{Field: "plate", Message: "plate does not match the country format", Code: "invalid_plate"}})
	case errors.Is(err, geo.ErrNotFound):
		response.NotFound(w, r, err.Error())
	case errors.Is(err, geo.ErrNotDistributor):
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, response.CodeValidationError,
			"Territories are assigned to distributors only",
			[]response.Detail{{Field: "distributor_uuid", Message: "organization is not a distributor", Code: "not_distributor"}})
	case errors.Is(err, geo.ErrDuplicate):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	case errors.Is(err, geo.ErrInUse):
		response.Conflict(w, r, response.CodeConflict, "The record is still referenced")
	case errors.Is(err, geo.ErrInvalid):
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, response.CodeValidationError, err.Error(), nil)
	default:
		response.InternalErr(w, r, err, "geo request failed")
	}
}

func actorID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func (h *Handler) record(r *http.Request, action, resource string, id *uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	var actor *int64
	if v := actorID(r); v != 0 {
		actor = &v
	}
	h.activity.Record(r.Context(), actor, action, resource, id, payload, r)
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(w, r, response.CodeValidationError, name+" is invalid")
		return 0, false
	}
	return id, true
}

func queryID(w http.ResponseWriter, r *http.Request, name string) (*int64, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return nil, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(w, r, response.CodeValidationError, name+" is invalid")
		return nil, false
	}
	return &id, true
}

func requestBrand(w http.ResponseWriter, r *http.Request) (brandctx.Brand, bool) {
	b, ok := brandctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("brand unresolved"), "Request brand could not be resolved")
		return brandctx.Brand{}, false
	}
	return b, true
}

// --- Pickers (every signed-in user) ----------------------------------------

// Countries lists countries (GET /v1/geo/countries; ?all=true includes inactive).
func (h *Handler) Countries(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Countries(r.Context(), r.URL.Query().Get("all") == "true")
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// PublicCountries lists the active countries (GET /v1/public/geo/countries);
// unlike Countries it never includes the hidden ones.
func (h *Handler) PublicCountries(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Countries(r.Context(), false)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Provinces lists a country's provinces (GET /v1/geo/countries/{iso2}/provinces).
func (h *Handler) Provinces(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Provinces(r.Context(), r.PathValue("iso2"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Districts lists a province's districts (GET /v1/geo/provinces/{id}/districts).
func (h *Handler) Districts(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	items, err := h.svc.Districts(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// --- Geography writes (platform.geo.write) ---------------------------------

type countryPatch struct {
	IsActive *bool `json:"is_active"`
}

// PatchCountry shows or hides a country (PATCH /v1/platform/geo/countries/{iso2}).
func (h *Handler) PatchCountry(w http.ResponseWriter, r *http.Request) {
	var in countryPatch
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.IsActive == nil {
		response.BadRequest(w, r, response.CodeValidationError, "is_active is required")
		return
	}
	c, err := h.svc.SetCountryActive(r.Context(), r.PathValue("iso2"), *in.IsActive)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "geo.country.updated", "geo", nil, map[string]any{"iso2": c.ISO2, "is_active": c.IsActive})
	response.JSON(w, r, http.StatusOK, c)
}

type nodeBody struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// CreateProvince adds a province (POST /v1/platform/geo/countries/{iso2}/provinces).
func (h *Handler) CreateProvince(w http.ResponseWriter, r *http.Request) {
	var in nodeBody
	if !decodeJSON(w, r, &in) {
		return
	}
	p, err := h.svc.CreateProvince(r.Context(), r.PathValue("iso2"), in.Code, in.Name)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "geo.province.created", "geo", nil, map[string]any{"id": p.ID, "code": p.Code, "name": p.Name})
	response.JSON(w, r, http.StatusCreated, p)
}

// DeleteProvince removes a province (DELETE /v1/platform/geo/provinces/{id}).
func (h *Handler) DeleteProvince(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteProvince(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "geo.province.deleted", "geo", nil, map[string]any{"id": id})
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// CreateDistrict adds a district (POST /v1/platform/geo/provinces/{id}/districts).
func (h *Handler) CreateDistrict(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in nodeBody
	if !decodeJSON(w, r, &in) {
		return
	}
	d, err := h.svc.CreateDistrict(r.Context(), id, in.Code, in.Name)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "geo.district.created", "geo", nil, map[string]any{"id": d.ID, "province_id": id, "name": d.Name})
	response.JSON(w, r, http.StatusCreated, d)
}

// DeleteDistrict removes a district (DELETE /v1/platform/geo/districts/{id}).
func (h *Handler) DeleteDistrict(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteDistrict(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "geo.district.deleted", "geo", nil, map[string]any{"id": id})
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// --- Territories (platform.territories.*) ----------------------------------

func (h *Handler) brandOrg(w http.ResponseWriter, r *http.Request, brand brandctx.Brand, raw string, field string) (db.Organization, bool) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, field+" is invalid")
		return db.Organization{}, false
	}
	org, err := h.q.GetOrganizationByUUID(r.Context(), id)
	if err != nil || org.BrandID != brand.ID {
		if err == nil || errors.Is(err, pgx.ErrNoRows) {
			response.NotFound(w, r, "Organization was not found")
		} else {
			response.InternalErr(w, r, err, "organization lookup failed")
		}
		return db.Organization{}, false
	}
	return org, true
}

// ListTerritories lists the brand's territories
// (GET /v1/platform/territories?distributor_uuid=).
func (h *Handler) ListTerritories(w http.ResponseWriter, r *http.Request) {
	brand, ok := requestBrand(w, r)
	if !ok {
		return
	}
	var distributor *int64
	if raw := r.URL.Query().Get("distributor_uuid"); strings.TrimSpace(raw) != "" {
		org, ok := h.brandOrg(w, r, brand, raw, "distributor_uuid")
		if !ok {
			return
		}
		distributor = &org.ID
	}
	items, err := h.svc.Territories(r.Context(), brand.ID, distributor)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type assignBody struct {
	DistributorUUID string `json:"distributor_uuid"`
	CountryID       int64  `json:"country_id"`
	ProvinceID      *int64 `json:"province_id"`
	DistrictID      *int64 `json:"district_id"`
}

// AssignTerritory gives an area to a distributor (POST /v1/platform/territories).
// Overlaps answer 409 TERRITORY_CONFLICT with the blocking territories.
func (h *Handler) AssignTerritory(w http.ResponseWriter, r *http.Request) {
	brand, ok := requestBrand(w, r)
	if !ok {
		return
	}
	var in assignBody
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.CountryID <= 0 {
		response.BadRequest(w, r, response.CodeValidationError, "country_id is required")
		return
	}
	org, ok := h.brandOrg(w, r, brand, in.DistributorUUID, "distributor_uuid")
	if !ok {
		return
	}
	t, err := h.svc.Assign(r.Context(), geo.AssignInput{
		BrandID: brand.ID, DistributorID: org.ID, CountryID: in.CountryID,
		ProvinceID: in.ProvinceID, DistrictID: in.DistrictID, ActorID: actorID(r),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "territories.assigned", "territories", &t.UUID, map[string]any{
		"distributor_uuid": org.Uuid.String(), "level": t.Level, "country": t.CountryISO2,
		"province_id": t.ProvinceID, "district_id": t.DistrictID,
	})
	response.JSON(w, r, http.StatusCreated, t)
}

// DeleteTerritory removes a territory (DELETE /v1/platform/territories/{uuid}).
func (h *Handler) DeleteTerritory(w http.ResponseWriter, r *http.Request) {
	brand, ok := requestBrand(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "territory uuid is invalid")
		return
	}
	if err := h.svc.DeleteTerritory(r.Context(), brand.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "territories.removed", "territories", &id, nil)
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// ResolveTerritory answers which distributor covers an address
// (GET /v1/platform/territories/resolve?country_id&province_id&district_id).
func (h *Handler) ResolveTerritory(w http.ResponseWriter, r *http.Request) {
	brand, ok := requestBrand(w, r)
	if !ok {
		return
	}
	country, ok := queryID(w, r, "country_id")
	if !ok {
		return
	}
	if country == nil {
		response.BadRequest(w, r, response.CodeValidationError, "country_id is required")
		return
	}
	province, ok := queryID(w, r, "province_id")
	if !ok {
		return
	}
	district, ok := queryID(w, r, "district_id")
	if !ok {
		return
	}
	if _, err := h.svc.ValidateAddress(r.Context(), *country, province, district); err != nil {
		writeError(w, r, err)
		return
	}
	m, err := h.svc.ResolveDistributor(r.Context(), brand.ID, *country, province, district)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"match": m})
}

// --- Plate formats ---------------------------------------------------------

// PlateFormats lists active formats for plate inputs (GET /v1/plate-formats).
func (h *Handler) PlateFormats(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.PlateFormats(r.Context(), false)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type validateBody struct {
	Country string `json:"country"`
	Plate   string `json:"plate"`
}

// ValidatePlate checks a plate against its plate country
// (POST /v1/plate-formats/validate). The plate country is independent of the
// customer: a German plate on a Turkish customer validates against DE.
// A mismatch answers 422 INVALID_PLATE.
func (h *Handler) ValidatePlate(w http.ResponseWriter, r *http.Request) {
	var in validateBody
	if !decodeJSON(w, r, &in) {
		return
	}
	check, err := h.svc.ValidatePlate(r.Context(), in.Country, in.Plate)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, check)
}

// PlatformPlateFormats lists every format (GET /v1/platform/plate-formats).
func (h *Handler) PlatformPlateFormats(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.PlateFormats(r.Context(), true)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type plateBody struct {
	Country         string  `json:"country"`
	Regex           *string `json:"regex"`
	InputMask       *string `json:"input_mask"`
	Example         *string `json:"example"`
	CountryLabel    *string `json:"country_label"`
	StripColor      *string `json:"strip_color"`
	BackgroundColor *string `json:"background_color"`
	TextColor       *string `json:"text_color"`
	IsActive        *bool   `json:"is_active"`
	SortOrder       *int32  `json:"sort_order"`
}

func (b plateBody) input() geo.PlateFormatInput {
	return geo.PlateFormatInput{
		Regex: b.Regex, InputMask: b.InputMask, Example: b.Example, CountryLabel: b.CountryLabel,
		StripColor: b.StripColor, BackgroundColor: b.BackgroundColor, TextColor: b.TextColor,
		IsActive: b.IsActive, SortOrder: b.SortOrder,
	}
}

// CreatePlateFormat adds a country's format (POST /v1/platform/plate-formats).
func (h *Handler) CreatePlateFormat(w http.ResponseWriter, r *http.Request) {
	var in plateBody
	if !decodeJSON(w, r, &in) {
		return
	}
	f, err := h.svc.CreatePlateFormat(r.Context(), in.Country, in.input())
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "plate_formats.created", "plate_formats", nil, map[string]any{"country": f.CountryISO2})
	response.JSON(w, r, http.StatusCreated, f)
}

// UpdatePlateFormat patches a format (PATCH /v1/platform/plate-formats/{iso2}).
func (h *Handler) UpdatePlateFormat(w http.ResponseWriter, r *http.Request) {
	var in plateBody
	if !decodeJSON(w, r, &in) {
		return
	}
	f, err := h.svc.UpdatePlateFormat(r.Context(), r.PathValue("iso2"), in.input())
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "plate_formats.updated", "plate_formats", nil, map[string]any{"country": f.CountryISO2})
	response.JSON(w, r, http.StatusOK, f)
}

type plateOrderBody struct {
	Countries []string `json:"countries"`
}

// ReorderPlateFormats sets the display order of the formats from a
// drag-and-drop list of ISO2 codes (PUT /v1/platform/plate-formats/order,
// TEC-367). The list may be a subset; the full, renumbered list returns.
func (h *Handler) ReorderPlateFormats(w http.ResponseWriter, r *http.Request) {
	var in plateOrderBody
	if !decodeJSON(w, r, &in) {
		return
	}
	if len(in.Countries) == 0 {
		response.ValidationError(w, r, []response.Detail{{Field: "countries", Message: "must not be empty", Code: "required"}})
		return
	}
	seen := make(map[string]bool, len(in.Countries))
	codes := make([]string, 0, len(in.Countries))
	for _, raw := range in.Countries {
		code, err := geo.NormalizeISO2(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "countries", Message: "invalid country " + strconv.Quote(raw), Code: "invalid"}})
			return
		}
		if seen[code] {
			response.ValidationError(w, r, []response.Detail{{Field: "countries", Message: "duplicate country " + code, Code: "duplicate"}})
			return
		}
		seen[code] = true
		codes = append(codes, code)
	}
	items, err := h.svc.ReorderPlateFormats(r.Context(), codes)
	var unknown *geo.UnknownPlateFormatsError
	if errors.As(err, &unknown) {
		response.ValidationError(w, r, []response.Detail{{
			Field: "countries", Message: "no plate format for " + strings.Join(unknown.Countries, ", "), Code: "unknown",
		}})
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "plate_formats.reordered", "plate_formats", nil, map[string]any{"countries": codes})
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// DeletePlateFormat removes a format (DELETE /v1/platform/plate-formats/{iso2}).
func (h *Handler) DeletePlateFormat(w http.ResponseWriter, r *http.Request) {
	iso2 := r.PathValue("iso2")
	if err := h.svc.DeletePlateFormat(r.Context(), iso2); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "plate_formats.deleted", "plate_formats", nil, map[string]any{"country": strings.ToUpper(iso2)})
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}
