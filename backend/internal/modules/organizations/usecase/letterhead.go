package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

var allowedPaperSizes = map[string]struct{}{
	"A4": {}, "A3": {}, "Letter": {}, "Legal": {},
}

// TenantSettings is letterhead branding stored on the organization row.
type TenantSettings struct {
	CompanyName  string  `json:"company_name"`
	Tagline      string  `json:"tagline"`
	PrimaryColor string  `json:"primary_color"`
	Address      string  `json:"address"`
	City         string  `json:"city"`
	District     string  `json:"district"`
	Phone        string  `json:"phone"`
	Email        string  `json:"email"`
	Website      string  `json:"website"`
	FooterText   string  `json:"footer_text"`
	PaperSize    string  `json:"paper_size"`
	LogoURL      *string `json:"logo_url,omitempty"`
	// TEC-240: map position (both null when unset).
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	// TEC-1003: the brand center's own service point (nil elsewhere).
	ServicePoint *ServicePoint `json:"service_point,omitempty"`
}

// ServicePoint is the name and logo the center shows on customer surfaces
// (warranty page and PDF, portal, WhatsApp) when Enabled; the panel keeps
// the organization name (TEC-1003).
type ServicePoint struct {
	Enabled bool    `json:"enabled"`
	Name    *string `json:"name"`
	LogoURL *string `json:"logo_url"`
}

// MaxServicePointName is the service point name limit (VARCHAR(200)).
const MaxServicePointName = 200

// LetterheadPatch is a partial update of organization export branding.
type LetterheadPatch struct {
	CompanyName  *string `json:"company_name"`
	Tagline      *string `json:"tagline"`
	PrimaryColor *string `json:"primary_color"`
	Address      *string `json:"address"`
	City         *string `json:"city"`
	District     *string `json:"district"`
	Phone        *string `json:"phone"`
	Email        *string `json:"email"`
	Website      *string `json:"website"`
	FooterText   *string `json:"footer_text"`
	PaperSize    *string `json:"paper_size"`
	// TEC-240: both together, or both null to clear.
	Latitude  OptionalCoordinate `json:"latitude"`
	Longitude OptionalCoordinate `json:"longitude"`
	// TEC-1003: center only. An empty name clears it.
	ServicePointEnabled *bool   `json:"service_point_enabled"`
	ServicePointName    *string `json:"service_point_name"`
}

func mapTenantSettings(row db.Organization) TenantSettings {
	color := strings.TrimSpace(row.PrimaryColor)
	if color == "" {
		color = "#0F172A"
	}
	paper := strings.TrimSpace(row.PaperSize)
	if paper == "" {
		paper = "A4"
	}
	return TenantSettings{
		CompanyName:  row.Name,
		Tagline:      row.Tagline,
		PrimaryColor: color,
		Address:      row.Address,
		City:         row.City,
		District:     row.District,
		Phone:        row.Phone,
		Email:        row.Email,
		Website:      row.Website,
		FooterText:   row.FooterText,
		PaperSize:    paper,
		LogoURL:      logoURL(row.Uuid, row.LogoObjectKey),
		Latitude:     numericFloat(row.Latitude),
		Longitude:    numericFloat(row.Longitude),
		ServicePoint: mapServicePoint(row),
	}
}

func mapServicePoint(row db.Organization) *ServicePoint {
	if row.Type != TypeCenter {
		return nil
	}
	sp := &ServicePoint{Enabled: row.ServicePointEnabled}
	if row.ServicePointName.Valid {
		name := row.ServicePointName.String
		sp.Name = &name
	}
	if row.ServicePointLogoObjectKey.Valid && strings.TrimSpace(row.ServicePointLogoObjectKey.String) != "" {
		u := fmt.Sprintf("/v1/public/organizations/service-point-logo/%s", row.Uuid.String())
		sp.LogoURL = &u
	}
	return sp
}

// GetTenantSettings returns letterhead fields for the active organization.
func (s *Service) GetTenantSettings(ctx context.Context, orgID int64) (TenantSettings, error) {
	row, err := s.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TenantSettings{}, ErrNotFound
		}
		return TenantSettings{}, err
	}
	return mapTenantSettings(row), nil
}

// PatchTenantSettings updates organization letterhead fields.
func (s *Service) PatchTenantSettings(ctx context.Context, orgID int64, in LetterheadPatch) (TenantSettings, error) {
	setCoords, lat, lng, err := coordinatePatch(in.Latitude, in.Longitude)
	if err != nil {
		return TenantSettings{}, err
	}
	if in.PrimaryColor != nil {
		color := strings.TrimSpace(*in.PrimaryColor)
		if !hexColor.MatchString(color) {
			return TenantSettings{}, ErrInvalidRequest
		}
		in.PrimaryColor = &color
	}
	if in.PaperSize != nil {
		paper := strings.TrimSpace(*in.PaperSize)
		if _, ok := allowedPaperSizes[paper]; !ok {
			return TenantSettings{}, ErrInvalidRequest
		}
		in.PaperSize = &paper
	}
	if in.CompanyName != nil {
		name := strings.TrimSpace(*in.CompanyName)
		if name == "" {
			return TenantSettings{}, ErrInvalidRequest
		}
		in.CompanyName = &name
	}
	if in.Phone != nil {
		iso2, err := s.orgCountryISO2(ctx, orgID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return TenantSettings{}, ErrNotFound
			}
			return TenantSettings{}, err
		}
		p, err := normalizePhone(*in.Phone, iso2)
		if err != nil {
			return TenantSettings{}, err
		}
		in.Phone = &p
	}
	spPatch := in.ServicePointEnabled != nil || in.ServicePointName != nil
	var spName pgtype.Text
	if in.ServicePointName != nil {
		name := strings.TrimSpace(*in.ServicePointName)
		if len([]rune(name)) > MaxServicePointName {
			return TenantSettings{}, ErrInvalidRequest
		}
		spName = pgtype.Text{String: name, Valid: name != ""}
	}
	if spPatch {
		org, err := s.q.GetOrganizationByID(ctx, orgID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return TenantSettings{}, ErrNotFound
			}
			return TenantSettings{}, err
		}
		if org.Type != TypeCenter {
			return TenantSettings{}, fmt.Errorf("%w: only the brand center has a service point", ErrInvalidRequest)
		}
	}
	// TEC-210: name / city / phone feed the organizations index.
	row, err := s.updateWithEvent(ctx, func(q *db.Queries) (db.Organization, error) {
		row, err := q.UpdateOrganizationLetterhead(ctx, db.UpdateOrganizationLetterheadParams{
			ID:           orgID,
			Name:         textNarg(in.CompanyName),
			City:         textNarg(in.City),
			District:     textNarg(in.District),
			Phone:        textNarg(in.Phone),
			Address:      textNarg(in.Address),
			Email:        textNarg(in.Email),
			Website:      textNarg(in.Website),
			Tagline:      textNarg(in.Tagline),
			FooterText:   textNarg(in.FooterText),
			PaperSize:    textNarg(in.PaperSize),
			PrimaryColor: textNarg(in.PrimaryColor),
		})
		if err != nil {
			return row, err
		}
		if spPatch {
			enabled := pgtype.Bool{}
			if in.ServicePointEnabled != nil {
				enabled = pgtype.Bool{Bool: *in.ServicePointEnabled, Valid: true}
			}
			if row, err = q.UpdateOrganizationServicePoint(ctx, db.UpdateOrganizationServicePointParams{
				ID: orgID, Enabled: enabled, SetName: in.ServicePointName != nil, Name: spName,
			}); err != nil {
				return row, err
			}
		}
		if !setCoords {
			return row, nil
		}
		return q.UpdateOrganizationCoordinates(ctx, db.UpdateOrganizationCoordinatesParams{
			ID: orgID, Latitude: lat, Longitude: lng,
		})
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TenantSettings{}, ErrNotFound
		}
		return TenantSettings{}, err
	}
	return mapTenantSettings(row), nil
}

func textNarg(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}
