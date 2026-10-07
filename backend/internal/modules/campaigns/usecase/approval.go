package usecase

// Approval chain and scheduling (TEC-406, F4-04c). A draft is submitted
// after the localization gate; the approver is resolved from the
// organization tree: dealer → its distributor (a dealer without an active
// distributor → the brand center), distributor → center (F4 user answer
// S4), center → approved at once. Only members of approver_org_id holding
// campaigns.approve (route permission) decide; anyone else gets 404. An
// approved campaign is scheduled (scheduled_at ≥ now + 5 min, stored in UTC,
// shown in the organization's time zone) or sent now (scheduled at now; the
// F4-04d worker picks it up). Every status change writes exactly one
// campaign_events row in the same transaction; the notified transitions
// also write an outbox event for the notification center.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Campaign event types (chk_campaign_events_type).
const (
	EventSubmitted        = "submitted"
	EventApproved         = "approved"
	EventRejected         = "rejected"
	EventChangesRequested = "changes_requested"
	EventScheduled        = "scheduled"
	EventCancelled        = "cancelled"
)

// Error codes of the approval and scheduling endpoints.
const (
	// CodeInvalidStatus: the action is not allowed in the current status (409).
	CodeInvalidStatus = "CAMPAIGN_INVALID_STATUS"
	// CodeOrganizationReadOnly: the campaign organization is not active
	// (expired contract / read only); it cannot send (422).
	CodeOrganizationReadOnly = "CAMPAIGN_ORGANIZATION_READ_ONLY"
	// CodeScheduleTooSoon: scheduled_at is earlier than now + 5 minutes (422).
	CodeScheduleTooSoon = "CAMPAIGN_SCHEDULE_TOO_SOON"
)

// MinScheduleLead is the earliest a campaign can be scheduled from now.
const MinScheduleLead = 5 * time.Minute

const maxReason = 2000

// editedReason is the reason of the event written when an approved or
// scheduled campaign is edited and returns to draft.
const editedReason = "edited after approval"

var (
	ErrInvalidStatus = errors.New("campaigns: action not allowed in the current status")
	ErrOrgReadOnly   = errors.New("campaigns: the organization cannot send campaigns")
	ErrScheduleSoon  = errors.New("campaigns: scheduled_at must be at least 5 minutes from now")
)

// DecisionInput carries the approver's reason (required for reject and
// request-changes).
type DecisionInput struct {
	Reason string `json:"reason"`
}

// ScheduleInput is the send time: RFC 3339 with an offset, or a local
// date-time (2006-01-02T15:04) read in the organization's time zone.
type ScheduleInput struct {
	ScheduledAt string `json:"scheduled_at"`
}

// Event is one entry of the campaign history.
type Event struct {
	UUID       uuid.UUID      `json:"uuid"`
	EventType  string         `json:"event_type"`
	FromStatus *string        `json:"from_status"`
	ToStatus   *string        `json:"to_status"`
	Reason     *string        `json:"reason"`
	Payload    map[string]any `json:"payload"`
	CreatedAt  time.Time      `json:"created_at"`
}

// move is one status change with its event.
type move struct {
	to, event   string
	approver    pgtype.Int8
	scheduledAt pgtype.Timestamptz
	reason      string
	payload     map[string]any
}

// transition moves row to m.to (409 when it moved meanwhile) and writes the
// campaign_events row.
func (s *Service) transition(ctx context.Context, q *db.Queries, c Caller, row db.Campaign, m move) (db.Campaign, error) {
	out, err := q.SetCampaignStatus(ctx, db.SetCampaignStatusParams{
		ID: row.ID, FromStatus: row.Status, Status: m.to, ApproverOrgID: m.approver,
		ScheduledAt: m.scheduledAt, ActorUserID: pgInt8(c.UserID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Campaign{}, ErrInvalidStatus
	}
	if err != nil {
		return db.Campaign{}, err
	}
	if m.payload == nil {
		m.payload = map[string]any{}
	}
	raw, err := json.Marshal(m.payload)
	if err != nil {
		return db.Campaign{}, err
	}
	var reason pgtype.Text
	if m.reason != "" {
		reason = pgtype.Text{String: m.reason, Valid: true}
	}
	if _, err := q.InsertCampaignEvent(ctx, db.InsertCampaignEventParams{
		CampaignID: row.ID, OrganizationID: row.OrganizationID, BrandID: row.BrandID, EventType: m.event,
		FromStatus: pgtype.Text{String: row.Status, Valid: true}, ToStatus: pgtype.Text{String: m.to, Valid: true},
		ActorUserID: pgInt8(c.UserID), ActorOrgID: pgInt8(c.OrganizationID), Reason: reason, Payload: raw,
	}); err != nil {
		return db.Campaign{}, err
	}
	return out, nil
}

// notify writes the outbox event of a notified transition.
func (s *Service) notify(ctx context.Context, tx pgx.Tx, name string, c Caller, row db.Campaign, org db.Organization,
	reason string, userIDs []int64) error {
	if s.out == nil {
		return nil
	}
	if userIDs == nil {
		userIDs = []int64{}
	}
	id, uid := row.ID, row.Uuid
	ev := events.New(name).WithTenant(row.OrganizationID).WithEntity("campaign", &id, &uid).WithPayload(map[string]any{
		"campaign_uuid": row.Uuid.String(), "campaign_name": row.Name, "brand_id": row.BrandID,
		"organization_id": org.ID, "organization_name": org.Name, "status": row.Status,
		"reason": reason, "notify_user_ids": userIDs,
	})
	if c.UserID != 0 {
		ev = ev.WithActor(c.UserID)
	}
	if err := s.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("campaigns: outbox: %w", err)
	}
	return nil
}

// lockInScope loads and locks a campaign of the caller's (write) scope.
func (s *Service) lockInScope(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID) (db.Campaign, error) {
	row, err := s.load(ctx, q, c, id)
	if err != nil {
		return db.Campaign{}, err
	}
	return q.GetCampaignByIDForUpdate(ctx, db.GetCampaignByIDForUpdateParams{ID: row.ID, BrandID: row.BrandID})
}

// lockForApprover locks a pending campaign whose approver is the caller's
// active organization; any other campaign is hidden (404).
func (s *Service) lockForApprover(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID) (db.Campaign, error) {
	row, err := q.GetCampaignByUUID(ctx, db.GetCampaignByUUIDParams{Uuid: id, BrandID: c.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Campaign{}, ErrNotFound
	}
	if err != nil {
		return db.Campaign{}, err
	}
	if !row.ApproverOrgID.Valid || row.ApproverOrgID.Int64 != c.OrganizationID {
		return db.Campaign{}, ErrNotFound
	}
	row, err = q.GetCampaignByIDForUpdate(ctx, db.GetCampaignByIDForUpdateParams{ID: row.ID, BrandID: row.BrandID})
	if err != nil {
		return db.Campaign{}, err
	}
	if row.Status != StatusPendingApproval || row.ApproverOrgID.Int64 != c.OrganizationID {
		return db.Campaign{}, ErrInvalidStatus
	}
	return row, nil
}

// requireSender refuses an organization that may not send (only active
// organizations send; read_only = contract expired, K23).
func requireSender(org db.Organization) error {
	if org.Status != "active" || org.DeletedAt.Valid {
		return ErrOrgReadOnly
	}
	return nil
}

// resolveApprover returns the organization deciding a campaign of org:
// dealer → its active distributor, otherwise the brand center; center →
// itself (approved at once).
func (s *Service) resolveApprover(ctx context.Context, q *db.Queries, org db.Organization) (db.Organization, error) {
	if org.Type == rbac.OrgTypeCenter {
		return org, nil
	}
	if org.Type == rbac.OrgTypeDealer && org.ParentID.Valid {
		parent, err := q.GetOrganizationByID(ctx, org.ParentID.Int64)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return db.Organization{}, err
		}
		if err == nil && parent.Type == rbac.OrgTypeDistributor && parent.BrandID == org.BrandID &&
			parent.Status == "active" && !parent.DeletedAt.Valid {
			return parent, nil
		}
	}
	return q.GetBrandCenter(ctx, org.BrandID)
}

// Submit sends a draft for approval (a center campaign is approved at once).
func (s *Service) Submit(ctx context.Context, c Caller, id uuid.UUID) (Campaign, error) {
	var out db.Campaign
	var org db.Organization
	err := s.inTxOut(ctx, func(tx pgx.Tx, q *db.Queries) error {
		row, err := s.lockInScope(ctx, q, c, id)
		if err != nil {
			return err
		}
		if row.Status != StatusDraft {
			return ErrInvalidStatus
		}
		if org, err = q.GetOrganizationByID(ctx, row.OrganizationID); err != nil {
			return err
		}
		if err := requireSender(org); err != nil {
			return err
		}
		if err := s.RequireLocalized(ctx, q, row); err != nil {
			return err
		}
		approver, err := s.resolveApprover(ctx, q, org)
		if err != nil {
			return err
		}
		approverID := pgtype.Int8{Int64: approver.ID, Valid: true}
		if approver.ID == org.ID {
			out, err = s.transition(ctx, q, c, row, move{
				to: StatusApproved, event: EventApproved, approver: approverID,
				payload: map[string]any{"auto": true},
			})
			return err
		}
		if out, err = s.transition(ctx, q, c, row, move{
			to: StatusPendingApproval, event: EventSubmitted, approver: approverID,
			payload: map[string]any{"approver_organization_uuid": approver.Uuid.String()},
		}); err != nil {
			return err
		}
		ids, err := q.ListTransferNotifyUserIDs(ctx, db.ListTransferNotifyUserIDsParams{
			OrganizationID: approver.ID, PermissionSlug: rbac.PermCampaignsApprove,
		})
		if err != nil {
			return err
		}
		return s.notify(ctx, tx, events.CampaignsSubmitted, c, out, org, "", ids)
	})
	if err != nil {
		return Campaign{}, err
	}
	return s.view(ctx, s.q, out, &org, true)
}

// Approve approves a pending campaign of the caller's organization queue.
func (s *Service) Approve(ctx context.Context, c Caller, id uuid.UUID, in DecisionInput) (Campaign, error) {
	reason, err := validateReason(in.Reason, false)
	if err != nil {
		return Campaign{}, err
	}
	return s.decide(ctx, c, id, move{to: StatusApproved, event: EventApproved, reason: reason}, events.CampaignsApproved)
}

// Reject closes a pending campaign; the reason is required.
func (s *Service) Reject(ctx context.Context, c Caller, id uuid.UUID, in DecisionInput) (Campaign, error) {
	reason, err := validateReason(in.Reason, true)
	if err != nil {
		return Campaign{}, err
	}
	return s.decide(ctx, c, id, move{to: StatusRejected, event: EventRejected, reason: reason}, events.CampaignsRejected)
}

// RequestChanges sends a pending campaign back to draft with a reason.
func (s *Service) RequestChanges(ctx context.Context, c Caller, id uuid.UUID, in DecisionInput) (Campaign, error) {
	reason, err := validateReason(in.Reason, true)
	if err != nil {
		return Campaign{}, err
	}
	return s.decide(ctx, c, id, move{to: StatusDraft, event: EventChangesRequested, reason: reason},
		events.CampaignsChangesRequested)
}

// decide applies an approver decision and tells the creator.
func (s *Service) decide(ctx context.Context, c Caller, id uuid.UUID, m move, eventName string) (Campaign, error) {
	var out db.Campaign
	var org db.Organization
	err := s.inTxOut(ctx, func(tx pgx.Tx, q *db.Queries) error {
		row, err := s.lockForApprover(ctx, q, c, id)
		if err != nil {
			return err
		}
		if m.to != StatusDraft {
			m.approver = row.ApproverOrgID
		}
		if out, err = s.transition(ctx, q, c, row, m); err != nil {
			return err
		}
		if org, err = q.GetOrganizationByID(ctx, row.OrganizationID); err != nil {
			return err
		}
		var ids []int64
		if row.CreatedByUserID.Valid && row.CreatedByUserID.Int64 != c.UserID {
			ids = []int64{row.CreatedByUserID.Int64}
		}
		return s.notify(ctx, tx, eventName, c, out, org, m.reason, ids)
	})
	if err != nil {
		return Campaign{}, err
	}
	return s.view(ctx, s.q, out, &org, true)
}

// Schedule sets (or moves) the send time of an approved campaign.
func (s *Service) Schedule(ctx context.Context, c Caller, id uuid.UUID, in ScheduleInput) (Campaign, error) {
	raw := strings.TrimSpace(in.ScheduledAt)
	if raw == "" {
		return Campaign{}, invalid("scheduled_at", "is required")
	}
	return s.schedule(ctx, c, id, func(org db.Organization) (time.Time, error) {
		at, err := parseScheduledAt(raw, org.Timezone)
		if err != nil {
			return time.Time{}, err
		}
		if at.Before(s.now().Add(MinScheduleLead)) {
			return time.Time{}, ErrScheduleSoon
		}
		return at, nil
	}, false)
}

// SendNow schedules an approved campaign for immediate sending.
func (s *Service) SendNow(ctx context.Context, c Caller, id uuid.UUID) (Campaign, error) {
	return s.schedule(ctx, c, id, func(db.Organization) (time.Time, error) { return s.now(), nil }, true)
}

func (s *Service) schedule(ctx context.Context, c Caller, id uuid.UUID, when func(db.Organization) (time.Time, error),
	sendNow bool) (Campaign, error) {
	var out db.Campaign
	var org db.Organization
	err := s.inTx(ctx, func(q *db.Queries) error {
		row, err := s.lockInScope(ctx, q, c, id)
		if err != nil {
			return err
		}
		if row.Status != StatusApproved && row.Status != StatusScheduled {
			return ErrInvalidStatus
		}
		if org, err = q.GetOrganizationByID(ctx, row.OrganizationID); err != nil {
			return err
		}
		if err := requireSender(org); err != nil {
			return err
		}
		at, err := when(org)
		if err != nil {
			return err
		}
		// The audience may have gained a locale since the approval.
		if err := s.RequireLocalized(ctx, q, row); err != nil {
			return err
		}
		at = at.UTC().Truncate(time.Second)
		out, err = s.transition(ctx, q, c, row, move{
			to: StatusScheduled, event: EventScheduled, approver: row.ApproverOrgID,
			scheduledAt: pgtype.Timestamptz{Time: at, Valid: true},
			payload:     map[string]any{"scheduled_at": at.Format(time.RFC3339), "timezone": org.Timezone, "send_now": sendNow},
		})
		return err
	})
	if err != nil {
		return Campaign{}, err
	}
	return s.view(ctx, s.q, out, &org, true)
}

// Cancel stops a campaign: before sending entirely; while sending the
// remaining pending recipients are skipped.
func (s *Service) Cancel(ctx context.Context, c Caller, id uuid.UUID) (Campaign, error) {
	var out db.Campaign
	err := s.inTx(ctx, func(q *db.Queries) error {
		row, err := s.lockInScope(ctx, q, c, id)
		if err != nil {
			return err
		}
		payload := map[string]any{}
		switch row.Status {
		case StatusDraft, StatusPendingApproval, StatusApproved, StatusScheduled:
		case StatusSending:
			n, err := q.SkipPendingCampaignRecipients(ctx, db.SkipPendingCampaignRecipientsParams{
				CampaignID: row.ID, Reason: "campaign cancelled",
			})
			if err != nil {
				return err
			}
			payload["skipped"] = n
		default:
			return ErrInvalidStatus
		}
		out, err = s.transition(ctx, q, c, row, move{
			to: StatusCancelled, event: EventCancelled, approver: row.ApproverOrgID,
			scheduledAt: row.ScheduledAt, payload: payload,
		})
		return err
	})
	if err != nil {
		return Campaign{}, err
	}
	return s.view(ctx, s.q, out, nil, true)
}

// reopen returns an approved or scheduled campaign to draft before it is
// edited: the change needs a new approval.
func (s *Service) reopen(ctx context.Context, q *db.Queries, c Caller, row db.Campaign) (db.Campaign, error) {
	return s.transition(ctx, q, c, row, move{
		to: StatusDraft, event: EventChangesRequested, reason: editedReason,
		payload: map[string]any{"cause": "edited", "previous_status": row.Status},
	})
}

// editable reports whether a campaign in status can be edited (a draft, or
// an approved / scheduled campaign that returns to draft).
func editable(status string) bool {
	return status == StatusDraft || status == StatusApproved || status == StatusScheduled
}

// lockEditable locks a campaign in scope for an edit. changed (optional)
// validates the edit and reports whether it changes anything; a draft is
// returned as is, an approved or scheduled campaign returns to draft when
// changed (or changed is nil); anything else is ErrNotDraft (409).
func (s *Service) lockEditable(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID, changed func(db.Campaign) (bool, error)) (db.Campaign, error) {
	row, err := s.lockInScope(ctx, q, c, id)
	if err != nil {
		return db.Campaign{}, err
	}
	if !editable(row.Status) {
		return db.Campaign{}, ErrNotDraft
	}
	diff := true
	if changed != nil {
		if diff, err = changed(row); err != nil {
			return db.Campaign{}, err
		}
	}
	if row.Status == StatusDraft || !diff {
		return row, nil
	}
	return s.reopen(ctx, q, c, row)
}

// ApprovalSort is the sort contract of the approval queue (oldest first).
var ApprovalSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"created_at": "created_at", "scheduled_at": "scheduled_at", "name": "name"},
	Default: apiquery.SortField{Field: "created_at"},
}

// ApprovalFilter is the parsed approval queue query.
type ApprovalFilter struct {
	Q                 string
	Channels          []string
	OrganizationUUIDs []uuid.UUID
	SortKey           string
	SortDesc          bool
	Limit             int32
	Offset            int32
}

// ParseApprovalFilter reads the queue parameters (list contract): q,
// channel and organization_uuid (multi-valued), sort created_at |
// scheduled_at | name (default created_at, id tiebreak).
func ParseApprovalFilter(values url.Values) (ApprovalFilter, error) {
	q := apiquery.Parse(values)
	f := ApprovalFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	if utf8.RuneCountInString(f.Q) > maxListQuery {
		return f, &apiquery.ValidationError{Details: []apiquery.Detail{{Field: "q", Message: "must be at most 100 characters", Code: "invalid"}}}
	}
	var err error
	if f.Channels, err = apiquery.EnumList(values, "channel", Channels...); err != nil {
		return f, err
	}
	if f.OrganizationUUIDs, err = uuidList(values, "organization_uuid"); err != nil {
		return f, err
	}
	sort, err := apiquery.ResolveSort(q.Sort, ApprovalSort)
	if err != nil {
		return f, err
	}
	f.SortKey, f.SortDesc = sort.Key, sort.Desc
	return f, nil
}

// Approvals lists the campaigns waiting for the caller's organization.
func (s *Service) Approvals(ctx context.Context, c Caller, f ApprovalFilter) (apiquery.Page[Campaign], error) {
	var q pgtype.Text
	if f.Q != "" {
		q = pgtype.Text{String: escapeLike(f.Q), Valid: true}
	}
	approvers := []int64{c.OrganizationID}
	rows, err := s.q.ListCampaignApprovals(ctx, db.ListCampaignApprovalsParams{
		BrandID: c.BrandID, ApproverOrgIds: approvers, Channels: f.Channels, OrganizationUuids: f.OrganizationUUIDs,
		Q: q, SortKey: f.SortKey, SortDesc: f.SortDesc, PageLimit: f.Limit, PageOffset: f.Offset,
	})
	if err != nil {
		return apiquery.Page[Campaign]{}, err
	}
	total, err := s.q.CountCampaignApprovals(ctx, db.CountCampaignApprovalsParams{
		BrandID: c.BrandID, ApproverOrgIds: approvers, Channels: f.Channels, OrganizationUuids: f.OrganizationUUIDs, Q: q,
	})
	if err != nil {
		return apiquery.Page[Campaign]{}, err
	}
	items, err := s.views(ctx, rows)
	if err != nil {
		return apiquery.Page[Campaign]{}, err
	}
	return apiquery.NewPage(items, total, f.Limit, f.Offset), nil
}

// loadForApprover returns a campaign whose approver is the caller's active
// organization (the approver reads it outside its own read scope).
func (s *Service) loadForApprover(ctx context.Context, c Caller, id uuid.UUID) (db.Campaign, error) {
	row, err := s.q.GetCampaignByUUID(ctx, db.GetCampaignByUUIDParams{Uuid: id, BrandID: c.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Campaign{}, ErrNotFound
	}
	if err != nil {
		return db.Campaign{}, err
	}
	if !row.ApproverOrgID.Valid || row.ApproverOrgID.Int64 != c.OrganizationID {
		return db.Campaign{}, ErrNotFound
	}
	return row, nil
}

// uuidList reads a multi-valued uuid filter (CSV or repeated).
func uuidList(values url.Values, key string) ([]uuid.UUID, error) {
	raw := apiquery.CSVValues(values, key)
	out := make([]uuid.UUID, 0, len(raw))
	for _, v := range raw {
		id, err := uuid.Parse(v)
		if err != nil {
			return nil, &apiquery.ValidationError{Details: []apiquery.Detail{{Field: key, Message: "must be uuids", Code: "invalid"}}}
		}
		out = append(out, id)
	}
	return out, nil
}

func validateReason(v string, required bool) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" && required {
		return "", invalid("reason", "is required")
	}
	if utf8.RuneCountInString(v) > maxReason {
		return "", invalid("reason", fmt.Sprintf("must be at most %d characters", maxReason))
	}
	return v, nil
}

// parseScheduledAt reads RFC 3339 (with offset) or a local date-time in
// the organization's time zone.
func parseScheduledAt(raw, tz string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil || tz == "" {
		loc = time.UTC
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04"} {
		if t, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, invalid("scheduled_at", "must be an RFC 3339 date-time")
}

func eventView(e db.CampaignEvent) Event {
	out := Event{UUID: e.Uuid, EventType: e.EventType, CreatedAt: e.CreatedAt.Time, Payload: map[string]any{}}
	_ = json.Unmarshal(e.Payload, &out.Payload)
	if e.FromStatus.Valid {
		v := e.FromStatus.String
		out.FromStatus = &v
	}
	if e.ToStatus.Valid {
		v := e.ToStatus.String
		out.ToStatus = &v
	}
	if e.Reason.Valid {
		v := e.Reason.String
		out.Reason = &v
	}
	return out
}
