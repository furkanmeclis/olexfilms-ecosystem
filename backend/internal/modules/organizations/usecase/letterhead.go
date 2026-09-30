package usecase

import (
	"context"
	"errors"
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
}

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
	}
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
	row, err := s.q.UpdateOrganizationLetterhead(ctx, db.UpdateOrganizationLetterheadParams{
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
