package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealershowcase/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/places"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-469 (F5-01d): the showcase Google rating. With GOOGLE_PLACES_API_KEY
// set, the daily Places worker refreshes showcases that have a place id;
// otherwise (or without a place id) the owner enters the rating by hand
// (F5 S7).

// Error codes of the manual rating entry.
const (
	CodeRatingOutOfRange      = "SHOWCASE_RATING_OUT_OF_RANGE"
	CodeRatingManagedByPlaces = "SHOWCASE_RATING_MANAGED_BY_PLACES"
)

// Manual rating bounds (the DB CHECKs).
const (
	MinRating = 1.0
	MaxRating = 5.0
)

// ErrRatingManagedByPlaces: Places is configured and the showcase has a
// place id, so the worker owns the rating (409).
var ErrRatingManagedByPlaces = errors.New("dealershowcase: the Google rating comes from Places")

// RatingRangeError is a manual rating outside 1.0–5.0 or a negative review
// count (422 SHOWCASE_RATING_OUT_OF_RANGE).
type RatingRangeError struct{ Field, Message string }

func (e *RatingRangeError) Error() string {
	return "dealershowcase: " + e.Field + " " + e.Message
}

// PlacesClient reads a place's rating (places.Client).
type PlacesClient interface {
	Configured() bool
	Rating(ctx context.Context, placeID string) (places.Rating, error)
}

// SetPlaces sets the Places client; nil or unconfigured keeps manual entry.
func (s *Service) SetPlaces(p PlacesClient) { s.places = p }

func (s *Service) placesConfigured() bool { return s.places != nil && s.places.Configured() }

// manualRatingAllowed: the owner may enter the rating while Places is not
// configured or the showcase has no place id.
func (s *Service) manualRatingAllowed(row db.DealerShowcase) bool {
	return !s.placesConfigured() || !row.GooglePlaceID.Valid
}

// RatingInput is the body of PUT /v1/showcase/google-rating. Both null
// clear the rating.
type RatingInput struct {
	Rating      *float64 `json:"rating"`
	ReviewCount *int64   `json:"review_count"`
}

func (in RatingInput) params() (pgtype.Numeric, pgtype.Int4, error) {
	if in.Rating == nil && in.ReviewCount == nil {
		return pgtype.Numeric{}, pgtype.Int4{}, nil
	}
	if in.Rating == nil {
		return pgtype.Numeric{}, pgtype.Int4{}, invalid("rating", "is required with review_count")
	}
	if in.ReviewCount == nil {
		return pgtype.Numeric{}, pgtype.Int4{}, invalid("review_count", "is required with rating")
	}
	r := *in.Rating
	if math.IsNaN(r) || r < MinRating || r > MaxRating {
		return pgtype.Numeric{}, pgtype.Int4{}, &RatingRangeError{Field: "rating", Message: "must be between 1.0 and 5.0"}
	}
	if *in.ReviewCount < 0 || *in.ReviewCount > math.MaxInt32 {
		return pgtype.Numeric{}, pgtype.Int4{}, &RatingRangeError{Field: "review_count", Message: "must be 0 or more"}
	}
	n, err := ratingNumeric(r)
	if err != nil {
		return pgtype.Numeric{}, pgtype.Int4{}, err
	}
	return n, pgtype.Int4{Int32: int32(*in.ReviewCount), Valid: true}, nil
}

// ratingNumeric rounds to one decimal (NUMERIC(2,1)).
func ratingNumeric(r float64) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	err := n.Scan(strconv.FormatFloat(math.Round(r*10)/10, 'f', 1, 64))
	return n, err
}

// SetGoogleRating writes (or with both values null clears) the manual
// Google rating. It is refused with ErrRatingManagedByPlaces while Places
// is configured and the showcase has a place id. Like the rest of the
// draft, a manual rating reaches the public page through the published
// snapshot when showcase.approval_required is on (submit → center review);
// with approval off it is live at once.
func (s *Service) SetGoogleRating(ctx context.Context, c Caller, orgUUID *uuid.UUID, in RatingInput) (Showcase, error) {
	rating, count, err := in.params()
	if err != nil {
		return Showcase{}, err
	}
	var (
		o   db.Organization
		out db.DealerShowcase
	)
	err = s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
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
		if !s.manualRatingAllowed(row) {
			return ErrRatingManagedByPlaces
		}
		out, err = q.SetDealerShowcaseGoogleRating(ctx, db.SetDealerShowcaseGoogleRatingParams{
			ID: row.ID, GoogleRating: rating, GoogleReviewCount: count,
			GoogleRatingSource: pgtype.Text{String: model.RatingSourceManual, Valid: rating.Valid},
		})
		return err
	})
	if err != nil {
		return Showcase{}, err
	}
	return s.view(ctx, s.q, o, out, true)
}

// snapshotRating is a Google rating: the live columns or the copy frozen
// in published_content.
type snapshotRating struct {
	Rating      float64    `json:"rating"`
	ReviewCount *int32     `json:"review_count"`
	Source      string     `json:"source"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

func ratingOf(rating pgtype.Numeric, count pgtype.Int4, source pgtype.Text, at pgtype.Timestamptz) *snapshotRating {
	r := numericPtr(rating)
	if r == nil {
		return nil
	}
	out := &snapshotRating{Rating: *r, Source: source.String, UpdatedAt: tsPtr(at)}
	if count.Valid {
		n := count.Int32
		out.ReviewCount = &n
	}
	return out
}

func parseRating(raw []byte) *snapshotRating {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var r snapshotRating
	if err := json.Unmarshal(raw, &r); err != nil || r.Rating == 0 {
		return nil
	}
	return &r
}

// publicRating picks the rating the public page shows: a Places rating is
// live (the worker refreshes it without review); a manual one is live
// while approval is off and otherwise the one of the published snapshot.
func publicRating(live, published *snapshotRating, approval bool) *snapshotRating {
	if live != nil && live.Source == model.RatingSourcePlaces {
		return live
	}
	if !approval {
		return live
	}
	return published
}
