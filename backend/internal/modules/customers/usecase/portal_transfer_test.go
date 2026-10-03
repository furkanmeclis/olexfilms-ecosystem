package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/google/uuid"
)

// TEC-243: a portal caller reaches only the user's own records; a session
// without a user reaches nothing; staff callers are not portal callers.
func TestPortalCaller(t *testing.T) {
	c := PortalCaller(7, 42, i18n.Locale("tr"))
	if !c.IsPortal() || c.portalUserID != 42 || c.UserID != 42 || c.Org.BrandID != 7 || c.Org.InternalID != 0 {
		t.Fatalf("portal caller = %+v", c)
	}
	if !c.Filter.AllowsOrg(99, 7) || c.Filter.AllowsOrg(99, 8) {
		t.Fatal("portal filter must allow the brand's organizations only")
	}
	if anon := PortalCaller(7, 0, ""); !anon.IsPortal() || anon.portalUserID != -1 {
		t.Fatalf("anonymous portal caller = %+v", anon)
	}
	if (Caller{UserID: 42}).IsPortal() {
		t.Fatal("staff caller reported as portal")
	}
}

func TestPortalStartTransferNeedsPortalCaller(t *testing.T) {
	s := transferService()
	if _, err := s.PortalStartTransfer(context.Background(), Caller{UserID: 1}, uuid.New(),
		StartTransferInput{Phone: "+905551112233"}, activity.Meta{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("staff caller on the portal start = %v", err)
	}
}
