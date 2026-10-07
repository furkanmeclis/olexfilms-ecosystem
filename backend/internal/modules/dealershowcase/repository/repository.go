// Package repository wraps the dealer showcase persistence (TEC-466,
// F5-01a): the center review queue list contract, the status CAS moves and
// the gallery photo cap.
package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealershowcase/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	// ErrInvalidTransition: the status graph (model.Transitions) does not
	// allow the move.
	ErrInvalidTransition = errors.New("dealershowcase: status transition not allowed")
	// ErrStatusConflict: the showcase is no longer in the expected status
	// (the CAS updated no row).
	ErrStatusConflict = errors.New("dealershowcase: status changed meanwhile")
	// ErrReorderMismatch: the reorder list does not match the showcase's
	// items.
	ErrReorderMismatch = errors.New("dealershowcase: reorder list does not match the items")
)

// PhotoLimitError is returned when the gallery already holds Max photos.
type PhotoLimitError struct {
	Count int64
	Max   int64
}

func (e *PhotoLimitError) Error() string {
	return fmt.Sprintf("dealershowcase: gallery holds %d photos, at most %d allowed", e.Count, e.Max)
}

// Store is the dealer showcase repository.
type Store struct {
	q *db.Queries
}

// New creates a repository on a pool or a transaction.
func New(conn db.DBTX) *Store {
	return &Store{q: db.New(conn)}
}

// FromQueries creates a repository on an existing query set.
func FromQueries(q *db.Queries) *Store {
	return &Store{q: q}
}

// Queries returns the generated query set.
func (s *Store) Queries() *db.Queries { return s.q }

// ReviewSort is the sort contract of the center review queue: status sorts
// by flow rank (draft, pending_review, published, rejected).
var ReviewSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"updated_at": "updated_at", "name": "name", "status": "status"},
	Default: apiquery.SortField{Field: "updated_at", Desc: true},
}

// ReviewFilter selects review queue rows. Nil slices mean no filter;
// Updated.Before is exclusive (apiquery.DateRange). Statuses must already be
// validated against model.Statuses; OrganizationIDs narrows to a scope.
type ReviewFilter struct {
	OrganizationIDs []int64
	Statuses        []string
	Updated         apiquery.TimeRange
	Q               string
	Sort            []apiquery.SortField
	Limit, Offset   int32
}

// ListReviews returns a page of the brand's showcases and the total. An
// unknown sort field is an *apiquery.ValidationError; equal sort values are
// ordered by id in the sort direction.
func (s *Store) ListReviews(ctx context.Context, brandID int64, f ReviewFilter) ([]db.ListDealerShowcaseReviewsRow, int64, error) {
	sort, err := apiquery.ResolveSort(f.Sort, ReviewSort)
	if err != nil {
		return nil, 0, err
	}
	p := db.ListDealerShowcaseReviewsParams{
		BrandID: brandID, OrganizationIds: f.OrganizationIDs, Statuses: f.Statuses,
		Q:           textNarg(escapeLike(strings.TrimSpace(f.Q))),
		UpdatedFrom: tsNarg(f.Updated.From), UpdatedBefore: tsNarg(f.Updated.Before),
		SortKey: sort.Key, SortDesc: sort.Desc, PageLimit: f.Limit, PageOffset: f.Offset,
	}
	rows, err := s.q.ListDealerShowcaseReviews(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountDealerShowcaseReviews(ctx, db.CountDealerShowcaseReviewsParams{
		BrandID: p.BrandID, OrganizationIds: p.OrganizationIds, Statuses: p.Statuses, Q: p.Q,
		UpdatedFrom: p.UpdatedFrom, UpdatedBefore: p.UpdatedBefore,
	})
	return rows, total, err
}

// Submit moves the showcase from → pending_review.
func (s *Store) Submit(ctx context.Context, id int64, from string, actorUserID *int64) (db.DealerShowcase, error) {
	if !model.CanTransition(from, model.StatusPendingReview) {
		return db.DealerShowcase{}, ErrInvalidTransition
	}
	return cas(s.q.SubmitDealerShowcase(ctx, db.SubmitDealerShowcaseParams{
		ID: id, FromStatus: from, ActorUserID: int8Narg(actorUserID),
	}))
}

// Publish moves the showcase from → published and stores the snapshot the
// public endpoint reads. reviewerUserID is the center reviewer (nil when
// the owner publishes with approval off).
func (s *Store) Publish(ctx context.Context, id int64, from string, snapshot []byte, reviewerUserID, actorUserID *int64) (db.DealerShowcase, error) {
	if !model.CanTransition(from, model.StatusPublished) {
		return db.DealerShowcase{}, ErrInvalidTransition
	}
	return cas(s.q.PublishDealerShowcase(ctx, db.PublishDealerShowcaseParams{
		ID: id, FromStatus: from, PublishedContent: snapshot,
		ReviewerUserID: int8Narg(reviewerUserID), ActorUserID: int8Narg(actorUserID),
	}))
}

// Reject moves a pending showcase to rejected with the reviewer's note.
func (s *Store) Reject(ctx context.Context, id, reviewerUserID int64, note string) (db.DealerShowcase, error) {
	return cas(s.q.RejectDealerShowcase(ctx, db.RejectDealerShowcaseParams{
		ID: id, ReviewerUserID: pgtype.Int8{Int64: reviewerUserID, Valid: true},
		ReviewNote: pgtype.Text{String: note, Valid: true},
	}))
}

func cas(row db.DealerShowcase, err error) (db.DealerShowcase, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return db.DealerShowcase{}, ErrStatusConflict
	}
	return row, err
}

// AddPhoto appends a gallery photo unless the showcase already holds
// maxPhotos (sysconfig showcase.max_photos); then it returns a
// *PhotoLimitError carrying the current count. The showcase row is locked
// first, so the caller must run it in a transaction for concurrent uploads
// to see each other's rows.
func (s *Store) AddPhoto(ctx context.Context, maxPhotos int64, p db.InsertDealerShowcasePhotoParams) (db.DealerShowcasePhoto, error) {
	if _, err := s.q.LockDealerShowcase(ctx, p.ShowcaseID); err != nil {
		return db.DealerShowcasePhoto{}, err
	}
	n, err := s.q.CountDealerShowcasePhotos(ctx, p.ShowcaseID)
	if err != nil {
		return db.DealerShowcasePhoto{}, err
	}
	if n >= maxPhotos {
		return db.DealerShowcasePhoto{}, &PhotoLimitError{Count: n, Max: maxPhotos}
	}
	return s.q.InsertDealerShowcasePhoto(ctx, p)
}

// ReorderServices sets the service order to uuids, which must list every
// service of the showcase exactly once.
func (s *Store) ReorderServices(ctx context.Context, showcaseID int64, uuids []uuid.UUID) error {
	cur, err := s.q.ListDealerShowcaseServices(ctx, showcaseID)
	if err != nil {
		return err
	}
	if !distinct(uuids) || len(uuids) != len(cur) {
		return ErrReorderMismatch
	}
	n, err := s.q.ReorderDealerShowcaseServices(ctx, db.ReorderDealerShowcaseServicesParams{ShowcaseID: showcaseID, Uuids: uuids})
	if err != nil {
		return err
	}
	if n != int64(len(uuids)) {
		return ErrReorderMismatch
	}
	return nil
}

// ReorderPhotos sets the gallery order to uuids, which must list every photo
// of the showcase exactly once.
func (s *Store) ReorderPhotos(ctx context.Context, showcaseID int64, uuids []uuid.UUID) error {
	n, err := s.q.CountDealerShowcasePhotos(ctx, showcaseID)
	if err != nil {
		return err
	}
	if !distinct(uuids) || int64(len(uuids)) != n {
		return ErrReorderMismatch
	}
	rows, err := s.q.ReorderDealerShowcasePhotos(ctx, db.ReorderDealerShowcasePhotosParams{ShowcaseID: showcaseID, Uuids: uuids})
	if err != nil {
		return err
	}
	if rows != n {
		return ErrReorderMismatch
	}
	return nil
}

func distinct(ids []uuid.UUID) bool {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func textNarg(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func int8Narg(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func tsNarg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
