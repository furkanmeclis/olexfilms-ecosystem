package storage_test

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
)

func TestLogoMIMEAndKeys(t *testing.T) {
	t.Parallel()
	ext, err := storage.LogoExtForMIME("image/png")
	if err != nil || ext != "png" {
		t.Fatalf("ext=%q err=%v", ext, err)
	}
	if _, err := storage.LogoExtForMIME("image/gif"); err == nil {
		t.Fatal("gif must be rejected")
	}
	if err := storage.ValidateLogoSize(storage.MaxLogoBytes + 1); err == nil {
		t.Fatal("oversized must fail")
	}
	tid := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	wid := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	if got := storage.TenantLogoObjectKey(tid, "jpg"); got != "tenants/11111111-1111-1111-1111-111111111111/logo.jpg" {
		t.Fatalf("tenant key=%s", got)
	}
	if got := storage.WorkspaceLogoObjectKey(tid, wid, "webp"); got != "tenants/11111111-1111-1111-1111-111111111111/workspaces/22222222-2222-2222-2222-222222222222/logo.webp" {
		t.Fatalf("ws key=%s", got)
	}
	aid := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	if got := storage.WidgetLogoObjectKey(aid, "png"); got != "channel-accounts/33333333-3333-3333-3333-333333333333/widget-logo.png" {
		t.Fatalf("widget key=%s", got)
	}
	if storage.MIMEFromLogoKey("tenants/x/logo.png") != "image/png" {
		t.Fatal("mime from key")
	}
}

func TestDetectLogoMIMEIgnoresDeclaredTypeForNonImages(t *testing.T) {
	if _, err := storage.DetectLogoMIME("image/png", []byte("<svg xmlns='http://www.w3.org/2000/svg'><script>alert(1)</script></svg>")); err == nil {
		t.Fatal("SVG declared as PNG must be rejected")
	}
	if mime, err := storage.DetectLogoMIME("text/plain", []byte("\x89PNG\r\n\x1a\n0000")); err != nil || mime != "image/png" {
		t.Fatalf("real PNG: got %q %v", mime, err)
	}
}
