package usecase

import (
	"errors"
	"net/url"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

// TEC-373: unit list parameters; LIKE wildcards in a barcode prefix are
// matched literally.
func TestParseUnitFilter(t *testing.T) {
	in, err := ParseUnitFilter(url.Values{
		"status": {"available,void"}, "barcode": {"AB"}, "barcode_match": {"prefix"}, "sort": {"-meters"},
		"location_uuid": {"6f1f9c2e-6b0e-4d0f-9d55-6d2a1d7b0c11"}, "updated_from": {"2026-10-01"},
	})
	if err != nil || len(in.Statuses) != 2 || !in.BarcodePrefix || in.Sort.Key != "meters" || !in.Sort.Desc ||
		len(in.LocationUUIDs) != 1 || in.UpdatedFrom == nil || !unitSQLOnly(in) {
		t.Fatalf("filter = %+v, %v", in, err)
	}
	if in, _ := ParseUnitFilter(url.Values{"q": {"x"}}); unitSQLOnly(in) || in.Sort.Key != "product" {
		t.Fatalf("default filter = %+v", in)
	}
	for _, bad := range []url.Values{
		{"status": {"lost"}}, {"sort": {"price"}}, {"barcode_match": {"fuzzy"}}, {"location_uuid": {"x"}},
	} {
		var ve *apiquery.ValidationError
		if _, err := ParseUnitFilter(bad); !errors.As(err, &ve) {
			t.Errorf("%v: want validation error, got %v", bad, err)
		}
	}
	if got := likePrefix(`A_1%\`); got != `A\_1\%\\` {
		t.Fatalf("likePrefix = %q", got)
	}
}
