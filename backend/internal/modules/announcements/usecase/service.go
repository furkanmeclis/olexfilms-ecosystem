// Package usecase implements the F3 announcement feed and authoring API.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusArchived  = "archived"

	BodyMarkdown = "markdown"
	BodyHTML     = "html"

	TargetAllNetwork   = "all_network"
	TargetDistributors = "distributors"
	TargetDealers      = "dealers"
	TargetRole         = "role"
	TargetSubtree      = "subtree"

	EventPublished = "announcement.published"
)

const (
	maxTitle = 200
	maxBody  = 20000

	notificationBatchSize = 500
)

var (
	ErrNotFound  = errors.New("announcements: not found")
	ErrForbidden = errors.New("announcements: forbidden")
	ErrConflict  = errors.New("announcements: conflict")
)

// ValidationError is one invalid input field.
type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string {
	return "announcements: invalid " + e.Field + ": " + e.Message
}

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// TxBeginner opens transactions.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// TaskEnqueuer enqueues background tasks.
type TaskEnqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Dispatcher sends Notification Center messages.
type Dispatcher interface {
	Dispatch(ctx context.Context, in model.DispatchInput) (model.DispatchResult, error)
}

// TargetResolver resolves the current target audience for an announcement.
type TargetResolver interface {
	GetAnnouncementByID(ctx context.Context, id int64) (db.Announcement, error)
	ListAnnouncementTargetUserIDs(ctx context.Context, announcementID int64) ([]int64, error)
}

// Caller is the authenticated user and active org.
type Caller struct {
	UserID int64
	Roles  []string
	Org    orgctx.Scope
}

// AudienceInput describes one target row.
type AudienceInput struct {
	TargetType             string     `json:"target_type"`
	RoleSlug               *string    `json:"role_slug,omitempty"`
	TargetOrganizationUUID *uuid.UUID `json:"target_organization_uuid,omitempty"`
}

// Input creates or updates an announcement.
type Input struct {
	DefaultLocale string          `json:"default_locale"`
	Title         string          `json:"title"`
	Body          string          `json:"body"`
	BodyFormat    string          `json:"body_format"`
	Pinned        bool            `json:"pinned"`
	Notify        bool            `json:"notify"`
	PublishAt     *time.Time      `json:"publish_at,omitempty"`
	ExpiresAt     *time.Time      `json:"expires_at,omitempty"`
	Audiences     []AudienceInput `json:"audiences"`
}

// LocaleInput upserts translated content.
type LocaleInput struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Audience is an API projection.
type Audience struct {
	TargetType             string     `json:"target_type"`
	RoleSlug               *string    `json:"role_slug,omitempty"`
	TargetOrganizationUUID *uuid.UUID `json:"target_organization_uuid,omitempty"`
}

// Announcement is the authoring/detail projection.
type Announcement struct {
	UUID          uuid.UUID  `json:"uuid"`
	DefaultLocale string     `json:"default_locale"`
	Locale        string     `json:"locale,omitempty"`
	Title         string     `json:"title"`
	Body          string     `json:"body"`
	BodyFormat    string     `json:"body_format"`
	Status        string     `json:"status"`
	Pinned        bool       `json:"pinned"`
	Notify        bool       `json:"notify"`
	ReadAt        *time.Time `json:"read_at,omitempty"`
	PublishAt     *time.Time `json:"publish_at,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	Audiences     []Audience `json:"audiences,omitempty"`
}

// ReadReport is the author read receipt view.
type ReadReport struct {
	TargetTotal int64      `json:"target_total"`
	ReadTotal   int64      `json:"read_total"`
	ReadRate    float64    `json:"read_rate"`
	Items       []ReadItem `json:"items"`
}

// ReadItem is one read receipt.
type ReadItem struct {
	UserUUID uuid.UUID `json:"user_uuid"`
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Surname  string    `json:"surname"`
	ReadAt   time.Time `json:"read_at"`
}

// Service coordinates announcement persistence and publication events.
type Service struct {
	pool TxBeginner
	q    *db.Queries
	out  outbox.Enqueuer
	now  func() time.Time
}

// New creates the service.
func New(pool TxBeginner, q *db.Queries, out outbox.Enqueuer) *Service {
	return &Service{pool: pool, q: q, out: out, now: func() time.Time { return time.Now().UTC() }}
}

func ptrTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func pgTime(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func pgInt8(v int64) pgtype.Int8 {
	return pgtype.Int8{Int64: v, Valid: v != 0}
}

func textPtr(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

func normalizeLocale(v string) (string, error) {
	v = strings.TrimSpace(v)
	switch v {
	case "tr", "en", "bg", "de", "el", "uk", "ru", "fr", "es", "it", "zh-CN", "az", "ar":
		return v, nil
	case "zh_CN":
		return "zh-CN", nil
	default:
		return "", invalid("locale", "is not supported")
	}
}

func cleanText(field, v string, max int, required bool) (string, error) {
	v = strings.TrimSpace(v)
	if required && v == "" {
		return "", invalid(field, "is required")
	}
	if utf8.RuneCountInString(v) > max {
		return "", invalid(field, fmt.Sprintf("must be at most %d characters", max))
	}
	return v, nil
}

func validateInput(in *Input, requireAudiences bool) error {
	var err error
	if in.DefaultLocale == "" {
		in.DefaultLocale = "tr"
	}
	if in.DefaultLocale, err = normalizeLocale(in.DefaultLocale); err != nil {
		return err
	}
	if in.Title, err = cleanText("title", in.Title, maxTitle, true); err != nil {
		return err
	}
	if in.Body, err = cleanText("body", in.Body, maxBody, true); err != nil {
		return err
	}
	if in.BodyFormat == "" {
		in.BodyFormat = BodyMarkdown
	}
	if in.BodyFormat != BodyMarkdown && in.BodyFormat != BodyHTML {
		return invalid("body_format", "must be markdown or html")
	}
	if in.ExpiresAt != nil && in.PublishAt != nil && !in.ExpiresAt.After(*in.PublishAt) {
		return invalid("expires_at", "must be after publish_at")
	}
	if requireAudiences && len(in.Audiences) == 0 {
		return invalid("audiences", "must include at least one target")
	}
	for i := range in.Audiences {
		a := &in.Audiences[i]
		a.TargetType = strings.TrimSpace(a.TargetType)
		switch a.TargetType {
		case TargetAllNetwork, TargetDistributors, TargetDealers:
			if a.TargetOrganizationUUID != nil {
				return invalid("audiences", "target_organization_uuid is only valid for subtree")
			}
		case TargetRole:
			if a.RoleSlug == nil || strings.TrimSpace(*a.RoleSlug) == "" {
				return invalid("audiences.role_slug", "is required for role targets")
			}
			if a.TargetOrganizationUUID != nil {
				return invalid("audiences", "target_organization_uuid is only valid for subtree")
			}
		case TargetSubtree:
			if a.TargetOrganizationUUID == nil || *a.TargetOrganizationUUID == uuid.Nil {
				return invalid("audiences.target_organization_uuid", "is required for subtree targets")
			}
		default:
			return invalid("audiences.target_type", "is invalid")
		}
		if a.RoleSlug != nil {
			s := strings.TrimSpace(*a.RoleSlug)
			a.RoleSlug = &s
		}
	}
	return nil
}

func (s *Service) requireWriter(c Caller) error {
	if c.Org.BrandID == 0 || c.Org.InternalID == 0 {
		return ErrForbidden
	}
	if c.Org.OrgType != "center" && c.Org.OrgType != "distributor" {
		return ErrForbidden
	}
	return nil
}

func (s *Service) addAudiences(ctx context.Context, q *db.Queries, announcementID int64, audiences []AudienceInput) error {
	for _, a := range audiences {
		arg := db.AddAnnouncementAudienceParams{AnnouncementID: announcementID, TargetType: a.TargetType}
		if a.RoleSlug != nil {
			arg.RoleSlug = pgtype.Text{String: *a.RoleSlug, Valid: true}
		}
		if a.TargetOrganizationUUID != nil {
			o, err := q.GetOrganizationByUUID(ctx, *a.TargetOrganizationUUID)
			if err != nil {
				return invalid("audiences.target_organization_uuid", "is unknown")
			}
			arg.TargetOrganizationID = pgInt8(o.ID)
		}
		if _, err := q.AddAnnouncementAudience(ctx, arg); err != nil {
			return err
		}
	}
	return nil
}

// Create stores a draft announcement.
func (s *Service) Create(ctx context.Context, c Caller, in Input) (Announcement, error) {
	if err := s.requireWriter(c); err != nil {
		return Announcement{}, err
	}
	if err := validateInput(&in, true); err != nil {
		return Announcement{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Announcement{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	row, err := q.CreateAnnouncement(ctx, db.CreateAnnouncementParams{
		OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, DefaultLocale: in.DefaultLocale,
		Title: in.Title, Body: in.Body, BodyFormat: in.BodyFormat, Status: StatusDraft,
		Pinned: in.Pinned, Notify: in.Notify, PublishAt: pgTime(in.PublishAt), ExpiresAt: pgTime(in.ExpiresAt),
		AuthorUserID: pgInt8(c.UserID),
	})
	if err != nil {
		return Announcement{}, err
	}
	if err := s.addAudiences(ctx, q, row.ID, in.Audiences); err != nil {
		return Announcement{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Announcement{}, err
	}
	return s.GetAuthor(ctx, c, row.Uuid)
}

// Update changes editable fields and replaces audiences while the author owns the row.
func (s *Service) Update(ctx context.Context, c Caller, id uuid.UUID, in Input) (Announcement, error) {
	if err := s.requireWriter(c); err != nil {
		return Announcement{}, err
	}
	if err := validateInput(&in, true); err != nil {
		return Announcement{}, err
	}
	existing, err := s.q.GetAnnouncementByUUID(ctx, id)
	if err != nil || existing.OrganizationID != c.Org.InternalID {
		return Announcement{}, ErrNotFound
	}
	if existing.Status == StatusPublished {
		return Announcement{}, ErrConflict
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Announcement{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	row, err := q.UpdateAnnouncement(ctx, db.UpdateAnnouncementParams{
		ID: existing.ID, OrganizationID: c.Org.InternalID, DefaultLocale: in.DefaultLocale,
		Title: in.Title, Body: in.Body, BodyFormat: in.BodyFormat, Pinned: in.Pinned,
		Notify: in.Notify, PublishAt: pgTime(in.PublishAt), ExpiresAt: pgTime(in.ExpiresAt),
	})
	if err != nil {
		return Announcement{}, err
	}
	if err := q.DeleteAnnouncementAudiences(ctx, row.ID); err != nil {
		return Announcement{}, err
	}
	if err := s.addAudiences(ctx, q, row.ID, in.Audiences); err != nil {
		return Announcement{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Announcement{}, err
	}
	return s.GetAuthor(ctx, c, row.Uuid)
}

// UpsertLocale stores translated content.
func (s *Service) UpsertLocale(ctx context.Context, c Caller, id uuid.UUID, locale string, in LocaleInput) (Announcement, error) {
	if err := s.requireWriter(c); err != nil {
		return Announcement{}, err
	}
	var err error
	if locale, err = normalizeLocale(locale); err != nil {
		return Announcement{}, err
	}
	if in.Title, err = cleanText("title", in.Title, maxTitle, true); err != nil {
		return Announcement{}, err
	}
	if in.Body, err = cleanText("body", in.Body, maxBody, true); err != nil {
		return Announcement{}, err
	}
	a, err := s.q.GetAnnouncementByUUID(ctx, id)
	if err != nil || a.OrganizationID != c.Org.InternalID {
		return Announcement{}, ErrNotFound
	}
	if _, err := s.q.UpsertAnnouncementLocale(ctx, db.UpsertAnnouncementLocaleParams{
		AnnouncementID: a.ID, Locale: locale, Title: in.Title, Body: in.Body,
	}); err != nil {
		return Announcement{}, err
	}
	return s.GetAuthor(ctx, c, id)
}

// Publish marks a draft as published and writes at most one outbox event.
func (s *Service) Publish(ctx context.Context, c Caller, id uuid.UUID) (Announcement, error) {
	if err := s.requireWriter(c); err != nil {
		return Announcement{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Announcement{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	a, err := q.GetAnnouncementByUUID(ctx, id)
	if err != nil || a.OrganizationID != c.Org.InternalID {
		return Announcement{}, ErrNotFound
	}
	if a.Status == StatusPublished {
		return Announcement{}, ErrConflict
	}
	row, err := q.SetAnnouncementStatus(ctx, db.SetAnnouncementStatusParams{ID: a.ID, OrganizationID: c.Org.InternalID, Status: StatusPublished})
	if err != nil {
		return Announcement{}, err
	}
	if row.Notify && s.out != nil {
		ev := events.New(EventPublished).
			WithTenant(c.Org.InternalID).
			WithActor(c.UserID).
			WithEntity("announcement", &row.ID, &row.Uuid).
			WithPayload(map[string]any{
				"announcement_id":   row.ID,
				"announcement_uuid": row.Uuid.String(),
				"brand_id":          row.BrandID,
			})
		if err := s.out.Enqueue(ctx, tx, ev); err != nil {
			return Announcement{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Announcement{}, err
	}
	return s.GetAuthor(ctx, c, id)
}

// Archive marks an announcement archived.
func (s *Service) Archive(ctx context.Context, c Caller, id uuid.UUID) (Announcement, error) {
	if err := s.requireWriter(c); err != nil {
		return Announcement{}, err
	}
	a, err := s.q.GetAnnouncementByUUID(ctx, id)
	if err != nil || a.OrganizationID != c.Org.InternalID {
		return Announcement{}, ErrNotFound
	}
	if _, err := s.q.SetAnnouncementStatus(ctx, db.SetAnnouncementStatusParams{ID: a.ID, OrganizationID: c.Org.InternalID, Status: StatusArchived}); err != nil {
		return Announcement{}, err
	}
	return s.GetAuthor(ctx, c, id)
}

// Pin toggles the pinned flag without changing content.
func (s *Service) Pin(ctx context.Context, c Caller, id uuid.UUID, pinned bool) (Announcement, error) {
	if err := s.requireWriter(c); err != nil {
		return Announcement{}, err
	}
	a, err := s.q.GetAnnouncementByUUID(ctx, id)
	if err != nil || a.OrganizationID != c.Org.InternalID {
		return Announcement{}, ErrNotFound
	}
	row, err := s.q.UpdateAnnouncement(ctx, db.UpdateAnnouncementParams{
		ID: a.ID, OrganizationID: c.Org.InternalID, DefaultLocale: a.DefaultLocale, Title: a.Title,
		Body: a.Body, BodyFormat: a.BodyFormat, Pinned: pinned, Notify: a.Notify,
		PublishAt: a.PublishAt, ExpiresAt: a.ExpiresAt,
	})
	if err != nil {
		return Announcement{}, err
	}
	return s.GetAuthor(ctx, c, row.Uuid)
}

func (s *Service) viewerParams(ctx context.Context, c Caller, locale string, unreadOnly bool, limit, offset int32) (db.ListVisibleAnnouncementsParams, error) {
	if c.Org.BrandID == 0 || c.Org.InternalID == 0 {
		return db.ListVisibleAnnouncementsParams{}, ErrForbidden
	}
	if locale == "" {
		locale = "tr"
	}
	var err error
	if locale, err = normalizeLocale(locale); err != nil {
		return db.ListVisibleAnnouncementsParams{}, err
	}
	lineage, err := s.q.OrganizationLineage(ctx, c.Org.InternalID)
	if err != nil {
		return db.ListVisibleAnnouncementsParams{}, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return db.ListVisibleAnnouncementsParams{
		Locale: locale, UserID: c.UserID, BrandID: c.Org.BrandID, Now: pgtype.Timestamptz{Time: s.now(), Valid: true},
		ViewerRoleSlugs: c.Roles, ViewerOrgType: c.Org.OrgType, ViewerOrgLineage: lineage,
		UnreadOnly: unreadOnly, PageLimit: limit, PageOffset: offset,
	}, nil
}

func countParams(arg db.ListVisibleAnnouncementsParams) db.CountVisibleAnnouncementsParams {
	return db.CountVisibleAnnouncementsParams{
		UserID: arg.UserID, BrandID: arg.BrandID, Now: arg.Now,
		ViewerRoleSlugs: arg.ViewerRoleSlugs, ViewerOrgType: arg.ViewerOrgType,
		ViewerOrgLineage: arg.ViewerOrgLineage, UnreadOnly: arg.UnreadOnly,
	}
}

// List returns visible announcements for the active organization.
func (s *Service) List(ctx context.Context, c Caller, locale string, unreadOnly bool, limit, offset int32) ([]Announcement, int64, error) {
	arg, err := s.viewerParams(ctx, c, locale, unreadOnly, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.q.ListVisibleAnnouncements(ctx, arg)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountVisibleAnnouncements(ctx, countParams(arg))
	if err != nil {
		return nil, 0, err
	}
	out := make([]Announcement, 0, len(rows))
	for _, r := range rows {
		out = append(out, Announcement{
			UUID: r.Uuid, DefaultLocale: r.DefaultLocale, Locale: r.Locale, Title: r.Title, Body: r.Body,
			BodyFormat: r.BodyFormat, Status: StatusPublished, Pinned: r.Pinned, ReadAt: ptrTime(r.ReadAt),
			PublishAt: ptrTime(r.PublishAt), ExpiresAt: ptrTime(r.ExpiresAt),
		})
	}
	return out, total, nil
}

// UnreadCount returns the nav badge count.
func (s *Service) UnreadCount(ctx context.Context, c Caller, locale string) (int64, error) {
	arg, err := s.viewerParams(ctx, c, locale, true, 1, 0)
	if err != nil {
		return 0, err
	}
	return s.q.CountVisibleAnnouncements(ctx, countParams(arg))
}

// GetVisible returns one visible announcement and marks it read.
func (s *Service) GetVisible(ctx context.Context, c Caller, id uuid.UUID, locale string) (Announcement, error) {
	items, _, err := s.List(ctx, c, locale, false, 100, 0)
	if err != nil {
		return Announcement{}, err
	}
	for _, a := range items {
		if a.UUID == id {
			row, err := s.q.GetAnnouncementByUUID(ctx, id)
			if err != nil {
				return Announcement{}, ErrNotFound
			}
			read, err := s.q.MarkAnnouncementRead(ctx, db.MarkAnnouncementReadParams{AnnouncementID: row.ID, UserID: c.UserID})
			if err != nil {
				return Announcement{}, err
			}
			a.ReadAt = ptrTime(read.ReadAt)
			return a, nil
		}
	}
	return Announcement{}, ErrNotFound
}

// GetAuthor returns one announcement written by the caller's organization.
func (s *Service) GetAuthor(ctx context.Context, c Caller, id uuid.UUID) (Announcement, error) {
	a, err := s.q.GetAnnouncementByUUID(ctx, id)
	if err != nil || a.OrganizationID != c.Org.InternalID {
		return Announcement{}, ErrNotFound
	}
	auds, err := s.q.ListAnnouncementAudiences(ctx, a.ID)
	if err != nil {
		return Announcement{}, err
	}
	out := Announcement{
		UUID: a.Uuid, DefaultLocale: a.DefaultLocale, Title: a.Title, Body: a.Body, BodyFormat: a.BodyFormat,
		Status: a.Status, Pinned: a.Pinned, Notify: a.Notify, PublishAt: ptrTime(a.PublishAt),
		ExpiresAt: ptrTime(a.ExpiresAt), CreatedAt: a.CreatedAt.Time, UpdatedAt: a.UpdatedAt.Time,
		Audiences: make([]Audience, 0, len(auds)),
	}
	for _, au := range auds {
		item := Audience{TargetType: au.TargetType, RoleSlug: textPtr(au.RoleSlug)}
		if au.TargetOrganizationID.Valid {
			if org, err := s.q.GetOrganizationByID(ctx, au.TargetOrganizationID.Int64); err == nil {
				item.TargetOrganizationUUID = &org.Uuid
			}
		}
		out.Audiences = append(out.Audiences, item)
	}
	return out, nil
}

// Reads returns author read receipts and target total.
func (s *Service) Reads(ctx context.Context, c Caller, id uuid.UUID, limit, offset int32) (ReadReport, error) {
	if err := s.requireWriter(c); err != nil {
		return ReadReport{}, err
	}
	a, err := s.q.GetAnnouncementByUUID(ctx, id)
	if err != nil || a.OrganizationID != c.Org.InternalID {
		return ReadReport{}, ErrNotFound
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	targets, err := s.q.ListAnnouncementTargetUserIDs(ctx, a.ID)
	if err != nil {
		return ReadReport{}, err
	}
	readTotal, err := s.q.CountAnnouncementReads(ctx, a.ID)
	if err != nil {
		return ReadReport{}, err
	}
	rows, err := s.q.ListAnnouncementReadReport(ctx, db.ListAnnouncementReadReportParams{
		AnnouncementID: a.ID, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return ReadReport{}, err
	}
	out := ReadReport{TargetTotal: int64(len(targets)), ReadTotal: readTotal}
	if out.TargetTotal > 0 {
		out.ReadRate = float64(out.ReadTotal) / float64(out.TargetTotal)
	}
	for _, r := range rows {
		out.Items = append(out.Items, ReadItem{
			UserUUID: r.UserUuid, Email: r.Email.String, Name: r.Name, Surname: r.Surname, ReadAt: r.ReadAt.Time,
		})
	}
	return out, nil
}

// EnqueuePublishedBatches resolves targets for an announcement.published event
// and enqueues one notification dispatch task per 500-user batch.
func EnqueuePublishedBatches(
	ctx context.Context,
	q TargetResolver,
	enq TaskEnqueuer,
	dispatcher Dispatcher,
	ev events.Event,
) error {
	if q == nil {
		return errors.New("announcements: target resolver is nil")
	}
	announcementID, ok := announcementIDFromEvent(ev)
	if !ok {
		return errors.New("announcements: announcement id missing from event")
	}
	a, err := q.GetAnnouncementByID(ctx, announcementID)
	if err != nil {
		return err
	}
	userIDs, err := q.ListAnnouncementTargetUserIDs(ctx, a.ID)
	if err != nil {
		return err
	}
	var orgID *int64
	if ev.TenantID != nil {
		v := *ev.TenantID
		orgID = &v
	}
	errs := make([]error, 0)
	for batch, start := 0, 0; start < len(userIDs); batch, start = batch+1, start+notificationBatchSize {
		end := start + notificationBatchSize
		if end > len(userIDs) {
			end = len(userIDs)
		}
		payload := queue.AnnouncementDispatchPayload{
			EventID:          ev.EventID,
			AnnouncementID:   a.ID,
			AnnouncementUUID: a.Uuid,
			BrandID:          a.BrandID,
			OrganizationID:   orgID,
			Title:            a.Title,
			Body:             a.Body,
			UserIDs:          append([]int64{}, userIDs[start:end]...),
			Batch:            batch,
		}
		if enq == nil {
			errs = append(errs, DispatchBatch(ctx, dispatcher, payload))
			continue
		}
		task, err := queue.NewAnnouncementDispatchTask(payload)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		_, err = enq.Enqueue(
			task,
			asynq.Queue(queue.QueueNotifications),
			asynq.TaskID(fmt.Sprintf("announcement:%s:%d", ev.EventID.String(), batch)),
		)
		if err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// DispatchBatch sends one already-resolved announcement notification batch.
func DispatchBatch(ctx context.Context, dispatcher Dispatcher, payload queue.AnnouncementDispatchPayload) error {
	if dispatcher == nil {
		return errors.New("announcements: dispatcher is nil")
	}
	brandID := payload.BrandID
	_, err := dispatcher.Dispatch(ctx, model.DispatchInput{
		EventID:        payload.EventID,
		EventCode:      "announcements.published",
		OrganizationID: payload.OrganizationID,
		BrandID:        &brandID,
		UserIDs:        payload.UserIDs,
		Channels:       []string{model.ChannelInapp, model.ChannelEmail, model.ChannelWebPush, model.ChannelExpoPush},
		Vars:           map[string]string{"title": payload.Title, "body": payload.Body},
		Payload:        map[string]any{"announcement_uuid": payload.AnnouncementUUID.String()},
	})
	return err
}

func announcementIDFromEvent(ev events.Event) (int64, bool) {
	if ev.EntityID != nil && *ev.EntityID > 0 {
		return *ev.EntityID, true
	}
	switch v := ev.Payload["announcement_id"].(type) {
	case int64:
		return v, v > 0
	case float64:
		n := int64(v)
		return n, n > 0
	case int:
		return int64(v), v > 0
	default:
		return 0, false
	}
}
