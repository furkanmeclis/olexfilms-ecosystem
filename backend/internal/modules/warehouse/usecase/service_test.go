package usecase

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

func managed(orgType string, id int64) Caller {
	return Caller{
		Org:    orgctx.Scope{InternalID: id, OrgType: orgType, BrandID: 1},
		Filter: scopefilter.Filter{Scope: rbac.ScopeManaged, OrgID: id, OrgIDs: []int64{id}},
	}
}

// K12: only the center and the distributor run warehouses.
func TestGuardOrgTypes(t *testing.T) {
	for _, typ := range []string{rbac.OrgTypeCenter, rbac.OrgTypeDistributor} {
		if id, err := guard(managed(typ, 7)); err != nil || id != 7 {
			t.Fatalf("%s: id=%d err=%v", typ, id, err)
		}
	}
	for _, typ := range []string{rbac.OrgTypeDealer, rbac.OrgTypePlatform, rbac.OrgTypeCustomer, ""} {
		if _, err := guard(managed(typ, 7)); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s: want ErrForbidden, got %v", typ, err)
		}
	}
	// A filter that does not reach the active organization.
	c := managed(rbac.OrgTypeDistributor, 7)
	c.Filter.OrgIDs = []int64{8}
	if _, err := guard(c); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign filter: %v", err)
	}
	// Own/assigned scopes do not fit organization-level records.
	c = managed(rbac.OrgTypeCenter, 7)
	c.Filter.Scope = rbac.ScopeOwn
	if _, err := guard(c); !errors.Is(err, ErrForbidden) {
		t.Fatalf("own scope: %v", err)
	}
	// The service refuses before touching the database.
	s := New(nil, nil)
	if _, err := s.ListWarehouses(context.Background(), managed(rbac.OrgTypeDealer, 3), nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("dealer list: %v", err)
	}
}

func TestNormCode(t *testing.T) {
	cases := map[string]string{" a1 ": "A1", "r_2": "R_2", "01": "01"}
	for in, want := range cases {
		got, err := normCode("code", in)
		if err != nil || got != want {
			t.Fatalf("%q: got %q err %v", in, got, err)
		}
	}
	for _, in := range []string{"", "  ", "A-1", "A.1", "Ç", "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456"} {
		if _, err := normCode("code", in); err == nil {
			t.Fatalf("%q: want error", in)
		}
	}
}

func TestChildTypes(t *testing.T) {
	ok := [][2]string{{"", TypeAisle}, {"", TypeShelf}, {TypeAisle, TypeShelf}, {TypeShelf, TypeBin}}
	for _, c := range ok {
		if !allowedChild(c[0], c[1]) {
			t.Fatalf("%v should be allowed", c)
		}
	}
	bad := [][2]string{{"", TypeBin}, {TypeAisle, TypeAisle}, {TypeAisle, TypeBin}, {TypeShelf, TypeShelf}, {TypeBin, TypeBin}}
	for _, c := range bad {
		if allowedChild(c[0], c[1]) {
			t.Fatalf("%v should be refused", c)
		}
	}
}

func ip(n int) *int { return &n }

func TestLevelCodes(t *testing.T) {
	got, err := levelCodes(0, GenerateLevel{Type: TypeShelf, From: ip(1), To: ip(3), Pad: 2})
	if err != nil || !reflect.DeepEqual(got, []string{"01", "02", "03"}) {
		t.Fatalf("range: %v %v", got, err)
	}
	got, err = levelCodes(0, GenerateLevel{Type: TypeShelf, From: ip(9), To: ip(10), Prefix: "r"})
	if err != nil || !reflect.DeepEqual(got, []string{"R9", "R10"}) {
		t.Fatalf("prefix: %v %v", got, err)
	}
	got, err = levelCodes(0, GenerateLevel{Type: TypeAisle, Codes: []string{"a", "B"}})
	if err != nil || !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Fatalf("codes: %v %v", got, err)
	}
	bad := []GenerateLevel{
		{Type: TypeAisle},
		{Type: TypeAisle, Codes: []string{"A"}, From: ip(1), To: ip(2)},
		{Type: TypeAisle, Codes: []string{"A", "a"}},
		{Type: TypeShelf, From: ip(5), To: ip(1)},
		{Type: TypeShelf, From: ip(1), To: ip(2), Pad: 9},
		{Type: TypeShelf, From: ip(1), To: ip(2), Prefix: "x-"},
	}
	for i, lv := range bad {
		if _, err := levelCodes(0, lv); err == nil {
			t.Fatalf("case %d: want error", i)
		}
	}
}

func TestOrderedIDs(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	found := map[uuid.UUID]int64{a: 1, b: 2}
	ids, err := orderedIDs([]uuid.UUID{b, a}, found)
	if err != nil || !reflect.DeepEqual(ids, []int64{2, 1}) {
		t.Fatalf("ids %v err %v", ids, err)
	}
	for _, in := range [][]uuid.UUID{nil, {a, a}, {a, uuid.New()}} {
		if _, err := orderedIDs(in, found); err == nil {
			t.Fatalf("%v: want error", in)
		}
	}
}

func TestGenerateRejectsBeforeDB(t *testing.T) {
	s := New(nil, nil)
	_, err := s.GenerateLocations(context.Background(), managed(rbac.OrgTypeCenter, 1), GenerateInput{RoomUUID: uuid.New()})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "levels" {
		t.Fatalf("want levels validation error, got %v", err)
	}
}
