package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
)

// fakeOrgResolver has no memberships; centers maps brand id → center org.
type fakeOrgResolver struct {
	centers map[int64]db.Organization
}

func (fakeOrgResolver) GetOrganizationMemberByUserAndOrgUUID(context.Context, int64, uuid.UUID) (db.GetOrganizationMemberByUserAndOrgUUIDRow, error) {
	return db.GetOrganizationMemberByUserAndOrgUUIDRow{}, pgx.ErrNoRows
}

func (f fakeOrgResolver) GetBrandCenter(_ context.Context, brandID int64) (db.Organization, error) {
	o, ok := f.centers[brandID]
	if !ok {
		return db.Organization{}, pgx.ErrNoRows
	}
	return o, nil
}

// TEC-522: an organization-less super admin works in the domain brand's
// center; everyone else still needs an organization claim.
func TestRequireOrganizationSuperAdminCenterFallback(t *testing.T) {
	tokens, err := jwt.NewManager("test-secret-test-secret-test-secret", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	center := db.Organization{ID: 11, Uuid: uuid.New(), Slug: "olex-merkez", Name: "Olex Merkez", Status: "active", Type: "center", BrandID: 3}
	resolver := fakeOrgResolver{centers: map[int64]db.Organization{3: center}}

	var got orgctx.Scope
	var gotOK bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, gotOK = orgctx.ScopeFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	call := func(superAdmin bool, brandID int64) int {
		tok, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New(), SessionID: uuid.New(), IsSuperAdmin: superAdmin})
		if err != nil {
			t.Fatal(err)
		}
		ctx := authctx.WithPrincipal(context.Background(), authctx.Principal{UserInternal: 1, IsSuperAdmin: superAdmin})
		ctx = brandctx.WithBrand(ctx, brandctx.Brand{ID: brandID, Slug: "olex"})
		req := httptest.NewRequest(http.MethodGet, "/v1/platform/contract-templates", nil).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		got, gotOK = orgctx.Scope{}, false
		RequireOrganizationResolver(tokens, resolver)(next).ServeHTTP(rec, req)
		return rec.Code
	}

	if code := call(true, 3); code != http.StatusOK {
		t.Fatalf("super admin: status %d", code)
	}
	if !gotOK || got.InternalID != center.ID || got.OrgType != "center" || got.BrandID != 3 || !got.SuperAdminFallback {
		t.Fatalf("super admin scope = %+v (ok=%v)", got, gotOK)
	}
	if code := call(false, 3); code != http.StatusForbidden {
		t.Fatalf("non super admin without org: status %d, want 403", code)
	}
	if code := call(true, 99); code != http.StatusForbidden {
		t.Fatalf("super admin on a brand without center: status %d, want 403", code)
	}
}

func TestRequireFeatureSkipsSuperAdminFallback(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	ctx := orgctx.WithScope(context.Background(), orgctx.Scope{InternalID: 7, SuperAdminFallback: true})
	rec := httptest.NewRecorder()
	RequireFeature(offChecker{key: "module.x"}, "module.x")(ok).ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil).WithContext(ctx))
	if rec.Code != http.StatusOK {
		t.Fatalf("fallback scope: got %d want 200", rec.Code)
	}
}
