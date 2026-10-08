package usecase

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealershowcase/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealershowcase/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Review decisions.
const (
	DecisionApprove = "approve"
	DecisionReject  = "reject"
)

// MaxReviewNote caps the reviewer's note (the DB CHECK).
const MaxReviewNote = 2000

// ReviewQueueItem is one row of GET /v1/platform/showcases.
type ReviewQueueItem struct {
	UUID         uuid.UUID       `json:"uuid"`
	Organization OrganizationRef `json:"organization"`
	Status       string          `json:"status"`
	SubmittedAt  *time.Time      `json:"submitted_at"`
	PublishedAt  *time.Time      `json:"published_at"`
	ReviewedAt   *time.Time      `json:"reviewed_at"`
	ReviewNote   *string         `json:"review_note"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// ParseReviewFilter reads the review queue query (docs/list-contract.md):
// limit, offset, sort (updated_at | name | status, default -updated_at),
// status=a,b, q, updated_from / updated_to. An unknown sort field or
// status is an *apiquery.ValidationError (400).
func ParseReviewFilter(values url.Values) (repository.ReviewFilter, error) {
	q := apiquery.Parse(values)
	statuses, err := apiquery.EnumList(values, "status", model.Statuses...)
	if err != nil {
		return repository.ReviewFilter{}, err
	}
	updated, err := apiquery.DateRange(values, "updated")
	if err != nil {
		return repository.ReviewFilter{}, err
	}
	if _, err := apiquery.ResolveSort(q.Sort, repository.ReviewSort); err != nil {
		return repository.ReviewFilter{}, err
	}
	return repository.ReviewFilter{
		Statuses: statuses, Updated: updated, Q: q.Q, Sort: q.Sort, Limit: q.Limit, Offset: q.Offset,
	}, nil
}

// ListReviews pages the brand's showcases for the center review queue.
func (s *Service) ListReviews(ctx context.Context, c Caller, f repository.ReviewFilter) (apiquery.Page[ReviewQueueItem], error) {
	f.OrganizationIDs = c.Filter.OrgIDsArg()
	list, total, err := repository.FromQueries(s.q).ListReviews(ctx, c.BrandID, f)
	if err != nil {
		return apiquery.Page[ReviewQueueItem]{}, err
	}
	items := make([]ReviewQueueItem, 0, len(list))
	for _, r := range list {
		items = append(items, ReviewQueueItem{
			UUID: r.Uuid,
			Organization: OrganizationRef{
				UUID: r.OrganizationUuid, Code: r.OrganizationSlug, Name: r.OrganizationName,
				Type: r.OrganizationType, City: r.OrganizationCity,
			},
			Status: r.Status, SubmittedAt: tsPtr(r.SubmittedAt), PublishedAt: tsPtr(r.PublishedAt),
			ReviewedAt: tsPtr(r.ReviewedAt), ReviewNote: textPtr(r.ReviewNote), UpdatedAt: r.UpdatedAt.Time.UTC(),
		})
	}
	return apiquery.NewPage(items, total, f.Limit, f.Offset), nil
}

// reviewTarget loads a dealer or distributor of the reviewer's brand and
// its showcase (no module check: a pending showcase stays decidable).
func (s *Service) reviewTarget(ctx context.Context, q *db.Queries, c Caller, orgUUID uuid.UUID) (db.Organization, db.DealerShowcase, error) {
	o, err := q.GetOrganizationByUUID(ctx, orgUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, db.DealerShowcase{}, ErrNotFound
	}
	if err != nil {
		return db.Organization{}, db.DealerShowcase{}, err
	}
	if o.DeletedAt.Valid || o.BrandID != c.BrandID || !c.Filter.AllowsOrg(o.ID, o.BrandID) ||
		(o.Type != rbac.OrgTypeDealer && o.Type != rbac.OrgTypeDistributor) {
		return db.Organization{}, db.DealerShowcase{}, ErrNotFound
	}
	row, ok, err := showcaseOf(ctx, q, o.ID)
	if err != nil {
		return db.Organization{}, db.DealerShowcase{}, err
	}
	if !ok {
		return db.Organization{}, db.DealerShowcase{}, ErrNotFound
	}
	return o, row, nil
}

// ReviewDetail is the reviewer's view of one showcase (the draft under
// review and the live snapshot).
func (s *Service) ReviewDetail(ctx context.Context, c Caller, orgUUID uuid.UUID) (Showcase, error) {
	o, row, err := s.reviewTarget(ctx, s.q, c, orgUUID)
	if err != nil {
		return Showcase{}, err
	}
	return s.view(ctx, s.q, o, row, true)
}

// ReviewInput is the body of POST /v1/platform/showcases/{org_uuid}/review.
type ReviewInput struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
}

// Review decides a pending showcase: approve publishes the current draft
// (snapshot written, reviewer recorded), reject keeps the previous snapshot
// live and needs a note (ErrReviewNoteRequired). The dealer owners are
// notified either way.
func (s *Service) Review(ctx context.Context, c Caller, orgUUID uuid.UUID, in ReviewInput) (Showcase, error) {
	note := strings.TrimSpace(in.Note)
	switch in.Decision {
	case DecisionApprove, DecisionReject:
	default:
		return Showcase{}, invalid("decision", "must be approve or reject")
	}
	if utf8.RuneCountInString(note) > MaxReviewNote {
		return Showcase{}, invalid("note", "at most 2000 characters")
	}
	if in.Decision == DecisionReject && note == "" {
		return Showcase{}, ErrReviewNoteRequired
	}
	var (
		o      db.Organization
		out    db.DealerShowcase
		orphan []string
	)
	err := s.inTx(ctx, func(tx pgx.Tx, q *db.Queries) error {
		var (
			row db.DealerShowcase
			err error
		)
		if o, row, err = s.reviewTarget(ctx, q, c, orgUUID); err != nil {
			return err
		}
		if row, err = q.LockDealerShowcase(ctx, row.ID); err != nil {
			return err
		}
		if row.Status != model.StatusPendingReview {
			return ErrInvalidTransition
		}
		owners, err := q.ListOrganizationOwnerUserIDs(ctx, o.ID)
		if err != nil {
			return err
		}
		owners = without(owners, c.UserID)
		store := repository.FromQueries(q)
		if in.Decision == DecisionReject {
			if out, err = store.Reject(ctx, row.ID, c.UserID, note); err != nil {
				return mapRepoErr(err)
			}
			return s.emit(ctx, tx, events.ShowcaseRejected, c, o, out, note, owners)
		}
		var snap []byte
		if snap, orphan, err = buildSnapshot(ctx, q, row); err != nil {
			return err
		}
		reviewer := c.UserID
		if out, err = store.Publish(ctx, row.ID, row.Status, snap, &reviewer, &reviewer); err != nil {
			return mapRepoErr(err)
		}
		return s.emit(ctx, tx, events.ShowcasePublished, c, o, out, "", owners)
	})
	if err != nil {
		return Showcase{}, err
	}
	s.dropObjects(ctx, orphan)
	return s.view(ctx, s.q, o, out, true)
}

func without(ids []int64, id int64) []int64 {
	out := make([]int64, 0, len(ids))
	for _, v := range ids {
		if v != id {
			out = append(out, v)
		}
	}
	return out
}
