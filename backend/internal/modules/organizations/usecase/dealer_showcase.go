package usecase

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TEC-250: the public dealer showcase (`/bayi/{code}`), code = organization
// slug.

// dealerCodeRe is the shape of an organization slug; anything else cannot
// match a dealer and is answered 404 without a query.
var dealerCodeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,99}$`)

// PublicDealer is the body of GET /v1/public/dealers/{code}. It carries
// showcase fields only: no tax number, account, members or settings.
type PublicDealer struct {
	Code      string   `json:"code"`
	Name      string   `json:"name"`
	LogoURL   *string  `json:"logo_url"`
	Address   string   `json:"address"`
	City      string   `json:"city"`
	District  string   `json:"district"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	WhatsApp  *string  `json:"whatsapp"`
	// Showcase is the F5 dealer showcase block (TEC-467): present only
	// while the dealer_showcase module is on and a snapshot is published;
	// otherwise the body stays the F2 skeleton above, key for key.
	Showcase any `json:"showcase,omitempty"`
}

// DealerShowcases supplies the F5 showcase data of the public dealer
// endpoints (TEC-467, dealershowcase usecase). nil keeps the F2 skeleton.
type DealerShowcases interface {
	// PublicBlock is the showcase block of a dealer code in a locale; nil
	// when the module is off or nothing was published.
	PublicBlock(ctx context.Context, brandID int64, code, locale string) (any, error)
	// Badges maps the organizations serving a live showcase to their
	// Google rating (nil when unrated).
	Badges(ctx context.Context, brandID int64, orgUUIDs []uuid.UUID) (map[uuid.UUID]*float64, error)
	// PublishedDates maps the codes of live showcases to their publish time.
	PublishedDates(ctx context.Context, brandID int64) (map[string]time.Time, error)
}

// SetShowcases enables the showcase extension of the public dealer
// endpoints (nil: F2 skeleton only).
func (s *Service) SetShowcases(d DealerShowcases) { s.showcases = d }

// PublicDealerByCode returns the showcase of an active, serving dealer or
// distributor of the brand. Passive, expired, deleted, other-brand (e.g.
// Glorian on the Olex domain) and non-dealer organizations are ErrNotFound.
func (s *Service) PublicDealerByCode(ctx context.Context, brandID int64, code, locale string) (PublicDealer, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	if !dealerCodeRe.MatchString(code) {
		return PublicDealer{}, ErrNotFound
	}
	row, err := s.q.GetPublicDealerBySlug(ctx, db.GetPublicDealerBySlugParams{Slug: code, BrandID: brandID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PublicDealer{}, ErrNotFound
		}
		return PublicDealer{}, err
	}
	d := mapPublicDealer(row)
	if s.showcases != nil {
		block, err := s.showcases.PublicBlock(ctx, brandID, code, locale)
		if err != nil {
			return PublicDealer{}, err
		}
		d.Showcase = block
	}
	return d, nil
}

func mapPublicDealer(r db.GetPublicDealerBySlugRow) PublicDealer {
	d := PublicDealer{
		Code: r.Slug, Name: r.Name,
		LogoURL: logoURL(r.Uuid, r.LogoObjectKey),
		Address: strings.TrimSpace(r.Address),
		City:    r.City, District: r.District,
	}
	// Coordinates come as a pair (chk_organizations_coordinates_pair).
	if lat, lng := numericFloat(r.Latitude), numericFloat(r.Longitude); lat != nil && lng != nil {
		d.Latitude, d.Longitude = lat, lng
	}
	if e164.MatchString(r.Phone) {
		p := r.Phone
		d.WhatsApp = &p
	}
	return d
}

// PublicDealerCodesLimit caps GET /v1/public/dealers (one sitemap file holds
// at most 50 000 URLs; the brand's dealer count is far below this).
const PublicDealerCodesLimit = 5000

// PublicDealerCode is one row of GET /v1/public/dealers (TEC-251): what the
// sitemap needs for `/bayi/{code}`, nothing else.
type PublicDealerCode struct {
	Code      string    `json:"code"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PublicDealerCodes lists the codes of the brand's active, serving dealers
// and distributors (same filters as PublicDealerByCode), sorted by code.
func (s *Service) PublicDealerCodes(ctx context.Context, brandID int64) ([]PublicDealerCode, error) {
	rows, err := s.q.ListPublicDealerCodes(ctx, db.ListPublicDealerCodesParams{
		BrandID: brandID, RowLimit: PublicDealerCodesLimit,
	})
	if err != nil {
		return nil, err
	}
	out := mapPublicDealerCodes(rows)
	if s.showcases != nil && len(out) > 0 {
		// TEC-467: a published showcase changes the page, so its publish
		// time counts as the last change.
		dates, err := s.showcases.PublishedDates(ctx, brandID)
		if err != nil {
			return nil, err
		}
		for i := range out {
			if t, ok := dates[out[i].Code]; ok && t.After(out[i].UpdatedAt) {
				out[i].UpdatedAt = t
			}
		}
	}
	return out, nil
}

// mapPublicDealerCodes drops slugs the showcase would answer 404 for.
func mapPublicDealerCodes(rows []db.ListPublicDealerCodesRow) []PublicDealerCode {
	out := make([]PublicDealerCode, 0, len(rows))
	for _, r := range rows {
		if !dealerCodeRe.MatchString(r.Slug) {
			continue
		}
		out = append(out, PublicDealerCode{Code: r.Slug, UpdatedAt: r.UpdatedAt.Time.UTC()})
	}
	return out
}
