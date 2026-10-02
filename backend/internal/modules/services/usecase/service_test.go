package usecase

import (
	"regexp"
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestServiceNoFormat(t *testing.T) {
	re := regexp.MustCompile(`^DS[A-Z0-9]{8}$`)
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		no, err := newServiceNo()
		if err != nil {
			t.Fatal(err)
		}
		if !re.MatchString(no) || len(no) > 32 {
			t.Fatalf("service no %q", no)
		}
		seen[no] = true
	}
	if len(seen) < 195 {
		t.Fatalf("service numbers repeat: %d unique of 200", len(seen))
	}
}

func dealerCaller(scope rbac.Scope, cancel bool) Caller {
	scopes := map[string]rbac.Scope{
		rbac.PermServicesRead:  rbac.ScopeManaged,
		rbac.PermServicesWrite: scope,
	}
	if cancel {
		scopes[rbac.PermServicesCancel] = rbac.ScopeBrand
	}
	return Caller{
		Principal: authctx.Principal{UserInternal: 7, PermissionScopes: scopes},
		Org:       orgctx.Scope{InternalID: 10, BrandID: 1, OrgType: OrgDealer},
		Filter:    scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{10}, OrgID: 10, UserID: 7},
	}
}

func centerCaller() Caller {
	return Caller{
		Principal: authctx.Principal{UserInternal: 1, PermissionScopes: map[string]rbac.Scope{
			rbac.PermServicesRead: rbac.ScopeBrand, rbac.PermServicesWrite: rbac.ScopeBrand,
			rbac.PermServicesCancel: rbac.ScopeBrand,
		}},
		Org:    orgctx.Scope{InternalID: 1, BrandID: 1, OrgType: OrgCenter},
		Filter: scopefilter.Filter{Scope: rbac.ScopeBrand, BrandID: 1, OrgID: 1, UserID: 1},
	}
}

func svcRow(status string, org int64, createdBy int64) db.Service {
	return db.Service{ID: 5, OrganizationID: org, BrandID: 1, Status: status,
		CreatedByUserID: pgtype.Int8{Int64: createdBy, Valid: createdBy != 0}}
}

func TestAvailableTransitions(t *testing.T) {
	dealer := dealerCaller(rbac.ScopeManaged, false)
	center := centerCaller()
	cases := []struct {
		name   string
		c      Caller
		status string
		want   []string
	}{
		{"dealer draft", dealer, StatusDraft, []string{StatusPending}},
		{"dealer pending", dealer, StatusPending, []string{StatusProcessing}},
		{"dealer processing", dealer, StatusProcessing, []string{StatusReady}},
		{"dealer ready", dealer, StatusReady, []string{}},
		{"dealer completed", dealer, StatusCompleted, []string{}},
		{"center draft", center, StatusDraft, []string{StatusPending, StatusProcessing, StatusCancelled}},
		{"center pending", center, StatusPending, []string{StatusProcessing, StatusReady, StatusCancelled}},
		{"center ready", center, StatusReady, []string{StatusCancelled}},
		{"center cancelled", center, StatusCancelled, []string{}},
	}
	for _, tc := range cases {
		got := availableTransitions(tc.c, svcRow(tc.status, 10, 7))
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
	// Completion is never offered (TEC-180).
	for from := range transitions {
		if _, ok := transitions[from][StatusCompleted]; ok {
			t.Fatalf("%s -> completed must not be in the stock-free machine", from)
		}
	}
}

func TestAllowsScopes(t *testing.T) {
	own := dealerCaller(rbac.ScopeOwn, false)
	if !own.allows(rbac.PermServicesWrite, svcRow(StatusDraft, 10, 7)) {
		t.Fatal("own scope must reach the caller's own service")
	}
	if own.allows(rbac.PermServicesWrite, svcRow(StatusDraft, 10, 8)) {
		t.Fatal("own scope must not reach a colleague's service")
	}
	managed := dealerCaller(rbac.ScopeManaged, false)
	if managed.allows(rbac.PermServicesWrite, svcRow(StatusDraft, 11, 7)) {
		t.Fatal("managed scope must not reach another organization")
	}
	if managed.allows(rbac.PermServicesCancel, svcRow(StatusDraft, 10, 7)) {
		t.Fatal("a dealer without services.cancel must not cancel")
	}
	if !centerCaller().allows(rbac.PermServicesCancel, svcRow(StatusDraft, 10, 7)) {
		t.Fatal("the center cancels in its brand")
	}
	other := svcRow(StatusDraft, 10, 7)
	other.BrandID = 2
	if centerCaller().allows(rbac.PermServicesCancel, other) {
		t.Fatal("brand scope must not cross brands")
	}
}

func TestFormLockAndItems(t *testing.T) {
	dealer := dealerCaller(rbac.ScopeManaged, false)
	for _, st := range []string{StatusCompleted, StatusCancelled} {
		if formEditable(dealer, svcRow(st, 10, 7)) {
			t.Fatalf("dealer edits %s", st)
		}
		if !formEditable(centerCaller(), svcRow(st, 10, 7)) {
			t.Fatalf("center cannot edit %s", st)
		}
	}
	for st, want := range map[string]bool{
		StatusDraft: true, StatusPending: true, StatusProcessing: true,
		StatusReady: false, StatusCompleted: false, StatusCancelled: false,
	} {
		if itemsEditable(st) != want {
			t.Fatalf("itemsEditable(%s) != %v", st, want)
		}
	}
}

func TestVisibleOwnScope(t *testing.T) {
	c := dealerCaller(rbac.ScopeManaged, false)
	c.Filter.Scope = rbac.ScopeOwn
	if visible(c, svcRow(StatusDraft, 10, 8)) {
		t.Fatal("own read scope must hide a colleague's service")
	}
	if !visible(c, svcRow(StatusDraft, 10, 7)) {
		t.Fatal("own read scope must show the caller's service")
	}
}

func TestNormalizeMetersAndParts(t *testing.T) {
	if m, _, ok := normalizeMeters("1.5"); !ok || m != "1.50" {
		t.Fatalf("1.5 -> %q %v", m, ok)
	}
	for _, bad := range []string{"0", "-1", "1.234", "abc", ""} {
		if _, _, ok := normalizeMeters(bad); ok {
			t.Fatalf("%q accepted", bad)
		}
	}
	parts, err := normalizeParts([]string{" hood ", "hood", "roof"})
	if err != nil || !slices.Equal(parts, []string{"hood", "roof"}) {
		t.Fatalf("parts %v %v", parts, err)
	}
	if _, err := normalizeParts([]string{""}); err == nil {
		t.Fatal("empty part accepted")
	}
}
