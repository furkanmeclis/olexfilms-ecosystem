package storage

import (
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/google/uuid"
)

// MaxLogoBytes is the maximum accepted logo upload size (2 MiB).
const MaxLogoBytes = 2 << 20

var logoMIMEToExt = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

var logoExtToMIME = map[string]string{
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"webp": "image/webp",
}

// NormalizeLogoMIME returns a canonical logo MIME or empty if unsupported.
func NormalizeLogoMIME(contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if _, ok := logoMIMEToExt[ct]; ok {
		return ct
	}
	return ""
}

// LogoExtForMIME returns the object-key extension for an allowed logo MIME.
func LogoExtForMIME(contentType string) (string, error) {
	mime := NormalizeLogoMIME(contentType)
	if mime == "" {
		return "", fmt.Errorf("logo content type must be image/jpeg, image/png, or image/webp")
	}
	return logoMIMEToExt[mime], nil
}

// MIMEFromLogoKey derives Content-Type from a logo object key extension.
func MIMEFromLogoKey(objectKey string) string {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(objectKey), "."))
	if mime, ok := logoExtToMIME[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

// DetectLogoMIME sniffs the body prefix and accepts only raster logo types.
// The declared Content-Type is used only when no bytes are available: a
// client-declared image/png must not smuggle HTML/SVG into public storage.
func DetectLogoMIME(declared string, head []byte) (string, error) {
	if len(head) == 0 {
		if mime := NormalizeLogoMIME(declared); mime != "" {
			return mime, nil
		}
		return "", fmt.Errorf("logo content type must be image/jpeg, image/png, or image/webp")
	}
	if mime := NormalizeLogoMIME(http.DetectContentType(head)); mime != "" {
		return mime, nil
	}
	return "", fmt.Errorf("logo content type must be image/jpeg, image/png, or image/webp")
}

// ValidateLogoSize rejects oversized uploads.
func ValidateLogoSize(size int64) error {
	if size <= 0 {
		return fmt.Errorf("logo file is required")
	}
	if size > MaxLogoBytes {
		return fmt.Errorf("logo must be at most 2 MiB")
	}
	return nil
}

// MaxHeroBytes is the maximum accepted vehicle hero image size (5 MiB).
// Hero images use the same raster-only MIME rules as logos (TEC-149).
const MaxHeroBytes = 5 << 20

// ValidateHeroSize rejects empty or oversized hero image uploads.
func ValidateHeroSize(size int64) error {
	if size <= 0 {
		return fmt.Errorf("hero image file is required")
	}
	if size > MaxHeroBytes {
		return fmt.Errorf("hero image must be at most 5 MiB")
	}
	return nil
}

// TenantLogoObjectKey builds tenants/{uuid}/logo.{ext}.
func TenantLogoObjectKey(tenantUUID uuid.UUID, ext string) string {
	return fmt.Sprintf("tenants/%s/logo.%s", tenantUUID.String(), strings.TrimPrefix(ext, "."))
}

// WorkspaceLogoObjectKey builds tenants/{tid}/workspaces/{wid}/logo.{ext}.
func WorkspaceLogoObjectKey(tenantUUID, workspaceUUID uuid.UUID, ext string) string {
	return fmt.Sprintf(
		"tenants/%s/workspaces/%s/logo.%s",
		tenantUUID.String(),
		workspaceUUID.String(),
		strings.TrimPrefix(ext, "."),
	)
}

// WidgetLogoObjectKey builds channel-accounts/{uuid}/widget-logo.{ext}.
func WidgetLogoObjectKey(accountUUID uuid.UUID, ext string) string {
	return fmt.Sprintf(
		"channel-accounts/%s/widget-logo.%s",
		accountUUID.String(),
		strings.TrimPrefix(ext, "."),
	)
}
