package usecase

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TestCountInScopeIsTheRealTotal: a distributor with 12 dealers; a page of
// 5 does not cap the count, the caller is excluded, q matches by SQL
// (TEC-647).
func TestCountInScopeIsTheRealTotal(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := db.New(pool)
	svc := New(pool, q)

	olex, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("olex brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, olex.ID)
	if err != nil {
		t.Fatalf("olex center: %v", err)
	}
	ctx = brandctx.WithBrand(ctx, brandctx.Brand{ID: olex.ID, Slug: olex.Slug, Status: "active"})
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	create := func(name, typ string, parentID int64) db.Organization {
		t.Helper()
		row, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
			Slug: "t647-" + name + "-" + suffix, Name: "T647 " + name, Status: "active",
			AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
			Type:           typ, ParentID: pgtype.Int8{Int64: parentID, Valid: true},
			BrandID: olex.ID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
			Settings: []byte("{}"),
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return row
	}
	dist := create("dist", TypeDistributor, center.ID)
	ids := []int64{dist.ID}
	for i := range 12 {
		ids = append(ids, create(fmt.Sprintf("d%02d", i), TypeDealer, dist.ID).ID)
	}
	f := scopefilter.Filter{Permission: rbac.PermOrganizationsRead, Scope: rbac.ScopeSubtree, OrgID: dist.ID, OrgIDs: ids}

	page, err := svc.ListInScope(ctx, f, ScopedListInput{Type: TypeDealer, Limit: 5})
	if err != nil || len(page) != 5 {
		t.Fatalf("page: %d %v", len(page), err)
	}
	for _, c := range []struct {
		in      ScopedListInput
		exclude uuid.UUID
		want    int64
	}{
		{ScopedListInput{Type: TypeDealer, Limit: 5}, dist.Uuid, 12},
		{ScopedListInput{}, uuid.Nil, 13},
		{ScopedListInput{}, dist.Uuid, 12},
		{ScopedListInput{Q: "T647 d0"}, dist.Uuid, 10},
	} {
		got, err := svc.CountInScope(ctx, f, c.in, c.exclude)
		if err != nil || got != c.want {
			t.Fatalf("%+v exclude %v: got %d (%v), want %d", c.in, c.exclude, got, err, c.want)
		}
	}
	if _, err := svc.CountInScope(ctx, f, ScopedListInput{Type: "fleet"}, uuid.Nil); err == nil {
		t.Fatal("invalid type must fail")
	}
}
