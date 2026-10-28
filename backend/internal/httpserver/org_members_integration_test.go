package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// TEC-1036 acceptance (HTTP): the platform edits a member's kind and
// organization roles in place (roles of the org type only, empty → default
// role), cannot demote or remove the last owner nor narrow / remove its own
// membership; every change is audited and emitted; the members list carries
// role_slugs; removal drops the membership; the user overview lists the
// memberships with roles.
func TestIntegrationPlatformOrganizationMembers(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dealer := it.org("t1036-dealer", "dealer", center)

	admin, apw := it.user("t1036-admin", rbac.RoleSuperAdmin)
	it.member(dealer, admin, "staff")
	owner, _ := it.user("t1036-owner")
	it.member(dealer, owner, "owner")
	staff, _ := it.user("t1036-staff")
	it.member(dealer, staff, "staff")
	adminTok := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": admin.Email.String, "password": apw,
	})).AccessToken
	plain, ppw := it.user("t1036-plain")
	plainTok := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": plain.Email.String, "password": ppw,
	})).AccessToken

	base := "/v1/platform/organizations/" + dealer.Uuid.String() + "/members/"
	put := func(tok, userUUID string, body map[string]any, want int) envelope {
		t.Helper()
		code, env := it.do("PUT", base+userUUID, hostOlex, tok, body)
		if code != want {
			t.Fatalf("PUT %s = %d %s, want %d", userUUID, code, errCode(env), want)
		}
		return env
	}
	memberRoles := func(userUUID string) (string, []string) {
		t.Helper()
		code, env := it.do("GET", "/v1/platform/organizations/"+dealer.Uuid.String(), hostOlex, adminTok, nil)
		if code != http.StatusOK {
			t.Fatalf("get org: %d", code)
		}
		var out struct {
			Members []struct {
				UUID      string   `json:"uuid"`
				Role      string   `json:"role"`
				RoleSlugs []string `json:"role_slugs"`
			} `json:"members"`
		}
		if err := json.Unmarshal(env.Data, &out); err != nil {
			t.Fatal(err)
		}
		for _, m := range out.Members {
			if m.UUID == userUUID {
				return m.Role, m.RoleSlugs
			}
		}
		return "", nil
	}

	// Without platform.organizations.write: 403.
	put(plainTok, staff.Uuid.String(), map[string]any{"role": "staff"}, http.StatusForbidden)

	// Staff → staff + accounting only (the default staff role goes away).
	env := put(adminTok, staff.Uuid.String(), map[string]any{
		"role": "staff", "role_slugs": []string{rbac.RoleDealerAccounting},
	}, http.StatusOK)
	var view struct {
		Role      string   `json:"role"`
		RoleSlugs []string `json:"role_slugs"`
	}
	if err := json.Unmarshal(env.Data, &view); err != nil || view.Role != "staff" ||
		!slices.Equal(view.RoleSlugs, []string{rbac.RoleDealerAccounting}) {
		t.Fatalf("update view = %+v %v", view, err)
	}
	if role, slugs := memberRoles(staff.Uuid.String()); role != "staff" || !slices.Equal(slugs, []string{rbac.RoleDealerAccounting}) {
		t.Fatalf("members list = %s %v", role, slugs)
	}
	// A center role cannot be granted in a dealer.
	put(adminTok, staff.Uuid.String(), map[string]any{
		"role": "staff", "role_slugs": []string{rbac.RoleCenterAccounting},
	}, http.StatusBadRequest)
	put(adminTok, staff.Uuid.String(), map[string]any{"role": "boss"}, http.StatusBadRequest)

	// Promote to owner with no explicit roles → default owner role.
	put(adminTok, staff.Uuid.String(), map[string]any{"role": "owner"}, http.StatusOK)
	if role, slugs := memberRoles(staff.Uuid.String()); role != "owner" || !slices.Equal(slugs, []string{rbac.RoleDealerOwner}) {
		t.Fatalf("promoted = %s %v", role, slugs)
	}
	// Two owners: one can be demoted, then the remaining one cannot.
	put(adminTok, owner.Uuid.String(), map[string]any{"role": "staff"}, http.StatusOK)
	env = put(adminTok, staff.Uuid.String(), map[string]any{"role": "staff"}, http.StatusConflict)
	if errCode(env) != response.CodeLastOwner {
		t.Fatalf("last owner demote code = %s", errCode(env))
	}
	if code, env := it.do("DELETE", base+staff.Uuid.String(), hostOlex, adminTok, nil); code != http.StatusConflict ||
		errCode(env) != response.CodeLastOwner {
		t.Fatalf("last owner remove = %d %s", code, errCode(env))
	}

	// Own membership: adding a role is fine, narrowing or removing is not.
	put(adminTok, admin.Uuid.String(), map[string]any{
		"role": "staff", "role_slugs": []string{rbac.RoleDealerStaff, rbac.RoleDealerAccounting},
	}, http.StatusOK)
	env = put(adminTok, admin.Uuid.String(), map[string]any{
		"role": "staff", "role_slugs": []string{rbac.RoleDealerAccounting},
	}, http.StatusConflict)
	if errCode(env) != response.CodeSelfMembership {
		t.Fatalf("self narrow code = %s", errCode(env))
	}
	if code, env := it.do("DELETE", base+admin.Uuid.String(), hostOlex, adminTok, nil); code != http.StatusConflict ||
		errCode(env) != response.CodeSelfMembership {
		t.Fatalf("self remove = %d %s", code, errCode(env))
	}

	// User overview lists the membership with its roles.
	code, env := it.do("GET", "/v1/platform/users/"+owner.Uuid.String()+"/overview", hostOlex, adminTok, nil)
	if code != http.StatusOK {
		t.Fatalf("overview: %d %s", code, errCode(env))
	}
	var ov struct {
		GlobalRoles []string `json:"global_roles"`
		Memberships []struct {
			Organization struct {
				UUID string `json:"uuid"`
				Type string `json:"type"`
			} `json:"organization"`
			Role       string   `json:"role"`
			RoleSlugs  []string `json:"role_slugs"`
			Warehouses []any    `json:"warehouses"`
		} `json:"memberships"`
		Customer struct {
			VehicleCount int64 `json:"vehicle_count"`
		} `json:"customer"`
		Sessions []any `json:"sessions"`
	}
	if err := json.Unmarshal(env.Data, &ov); err != nil {
		t.Fatal(err)
	}
	if len(ov.Memberships) != 1 || ov.Memberships[0].Organization.UUID != dealer.Uuid.String() ||
		ov.Memberships[0].Organization.Type != "dealer" || ov.Memberships[0].Role != "staff" ||
		!slices.Equal(ov.Memberships[0].RoleSlugs, []string{rbac.RoleDealerStaff}) ||
		ov.Memberships[0].Warehouses == nil || ov.GlobalRoles == nil || ov.Sessions == nil {
		t.Fatalf("overview = %+v", ov)
	}
	code, env = it.do("GET", "/v1/platform/users/"+admin.Uuid.String()+"/overview", hostOlex, adminTok, nil)
	if err := json.Unmarshal(env.Data, &ov); code != http.StatusOK || err != nil ||
		!slices.Contains(ov.GlobalRoles, rbac.RoleSuperAdmin) || len(ov.Sessions) == 0 {
		t.Fatalf("admin overview = %d %+v", code, ov)
	}
	if code, _ := it.do("GET", "/v1/platform/users/"+owner.Uuid.String()+"/overview", hostOlex, plainTok, nil); code != http.StatusForbidden {
		t.Fatalf("overview without users.read = %d", code)
	}

	// Remove the former owner (now staff).
	if code, env := it.do("DELETE", base+owner.Uuid.String(), hostOlex, adminTok, nil); code != http.StatusNoContent {
		t.Fatalf("remove = %d %s", code, errCode(env))
	}
	if role, _ := memberRoles(owner.Uuid.String()); role != "" {
		t.Fatalf("removed member still listed (%s)", role)
	}
	if code, _ := it.do("DELETE", base+owner.Uuid.String(), hostOlex, adminTok, nil); code != http.StatusNotFound {
		t.Fatalf("remove twice = %d", code)
	}

	var audits int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM activity_events
		WHERE resource = 'organizations' AND resource_uuid = $1 AND actor_user_id = $2
		  AND action IN ('organizations.member_updated', 'organizations.member_removed')`,
		dealer.Uuid, admin.ID).Scan(&audits); err != nil || audits != 5 {
		t.Fatalf("audit events = %d %v", audits, err)
	}
	var evs int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_name IN ($1, $2)
		AND (payload->'data'->>'organization_id')::bigint = $3`,
		events.TenantMemberUpdated, events.TenantMemberRemoved, dealer.ID).Scan(&evs); err != nil || evs != 5 {
		t.Fatalf("outbox member events = %d %v", evs, err)
	}
}
