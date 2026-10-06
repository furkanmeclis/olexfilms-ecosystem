package apiquery_test

import (
	"errors"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

var testSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"name": "name", "created": "created_at"},
	Default: apiquery.SortField{Field: "created", Desc: true},
}

func wantValidation(t *testing.T, err error, field string) {
	t.Helper()
	var ve *apiquery.ValidationError
	if !errors.As(err, &ve) || len(ve.Details) == 0 || ve.Details[0].Field != field {
		t.Fatalf("want ValidationError on %q, got %v", field, err)
	}
}

func TestResolveSort(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  string
		want apiquery.ResolvedSort
	}{
		{"", apiquery.ResolvedSort{Key: "created_at", Desc: true}},
		{"name", apiquery.ResolvedSort{Key: "name"}},
		{"-name", apiquery.ResolvedSort{Key: "name", Desc: true}},
		// API field maps to the trusted key; extra fields are ignored.
		{"created,-name", apiquery.ResolvedSort{Key: "created_at"}},
	}
	for _, c := range cases {
		got, err := apiquery.ResolveSort(apiquery.ParseSort(c.raw), testSpec)
		if err != nil || got != c.want {
			t.Fatalf("ResolveSort(%q) = %+v, %v; want %+v", c.raw, got, err, c.want)
		}
	}
	for _, raw := range []string{"bogus", "-name,bogus", "created_at"} {
		_, err := apiquery.ResolveSort(apiquery.ParseSort(raw), testSpec)
		wantValidation(t, err, "sort")
	}
	if got := testSpec.DefaultString(); got != "-created" {
		t.Fatalf("DefaultString = %q", got)
	}
	if got := testSpec.Fields(); !reflect.DeepEqual(got, []string{"created", "name"}) {
		t.Fatalf("Fields = %v", got)
	}
}

func TestResolveSortBadDefaultPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for a default outside the whitelist")
		}
	}()
	_, _ = apiquery.ResolveSort(nil, apiquery.SortSpec{
		Columns: apiquery.SortColumns{"name": "name"}, Default: apiquery.SortField{Field: "x"},
	})
}

func TestBuiltinSortSpecsDefaults(t *testing.T) {
	t.Parallel()
	for name, spec := range map[string]apiquery.SortSpec{
		"users": apiquery.UsersSortSpec, "tenants": apiquery.TenantsSortSpec,
		"notifications": apiquery.NotificationsSortSpec, "logs": apiquery.LogsSortSpec,
	} {
		if _, err := apiquery.ResolveSort(nil, spec); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, ok := apiquery.LogsSort["message"]; ok {
		t.Fatal("logs message must not be sortable")
	}
	for _, f := range []string{"city", "access_ends_at"} {
		if _, ok := apiquery.TenantsSort[f]; !ok {
			t.Fatalf("tenants sort lacks %s", f)
		}
	}
}

func TestCSVValuesAndEnumList(t *testing.T) {
	t.Parallel()
	v := url.Values{"status": {"active, disabled,,active", "pending"}}
	if got := apiquery.CSVValues(v, "status"); !reflect.DeepEqual(got, []string{"active", "disabled", "pending"}) {
		t.Fatalf("CSVValues = %v", got)
	}
	got, err := apiquery.EnumList(v, "status", "active", "disabled", "pending")
	if err != nil || len(got) != 3 {
		t.Fatalf("EnumList = %v, %v", got, err)
	}
	got, err = apiquery.EnumList(url.Values{}, "status", "active")
	if err != nil || got != nil {
		t.Fatalf("absent = %v, %v; want nil", got, err)
	}
	_, err = apiquery.EnumList(url.Values{"status": {"active,bogus"}}, "status", "active")
	wantValidation(t, err, "status")
}

func TestDateRange(t *testing.T) {
	t.Parallel()
	r, err := apiquery.DateRange(url.Values{"created_from": {"2026-01-02"}, "created_to": {"2026-01-05"}}, "created")
	if err != nil {
		t.Fatal(err)
	}
	if !r.From.Equal(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)) ||
		!r.Before.Equal(time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date range = %v .. %v", r.From, r.Before)
	}
	r, err = apiquery.DateRange(url.Values{"created_to": {"2026-01-05T10:00:00+03:00"}}, "created")
	if err != nil || r.From != nil {
		t.Fatalf("rfc3339 = %+v, %v", r, err)
	}
	if !r.Before.Equal(time.Date(2026, 1, 5, 7, 0, 0, 1000, time.UTC)) {
		t.Fatalf("rfc3339 to = %v (want instant inclusive)", r.Before)
	}
	r, err = apiquery.DateRange(url.Values{}, "created")
	if err != nil || r.From != nil || r.Before != nil {
		t.Fatalf("empty = %+v, %v", r, err)
	}
	// Same day from/to is a valid one-day range.
	if _, err = apiquery.DateRange(url.Values{"d_from": {"2026-01-05"}, "d_to": {"2026-01-05"}}, "d"); err != nil {
		t.Fatal(err)
	}
	_, err = apiquery.DateRange(url.Values{"created_from": {"05.01.2026"}}, "created")
	wantValidation(t, err, "created_from")
	_, err = apiquery.DateRange(url.Values{"created_to": {"nope"}}, "created")
	wantValidation(t, err, "created_to")
	_, err = apiquery.DateRange(url.Values{"created_from": {"2026-01-06"}, "created_to": {"2026-01-05"}}, "created")
	wantValidation(t, err, "created_from")
}

func TestNumRangeAndBool(t *testing.T) {
	t.Parallel()
	r, err := apiquery.NumRange(url.Values{"total_min": {"10.5"}, "total_max": {"20"}}, "total")
	if err != nil || *r.Min != 10.5 || *r.Max != 20 {
		t.Fatalf("NumRange = %+v, %v", r, err)
	}
	r, err = apiquery.NumRange(url.Values{"total_max": {"5"}}, "total")
	if err != nil || r.Min != nil || *r.Max != 5 {
		t.Fatalf("max only = %+v, %v", r, err)
	}
	_, err = apiquery.NumRange(url.Values{"total_min": {"abc"}}, "total")
	wantValidation(t, err, "total_min")
	_, err = apiquery.NumRange(url.Values{"total_max": {"NaN"}}, "total")
	wantValidation(t, err, "total_max")
	_, err = apiquery.NumRange(url.Values{"total_min": {"9"}, "total_max": {"1"}}, "total")
	wantValidation(t, err, "total_min")

	b, err := apiquery.Bool(url.Values{"pinned": {"true"}}, "pinned")
	if err != nil || b == nil || !*b {
		t.Fatalf("Bool true = %v, %v", b, err)
	}
	b, err = apiquery.Bool(url.Values{}, "pinned")
	if err != nil || b != nil {
		t.Fatalf("Bool absent = %v, %v", b, err)
	}
	_, err = apiquery.Bool(url.Values{"pinned": {"1"}}, "pinned")
	wantValidation(t, err, "pinned")
}
