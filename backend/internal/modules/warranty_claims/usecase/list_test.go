package usecase

import (
	"errors"
	"net/url"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

// TEC-377: list parameters of GET /v1/warranty-claims.
func TestParseListFilter(t *testing.T) {
	f, err := ParseListFilter(url.Values{
		"q": {"kaput"}, "status": {"open,approved"}, "service_uuid": {"7f0d7f6a-56c4-4c38-9a59-6f2f1a1b2c3d"},
		"created_from": {"2026-03-01"}, "created_to": {"2026-03-01"}, "sort": {"claim_no"}, "limit": {"500"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.Q != "kaput" || len(f.Statuses) != 2 || f.ServiceID.String() == "00000000-0000-0000-0000-000000000000" ||
		f.CreatedFrom == nil || f.CreatedTo.Sub(*f.CreatedFrom).Hours() != 24 || f.SortKey != "claim_no" || f.SortDesc ||
		f.Limit != apiquery.DefaultLimit {
		t.Fatalf("filter = %+v", f)
	}
	var ve *apiquery.ValidationError
	for _, bad := range []url.Values{
		{"status": {"bogus"}}, {"sort": {"description"}}, {"service_uuid": {"x"}}, {"warranty_uuid": {"x"}},
		{"organization_uuid": {"x"}}, {"created_from": {"03/01/2026"}},
	} {
		if _, err := ParseListFilter(bad); !errors.As(err, &ve) {
			t.Fatalf("%v: err = %v", bad, err)
		}
	}
	if escapeLike(`50%_\`) != `50\%\_\\` {
		t.Fatal("escapeLike")
	}
}
