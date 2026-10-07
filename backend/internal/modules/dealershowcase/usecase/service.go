// Package usecase is the dealer showcase API (TEC-467, F5-01b): the panel
// editor of an organization's own showcase (or, with ?org=, a dealer of the
// caller's scope), gallery photos, the submit / center review flow and the
// public block of /bayi/{code}.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealershowcase/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Error codes answered by the handler.
const (
	CodePhotoLimit         = "SHOWCASE_PHOTO_LIMIT"
	CodeReviewNoteRequired = "SHOWCASE_REVIEW_NOTE_REQUIRED"
	CodeInvalidTransition  = "INVALID_TRANSITION"
	CodeUnsupportedMedia   = "UNSUPPORTED_MEDIA_TYPE"
)

var (
	// ErrNotFound: no such organization, showcase, service or photo in the
	// caller's scope (404).
	ErrNotFound = errors.New("dealershowcase: not found")
	// ErrFeatureDisabled: the target organization does not have the
	// dealer_showcase module (403 FEATURE_DISABLED).
	ErrFeatureDisabled = errors.New("dealershowcase: module disabled for the organization")
	// ErrInvalidTransition: the status does not allow the move (409).
	ErrInvalidTransition = errors.New("dealershowcase: status transition not allowed")
	// ErrReviewNoteRequired: a rejection without a note (422).
	ErrReviewNoteRequired = errors.New("dealershowcase: a rejection needs a note")
	// ErrDuplicatePhoto: the same image is already in the gallery (409).
	ErrDuplicatePhoto = errors.New("dealershowcase: photo already in the gallery")
)

// ValidationError is one invalid input field (400).
type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string {
	return "dealershowcase: invalid " + e.Field + ": " + e.Message
}

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// TxBeginner opens transactions (pool or an outer test transaction).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// FeatureChecker answers whether a module is on for an organization
// (features.Service).
type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

// Settings reads system settings (sysconfig.Service).
type Settings interface {
	Bool(ctx context.Context, key string) bool
	Int(ctx context.Context, key string) int64
}

// Storage removes gallery objects (S3 / SeaweedFS).
type Storage interface {
	Delete(ctx context.Context, path string) error
}

// Outbox writes events in the caller's transaction (outbox.Store).
type Outbox interface {
	Enqueue(ctx context.Context, tx pgx.Tx, ev events.Event) error
}

// Service is the dealer showcase use case.
type Service struct {
	db       TxBeginner
	q        *db.Queries
	features FeatureChecker
	settings Settings
	out      Outbox
	storage  Storage
	now      func() time.Time
}

// SetStorage sets the gallery store; objects that left both the gallery
// and the live snapshot are removed after commit (nil: kept).
func (s *Service) SetStorage(st Storage) { s.storage = st }

// dropObjects removes storage objects after a commit (best effort).
func (s *Service) dropObjects(ctx context.Context, keys []string) {
	if s.storage == nil {
		return
	}
	for _, k := range keys {
		_ = s.storage.Delete(ctx, k)
	}
}

// New creates the service. features, settings and out may be nil (tests):
// then every module is on, the sysconfig defaults apply and no event is
// written.
func New(conn TxBeginner, q *db.Queries, features FeatureChecker, settings Settings, out Outbox) *Service {
	return &Service{db: conn, q: q, features: features, settings: settings, out: out, now: time.Now}
}

// Caller is the authenticated user in the active organization with the
// resolved scope of the route permission (showcase.read / showcase.write /
// platform.showcase.review).
type Caller struct {
	UserID         int64
	OrganizationID int64
	BrandID        int64
	Filter         scopefilter.Filter
}

func (c Caller) actor() pgtype.Int8 {
	return pgtype.Int8{Int64: c.UserID, Valid: c.UserID != 0}
}

func (c Caller) actorPtr() *int64 {
	if c.UserID == 0 {
		return nil
	}
	id := c.UserID
	return &id
}

func (s *Service) approvalRequired(ctx context.Context) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.Bool(ctx, sysconfig.KeyShowcaseApprovalRequired)
}

func (s *Service) maxPhotos(ctx context.Context) int64 {
	if s.settings == nil {
		return sysconfig.DefaultShowcaseMaxPhotos
	}
	if n := s.settings.Int(ctx, sysconfig.KeyShowcaseMaxPhotos); n > 0 {
		return n
	}
	return sysconfig.DefaultShowcaseMaxPhotos
}

func (s *Service) moduleOn(ctx context.Context, orgID int64) (bool, error) {
	if s.features == nil {
		return true, nil
	}
	return s.features.Enabled(ctx, orgID, features.ModuleDealerShowcase)
}

// target resolves the organization whose showcase the caller works on:
// its own organization, or with orgUUID (?org=) a dealer or distributor of
// the brand inside the caller's permission scope. Anything outside the
// scope is ErrNotFound; a target without the module is ErrFeatureDisabled
// (the caller's own module is checked by RequireFeature).
func (s *Service) target(ctx context.Context, q *db.Queries, c Caller, orgUUID *uuid.UUID) (db.Organization, error) {
	var (
		o   db.Organization
		err error
	)
	if orgUUID == nil {
		o, err = q.GetOrganizationByID(ctx, c.OrganizationID)
	} else {
		o, err = q.GetOrganizationByUUID(ctx, *orgUUID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, ErrNotFound
	}
	if err != nil {
		return db.Organization{}, err
	}
	if o.DeletedAt.Valid || o.BrandID != c.BrandID || !c.Filter.AllowsOrg(o.ID, o.BrandID) ||
		(o.Type != rbac.OrgTypeDealer && o.Type != rbac.OrgTypeDistributor) {
		return db.Organization{}, ErrNotFound
	}
	if o.ID != c.OrganizationID {
		on, err := s.moduleOn(ctx, o.ID)
		if err != nil {
			return db.Organization{}, err
		}
		if !on {
			return db.Organization{}, ErrFeatureDisabled
		}
	}
	return o, nil
}

// showcaseOf returns the organization's showcase; ok is false when it has
// none yet.
func showcaseOf(ctx context.Context, q *db.Queries, orgID int64) (db.DealerShowcase, bool, error) {
	row, err := q.GetDealerShowcaseByOrg(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.DealerShowcase{}, false, nil
	}
	if err != nil {
		return db.DealerShowcase{}, false, err
	}
	return row, true, nil
}

// ensure opens an empty draft showcase when the organization has none.
func ensure(ctx context.Context, q *db.Queries, c Caller, o db.Organization) (db.DealerShowcase, error) {
	row, err := q.EnsureDealerShowcase(ctx, db.EnsureDealerShowcaseParams{
		OrganizationID: o.ID, BrandID: o.BrandID, ActorUserID: c.actor(),
	})
	return db.DealerShowcase(row), err
}

// inTx runs fn in a transaction.
func (s *Service) inTx(ctx context.Context, fn func(tx pgx.Tx, q *db.Queries) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx, s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Get returns the showcase editor view; an organization without a showcase
// gets an empty draft view (uuid null).
func (s *Service) Get(ctx context.Context, c Caller, orgUUID *uuid.UUID) (Showcase, error) {
	o, err := s.target(ctx, s.q, c, orgUUID)
	if err != nil {
		return Showcase{}, err
	}
	row, ok, err := showcaseOf(ctx, s.q, o.ID)
	if err != nil {
		return Showcase{}, err
	}
	return s.view(ctx, s.q, o, row, ok)
}

// Save replaces the draft content (content, working hours, social links,
// SEO keywords, Google place id). The status and the published snapshot are
// left alone: a draft edit never changes the live page.
func (s *Service) Save(ctx context.Context, c Caller, orgUUID *uuid.UUID, in Input) (Showcase, error) {
	p, err := in.params()
	if err != nil {
		return Showcase{}, err
	}
	o, err := s.target(ctx, s.q, c, orgUUID)
	if err != nil {
		return Showcase{}, err
	}
	p.OrganizationID, p.BrandID, p.ActorUserID = o.ID, o.BrandID, c.actor()
	row, err := s.q.UpsertDealerShowcase(ctx, p)
	if err != nil {
		return Showcase{}, err
	}
	return s.view(ctx, s.q, o, row, true)
}

// Submit sends the draft live. With showcase.approval_required off it
// publishes directly (status published, snapshot written); with it on the
// showcase waits in pending_review and the brand center's reviewers are
// notified.
func (s *Service) Submit(ctx context.Context, c Caller, orgUUID *uuid.UUID) (Showcase, error) {
	approval := s.approvalRequired(ctx)
	var (
		o      db.Organization
		out    db.DealerShowcase
		orphan []string
	)
	err := s.inTx(ctx, func(tx pgx.Tx, q *db.Queries) error {
		var err error
		if o, err = s.target(ctx, q, c, orgUUID); err != nil {
			return err
		}
		row, err := ensure(ctx, q, c, o)
		if err != nil {
			return err
		}
		if row, err = q.LockDealerShowcase(ctx, row.ID); err != nil {
			return err
		}
		store := repository.FromQueries(q)
		if approval {
			if out, err = store.Submit(ctx, row.ID, row.Status, c.actorPtr()); err != nil {
				return mapRepoErr(err)
			}
			center, err := q.GetBrandCenter(ctx, o.BrandID)
			if err != nil {
				return err
			}
			ids, err := q.ListTransferNotifyUserIDs(ctx, db.ListTransferNotifyUserIDsParams{
				OrganizationID: center.ID, PermissionSlug: rbac.PermPlatformShowcaseReview,
			})
			if err != nil {
				return err
			}
			return s.emit(ctx, tx, events.ShowcaseReviewRequested, c, o, out, "", ids)
		}
		var snap []byte
		if snap, orphan, err = buildSnapshot(ctx, q, row); err != nil {
			return err
		}
		if out, err = store.Publish(ctx, row.ID, row.Status, snap, nil, c.actorPtr()); err != nil {
			return mapRepoErr(err)
		}
		return s.emit(ctx, tx, events.ShowcasePublished, c, o, out, "", nil)
	})
	if err != nil {
		return Showcase{}, err
	}
	s.dropObjects(ctx, orphan)
	return s.view(ctx, s.q, o, out, true)
}

func mapRepoErr(err error) error {
	switch {
	case errors.Is(err, repository.ErrInvalidTransition), errors.Is(err, repository.ErrStatusConflict):
		return ErrInvalidTransition
	case errors.Is(err, repository.ErrReorderMismatch):
		return invalid("uuids", "must list every item exactly once")
	}
	return err
}

// emit writes a showcase.* outbox event; userIDs are notified.
func (s *Service) emit(ctx context.Context, tx pgx.Tx, name string, c Caller, o db.Organization,
	row db.DealerShowcase, reason string, userIDs []int64) error {
	if s.out == nil {
		return nil
	}
	if userIDs == nil {
		userIDs = []int64{}
	}
	id, uid := row.ID, row.Uuid
	ev := events.New(name).WithTenant(o.ID).WithEntity("dealer_showcase", &id, &uid).WithPayload(map[string]any{
		"organization_uuid": o.Uuid.String(), "organization_id": o.ID, "organization_name": o.Name,
		"brand_id": o.BrandID, "status": row.Status, "reason": reason, "notify_user_ids": userIDs,
	})
	if c.UserID != 0 {
		ev = ev.WithActor(c.UserID)
	}
	if err := s.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("dealershowcase: outbox: %w", err)
	}
	return nil
}

// snapshot is published_content: everything the public page shows, frozen
// at publish time. Photos keep their storage key and type so the public
// file endpoint serves them from the snapshot even after a later draft
// removes them from the gallery.
type snapshot struct {
	Content      json.RawMessage   `json:"content"`
	WorkingHours json.RawMessage   `json:"working_hours"`
	SocialLinks  json.RawMessage   `json:"social_links"`
	SeoKeywords  []string          `json:"seo_keywords"`
	Services     []snapshotService `json:"services"`
	Photos       []snapshotPhoto   `json:"photos"`
}

type snapshotService struct {
	UUID         uuid.UUID       `json:"uuid"`
	Kind         string          `json:"kind"`
	CategoryName string          `json:"category_name,omitempty"`
	Title        json.RawMessage `json:"title"`
	Description  json.RawMessage `json:"description"`
}

type snapshotPhoto struct {
	UUID       uuid.UUID       `json:"uuid"`
	StorageKey string          `json:"storage_key"`
	Mime       string          `json:"mime"`
	Caption    json.RawMessage `json:"caption"`
}

// buildSnapshot freezes the draft of row (visible services and the
// gallery in their order). It also returns the storage keys of the
// previous snapshot that are no longer in the gallery: once the new
// snapshot is committed nothing serves them.
func buildSnapshot(ctx context.Context, q *db.Queries, row db.DealerShowcase) ([]byte, []string, error) {
	snap := snapshot{
		Content: row.Content, WorkingHours: row.WorkingHours, SocialLinks: row.SocialLinks,
		SeoKeywords: row.SeoKeywords, Services: []snapshotService{}, Photos: []snapshotPhoto{},
	}
	if snap.SeoKeywords == nil {
		snap.SeoKeywords = []string{}
	}
	services, err := q.ListDealerShowcaseServices(ctx, row.ID)
	if err != nil {
		return nil, nil, err
	}
	for _, sv := range services {
		if !sv.Visible {
			continue
		}
		item := snapshotService{UUID: sv.Uuid, Kind: sv.Kind, Title: sv.Title, Description: sv.Description}
		if sv.CategoryID.Valid {
			cat, err := q.GetProductCategory(ctx, db.GetProductCategoryParams{ID: sv.CategoryID.Int64, BrandID: row.BrandID})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, nil, err
			}
			item.CategoryName = cat.Name
		}
		snap.Services = append(snap.Services, item)
	}
	photos, err := q.ListDealerShowcasePhotos(ctx, row.ID)
	if err != nil {
		return nil, nil, err
	}
	for _, p := range photos {
		snap.Photos = append(snap.Photos, snapshotPhoto{UUID: p.Uuid, StorageKey: p.StorageKey, Mime: p.Mime, Caption: p.Caption})
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return nil, nil, err
	}
	prev, err := parseSnapshot(row.PublishedContent)
	if err != nil || prev == nil {
		return raw, nil, err
	}
	live := make(map[string]bool, len(photos))
	for _, p := range photos {
		live[p.StorageKey] = true
	}
	var orphan []string
	for _, p := range prev.Photos {
		if !live[p.StorageKey] {
			orphan = append(orphan, p.StorageKey)
		}
	}
	return raw, orphan, nil
}

func parseSnapshot(raw []byte) (*snapshot, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var s snapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("dealershowcase: published_content: %w", err)
	}
	return &s, nil
}

// snapshotHasKey reports whether the published snapshot still serves key.
func snapshotHasKey(raw []byte, key string) bool {
	s, err := parseSnapshot(raw)
	if err != nil || s == nil {
		return false
	}
	for _, p := range s.Photos {
		if p.StorageKey == key {
			return true
		}
	}
	return false
}
