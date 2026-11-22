package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// memberResolver returns one membership row of the given organization type.
type memberResolver struct{ orgType string }

func (m memberResolver) GetOrganizationMemberByUserAndOrgUUID(_ context.Context, userID int64, orgUUID uuid.UUID) (db.GetOrganizationMemberByUserAndOrgUUIDRow, error) {
	return db.GetOrganizationMemberByUserAndOrgUUIDRow{
		OrganizationID: 157, UserID: userID, Role: "staff", OrganizationUuid: orgUUID,
		OrganizationSlug: "org", OrganizationStatus: "active", OrganizationName: "Org",
		OrganizationBrandID: 1, OrganizationType: m.orgType, BrandSlug: "olex",
	}, nil
}

// TEC-1266: a super admin inside a distributor or dealer panel is capped to
// that organization's reach; in a center (and for non super admins) nothing
// changes.
func TestRequireOrganizationCapsSuperAdminScope(t *testing.T) {
	tokens, err := jwt.NewManager("test-secret-test-secret-test-secret", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		orgType     string
		superAdmin  bool
		wantCeiling rbac.Scope
		wantRead    rbac.Scope
	}{
		{rbac.OrgTypeDistributor, true, rbac.ScopeSubtree, rbac.ScopeSubtree},
		{rbac.OrgTypeDealer, true, rbac.ScopeManaged, rbac.ScopeManaged},
		{rbac.OrgTypeCenter, true, "", rbac.ScopeAll},
		{rbac.OrgTypeDistributor, false, "", ""},
	}
	for _, tc := range cases {
		var got authctx.Principal
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = authctx.MustPrincipal(r.Context())
			w.WriteHeader(http.StatusOK)
		})
		oid := uuid.New()
		tok, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New(), SessionID: uuid.New(), IsSuperAdmin: tc.superAdmin, OrganizationID: &oid})
		if err != nil {
			t.Fatal(err)
		}
		ctx := authctx.WithPrincipal(context.Background(), authctx.Principal{UserInternal: 2, IsSuperAdmin: tc.superAdmin})
		ctx = brandctx.WithBrand(ctx, brandctx.Brand{ID: 1, Slug: "olex"})
		req := httptest.NewRequest(http.MethodGet, "/v1/reports/overview", nil).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		RequireOrganizationResolver(tokens, memberResolver{orgType: tc.orgType})(next).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s super=%v: status %d", tc.orgType, tc.superAdmin, rec.Code)
		}
		if got.ScopeCeiling != tc.wantCeiling {
			t.Fatalf("%s super=%v: ceiling %q, want %q", tc.orgType, tc.superAdmin, got.ScopeCeiling, tc.wantCeiling)
		}
		if tc.superAdmin {
			if s, _ := got.ScopeFor(rbac.PermServicesRead); s != tc.wantRead {
				t.Fatalf("%s: services.read scope %q, want %q", tc.orgType, s, tc.wantRead)
			}
		}
	}
}
