package usecase

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
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
}

// PublicDealerByCode returns the showcase of an active, serving dealer or
// distributor of the brand. Passive, expired, deleted, other-brand (e.g.
// Glorian on the Olex domain) and non-dealer organizations are ErrNotFound.
func (s *Service) PublicDealerByCode(ctx context.Context, brandID int64, code string) (PublicDealer, error) {
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
	return mapPublicDealer(row), nil
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
