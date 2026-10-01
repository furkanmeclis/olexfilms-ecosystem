// Package handler serves the module (feature flag) endpoints of TEC-86:
// the "Özellikler" page of every organization, the distributor's dealer
// settings and the platform admin's module catalog.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Notifier delivers the "request a module" notification.
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
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
	switch {
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
	OrganizationType string           `json:"organization_type"`
	Items            []features.State `json:"items"`
	// Enabled lists the keys that are on (menus and route guards).
	Enabled []string `json:"enabled"`
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
	out := featuresResponse{OrganizationType: scope.OrgType, Items: []features.State{}, Enabled: []string{}}
	for _, st := range states {
		if st.Visible {
			out.Items = append(out.Items, st)
		}
		if st.Enabled {
			out.Enabled = append(out.Enabled, st.Key)
		}
	}
	response.JSON(w, r, http.StatusOK, out)
}

type requestBody struct {
	Note string `json:"note"`
}

// Request asks the level above to switch a module on
// (POST /v1/features/{key}/request): the parent distributor's owners, or the
// platform admins for a distributor or a dealer under the center.
func (h *Handler) Request(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	key := r.PathValue("key")
	def, ok := features.ModuleByKey(key)
	if !ok {
		writeError(w, r, features.ErrUnknownModule)
		return
	}
	var body requestBody
	if r.ContentLength != 0 {
		if err := decodeJSON(w, r, &body); err != nil {
			return
		}
	}
	if len(body.Note) > 1000 {
		response.BadRequest(w, r, response.CodeValidationError, "note is too long")
		return
	}
	if def.Level == features.LevelCore {
		writeError(w, r, features.ErrCoreModule)
		return
	}
	recipients, err := h.upstreamRecipients(r.Context(), scope.InternalID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.notifier != nil {
		title := "Module request"
		bodyText := fmt.Sprintf("%s requested the module %q.", scope.Name, key)
		if body.Note != "" {
			bodyText += " " + body.Note
		}
		for _, uid := range recipients {
			id := uid
			if _, err := h.notifier.Enqueue(r.Context(), notifmodel.EnqueueInput{
				UserID: &id, Channels: []string{notifmodel.ChannelInapp},
				Title: title, Body: bodyText, SourceEvent: "features.module_requested",
				Payload: map[string]any{
					"module_key": key, "organization_uuid": scope.UUID.String(),
					"organization_name": scope.Name, "note": body.Note,
				},
			}); err != nil {
				h.log.Warn("feature_request_notify_failed", "user_id", id, "error", err)
			}
		}
	}
	orgUUID := scope.UUID
	h.record(r, "modules.requested", &orgUUID, map[string]any{"module_key": key, "recipients": len(recipients)})
	response.JSON(w, r, http.StatusAccepted, map[string]any{"status": "requested", "recipients": len(recipients)})
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

// Dealers returns the module matrix of the distributor's dealers
// (GET /v1/tenant/modules/dealers).
func (h *Handler) Dealers(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	f, _ := scopefilter.From(r.Context())
	if f.Scope != rbac.ScopeSubtree && f.Scope != rbac.ScopeAll {
		writeError(w, r, features.ErrNotDistributor)
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
		out = append(out, dr)
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
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
