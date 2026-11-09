package httpserver

import (
	"context"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	searchusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
)

// TEC-1061 decision follow-up: the global search (GET /v1/search and the
// products group of /v1/search/global) never shows a dealer or distributor
// a product the center closed to dealer orders; the center still finds it.
// The check reads Postgres, so it holds before the index is refreshed.
func TestIntegrationSearchHidesClosedProductsOutsideCenter(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dealer := it.org("t1061s-dealer", "dealer", center)
	dist := it.org("t1061s-dist", "distributor", center)

	open := it.product(center, "T1061S-OPEN")
	shut := it.product(center, "T1061S-SHUT")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET dealer_orderable = false WHERE id = $1`, shut.ID); err != nil {
		t.Fatal(err)
	}

	svc := searchusecase.New(nil, nil, it.q, nil)
	hits := []searchengine.Hit{
		{Spec: searchengine.SpecProducts, ID: shut.Uuid.String()},
		{Spec: searchengine.SpecProducts, ID: open.Uuid.String()},
	}
	as := func(org db.Organization, name string) context.Context {
		u, _ := it.user(name)
		it.member(org, u, "owner")
		id := org.Uuid
		return authctx.WithPrincipal(ctx, authctx.Principal{UserInternal: u.ID, OrganizationUUID: &id})
	}
	for _, c := range []struct {
		label    string
		ctx      context.Context
		wantShut bool
	}{
		{"center", as(center, "t1061s-center"), true},
		{"dealer", as(dealer, "t1061s-dealer-owner"), false},
		{"distributor", as(dist, "t1061s-dist-owner"), false},
	} {
		got := map[string]bool{}
		for _, h := range svc.VisibleHits(c.ctx, hits, 10) {
			got[h.ID] = true
		}
		if !got[open.Uuid.String()] {
			t.Fatalf("%s: open product missing: %v", c.label, got)
		}
		if got[shut.Uuid.String()] != c.wantShut {
			t.Fatalf("%s: closed product shown = %v, want %v", c.label, got[shut.Uuid.String()], c.wantShut)
		}
	}
}
