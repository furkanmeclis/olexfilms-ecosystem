package usecase

import (
	"errors"
	"net/url"
	"strconv"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

func TestParseListValues(t *testing.T) {
	f, err := ParseListValues(url.Values{
		"status": {"active,pending"}, "q": {"filo"}, "sort": {"-vehicle_count"},
		"vehicle_count_min": {"2"}, "vehicle_count_max": {"9"}, "limit": {"50"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Statuses) != 2 || f.Q != "filo" || len(f.Sort) != 1 || !f.Sort[0].Desc || f.Limit != 50 ||
		f.VehicleCountMin == nil || *f.VehicleCountMin != 2 || f.VehicleCountMax == nil || *f.VehicleCountMax != 9 {
		t.Fatalf("filter = %+v", f)
	}
	var qe *apiquery.ValidationError
	if _, err := ParseListValues(url.Values{"status": {"active,closed"}}); !errors.As(err, &qe) {
		t.Fatalf("bad status: err = %v", err)
	}
	if _, err := ParseListValues(url.Values{"vehicle_count_min": {"x"}}); !errors.As(err, &qe) {
		t.Fatalf("bad vehicle_count_min: err = %v", err)
	}
}

func TestListExportQuery(t *testing.T) {
	c := Caller{OrgID: 7, BrandID: 1, Filter: scopefilter.Filter{Scope: rbac.ScopeSubtree, OrgID: 7, OrgIDs: []int64{7, 9}}}
	q, err := ListExportQuery(c, map[string]string{"status": "active", "q": " filo ", "limit": "5", "unknown": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if q[ioengine.QueryScopeFilter] != "7,9" || q["status"] != "active" || q["q"] != "filo" {
		t.Fatalf("query = %+v", q)
	}
	if _, ok := q["limit"]; ok {
		t.Fatalf("limit kept: %+v", q)
	}
	if _, ok := q["unknown"]; ok {
		t.Fatalf("unknown key kept: %+v", q)
	}
	brand := Caller{OrgID: 1, BrandID: 1, Filter: scopefilter.Filter{Scope: rbac.ScopeBrand, OrgID: 1, BrandID: 1}}
	if q, err := ListExportQuery(brand, nil); err != nil || q[ioengine.QueryScopeFilter] != ioengine.ScopeFilterBrand {
		t.Fatalf("brand query = %+v, %v", q, err)
	}
	var qe *apiquery.ValidationError
	if _, err := ListExportQuery(c, map[string]string{"status": "closed"}); !errors.As(err, &qe) {
		t.Fatalf("bad status: err = %v", err)
	}
	empty := Caller{OrgID: 7, BrandID: 1, Filter: scopefilter.Filter{Scope: rbac.ScopeManaged, OrgID: 7, OrgIDs: []int64{}}}
	if _, err := ListExportQuery(empty, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("empty scope: err = %v", err)
	}
}

func TestFleetListExportAdapter(t *testing.T) {
	f := newAPIFixture(t)
	ctx := f.ctx
	// d1 opens fleet A (active link); d2 opens fleet B and d1 asks for a
	// link to it (pending for d1).
	taxA := validVKN(t, f.suffix)
	if _, err := f.svc.Open(ctx, f.dealer(f.d1), OpenInput{Name: "T477 Filo A", LegalName: "T477 Filo A A.Ş.", TaxNumber: taxA}); err != nil {
		t.Fatal(err)
	}
	b, err := f.svc.Open(ctx, f.dealer(f.d2), OpenInput{Name: "T477 Filo B", LegalName: "T477 Filo B Ltd.", TaxNumber: validVKN(t, f.suffix[:len(f.suffix)-1])})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.RequestLink(ctx, f.dealer(f.d1), b.UUID); err != nil {
		t.Fatal(err)
	}

	export := func(c Caller, body map[string]string, loc i18n.Locale) ioengine.Dataset {
		t.Helper()
		q, err := ListExportQuery(c, body)
		if err != nil {
			t.Fatal(err)
		}
		q[ioengine.QueryOrganizationID] = strconv.FormatInt(c.OrgID, 10)
		ds, err := NewListExportAdapter(f.svc).Export(ctx, q, loc)
		if err != nil {
			t.Fatal(err)
		}
		return ds
	}

	ds := export(f.dealer(f.d1), map[string]string{"status": "active"}, i18n.LocaleEN)
	if ds.Resource != ResourceList || len(ds.Columns) != 8 || len(ds.Rows) != 1 {
		t.Fatalf("active export = %+v", ds)
	}
	row := ds.Rows[0]
	if row["name"] != "T477 Filo A" || row["legal_name"] != "T477 Filo A A.Ş." || row["tax_number"] != taxA ||
		row["vehicle_count"] != "0" || row["last_service_at"] != "" || row["link_status"] != "Active" ||
		row["dealer"] != f.d1.Name || row["started_at"] == "" {
		t.Fatalf("active row = %+v", row)
	}

	ds = export(f.dealer(f.d1), map[string]string{"status": "pending"}, i18n.LocaleTR)
	if len(ds.Rows) != 1 || ds.Rows[0]["name"] != "T477 Filo B" || ds.Rows[0]["link_status"] != "Onay bekliyor" ||
		ds.Rows[0]["started_at"] != "" {
		t.Fatalf("pending export = %+v", ds.Rows)
	}

	ds = export(f.dealer(f.d1), map[string]string{"sort": "-name"}, i18n.LocaleEN)
	if len(ds.Rows) != 2 || ds.Rows[0]["name"] != "T477 Filo B" || ds.Rows[1]["name"] != "T477 Filo A" {
		t.Fatalf("sorted export = %+v", ds.Rows)
	}

	// The worker re-authorizes the stored scope against the job
	// organization: d2's id stored by a d1 job is dropped (no rows).
	q, err := ListExportQuery(f.dealer(f.d2), nil)
	if err != nil {
		t.Fatal(err)
	}
	q[ioengine.QueryOrganizationID] = strconv.FormatInt(f.d1.ID, 10)
	if _, err := NewListExportAdapter(f.svc).Export(ctx, q, i18n.LocaleEN); !errors.Is(err, ioengine.ErrJobScope) {
		t.Fatalf("foreign scope: err = %v", err)
	}
}
