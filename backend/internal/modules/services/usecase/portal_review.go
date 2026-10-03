package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-244 (F2-03h): the portal service review form.
//
// The portal user who owns a service (portalService: its customer or a
// holder of one of its warranties) rates the platform and the product
// quality (1-5) with an optional comment, once per service. Anything that
// is not theirs is ErrNotFound; a second review is ErrAlreadyReviewed
// (409); a service that is not completed yet is ErrReviewNotCompleted
// (422). The dealer's google_business_url is returned with the review so
// the portal can show the "review us on Google" button. Processing and
// reporting of the answers come with F5.

// MaxReviewCommentLength is the limit of the review comment (characters).
const MaxReviewCommentLength = 2000

var (
	// ErrAlreadyReviewed: the service already has a review (409).
	ErrAlreadyReviewed = errors.New("services: service already reviewed")
	// ErrReviewNotCompleted: only a completed service can be reviewed (422).
	ErrReviewNotCompleted = errors.New("services: service is not completed")
)

// ServiceReviewInput is the body of POST /v1/portal/services/{uuid}/review.
// The ratings are pointers so a missing one is a validation error.
type ServiceReviewInput struct {
	PlatformRating *int    `json:"platform_rating"`
	ProductRating  *int    `json:"product_rating"`
	Comment        *string `json:"comment"`
}

// ServiceReviewView is a stored review.
type ServiceReviewView struct {
	UUID           uuid.UUID `json:"uuid"`
	PlatformRating int       `json:"platform_rating"`
	ProductRating  int       `json:"product_rating"`
	Comment        *string   `json:"comment"`
	CreatedAt      time.Time `json:"created_at"`
}

// PortalServiceReview is the review state of a service for the portal:
// the stored review (nil when none), whether the user can send one now and
// the dealer's Google review link (nil when the dealer has none).
type PortalServiceReview struct {
	Review            *ServiceReviewView `json:"review"`
	CanReview         bool               `json:"can_review"`
	GoogleBusinessURL *string            `json:"google_business_url"`
}

// normalizeReview validates the input (pure): both ratings 1-5, the
// comment trimmed, empty means none, at most MaxReviewCommentLength.
func normalizeReview(in ServiceReviewInput) (platform, product int16, comment pgtype.Text, err error) {
	rating := func(field string, v *int) (int16, error) {
		if v == nil {
			return 0, invalid(field, "is required")
		}
		if *v < 1 || *v > 5 {
			return 0, invalid(field, "must be between 1 and 5")
		}
		return int16(*v), nil
	}
	if platform, err = rating("platform_rating", in.PlatformRating); err != nil {
		return 0, 0, pgtype.Text{}, err
	}
	if product, err = rating("product_rating", in.ProductRating); err != nil {
		return 0, 0, pgtype.Text{}, err
	}
	if in.Comment != nil {
		c := strings.TrimSpace(*in.Comment)
		if utf8.RuneCountInString(c) > MaxReviewCommentLength {
			return 0, 0, pgtype.Text{}, invalid("comment", fmt.Sprintf("at most %d characters", MaxReviewCommentLength))
		}
		if c != "" {
			comment = pgtype.Text{String: c, Valid: true}
		}
	}
	return platform, product, comment, nil
}

func reviewView(r db.ServiceReview) *ServiceReviewView {
	return &ServiceReviewView{
		UUID: r.Uuid, PlatformRating: int(r.PlatformRating), ProductRating: int(r.ProductRating),
		Comment: textPtr(r.Comment), CreatedAt: r.CreatedAt.Time,
	}
}

// googleURL is the dealer's review link (nil when unset).
func googleURL(org db.Organization) *string {
	if !org.GoogleBusinessUrl.Valid {
		return nil
	}
	u := strings.TrimSpace(org.GoogleBusinessUrl.String)
	if u == "" {
		return nil
	}
	return &u
}

// PortalGetReview returns the review state of a service the user owns.
func (s *Service) PortalGetReview(ctx context.Context, brandID, userID int64, id uuid.UUID) (PortalServiceReview, error) {
	svc, _, err := s.portalService(ctx, brandID, userID, id)
	if err != nil {
		return PortalServiceReview{}, err
	}
	org, err := s.q.GetOrganizationByID(ctx, svc.OrganizationID)
	if err != nil {
		return PortalServiceReview{}, fmt.Errorf("services: review organization: %w", err)
	}
	out := PortalServiceReview{GoogleBusinessURL: googleURL(org)}
	r, err := s.q.GetServiceReviewByService(ctx, svc.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		out.CanReview = svc.Status == StatusCompleted
	case err != nil:
		return PortalServiceReview{}, fmt.Errorf("services: get review: %w", err)
	default:
		out.Review = reviewView(r)
	}
	return out, nil
}

// PortalCreateReview stores the review of a completed service the user
// owns and returns the new review state.
func (s *Service) PortalCreateReview(ctx context.Context, brandID, userID int64, id uuid.UUID,
	in ServiceReviewInput,
) (PortalServiceReview, error) {
	svc, _, err := s.portalService(ctx, brandID, userID, id)
	if err != nil {
		return PortalServiceReview{}, err
	}
	platform, product, comment, err := normalizeReview(in)
	if err != nil {
		return PortalServiceReview{}, err
	}
	if svc.Status != StatusCompleted {
		return PortalServiceReview{}, ErrReviewNotCompleted
	}
	org, err := s.q.GetOrganizationByID(ctx, svc.OrganizationID)
	if err != nil {
		return PortalServiceReview{}, fmt.Errorf("services: review organization: %w", err)
	}
	r, err := s.q.CreateServiceReview(ctx, db.CreateServiceReviewParams{
		OrganizationID: svc.OrganizationID, BrandID: svc.BrandID, ServiceID: svc.ID,
		CustomerUserID: userID, PlatformRating: platform, ProductRating: product, Comment: comment,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return PortalServiceReview{}, ErrAlreadyReviewed
	}
	if err != nil {
		return PortalServiceReview{}, fmt.Errorf("services: create review: %w", err)
	}
	return PortalServiceReview{Review: reviewView(r), GoogleBusinessURL: googleURL(org)}, nil
}
