package usecase

import (
	"errors"
	"net/url"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

// TEC-377: list parameters of GET /v1/services.
func TestParseListFilter(t *testing.T) {
	f, err := ParseListFilter(url.Values{
		"status": {"pending,completed"}, "organization_uuid": {"7f0d7f6a-56c4-4c38-9a59-6f2f1a1b2c3d"},
		"created_to": {"2026-01-11"}, "sort": {"-organization"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Statuses) != 2 || len(f.OrganizationUUIDs) != 1 || f.CreatedTo == nil ||
		f.Sort.Key != "organization" || !f.Sort.Desc || !f.SortExplicit || !f.sqlOnly() {
		t.Fatalf("filter = %+v", f)
	}
	if k, d := (ListFilter{}).sortArgs(); k != "created_at" || !d {
		t.Fatalf("default sort = %s %v", k, d)
	}
	if (ListFilter{Q: "x", Statuses: []string{"ready"}}).sqlOnly() {
		t.Fatal("q + status stays on the index")
	}
	var ve *apiquery.ValidationError
	for _, bad := range []url.Values{
		{"status": {"open"}}, {"sort": {"customer"}}, {"organization_uuid": {"x"}}, {"completed_from": {"x"}},
	} {
		if _, err := ParseListFilter(bad); !errors.As(err, &ve) {
			t.Fatalf("%v: err = %v", bad, err)
		}
	}
}

// TEC-377: the export job keeps the scope (and the user of an own scope);
// a customer scope cannot export.
func TestListExportQueryScope(t *testing.T) {
	own := Caller{Filter: scopefilter.Filter{Scope: rbac.ScopeOwn, UserID: 9, OrgID: 5, OrgIDs: []int64{5}}}
	q, err := ListExportQuery(own, map[string]string{"status": "ready", "sort": "plate", "limit": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if q[ioengine.QueryScopeFilter] != "5" || q[QueryScopeUser] != "9" || q["sort"] != "plate" || q["limit"] != "" {
		t.Fatalf("query = %v", q)
	}
	brand := Caller{Filter: scopefilter.Filter{Scope: rbac.ScopeBrand, BrandID: 2}}
	if q, err := ListExportQuery(brand, nil); err != nil || q[ioengine.QueryScopeFilter] != ioengine.ScopeFilterBrand || q[QueryScopeUser] != "" {
		t.Fatalf("brand query = %v %v", q, err)
	}
	if _, err := ListExportQuery(Caller{Filter: scopefilter.Filter{Scope: rbac.ScopeCustomer, UserID: 3}}, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("customer scope = %v", err)
	}
	if _, err := ListExportQuery(brand, map[string]string{"sort": "x"}); err == nil {
		t.Fatal("bad sort must fail")
	}
}
