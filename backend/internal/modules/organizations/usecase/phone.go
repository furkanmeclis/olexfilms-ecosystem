package usecase

import (
	"context"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/orgphone"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
)

// normalizePhone stores organization phones in E.164 (K29, TEC-159): the
// default region is the organization's country (TR when unknown). Empty
// means no phone; an unparseable number is a validation error.
func normalizePhone(raw, countryISO2 string) (string, error) {
	p, err := orgphone.Normalize(raw, countryISO2)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	return p, nil
}

// addressISO2 is the country code of a validated address ("" when none).
func addressISO2(a *geo.Address) string {
	if a == nil {
		return ""
	}
	return a.Country.Iso2
}

// orgCountryISO2 is the stored country of an organization ("" when none).
func (s *Service) orgCountryISO2(ctx context.Context, orgID int64) (string, error) {
	return s.q.GetOrganizationCountryISO2(ctx, orgID)
}
