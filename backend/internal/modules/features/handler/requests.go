package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	notifcatalog "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ModuleRequestsSortSpec is the sort whitelist of the module request queues
// (TEC-508, docs/list-contract.md).
var ModuleRequestsSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"created_at": "created_at", "decided_at": "decided_at", "module_key": "module_key",
		"status": "status", "organization_name": "organization_name",
	},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

func moduleKeys() []string {
	out := make([]string, 0, len(features.Modules))
	for _, m := range features.Modules {
		out = append(out, m.Key)
	}
	return out
}

// moduleRequestRow is one request of a decision queue.
type moduleRequestRow struct {
	UUID             uuid.UUID  `json:"uuid"`
	OrganizationUUID uuid.UUID  `json:"organization_uuid"`
	OrganizationName string     `json:"organization_name"`
	OrganizationType string     `json:"organization_type"`
	ModuleKey        string     `json:"module_key"`
	Note             string     `json:"note"`
	Status           string     `json:"status"`
	RequestedByUUID  *uuid.UUID `json:"requested_by_uuid"`
	RequestedByName  string     `json:"requested_by_name"`
	DecidedByUUID    *uuid.UUID `json:"decided_by_uuid"`
	DecidedByName    string     `json:"decided_by_name"`
	DecisionNote     string     `json:"decision_note"`
	CreatedAt        time.Time  `json:"created_at"`
	DecidedAt        *time.Time `json:"decided_at"`
}

func optUUID(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := uuid.UUID(v.Bytes)
	return &id
}

func optTime(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func tsArg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// listRequests serves a decision queue: limit/offset/total, sort
// (created_at default desc, decided_at, module_key, status,
// organization_name), q (organization name, module key), status and
// module_key (CSV), created_from/created_to.
func (h *Handler) listRequests(w http.ResponseWriter, r *http.Request, queue string, distributorID int64) {
	qv := r.URL.Query()
	q := apiquery.Parse(qv)
	sort, err := apiquery.ResolveSort(q.Sort, ModuleRequestsSortSpec)
	if err != nil {
		writeError(w, r, err)
		return
	}
	statuses, err := apiquery.EnumList(qv, "status", features.RequestStatuses...)
	if err != nil {
		writeError(w, r, err)
		return
	}
	keys, err := apiquery.EnumList(qv, "module_key", moduleKeys()...)
	if err != nil {
		writeError(w, r, err)
		return
	}
	created, err := apiquery.DateRange(qv, "created")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var search pgtype.Text
	if s := strings.TrimSpace(q.Q); s != "" {
		search = pgtype.Text{String: s, Valid: true}
	}
	dist := pgtype.Int8{Int64: distributorID, Valid: distributorID != 0}
	rows, err := h.q.ListModuleRequestsPage(r.Context(), db.ListModuleRequestsPageParams{
		Queue: queue, DistributorID: dist, Statuses: statuses, ModuleKeys: keys, Q: search,
		CreatedFrom: tsArg(created.From), CreatedBefore: tsArg(created.Before),
		SortKey: sort.Key, SortDesc: sort.Desc, LimitCount: q.Limit, OffsetCount: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	total, err := h.q.CountModuleRequestsPage(r.Context(), db.CountModuleRequestsPageParams{
		Queue: queue, DistributorID: dist, Statuses: statuses, ModuleKeys: keys, Q: search,
		CreatedFrom: tsArg(created.From), CreatedBefore: tsArg(created.Before),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]moduleRequestRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, moduleRequestRow{
			UUID: row.Uuid, OrganizationUUID: row.OrganizationUuid, OrganizationName: row.OrganizationName,
			OrganizationType: row.OrganizationType, ModuleKey: row.ModuleKey, Note: row.Note, Status: row.Status,
			RequestedByUUID: optUUID(row.RequestedByUuid), RequestedByName: row.RequestedByName,
			DecidedByUUID: optUUID(row.DecidedByUuid), DecidedByName: row.DecidedByName,
			DecisionNote: row.DecisionNote, CreatedAt: row.CreatedAt.Time, DecidedAt: optTime(row.DecidedAt),
		})
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(out, total, q.Limit, q.Offset))
}

// distributorQueue returns the active distributor (the queue of its direct
// dealers' requests).
func distributorQueue(w http.ResponseWriter, r *http.Request) (int64, bool) {
	scope := orgctx.MustScope(r.Context())
	if scope.OrgType != features.OrgDistributor {
		writeError(w, r, features.ErrNotDistributor)
		return 0, false
	}
	return scope.InternalID, true
}

// TenantRequests lists the module requests of the distributor's dealers
// (GET /v1/tenant/modules/requests).
func (h *Handler) TenantRequests(w http.ResponseWriter, r *http.Request) {
	dist, ok := distributorQueue(w, r)
	if !ok {
		return
	}
	h.listRequests(w, r, features.QueueDistributor, dist)
}

// TenantApproveRequest opens the module for the requesting dealer
// (POST /v1/tenant/modules/requests/{uuid}/approve). 409
// MODULE_BLOCKED_BY_PARENT when the distributor itself does not have it:
// request it from the level above first.
func (h *Handler) TenantApproveRequest(w http.ResponseWriter, r *http.Request) {
	if dist, ok := distributorQueue(w, r); ok {
		h.decide(w, r, features.QueueDistributor, dist, true)
	}
}

// TenantRejectRequest rejects a dealer's request
// (POST /v1/tenant/modules/requests/{uuid}/reject).
func (h *Handler) TenantRejectRequest(w http.ResponseWriter, r *http.Request) {
	if dist, ok := distributorQueue(w, r); ok {
		h.decide(w, r, features.QueueDistributor, dist, false)
	}
}

// PlatformRequests lists the requests the center decides: distributors and
// dealers without a distributor (GET /v1/platform/modules/requests).
func (h *Handler) PlatformRequests(w http.ResponseWriter, r *http.Request) {
	h.listRequests(w, r, features.QueuePlatform, 0)
}

// PlatformApproveRequest sets the module on for the requester as the admin
// (POST /v1/platform/modules/requests/{uuid}/approve).
func (h *Handler) PlatformApproveRequest(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, features.QueuePlatform, 0, true)
}

// PlatformRejectRequest (POST /v1/platform/modules/requests/{uuid}/reject).
func (h *Handler) PlatformRejectRequest(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, features.QueuePlatform, 0, false)
}

type decisionBody struct {
	Note string `json:"note"`
}

func (h *Handler) decide(w http.ResponseWriter, r *http.Request, queue string, distributorID int64, approve bool) {
	id, ok := parseUUID(w, r, "uuid")
	if !ok {
		return
	}
	var body decisionBody
	if r.ContentLength != 0 {
		if err := decodeJSON(w, r, &body); err != nil {
			return
		}
	}
	if len(body.Note) > features.MaxRequestNote {
		response.BadRequest(w, r, response.CodeValidationError, "note is too long")
		return
	}
	req, err := h.svc.DecideRequest(r.Context(), features.DecideInput{
		ActorID: actorID(r), Queue: queue, DistributorID: distributorID, UUID: id, Approve: approve, Note: body.Note,
	})
	if approve && queue == features.QueueDistributor && errors.Is(err, features.ErrUpstreamDisabled) {
		response.Error(w, r, http.StatusConflict, response.CodeModuleBlockedByParent,
			"The module is off for your organization; request it from the level above first")
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	action := "modules.request.rejected"
	if approve {
		action = "modules.request.approved"
	}
	h.record(r, action, &id, map[string]any{"module_key": req.ModuleKey, "note": req.DecisionNote})
	response.JSON(w, r, http.StatusOK, summarize(req))
}

// DecisionNotifier sends features.module_request_decided to the requester
// (the requesting user, else the requesting organization's owners) with the
// decision label in each recipient's language. It implements
// features.DecisionNotifier.
type DecisionNotifier struct {
	q        *db.Queries
	notifier Notifier
	log      *slog.Logger
}

// NewDecisionNotifier creates the notifier; notifier nil disables it.
func NewDecisionNotifier(q *db.Queries, notifier Notifier, log *slog.Logger) *DecisionNotifier {
	if log == nil {
		log = slog.Default()
	}
	return &DecisionNotifier{q: q, notifier: notifier, log: log}
}

// ModuleRequestDecided notifies the requester of a decided request.
func (n *DecisionNotifier) ModuleRequestDecided(ctx context.Context, req db.ModuleRequest) {
	if n == nil || n.notifier == nil || (req.Status != features.RequestApproved && req.Status != features.RequestRejected) {
		return
	}
	org, err := n.q.GetOrganizationByID(ctx, req.OrganizationID)
	if err != nil {
		n.log.Warn("module_request_decided_notify_failed", "request_id", req.ID, "error", err)
		return
	}
	recipients := []int64{}
	if req.RequestedByUserID.Valid {
		recipients = append(recipients, req.RequestedByUserID.Int64)
	} else if ids, err := n.q.ListOrganizationOwnerUserIDs(ctx, org.ID); err == nil {
		recipients = ids
	}
	note := req.DecisionNote
	if note == features.AutoDecisionNote {
		note = ""
	}
	orgID, brandID := org.ID, org.BrandID
	for _, uid := range recipients {
		src := i18n.Sources{OrgLocale: org.Locale}
		if u, err := n.q.GetUserByID(ctx, uid); err == nil {
			src.UserLocale = u.Locale.String
		}
		locale := string(i18n.Resolve(src).Locale)
		if _, err := n.notifier.Dispatch(ctx, notifmodel.DispatchInput{
			EventID: uuid.New(), EventCode: notifcatalog.EventFeaturesModuleRequestDecided,
			OrganizationID: &orgID, BrandID: &brandID, UserIDs: []int64{uid},
			Vars: map[string]string{
				"organization_name": org.Name, "module_key": req.ModuleKey,
				"decision": notifcatalog.ModuleRequestDecisionLabel(locale, req.Status), "decision_note": note,
			},
			Payload: map[string]any{
				"request_uuid": req.Uuid.String(), "module_key": req.ModuleKey, "status": req.Status,
				"organization_uuid": org.Uuid.String(), "automatic": req.DecisionNote == features.AutoDecisionNote,
			},
		}); err != nil {
			n.log.Warn("module_request_decided_notify_failed", "request_id", req.ID, "error", err)
		}
	}
}
