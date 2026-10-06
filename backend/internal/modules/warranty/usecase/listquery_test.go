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

// TEC-377: list parameters of GET /v1/warranties.
func TestParseListFilter(t *testing.T) {
	f, err := ParseListFilter(url.Values{
		"status": {"active,void"}, "end_from": {"2026-01-01"}, "end_to": {"2026-01-31"},
		"days_left_max": {"30"}, "sort": {"-end_at"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Statuses) != 2 || f.EndFrom == nil || f.EndBefore == nil || f.DaysLeftMax == nil ||
		f.Sort.Key != "end_at" || !f.Sort.Desc || !f.sqlOnly() {
		t.Fatalf("filter = %+v", f)
	}
	if k, d := (ListFilter{}).sortArgs(); k != "expiry" || d {
		t.Fatalf("default sort = %s %v", k, d)
	}
	var ve *apiquery.ValidationError
	for _, bad := range []url.Values{{"status": {"open"}}, {"sort": {"holder"}}, {"organization_uuid": {"x"}}, {"start_to": {"x"}}} {
		if _, err := ParseListFilter(bad); !errors.As(err, &ve) {
			t.Fatalf("%v: err = %v", bad, err)
		}
	}
	var fe *ValidationError
	if _, err := ParseListFilter(url.Values{"days_left_min": {"x"}}); !errors.As(err, &fe) {
		t.Fatalf("days_left_min: %v", err)
	}
}

func TestListExportQueryScope(t *testing.T) {
	own := Caller{Filter: scopefilter.Filter{Scope: rbac.ScopeAssigned, UserID: 9, OrgID: 5, OrgIDs: []int64{5}}}
	q, err := ListExportQuery(own, map[string]string{"sort": "product"})
	if err != nil || q[ioengine.QueryScopeFilter] != "5" || q[QueryScopeUser] != "9" || q["sort"] != "product" {
		t.Fatalf("query = %v %v", q, err)
	}
	if _, err := ListExportQuery(Caller{Filter: scopefilter.Filter{Scope: rbac.ScopeCustomer, UserID: 3}}, nil); !errors.Is(err, ErrExportScope) {
		t.Fatalf("customer scope = %v", err)
	}
}
