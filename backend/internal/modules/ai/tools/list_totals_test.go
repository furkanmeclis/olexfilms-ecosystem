package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

// TEC-647: list totals are real counts, never the capped row count.

// orgFixture is a scope of organizations: ListInScope pages it like the
// SQL does, CountInScope counts it.
type orgFixture []orgusecase.Organization

func (o orgFixture) match(in orgusecase.ScopedListInput) []orgusecase.Organization {
	var out []orgusecase.Organization
	for _, org := range o {
		if (in.Type == "" || org.Type == in.Type) && strings.Contains(org.Name, in.Q) {
			out = append(out, org)
		}
	}
	return out
}

func (o orgFixture) ListInScope(_ context.Context, _ scopefilter.Filter, in orgusecase.ScopedListInput) ([]orgusecase.Organization, error) {
	rows := o.match(in)
	return rows[:min(len(rows), int(in.Limit))], nil
}

func (o orgFixture) CountInScope(_ context.Context, _ scopefilter.Filter, in orgusecase.ScopedListInput, exclude uuid.UUID) (int64, error) {
	var n int64
	for _, org := range o.match(in) {
		if org.UUID != exclude {
			n++
		}
	}
	return n, nil
}

// centerWithDealers is the caller's own center (sorted first, like the
// SQL's type order) plus n dealers.
func centerWithDealers(n int, ownType string) orgFixture {
	f := orgFixture{{UUID: orgUUID, Slug: "self", Name: "Merkez", Type: ownType}}
	for i := range n {
		f = append(f, orgusecase.Organization{UUID: uuid.New(), Slug: fmt.Sprintf("d%d", i), Name: fmt.Sprintf("Bayi %02d", i), Type: "dealer"})
	}
	return f
}

type listOut struct {
	Items     []json.RawMessage `json:"items"`
	Returned  int               `json:"returned"`
	Total     *int64            `json:"total"`
	Truncated bool              `json:"truncated"`
	Hint      string            `json:"hint"`
}

func runList(t *testing.T, tool Tool, p Principal, input string) listOut {
	t.Helper()
	res, err := tool.Run(context.Background(), Env{Principal: p}, json.RawMessage(input))
	if err != nil || res.IsError {
		t.Fatalf("%s: %v %+v", input, err, res)
	}
	var out listOut
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatalf("decode %s: %v", res.Content, err)
	}
	return out
}

func TestListSubOrganizationsReportsRealTotal(t *testing.T) {
	p := superAdmin()
	tool := ListSubOrganizations{orgs: centerWithDealers(12, OrgDealer), tree: fakeTree{}}
	// The caller is itself in the type filter here: it is neither listed
	// nor counted.
	out := runList(t, tool, p, `{"limit":5}`)
	if out.Returned != 5 || out.Total == nil || *out.Total != 12 || !out.Truncated || !strings.Contains(out.Hint, "5 of 12") {
		t.Fatalf("limit 5 of 12: %+v", out)
	}
	out = runList(t, tool, p, `{"limit":20}`)
	if out.Returned != 12 || out.Total == nil || *out.Total != 12 || out.Truncated || out.Hint != "" {
		t.Fatalf("all 12: %+v", out)
	}
	// limit 11: 13 rows hold the caller plus 12 dealers, so the cut is
	// still seen with the caller's row dropped.
	out = runList(t, tool, p, `{"limit":11}`)
	if out.Returned != 11 || out.Total == nil || *out.Total != 12 || !out.Truncated {
		t.Fatalf("limit 11 of 12: %+v", out)
	}
	tool.orgs = centerWithDealers(12, OrgCenter)
	if out = runList(t, tool, p, `{"limit":5}`); out.Total == nil || *out.Total != 12 {
		t.Fatalf("center caller: %+v", out)
	}
}

func TestListSubOrganizationsQueryOnlyReportsCut(t *testing.T) {
	tool := ListSubOrganizations{orgs: centerWithDealers(12, OrgCenter), tree: fakeTree{}}
	out := runList(t, tool, superAdmin(), `{"query":"Bayi","limit":5}`)
	if out.Returned != 5 || out.Total != nil || !out.Truncated || !strings.Contains(out.Hint, "total not counted") {
		t.Fatalf("query cut: %+v", out)
	}
	out = runList(t, tool, superAdmin(), `{"query":"Bayi 03","limit":5}`)
	if out.Returned != 1 || out.Truncated || out.Hint != "" {
		t.Fatalf("query fits: %+v", out)
	}
}

// cappedDealers answers like the dealer finder at its NearbyMaxResults cap.
type cappedDealers int

func (n cappedDealers) rows() []orgusecase.NearbyDealer {
	out := make([]orgusecase.NearbyDealer, int(n))
	for i := range out {
		out[i] = orgusecase.NearbyDealer{UUID: uuid.New(), Slug: fmt.Sprintf("d%d", i), Name: "Dealer", City: "Istanbul"}
	}
	return out
}

func (n cappedDealers) NearbyDealers(context.Context, int64, orgusecase.NearbyInput) ([]orgusecase.NearbyDealer, error) {
	return n.rows(), nil
}

func (n cappedDealers) AreaDealers(context.Context, int64, orgusecase.AreaInput) ([]orgusecase.NearbyDealer, error) {
	return n.rows(), nil
}

func TestFindNearestDealersCappedHasNoTotal(t *testing.T) {
	p := realmPrincipal(RealmVisitor)
	tool := FindNearestDealers{dealers: cappedDealers(orgusecase.NearbyMaxResults)}
	out := runList(t, tool, p, `{"city":"Istanbul","limit":5}`)
	if out.Returned != 5 || out.Total != nil || !out.Truncated {
		t.Fatalf("at the finder cap: %+v", out)
	}
	tool.dealers = cappedDealers(12)
	out = runList(t, tool, p, `{"city":"Istanbul","limit":5}`)
	if out.Returned != 5 || out.Total == nil || *out.Total != 12 || !out.Truncated {
		t.Fatalf("under the cap: %+v", out)
	}
}

func TestCutListEnvelope(t *testing.T) {
	l := NewCutList([]int{1, 2}, true)
	if l.Total != nil || !l.Truncated || !strings.Contains(l.Hint, "first 2") {
		t.Fatalf("cut: %+v", l)
	}
	if l := NewCutList[int](nil, false); l.Truncated || l.Items == nil || l.Hint != "" || l.Total != nil {
		t.Fatalf("not cut: %+v", l)
	}
	if b, _ := json.Marshal(NewList[int](nil, 0)); !strings.Contains(string(b), `"total":0`) {
		t.Fatalf("exact zero total must stay: %s", b)
	}
}
