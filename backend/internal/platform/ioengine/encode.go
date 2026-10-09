package ioengine

import (
	"context"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/jackc/pgx/v5/pgtype"
)

// LetterheadFromSettings builds letterhead from app_settings row.
func LetterheadFromSettings(s db.AppSetting) Letterhead {
	return Letterhead{
		CompanyName:  s.CompanyName,
		Tagline:      s.Tagline,
		PrimaryColor: s.PrimaryColor,
		Address:      s.Address,
		Phone:        s.Phone,
		Email:        s.Email,
		Website:      s.Website,
		FooterText:   s.FooterText,
		PaperSize:    s.PaperSize,
	}
}

// LetterheadFromOrganization builds PDF/XLSX branding from the tenant record.
// Empty org chrome (color, paper size) falls back to platform app_settings.
func LetterheadFromOrganization(org db.Organization, layout db.AppSetting) Letterhead {
	color := firstNonEmpty(org.PrimaryColor, layout.PrimaryColor, "#0F172A")
	paper := firstNonEmpty(org.PaperSize, layout.PaperSize, "A4")
	return Letterhead{
		CompanyName:  strings.TrimSpace(org.Name),
		Tagline:      strings.TrimSpace(org.Tagline),
		PrimaryColor: color,
		Address:      organizationAddress(org),
		Phone:        strings.TrimSpace(org.Phone),
		Email:        strings.TrimSpace(org.Email),
		Website:      strings.TrimSpace(org.Website),
		FooterText:   strings.TrimSpace(org.FooterText),
		PaperSize:    paper,
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func organizationAddress(org db.Organization) string {
	parts := make([]string, 0, 3)
	for _, part := range []string{org.Address, org.District, org.City} {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, ", ")
}

// LoadLetterheadLogo fetches logo bytes when configured.
func LoadLetterheadLogo(ctx context.Context, driver storage.Driver, s db.AppSetting) (Letterhead, error) {
	return attachLetterheadLogo(ctx, driver, LetterheadFromSettings(s), s.LogoObjectKey)
}

// LoadOrganizationLetterhead fetches tenant logo bytes onto organization branding.
func LoadOrganizationLetterhead(
	ctx context.Context,
	driver storage.Driver,
	org db.Organization,
	layout db.AppSetting,
) (Letterhead, error) {
	return attachLetterheadLogo(ctx, driver, LetterheadFromOrganization(org, layout), org.LogoObjectKey)
}

func attachLetterheadLogo(
	ctx context.Context,
	driver storage.Driver,
	lh Letterhead,
	key pgtype.Text,
) (Letterhead, error) {
	if !key.Valid || key.String == "" || driver == nil {
		return lh, nil
	}
	rc, _, err := driver.Download(ctx, key.String)
	if err != nil {
		return lh, fmt.Errorf("letterhead logo: %w", err)
	}
	defer func() { _ = rc.Close() }()
	buf := make([]byte, 0, 64*1024)
	tmp := make([]byte, 32*1024)
	for {
		n, readErr := rc.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if readErr != nil {
			break
		}
	}
	lh.LogoBytes = buf
	lh.LogoMIME = storage.MIMEFromLogoKey(key.String)
	return lh, nil
}

// EncodeExport selects encoder by format. PDF needs Gotenberg: use
// EncodePDF (ErrPDFRendererRequired here).
func EncodeExport(format ExportFormat, ds Dataset, locale string, lh *Letterhead, title string) ([]byte, error) {
	switch format {
	case ExportPDF:
		return nil, ErrPDFRendererRequired
	case ExportXLSX:
		var head *Letterhead
		if lh != nil {
			head = lh
		}
		return EncodeXLSX(ds, locale, head)
	case ExportCSV:
		return EncodeCSV(ds, locale)
	case ExportJSON:
		return EncodeJSON(ds, locale)
	default:
		return nil, fmt.Errorf("ioengine: unknown export format %q", format)
	}
}
