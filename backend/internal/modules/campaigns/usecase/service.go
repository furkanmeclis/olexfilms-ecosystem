// Package usecase implements the F4 campaign authoring API (TEC-405,
// F4-04b): draft CRUD, per-locale contents and media, the audience filter
// with its preview and the mandatory localization gate used before the
// campaign is submitted (F4-04c).
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	platstorage "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Campaign statuses (chk_campaigns_status).
const (
	StatusDraft           = "draft"
	StatusPendingApproval = "pending_approval"
	StatusApproved        = "approved"
	StatusScheduled       = "scheduled"
	StatusSending         = "sending"
	StatusSent            = "sent"
	StatusPartiallyFailed = "partially_failed"
	StatusCancelled       = "cancelled"
	StatusRejected        = "rejected"
)

// Channels (campaign_channels_valid; no SMS, design §9).
const (
	ChannelPush     = "push"
	ChannelWhatsApp = "whatsapp"
	ChannelEmail    = "email"
)

// Channels lists the campaign channels in display order.
var Channels = []string{ChannelPush, ChannelWhatsApp, ChannelEmail}

// Statuses lists the campaign statuses in flow order.
var Statuses = []string{
	StatusDraft, StatusPendingApproval, StatusApproved, StatusScheduled, StatusSending,
	StatusSent, StatusPartiallyFailed, StatusCancelled, StatusRejected,
}

// Channel content limits (characters).
const (
	MaxPushTitle    = 65
	MaxPushBody     = 240
	MaxWhatsAppBody = 4096
	MaxEmailSubject = 150

	maxName     = 200
	maxTitle    = 200
	maxBody     = 20000
	maxDeeplink = 2048
)

// Error codes answered by the handler.
const (
	CodeLocaleMissing = "CAMPAIGN_LOCALE_MISSING"
	CodeNotDraft      = "CAMPAIGN_NOT_DRAFT"
)

var (
	ErrNotFound = errors.New("campaigns: not found")
	// ErrNotDraft: the campaign left draft; it can no longer be edited (409).
	ErrNotDraft = errors.New("campaigns: campaign is not a draft")
)

// ValidationError is one invalid input field (400).
type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return "campaigns: invalid " + e.Field + ": " + e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// LocaleMissingError lists the audience locales whose content is missing
// or incomplete for a selected channel (422 CAMPAIGN_LOCALE_MISSING).
type LocaleMissingError struct{ Locales []string }

func (e *LocaleMissingError) Error() string {
	return "campaigns: content missing for locales " + strings.Join(e.Locales, ", ")
}

// TxBeginner opens transactions (pool or an outer test transaction).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Storage keeps campaign media (S3 / SeaweedFS).
type Storage interface {
	Upload(ctx context.Context, file platstorage.File, path string) error
	Delete(ctx context.Context, path string) error
}

// Caller is the authenticated user in the active organization with the
// resolved scope of the route permission.
type Caller struct {
	UserID         int64
	OrganizationID int64
	OrgType        string
	BrandID        int64
	Filter         scopefilter.Filter
}

// Input creates a campaign.
type Input struct {
	Name           string          `json:"name"`
	Channels       []string        `json:"channels"`
	AudienceFilter *AudienceFilter `json:"audience_filter"`
}

// PatchInput updates a draft; absent fields keep their value.
type PatchInput struct {
	Name           *string         `json:"name,omitempty"`
	Channels       []string        `json:"channels,omitempty"`
	AudienceFilter *AudienceFilter `json:"audience_filter,omitempty"`
}

// ContentInput is the content of one locale.
type ContentInput struct {
	Title    string  `json:"title"`
	Body     string  `json:"body"`
	Deeplink *string `json:"deeplink,omitempty"`
}

// Campaign is the API projection. Times are UTC; timezone is the
// organization's zone the panel shows them in (TEC-406).
type Campaign struct {
	UUID                     uuid.UUID      `json:"uuid"`
	OrganizationUUID         uuid.UUID      `json:"organization_uuid"`
	OrganizationName         string         `json:"organization_name"`
	Timezone                 string         `json:"timezone"`
	ApproverOrganizationUUID *uuid.UUID     `json:"approver_organization_uuid"`
	Name                     string         `json:"name"`
	Channels                 []string       `json:"channels"`
	AudienceFilter           AudienceFilter `json:"audience_filter"`
	Status                   string         `json:"status"`
	ScheduledAt              *time.Time     `json:"scheduled_at"`
	StartedAt                *time.Time     `json:"started_at"`
	FinishedAt               *time.Time     `json:"finished_at"`
	RecipientsTotal          int32          `json:"recipients_total"`
	RecipientsSent           int32          `json:"recipients_sent"`
	RecipientsFailed         int32          `json:"recipients_failed"`
	RecipientsSkipped        int32          `json:"recipients_skipped"`
	CreatedAt                time.Time      `json:"created_at"`
	UpdatedAt                time.Time      `json:"updated_at"`
	Contents                 []Content      `json:"contents,omitempty"`
	Events                   []Event        `json:"events,omitempty"`
}

// Content is the content of one locale with its media.
type Content struct {
	Locale    string    `json:"locale"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Deeplink  *string   `json:"deeplink"`
	Media     []Media   `json:"media"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Media is one image or PDF of a content.
type Media struct {
	UUID      uuid.UUID `json:"uuid"`
	Locale    string    `json:"locale"`
	Kind      string    `json:"kind"`
	MimeType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	FileName  *string   `json:"file_name"`
	CreatedAt time.Time `json:"created_at"`
}

// Service coordinates campaign persistence.
type Service struct {
	pool    TxBeginner
	q       *db.Queries
	storage Storage
	out     outbox.Enqueuer
	now     func() time.Time
}

// New creates the service. storage may be nil (media uploads then fail).
func New(pool TxBeginner, q *db.Queries, storage Storage) *Service {
	return &Service{pool: pool, q: q, storage: storage, now: func() time.Time { return time.Now().UTC() }}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// SetOutbox sets the outbox of the approval notifications (TEC-406); nil
// writes none.
func (s *Service) SetOutbox(out outbox.Enqueuer) { s.out = out }

func (s *Service) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	return s.inTxOut(ctx, func(_ pgx.Tx, q *db.Queries) error { return fn(q) })
}

func (s *Service) inTxOut(ctx context.Context, fn func(tx pgx.Tx, q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx, s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Create stores a new draft of the caller's active organization.
func (s *Service) Create(ctx context.Context, c Caller, in Input) (Campaign, error) {
	if !c.Filter.AllowsOrg(c.OrganizationID, c.BrandID) {
		return Campaign{}, ErrNotFound
	}
	name, err := validateName(in.Name)
	if err != nil {
		return Campaign{}, err
	}
	channels, err := validateChannels(in.Channels)
	if err != nil {
		return Campaign{}, err
	}
	if in.AudienceFilter == nil {
		return Campaign{}, invalid("audience_filter", "is required")
	}
	org, err := s.q.GetOrganizationByID(ctx, c.OrganizationID)
	if err != nil {
		return Campaign{}, err
	}
	filter, err := s.validateFilter(ctx, s.q, org, *in.AudienceFilter)
	if err != nil {
		return Campaign{}, err
	}
	raw, err := json.Marshal(filter)
	if err != nil {
		return Campaign{}, err
	}
	row, err := s.q.CreateCampaign(ctx, db.CreateCampaignParams{
		OrganizationID: org.ID, BrandID: org.BrandID, Name: name, Channels: channels,
		AudienceFilter: raw, CreatedByUserID: pgInt8(c.UserID),
	})
	if err != nil {
		return Campaign{}, err
	}
	return s.view(ctx, s.q, row, &org, false)
}

// Get returns a campaign with its contents, media and history; the
// approver organization reads it too (TEC-406).
func (s *Service) Get(ctx context.Context, c Caller, id uuid.UUID) (Campaign, error) {
	row, err := s.load(ctx, s.q, c, id)
	if errors.Is(err, ErrNotFound) {
		row, err = s.loadForApprover(ctx, c, id)
	}
	if err != nil {
		return Campaign{}, err
	}
	return s.view(ctx, s.q, row, nil, true)
}

// Update changes name, channels or audience of a draft. An approved or
// scheduled campaign returns to draft first when something changes
// (TEC-406); other statuses answer 409.
func (s *Service) Update(ctx context.Context, c Caller, id uuid.UUID, in PatchInput) (Campaign, error) {
	var out db.Campaign
	err := s.inTx(ctx, func(q *db.Queries) error {
		var name string
		var channels []string
		var rawFilter []byte
		row, err := s.lockEditable(ctx, q, c, id, func(row db.Campaign) (bool, error) {
			var err error
			name, channels, rawFilter = row.Name, row.Channels, row.AudienceFilter
			if in.Name != nil {
				if name, err = validateName(*in.Name); err != nil {
					return false, err
				}
			}
			if in.Channels != nil {
				if channels, err = validateChannels(in.Channels); err != nil {
					return false, err
				}
				contents, err := q.ListCampaignContents(ctx, row.ID)
				if err != nil {
					return false, err
				}
				for _, ct := range contents {
					if err := checkLimits(channels, ct.Title, ct.Body); err != nil {
						return false, err
					}
				}
			}
			if in.AudienceFilter != nil {
				org, err := q.GetOrganizationByID(ctx, row.OrganizationID)
				if err != nil {
					return false, err
				}
				filter, err := s.validateFilter(ctx, q, org, *in.AudienceFilter)
				if err != nil {
					return false, err
				}
				if rawFilter, err = json.Marshal(filter); err != nil {
					return false, err
				}
			}
			return name != row.Name || !slices.Equal(channels, row.Channels) ||
				!jsonEqual(rawFilter, row.AudienceFilter), nil
		})
		if err != nil {
			return err
		}
		if row.Status != StatusDraft {
			out = row // approved / scheduled and nothing changed
			return nil
		}
		out, err = q.UpdateCampaignDraft(ctx, db.UpdateCampaignDraftParams{
			ID: row.ID, Name: name, Channels: channels, AudienceFilter: rawFilter, ActorUserID: pgInt8(c.UserID),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotDraft
		}
		return err
	})
	if err != nil {
		return Campaign{}, err
	}
	return s.view(ctx, s.q, out, nil, true)
}

// Delete removes a draft with its contents; stored media objects are
// removed afterwards (best effort).
func (s *Service) Delete(ctx context.Context, c Caller, id uuid.UUID) error {
	var keys []string
	err := s.inTx(ctx, func(q *db.Queries) error {
		row, err := s.lockDraft(ctx, q, c, id)
		if err != nil {
			return err
		}
		media, err := q.ListCampaignMedia(ctx, row.ID)
		if err != nil {
			return err
		}
		for _, m := range media {
			keys = append(keys, m.StorageKey)
		}
		n, err := q.DeleteDraftCampaign(ctx, row.ID)
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotDraft
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.removeObjects(ctx, keys...)
	return nil
}

// PutContent creates or replaces the content of one locale of a draft; a
// change of an approved or scheduled campaign returns it to draft.
func (s *Service) PutContent(ctx context.Context, c Caller, id uuid.UUID, locale string, in ContentInput) (Content, error) {
	if !i18n.IsSupported(locale) {
		return Content{}, invalid("locale", "must be one of "+i18n.SupportedList())
	}
	title, body := strings.TrimSpace(in.Title), strings.TrimSpace(in.Body)
	if utf8.RuneCountInString(title) > maxTitle {
		return Content{}, invalid("title", fmt.Sprintf("must be at most %d characters", maxTitle))
	}
	if utf8.RuneCountInString(body) > maxBody {
		return Content{}, invalid("body", fmt.Sprintf("must be at most %d characters", maxBody))
	}
	var deeplink pgtype.Text
	if in.Deeplink != nil {
		if d := strings.TrimSpace(*in.Deeplink); d != "" {
			if utf8.RuneCountInString(d) > maxDeeplink || strings.ContainsAny(d, " \t\r\n") {
				return Content{}, invalid("deeplink", "must be a link of at most 2048 characters")
			}
			deeplink = pgtype.Text{String: d, Valid: true}
		}
	}
	var out Content
	err := s.inTx(ctx, func(q *db.Queries) error {
		row, err := s.lockEditable(ctx, q, c, id, func(row db.Campaign) (bool, error) {
			if err := checkLimits(row.Channels, title, body); err != nil {
				return false, err
			}
			cur, err := q.GetCampaignContent(ctx, db.GetCampaignContentParams{CampaignID: row.ID, Locale: locale})
			if errors.Is(err, pgx.ErrNoRows) {
				return true, nil
			}
			if err != nil {
				return false, err
			}
			return cur.Title != title || cur.Body != body || cur.Deeplink != deeplink, nil
		})
		if err != nil {
			return err
		}
		ct, err := q.UpsertCampaignContent(ctx, db.UpsertCampaignContentParams{
			CampaignID: row.ID, Locale: locale, Title: title, Body: body, Deeplink: deeplink,
		})
		if err != nil {
			return err
		}
		media, err := q.ListCampaignMedia(ctx, row.ID)
		if err != nil {
			return err
		}
		out = contentView(ct, media)
		return nil
	})
	return out, err
}

// load returns a campaign of the brand inside the caller's scope.
func (s *Service) load(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID) (db.Campaign, error) {
	row, err := q.GetCampaignByUUID(ctx, db.GetCampaignByUUIDParams{Uuid: id, BrandID: c.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Campaign{}, ErrNotFound
	}
	if err != nil {
		return db.Campaign{}, err
	}
	if !c.Filter.AllowsOrg(row.OrganizationID, row.BrandID) {
		return db.Campaign{}, ErrNotFound
	}
	return row, nil
}

// lockDraft locks a campaign in scope and requires the draft status.
func (s *Service) lockDraft(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID) (db.Campaign, error) {
	row, err := s.load(ctx, q, c, id)
	if err != nil {
		return db.Campaign{}, err
	}
	row, err = q.GetCampaignByIDForUpdate(ctx, db.GetCampaignByIDForUpdateParams{ID: row.ID, BrandID: row.BrandID})
	if err != nil {
		return db.Campaign{}, err
	}
	if row.Status != StatusDraft {
		return db.Campaign{}, ErrNotDraft
	}
	return row, nil
}

// view builds the projection; org is the campaign organization when the
// caller has it (nil loads it). withContents adds contents, media and the
// event history.
func (s *Service) view(ctx context.Context, q *db.Queries, row db.Campaign, org *db.Organization, withContents bool) (Campaign, error) {
	orgs := map[int64]db.Organization{}
	if org != nil {
		orgs[org.ID] = *org
	}
	out, err := s.viewWith(ctx, q, row, orgs)
	if err != nil {
		return Campaign{}, err
	}
	if !withContents {
		return out, nil
	}
	evs, err := q.ListCampaignEvents(ctx, row.ID)
	if err != nil {
		return Campaign{}, err
	}
	out.Events = make([]Event, 0, len(evs))
	for _, e := range evs {
		out.Events = append(out.Events, eventView(e))
	}
	contents, err := q.ListCampaignContents(ctx, row.ID)
	if err != nil {
		return Campaign{}, err
	}
	media, err := q.ListCampaignMedia(ctx, row.ID)
	if err != nil {
		return Campaign{}, err
	}
	out.Contents = make([]Content, 0, len(contents))
	for _, ct := range contents {
		out.Contents = append(out.Contents, contentView(ct, media))
	}
	return out, nil
}

func (s *Service) removeObjects(ctx context.Context, keys ...string) {
	if s.storage == nil {
		return
	}
	for _, k := range keys {
		_ = s.storage.Delete(ctx, k)
	}
}

// viewWith projects a row, loading the campaign and approver organizations
// through the orgs cache.
func (s *Service) viewWith(ctx context.Context, q *db.Queries, row db.Campaign, orgs map[int64]db.Organization) (Campaign, error) {
	get := func(id int64) (db.Organization, error) {
		if o, ok := orgs[id]; ok {
			return o, nil
		}
		o, err := q.GetOrganizationByID(ctx, id)
		if err != nil {
			return db.Organization{}, err
		}
		orgs[id] = o
		return o, nil
	}
	org, err := get(row.OrganizationID)
	if err != nil {
		return Campaign{}, err
	}
	out := campaignView(row, org)
	if row.ApproverOrgID.Valid {
		approver, err := get(row.ApproverOrgID.Int64)
		if err != nil {
			return Campaign{}, err
		}
		au := approver.Uuid
		out.ApproverOrganizationUUID = &au
	}
	return out, nil
}

// views projects a page of rows.
func (s *Service) views(ctx context.Context, rows []db.Campaign) ([]Campaign, error) {
	orgs := map[int64]db.Organization{}
	items := make([]Campaign, 0, len(rows))
	for _, r := range rows {
		v, err := s.viewWith(ctx, s.q, r, orgs)
		if err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return items, nil
}

func campaignView(row db.Campaign, org db.Organization) Campaign {
	var filter AudienceFilter
	_ = json.Unmarshal(row.AudienceFilter, &filter)
	return Campaign{
		UUID: row.Uuid, OrganizationUUID: org.Uuid, OrganizationName: org.Name, Timezone: org.Timezone,
		Name: row.Name, Channels: row.Channels,
		AudienceFilter: filter.normalizedLists(), Status: row.Status,
		ScheduledAt: ptrTime(row.ScheduledAt), StartedAt: ptrTime(row.StartedAt), FinishedAt: ptrTime(row.FinishedAt),
		RecipientsTotal: row.RecipientsTotal, RecipientsSent: row.RecipientsSent,
		RecipientsFailed: row.RecipientsFailed, RecipientsSkipped: row.RecipientsSkipped,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func contentView(ct db.CampaignContent, media []db.ListCampaignMediaRow) Content {
	out := Content{Locale: ct.Locale, Title: ct.Title, Body: ct.Body, UpdatedAt: ct.UpdatedAt.Time, Media: []Media{}}
	if ct.Deeplink.Valid {
		d := ct.Deeplink.String
		out.Deeplink = &d
	}
	for _, m := range media {
		if m.ContentID == ct.ID {
			out.Media = append(out.Media, mediaView(m.Uuid, m.Locale, m.Kind, m.MimeType, m.SizeBytes, m.FileName, m.CreatedAt))
		}
	}
	return out
}

func mediaView(id uuid.UUID, locale, kind, mime string, size int64, name pgtype.Text, at pgtype.Timestamptz) Media {
	m := Media{UUID: id, Locale: locale, Kind: kind, MimeType: mime, SizeBytes: size, CreatedAt: at.Time}
	if name.Valid {
		n := name.String
		m.FileName = &n
	}
	return m
}

func validateName(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", invalid("name", "is required")
	}
	if utf8.RuneCountInString(v) > maxName {
		return "", invalid("name", fmt.Sprintf("must be at most %d characters", maxName))
	}
	return v, nil
}

// validateChannels returns the channels in canonical order without
// duplicates.
func validateChannels(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, invalid("channels", "at least one channel is required")
	}
	seen := map[string]bool{}
	for _, ch := range in {
		if !contains(Channels, ch) {
			return nil, invalid("channels", "must be push, whatsapp or email")
		}
		seen[ch] = true
	}
	out := make([]string, 0, len(seen))
	for _, ch := range Channels {
		if seen[ch] {
			out = append(out, ch)
		}
	}
	return out, nil
}

// checkLimits applies the length limits of every selected channel.
func checkLimits(channels []string, title, body string) error {
	tl, bl := utf8.RuneCountInString(title), utf8.RuneCountInString(body)
	for _, ch := range channels {
		switch ch {
		case ChannelPush:
			if tl > MaxPushTitle {
				return invalid("title", fmt.Sprintf("push title must be at most %d characters", MaxPushTitle))
			}
			if bl > MaxPushBody {
				return invalid("body", fmt.Sprintf("push body must be at most %d characters", MaxPushBody))
			}
		case ChannelWhatsApp:
			if bl > MaxWhatsAppBody {
				return invalid("body", fmt.Sprintf("WhatsApp body must be at most %d characters", MaxWhatsAppBody))
			}
		case ChannelEmail:
			if tl > MaxEmailSubject {
				return invalid("title", fmt.Sprintf("e-mail subject must be at most %d characters", MaxEmailSubject))
			}
		}
	}
	return nil
}

// complete reports whether a content is filled for every selected channel
// (push: title + body, WhatsApp: body, e-mail: subject + body) and inside
// the channel limits.
func complete(channels []string, title, body string) bool {
	if checkLimits(channels, title, body) != nil {
		return false
	}
	for _, ch := range channels {
		switch ch {
		case ChannelPush, ChannelEmail:
			if title == "" || body == "" {
				return false
			}
		case ChannelWhatsApp:
			if body == "" {
				return false
			}
		}
	}
	return true
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// jsonEqual compares two JSON documents by value.
func jsonEqual(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func pgInt8(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: v != 0} }

func ptrTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
