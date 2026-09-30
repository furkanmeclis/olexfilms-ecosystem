package usecase

import (
	"context"
	"errors"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrNotFound = errors.New("not found")

// Service manages app_settings singleton.
type Service struct {
	q *db.Queries
}

// New creates a settings service.
func New(q *db.Queries) *Service {
	return &Service{q: q}
}

// LetterheadPublic is the public branding subset.
type LetterheadPublic struct {
	CompanyName  string  `json:"company_name"`
	Tagline      string  `json:"tagline"`
	PrimaryColor string  `json:"primary_color"`
	LogoURL      *string `json:"logo_url,omitempty"`
}

// Settings is the full settings payload.
type Settings struct {
	CompanyName  string  `json:"company_name"`
	Tagline      string  `json:"tagline"`
	PrimaryColor string  `json:"primary_color"`
	Address      string  `json:"address"`
	Phone        string  `json:"phone"`
	Email        string  `json:"email"`
	Website      string  `json:"website"`
	FooterText   string  `json:"footer_text"`
	PaperSize    string  `json:"paper_size"`
	LogoURL      *string `json:"logo_url,omitempty"`
}

func mapSettings(row db.AppSetting) Settings {
	var logo *string
	if row.LogoObjectKey.Valid && row.LogoObjectKey.String != "" {
		u := "/v1/platform/settings/logo"
		logo = &u
	}
	return Settings{
		CompanyName: row.CompanyName, Tagline: row.Tagline, PrimaryColor: row.PrimaryColor,
		Address: row.Address, Phone: row.Phone, Email: row.Email, Website: row.Website,
		FooterText: row.FooterText, PaperSize: row.PaperSize, LogoURL: logo,
	}
}

// Get returns current settings.
func (s *Service) Get(ctx context.Context) (Settings, error) {
	row, err := s.q.GetAppSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	return mapSettings(row), nil
}

// PublicLetterhead returns branding for app config.
func (s *Service) PublicLetterhead(ctx context.Context) (LetterheadPublic, error) {
	row, err := s.q.GetAppSettings(ctx)
	if err != nil {
		return LetterheadPublic{}, err
	}
	var logo *string
	if row.LogoObjectKey.Valid && row.LogoObjectKey.String != "" {
		u := "/v1/platform/settings/logo"
		logo = &u
	}
	return LetterheadPublic{
		CompanyName: row.CompanyName, Tagline: row.Tagline,
		PrimaryColor: row.PrimaryColor, LogoURL: logo,
	}, nil
}

// PatchInput partial update.
type PatchInput struct {
	CompanyName  *string `json:"company_name"`
	Tagline      *string `json:"tagline"`
	PrimaryColor *string `json:"primary_color"`
	Address      *string `json:"address"`
	Phone        *string `json:"phone"`
	Email        *string `json:"email"`
	Website      *string `json:"website"`
	FooterText   *string `json:"footer_text"`
	PaperSize    *string `json:"paper_size"`
}

// Patch updates settings.
func (s *Service) Patch(ctx context.Context, in PatchInput) (Settings, error) {
	row, err := s.q.UpdateAppSettings(ctx, db.UpdateAppSettingsParams{
		CompanyName: textNarg(in.CompanyName), Tagline: textNarg(in.Tagline),
		PrimaryColor: textNarg(in.PrimaryColor), Address: textNarg(in.Address),
		Phone: textNarg(in.Phone), Email: textNarg(in.Email), Website: textNarg(in.Website),
		FooterText: textNarg(in.FooterText), PaperSize: textNarg(in.PaperSize),
	})
	if err != nil {
		return Settings{}, err
	}
	return mapSettings(row), nil
}

// SetLogo stores logo object key.
func (s *Service) SetLogo(ctx context.Context, objectKey string) (Settings, error) {
	row, err := s.q.SetAppSettingsLogo(ctx, pgtype.Text{String: objectKey, Valid: objectKey != ""})
	if err != nil {
		return Settings{}, err
	}
	return mapSettings(row), nil
}

// ClearLogo removes logo.
func (s *Service) ClearLogo(ctx context.Context) (Settings, error) {
	row, err := s.q.ClearAppSettingsLogo(ctx)
	if err != nil {
		return Settings{}, err
	}
	return mapSettings(row), nil
}

// LogoObjectKey returns stored logo key if any.
func (s *Service) LogoObjectKey(ctx context.Context) (string, error) {
	row, err := s.q.GetAppSettings(ctx)
	if err != nil {
		return "", err
	}
	if !row.LogoObjectKey.Valid {
		return "", ErrNotFound
	}
	return row.LogoObjectKey.String, nil
}

func textNarg(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}
