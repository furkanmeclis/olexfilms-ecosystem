// Package orgphone keeps organization phones in E.164 (K29, TEC-159).
//
// New writes go through Normalize in the organizations usecase. Rows written
// before migration 000048 had their free-text phone moved into phone_raw;
// Run parses those with the organization's country as the default region.
package orgphone

import (
	"context"
	"errors"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	"github.com/google/uuid"
)

// ErrInvalid is returned for a non-empty phone that is not a valid number.
var ErrInvalid = errors.New("phone must be a valid number (E.164, e.g. +905551234567)")

// Normalize returns the E.164 form of raw, parsed with the organization's
// country (ISO alpha-2, empty means TR) as default region. An empty or blank
// input means "no phone" and returns "".
func Normalize(raw, countryISO2 string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	e164, err := phone.NormalizeE164(raw, phone.Region(countryISO2))
	if err != nil {
		return "", ErrInvalid
	}
	return e164, nil
}

// Store is the slice of db.Querier Run needs.
type Store interface {
	ListOrganizationsWithRawPhone(ctx context.Context) ([]db.ListOrganizationsWithRawPhoneRow, error)
	ResolveOrganizationRawPhone(ctx context.Context, arg db.ResolveOrganizationRawPhoneParams) (int64, error)
}

// Item is one organization in the report.
type Item struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Country string    `json:"country"`
	Raw     string    `json:"raw"`
	E164    string    `json:"e164,omitempty"`
}

// Report lists what Run did (or would do in a dry run).
type Report struct {
	DryRun    bool   `json:"dry_run"`
	Scanned   int    `json:"scanned"`
	Converted []Item `json:"converted"`
	// Superseded rows already got a phone through the API after the
	// migration; only phone_raw is cleared, the newer phone is kept.
	Superseded []Item `json:"superseded"`
	// Unresolved rows cannot be parsed; the original stays in phone_raw
	// for a person to fix (nothing is deleted).
	Unresolved []Item `json:"unresolved"`
}

// Run converts every phone_raw it can parse. It is idempotent: a converted
// or superseded row gets phone_raw cleared, so a second run scans only the
// unresolved rows and changes nothing.
func Run(ctx context.Context, s Store, dryRun bool) (Report, error) {
	rows, err := s.ListOrganizationsWithRawPhone(ctx)
	if err != nil {
		return Report{}, err
	}
	rep := Report{DryRun: dryRun, Scanned: len(rows), Converted: []Item{}, Superseded: []Item{}, Unresolved: []Item{}}
	for _, row := range rows {
		item := Item{UUID: row.Uuid, Name: row.Name, Country: row.CountryIso2.String, Raw: row.PhoneRaw.String}
		var e164 string
		switch {
		case row.Phone != "":
			rep.Superseded = append(rep.Superseded, item)
		default:
			e164, err = Normalize(row.PhoneRaw.String, row.CountryIso2.String)
			if err != nil || e164 == "" {
				rep.Unresolved = append(rep.Unresolved, item)
				continue
			}
			item.E164 = e164
			rep.Converted = append(rep.Converted, item)
		}
		if dryRun {
			continue
		}
		if _, err := s.ResolveOrganizationRawPhone(ctx, db.ResolveOrganizationRawPhoneParams{ID: row.ID, PhoneE164: e164}); err != nil {
			return rep, err
		}
	}
	return rep, nil
}
