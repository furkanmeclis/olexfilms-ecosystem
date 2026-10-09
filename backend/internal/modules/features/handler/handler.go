// Package handler serves the module (feature flag) endpoints of TEC-86:
// the "Özellikler" page of every organization, the distributor's dealer
// settings and the platform admin's module catalog.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	notifcatalog "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Notifier dispatches the "request a module" notification center event
// (catalog event features.module_requested, templates per role/language).
type Notifier interface {
	Dispatch(ctx context.Context, in notifmodel.DispatchInput) (notifmodel.DispatchResult, error)
}

// Handler serves module endpoints.
type Handler struct {
	svc      *features.Service
	q        *db.Queries
	notifier Notifier
	activity *activity.Recorder
	log      *slog.Logger
}

// New creates the handler. notifier and rec may be nil.
func New(svc *features.Service, q *db.Queries, notifier Notifier, rec *activity.Recorder, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{svc: svc, q: q, notifier: notifier, activity: rec, log: log}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return err
	}
	return nil
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *apiquery.ValidationError
	switch {
	case errors.As(err, &ve):
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
	case errors.Is(err, features.ErrUnknownModule):
		response.NotFound(w, r, "Module was not found")
	case errors.Is(err, features.ErrOrganizationNotFound):
		response.NotFound(w, r, "Organization was not found")
	case errors.Is(err, features.ErrCoreModule):
		response.Error(w, r, http.StatusUnprocessableEntity, response.CodeModuleCore, "Core modules cannot be switched off")
	case errors.Is(err, features.ErrUpstreamDisabled):
		response.Error(w, r, http.StatusConflict, response.CodeFeatureDisabled,
			"The module is disabled at a higher level and cannot be enabled here")
	case errors.Is(err, features.ErrAdminOverride):
		response.Error(w, r, http.StatusConflict, response.CodeModuleAdminOverride,
			"The platform admin set this module for the dealer")
	case errors.Is(err, features.ErrNotDistributor):
		response.Forbidden(w, r, "Dealer module settings are available to distributors only")
	case errors.Is(err, features.ErrNotOwnDealer):
		response.Forbidden(w, r, "Every organization must be a dealer of the active distributor")
	case errors.Is(err, features.ErrModuleEnabled):
		response.Error(w, r, http.StatusConflict, response.CodeConflict, "The module is already enabled")
	case errors.Is(err, features.ErrRequestNotFound):
		response.NotFound(w, r, "Module request was not found")
	case errors.Is(err, features.ErrRequestDecided):
		response.Error(w, r, http.StatusConflict, response.CodeConflict, "The module request is no longer pending")
	default:
		response.InternalErr(w, r, err, "module request failed")
	}
}

func actorID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func (h *Handler) record(r *http.Request, action string, resource *uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	var actor *int64
	if id := actorID(r); id != 0 {
		actor = &id
	}
	h.activity.Record(r.Context(), actor, action, "modules", resource, payload, r)
}

// --- Özellikler (every organization) ---------------------------------------

type featuresResponse struct {
	OrganizationType string        `json:"organization_type"`
	Items            []featureItem `json:"items"`
	// Enabled lists the keys that are on (menus and route guards).
	Enabled []string `json:"enabled"`
}

// featureItem is a module state with the Özellikler page meta (TEC-508).
type featureItem struct {
	features.State
	// Description is the module's one-sentence description in the request
	// locale (backend i18n catalog, 13 languages).
	Description string `json:"description"`
	// FreeDefault: on by default, free, needs no service record.
	FreeDefault bool `json:"free_default"`
	// Price of the cheapest active module bundle that contains the module.
	Price *modulePrice `json:"price"`
	// ContactForPrice: a paid module with no bundle on sale.
	ContactForPrice bool `json:"contact_for_price"`
	// Request is the organization's newest request of the module.
	Request *requestSummary `json:"request"`
}

type modulePrice struct {
	Amount     string    `json:"amount"`
	Currency   string    `json:"currency"`
	Recurrence string    `json:"recurrence"`
	ItemUUID   uuid.UUID `json:"item_uuid"`
	ItemName   string    `json:"item_name"`
}

type requestSummary struct {
	UUID         uuid.UUID  `json:"uuid"`
	Status       string     `json:"status"`
	Note         string     `json:"note"`
	DecisionNote string     `json:"decision_note"`
	CreatedAt    time.Time  `json:"created_at"`
	DecidedAt    *time.Time `json:"decided_at"`
}

func summarize(r db.ModuleRequest) *requestSummary {
	out := &requestSummary{
		UUID: r.Uuid, Status: r.Status, Note: r.Note, DecisionNote: r.DecisionNote, CreatedAt: r.CreatedAt.Time,
	}
	if r.DecidedAt.Valid {
		t := r.DecidedAt.Time
		out.DecidedAt = &t
	}
	return out
}

// List returns the active organization's modules (GET /v1/features). The
// list is limited to what the level above has access to.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	states, err := h.svc.Snapshot(r.Context(), scope.InternalID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	prices, err := h.bundlePrices(r.Context(), scope)
	if err != nil {
		writeError(w, r, err)
		return
	}
	requests, err := h.svc.LatestRequests(r.Context(), scope.InternalID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	locale := i18n.FromContext(r.Context()).Locale
	out := featuresResponse{OrganizationType: scope.OrgType, Items: []featureItem{}, Enabled: []string{}}
	for _, st := range states {
		if st.Visible {
			it := featureItem{
				State: st, Description: i18n.ModuleDescription(locale, st.Key),
				FreeDefault: st.DefaultEnabled, Price: prices[st.Key],
			}
			it.ContactForPrice = st.Paid && !it.FreeDefault && it.Price == nil
			if req, ok := requests[st.Key]; ok {
				it.Request = summarize(req)
			}
			out.Items = append(out.Items, it)
		}
		if st.Enabled {
			out.Enabled = append(out.Enabled, st.Key)
		}
	}
	response.JSON(w, r, http.StatusOK, out)
}

// bundlePrices returns, per module key, the cheapest active module bundle of
// the brand's service catalog with the buyer's distributor override.
func (h *Handler) bundlePrices(ctx context.Context, scope orgctx.Scope) (map[string]*modulePrice, error) {
	org, err := h.q.GetOrganizationByID(ctx, scope.InternalID)
	if err != nil {
		return nil, err
	}
	var override pgtype.Int8
	switch {
	case org.Type == features.OrgDistributor:
		override = pgtype.Int8{Int64: org.ID, Valid: true}
	case org.Type == features.OrgDealer && org.ParentID.Valid:
		if p, err := h.q.GetOrganizationByID(ctx, org.ParentID.Int64); err == nil && p.Type == features.OrgDistributor {
			override = pgtype.Int8{Int64: p.ID, Valid: true}
		}
	}
	rows, err := h.q.ListModuleBundlePrices(ctx, db.ListModuleBundlePricesParams{BrandID: org.BrandID, OverrideOrgID: override})
	if err != nil {
		return nil, err
	}
	out := map[string]*modulePrice{}
	for _, row := range rows {
		if _, ok := out[row.ModuleKey]; ok {
			continue // rows come cheapest first per module
		}
		out[row.ModuleKey] = &modulePrice{
			Amount: numText(row.Price), Currency: row.Currency, Recurrence: row.Recurrence,
			ItemUUID: row.ItemUuid, ItemName: row.ItemName,
		}
	}
	return out, nil
}

func numText(n pgtype.Numeric) string {
	v, err := n.Value()
	if err != nil || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

type requestBody struct {
	Note string `json:"note"`
}

// Request asks the level above to switch a module on
// (POST /v1/features/{key}/request): the request is stored (one pending per
// organization x module) and the parent distributor's owners, or the
// platform admins for a distributor or a dealer under the center, are
// notified.
func (h *Handler) Request(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	key := r.PathValue("key")
	if _, ok := features.ModuleByKey(key); !ok {
		writeError(w, r, features.ErrUnknownModule)
		return
	}
	var body requestBody
	if r.ContentLength != 0 {
		if err := decodeJSON(w, r, &body); err != nil {
			return
		}
	}
	if len(body.Note) > features.MaxRequestNote {
		response.BadRequest(w, r, response.CodeValidationError, "note is too long")
		return
	}
	req, err := h.svc.RequestModule(r.Context(), features.RequestInput{
		OrgID: scope.InternalID, BrandID: scope.BrandID, ActorID: actorID(r), Key: key, Note: body.Note,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	recipients, err := h.upstreamRecipients(r.Context(), scope.InternalID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.notifier != nil && len(recipients) > 0 {
		orgID, brandID := scope.InternalID, scope.BrandID
		if _, err := h.notifier.Dispatch(r.Context(), notifmodel.DispatchInput{
			EventID: uuid.New(), EventCode: notifcatalog.EventFeaturesModuleRequest,
			OrganizationID: &orgID, BrandID: &brandID, UserIDs: recipients,
			Vars: map[string]string{
				"organization_name": scope.Name, "module_key": key, "note": body.Note,
			},
			Payload: map[string]any{
				"module_key": key, "organization_uuid": scope.UUID.String(),
				"organization_name": scope.Name, "note": body.Note, "request_uuid": req.Uuid.String(),
			},
		}); err != nil {
			h.log.Warn("feature_request_notify_failed", "recipients", len(recipients), "error", err)
		}
	}
	orgUUID := scope.UUID
	h.record(r, "modules.requested", &orgUUID, map[string]any{
		"module_key": key, "recipients": len(recipients), "request_uuid": req.Uuid.String(),
	})
	response.JSON(w, r, http.StatusAccepted, map[string]any{
		"status": "requested", "recipients": len(recipients), "request": summarize(req),
	})
}

// CancelRequest withdraws the organization's pending request of a module
// (DELETE /v1/features/{key}/request).
func (h *Handler) CancelRequest(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	key := r.PathValue("key")
	req, err := h.svc.CancelRequest(r.Context(), scope.InternalID, actorID(r), key)
	if err != nil {
		writeError(w, r, err)
		return
	}
	orgUUID := scope.UUID
	h.record(r, "modules.request.cancelled", &orgUUID, map[string]any{"module_key": key, "request_uuid": req.Uuid.String()})
	response.JSON(w, r, http.StatusOK, summarize(req))
}

func (h *Handler) upstreamRecipients(ctx context.Context, orgID int64) ([]int64, error) {
	parent, err := h.q.SupplierOf(ctx, orgID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if err == nil && parent.Type == features.OrgDistributor {
		return h.q.ListOrganizationOwnerUserIDs(ctx, parent.ID)
	}
	return h.q.ListUserIDsByRoleSlug(ctx, rbac.RoleSuperAdmin)
}

// --- Distributor ------------------------------------------------------------

type dealerModule struct {
	Key           string `json:"key"`
	Enabled       bool   `json:"enabled"`
	Visible       bool   `json:"visible"`
	Source        string `json:"source"`
	AdminOverride bool   `json:"admin_override"`
}

type dealerRow struct {
	UUID    uuid.UUID      `json:"uuid"`
	Name    string         `json:"name"`
	Slug    string         `json:"slug"`
	Modules []dealerModule `json:"modules"`
}

// DealersSortSpec is the sort whitelist of GET /v1/tenant/modules/dealers
// (TEC-367, docs/list-contract.md). The matrix is resolved in memory, so
// sorting and paging happen here, not in SQL.
var DealersSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"name": "name", "slug": "slug"},
	Default: apiquery.SortField{Field: "name"},
}

// DealerStates are the values of the dealer list state filter.
var DealerStates = []string{"enabled", "disabled"}

// DealerSources are the resolved sources a dealer module state can have.
var DealerSources = []string{
	features.FromCore, features.FromSystem, features.FromDefault, features.FromUpstream, features.FromStandard,
	features.SourceAdmin, features.SourceDistributor, features.SourceService,
}

// dealerListFilter is the parsed query of GET /v1/tenant/modules/dealers.
// State and Source apply to the Module key's state.
type dealerListFilter struct {
	Q       string
	Module  string
	States  []string
	Sources []string
}

func parseDealerFilter(qv url.Values) (dealerListFilter, error) {
	f := dealerListFilter{Q: strings.ToLower(strings.TrimSpace(qv.Get("q"))), Module: strings.TrimSpace(qv.Get("module"))}
	var err error
	if f.States, err = apiquery.EnumList(qv, "state", DealerStates...); err != nil {
		return f, err
	}
	if f.Sources, err = apiquery.EnumList(qv, "source", DealerSources...); err != nil {
		return f, err
	}
	if f.Module == "" && (len(f.States) > 0 || len(f.Sources) > 0) {
		return f, &apiquery.ValidationError{Details: []apiquery.Detail{{
			Field: "module", Message: "module is required with state or source", Code: "required",
		}}}
	}
	return f, nil
}

func (f dealerListFilter) match(d dealerRow) bool {
	if f.Q != "" && !strings.Contains(strings.ToLower(d.Name), f.Q) && !strings.Contains(strings.ToLower(d.Slug), f.Q) {
		return false
	}
	if len(f.States) == 0 && len(f.Sources) == 0 {
		return true
	}
	for _, m := range d.Modules {
		if m.Key != f.Module {
			continue
		}
		state := "disabled"
		if m.Enabled {
			state = "enabled"
		}
		return (len(f.States) == 0 || slices.Contains(f.States, state)) &&
			(len(f.Sources) == 0 || slices.Contains(f.Sources, m.Source))
	}
	return false
}

// Dealers returns the module matrix of the distributor's dealers
// (GET /v1/tenant/modules/dealers): q (name/slug), module + state/source
// filters, sort name|slug, limit/offset paging with total.
func (h *Handler) Dealers(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	f, _ := scopefilter.From(r.Context())
	if f.Scope != rbac.ScopeSubtree && f.Scope != rbac.ScopeAll {
		writeError(w, r, features.ErrNotDistributor)
		return
	}
	qv := r.URL.Query()
	q := apiquery.Parse(qv)
	sort, err := apiquery.ResolveSort(q.Sort, DealersSortSpec)
	if err != nil {
		writeError(w, r, err)
		return
	}
	lf, err := parseDealerFilter(qv)
	if err != nil {
		writeError(w, r, err)
		return
	}
	rows, err := h.svc.DealerMatrix(r.Context(), scope.InternalID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]dealerRow, 0, len(rows))
	for _, d := range rows {
		if !f.AllowsOrg(d.OrgID, scope.BrandID) {
			continue
		}
		dr := dealerRow{UUID: d.UUID, Name: d.Name, Slug: d.Slug, Modules: []dealerModule{}}
		for _, st := range d.Modules {
			if st.Level == features.LevelCore {
				continue
			}
			dr.Modules = append(dr.Modules, dealerModule{
				Key: st.Key, Enabled: st.Enabled, Visible: st.Visible, Source: st.Source, AdminOverride: st.AdminOverride,
			})
		}
		if lf.match(dr) {
			out = append(out, dr)
		}
	}
	slices.SortStableFunc(out, func(a, b dealerRow) int {
		var c int
		if sort.Key == "slug" {
			c = strings.Compare(a.Slug, b.Slug)
		} else {
			c = strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		}
		if c == 0 {
			c = strings.Compare(a.UUID.String(), b.UUID.String())
		}
		if sort.Desc {
			c = -c
		}
		return c
	})
	total := int64(len(out))
	if qv.Get("limit") == "" && qv.Get("offset") == "" {
		// Pre-TEC-367 callers read the whole matrix; without paging
		// params the page is every matching dealer.
		response.JSON(w, r, http.StatusOK, apiquery.NewPage(out, total, int32(len(out)), 0))
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.FromSlice(out, q.Limit, q.Offset))
}

// dealerIDs resolves dealer UUIDs inside the caller's modules.manage scope.
// Anything outside it (another distributor's dealer) is refused.
func (h *Handler) dealerIDs(r *http.Request, ids []uuid.UUID) ([]int64, error) {
	f, ok := scopefilter.From(r.Context())
	if !ok {
		return nil, features.ErrNotOwnDealer
	}
	out := make([]int64, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		o, err := h.q.GetOrganizationByUUID(r.Context(), id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, features.ErrNotOwnDealer
		}
		if err != nil {
			return nil, err
		}
		if !f.AllowsOrg(o.ID, o.BrandID) {
			return nil, features.ErrNotOwnDealer
		}
		if !seen[o.ID] {
			seen[o.ID] = true
			out = append(out, o.ID)
		}
	}
	return out, nil
}

type enabledBody struct {
	Enabled *bool `json:"enabled"`
}

func decodeEnabled(w http.ResponseWriter, r *http.Request) (bool, bool) {
	var body enabledBody
	if err := decodeJSON(w, r, &body); err != nil {
		return false, false
	}
	if body.Enabled == nil {
		response.BadRequest(w, r, response.CodeValidationError, "enabled is required")
		return false, false
	}
	return *body.Enabled, true
}

func parseUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, name+" is invalid")
		return uuid.Nil, false
	}
	return id, true
}

// SetDealer switches a module for one dealer
// (PUT /v1/tenant/modules/dealers/{uuid}/{key}).
func (h *Handler) SetDealer(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, r, "uuid")
	if !ok {
		return
	}
	enabled, ok := decodeEnabled(w, r)
	if !ok {
		return
	}
	h.applyDealers(w, r, []uuid.UUID{id}, r.PathValue("key"), &enabled)
}

// ClearDealer drops a dealer's own value
// (DELETE /v1/tenant/modules/dealers/{uuid}/{key}).
func (h *Handler) ClearDealer(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, r, "uuid")
	if !ok {
		return
	}
	h.applyDealers(w, r, []uuid.UUID{id}, r.PathValue("key"), nil)
}

type bulkBody struct {
	DealerUUIDs []uuid.UUID `json:"dealer_uuids"`
	Key         string      `json:"key"`
	Enabled     *bool       `json:"enabled"`
}

// BulkDealers switches a module for several dealers at once, all or nothing
// (POST /v1/tenant/modules/dealers/bulk). enabled null clears the values.
func (h *Handler) BulkDealers(w http.ResponseWriter, r *http.Request) {
	var body bulkBody
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if len(body.DealerUUIDs) == 0 || len(body.DealerUUIDs) > 500 {
		response.BadRequest(w, r, response.CodeValidationError, "dealer_uuids must hold 1 to 500 dealers")
		return
	}
	h.applyDealers(w, r, body.DealerUUIDs, body.Key, body.Enabled)
}

func (h *Handler) applyDealers(w http.ResponseWriter, r *http.Request, uuids []uuid.UUID, key string, enabled *bool) {
	scope := orgctx.MustScope(r.Context())
	ids, err := h.dealerIDs(r, uuids)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if enabled == nil {
		err = h.svc.ClearForDealers(r.Context(), scope.InternalID, ids, key)
	} else {
		err = h.svc.SetForDealers(r.Context(), actorID(r), scope.InternalID, ids, key, *enabled)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	dealers := make([]string, 0, len(uuids))
	for _, id := range uuids {
		dealers = append(dealers, id.String())
	}
	orgUUID := scope.UUID
	h.record(r, "modules.dealers.updated", &orgUUID, map[string]any{
		"module_key": key, "enabled": enabled, "dealer_uuids": dealers,
	})
	response.JSON(w, r, http.StatusOK, map[string]any{"updated": len(ids)})
}

// DealerStandard returns the distributor's dealer standard
// (GET /v1/tenant/modules/dealer-standard).
func (h *Handler) DealerStandard(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	items, err := h.svc.DealerStandard(r.Context(), scope.InternalID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// SetDealerStandard changes the dealer standard
// (PUT /v1/tenant/modules/dealer-standard/{key}).
func (h *Handler) SetDealerStandard(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	enabled, ok := decodeEnabled(w, r)
	if !ok {
		return
	}
	key := r.PathValue("key")
	if err := h.svc.SetDealerStandard(r.Context(), actorID(r), scope.InternalID, key, enabled); err != nil {
		writeError(w, r, err)
		return
	}
	orgUUID := scope.UUID
	h.record(r, "modules.dealer_standard.updated", &orgUUID, map[string]any{"module_key": key, "enabled": enabled})
	h.DealerStandard(w, r)
}

// ClearDealerStandard drops a standard value
// (DELETE /v1/tenant/modules/dealer-standard/{key}).
func (h *Handler) ClearDealerStandard(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	key := r.PathValue("key")
	if err := h.svc.ClearDealerStandard(r.Context(), scope.InternalID, key); err != nil {
		writeError(w, r, err)
		return
	}
	orgUUID := scope.UUID
	h.record(r, "modules.dealer_standard.cleared", &orgUUID, map[string]any{"module_key": key})
	h.DealerStandard(w, r)
}

// --- Platform ---------------------------------------------------------------

// PlatformList lists the catalog with system switches
// (GET /v1/platform/modules).
func (h *Handler) PlatformList(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.PlatformModules(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type platformPatch struct {
	Enabled        *bool `json:"enabled"`
	DefaultEnabled *bool `json:"default_enabled"`
	Paid           *bool `json:"paid"`
}

// PlatformPatch changes a module system wide
// (PATCH /v1/platform/modules/{key}).
func (h *Handler) PlatformPatch(w http.ResponseWriter, r *http.Request) {
	var body platformPatch
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	key := r.PathValue("key")
	m, err := h.svc.UpdatePlatformModule(r.Context(), actorID(r), key, features.PlatformModuleInput{
		Enabled: body.Enabled, DefaultEnabled: body.DefaultEnabled, Paid: body.Paid,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "modules.platform.updated", nil, map[string]any{
		"module_key": key, "enabled": body.Enabled, "default_enabled": body.DefaultEnabled, "paid": body.Paid,
	})
	response.JSON(w, r, http.StatusOK, m)
}

func (h *Handler) platformOrg(w http.ResponseWriter, r *http.Request) (db.Organization, bool) {
	id, ok := parseUUID(w, r, "uuid")
	if !ok {
		return db.Organization{}, false
	}
	o, err := h.q.GetOrganizationByUUID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, features.ErrOrganizationNotFound)
		return db.Organization{}, false
	}
	if err != nil {
		writeError(w, r, err)
		return db.Organization{}, false
	}
	return o, true
}

// PlatformOrgModules resolves one organization for the admin
// (GET /v1/platform/organizations/{uuid}/modules).
func (h *Handler) PlatformOrgModules(w http.ResponseWriter, r *http.Request) {
	o, ok := h.platformOrg(w, r)
	if !ok {
		return
	}
	items, err := h.svc.OrgStates(r.Context(), o.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"organization_type": o.Type, "items": items})
}

// PlatformSetOrgModule sets an organization's value as admin
// (PUT /v1/platform/organizations/{uuid}/modules/{key}).
func (h *Handler) PlatformSetOrgModule(w http.ResponseWriter, r *http.Request) {
	o, ok := h.platformOrg(w, r)
	if !ok {
		return
	}
	enabled, ok := decodeEnabled(w, r)
	if !ok {
		return
	}
	key := r.PathValue("key")
	st, err := h.svc.SetByAdmin(r.Context(), actorID(r), o.ID, key, enabled)
	if err != nil {
		writeError(w, r, err)
		return
	}
	orgUUID := o.Uuid
	h.record(r, "modules.organization.updated", &orgUUID, map[string]any{"module_key": key, "enabled": enabled})
	response.JSON(w, r, http.StatusOK, st)
}

// PlatformClearOrgModule drops an organization's own value
// (DELETE /v1/platform/organizations/{uuid}/modules/{key}).
func (h *Handler) PlatformClearOrgModule(w http.ResponseWriter, r *http.Request) {
	o, ok := h.platformOrg(w, r)
	if !ok {
		return
	}
	key := r.PathValue("key")
	st, err := h.svc.ClearByAdmin(r.Context(), o.ID, key)
	if err != nil {
		writeError(w, r, err)
		return
	}
	orgUUID := o.Uuid
	h.record(r, "modules.organization.cleared", &orgUUID, map[string]any{"module_key": key})
	response.JSON(w, r, http.StatusOK, st)
}
