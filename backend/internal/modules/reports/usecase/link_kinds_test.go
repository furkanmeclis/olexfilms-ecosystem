package usecase

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/refs"
)

// TEC-1216: report row links use the shared record kind vocabulary
// (docs/refs.md); "report" is the only navigation-only kind.
func TestLinkKindsUseRefsVocabulary(t *testing.T) {
	for _, k := range []string{LinkService, LinkOrder, LinkCustomer, LinkOrganization, LinkWarranty, LinkMeasurement, LinkProduct} {
		if !refs.IsKind(k) {
			t.Fatalf("report link kind %q is not in refs.Kinds", k)
		}
	}
	if refs.IsKind(LinkReport) {
		t.Fatal("report must stay a navigation-only link kind")
	}
}
