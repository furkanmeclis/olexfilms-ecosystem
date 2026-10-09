package migrator

import (
	"database/sql"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
)

// The TEC-257 step queries stay inside the read-only guard.
func TestStockQueriesPassGuard(t *testing.T) {
	for _, q := range []string{whWarehousesQuery, whSitesQuery, whLocationsQuery, hubStockItemsQuery, whBarcodesQuery, whBinStocksQuery} {
		if err := source.CheckReadOnly(q); err != nil {
			t.Errorf("guard refused %q: %v", q, err)
		}
	}
}

func TestWHCode(t *testing.T) {
	cases := map[string]string{
		"SYN-WH-1":                           "SYN_WH_1",
		" a.01 ":                             "A_01",
		"Depo İç":                            "DEPO___",
		"":                                   "",
		"0123456789012345678901234567890123": "01234567890123456789012345678901",
	}
	for in, want := range cases {
		if got := whCode(in); got != want {
			t.Errorf("whCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLocationType(t *testing.T) {
	aisle := &importedLocation{Type: locAisle}
	shelf := &importedLocation{Type: locShelf}
	bin := &importedLocation{Type: locBin}
	cases := []struct {
		legacy  string
		parent  *importedLocation
		typ     string
		asShelf bool
		ok      bool
	}{
		{"aisle", nil, locAisle, false, true},
		{"shelf", nil, locShelf, false, true},
		{"bin", nil, locShelf, true, true},
		{"shelf", aisle, locShelf, false, true},
		{"bin", aisle, locShelf, true, true},
		{"BIN", shelf, locBin, false, true},
		{"aisle", aisle, "", false, false},
		{"bin", bin, "", false, false},
		{"zone", nil, "", false, false},
	}
	for _, c := range cases {
		typ, asShelf, ok := locationType(c.legacy, c.parent)
		if typ != c.typ || asShelf != c.asShelf || ok != c.ok {
			t.Errorf("locationType(%q, %+v) = %q, %v, %v", c.legacy, c.parent, typ, asShelf, ok)
		}
	}
}

func TestHubOwnership(t *testing.T) {
	const center, dealer = 1, 7
	cases := []struct {
		loc, status string
		dealer      int64
		want        ownership
		ok          bool
	}{
		{"center", "available", 0, ownership{Status: unitAvailable, OwnerType: ownerOrg, OwnerID: center, Holder: center, Quantity: 1}, true},
		{"dealer", "available", dealer, ownership{Status: unitAvailable, OwnerType: ownerOrg, OwnerID: dealer, Holder: dealer, Quantity: 1}, true},
		{"dealer", "reserved", dealer, ownership{Status: unitAvailable, OwnerType: ownerOrg, OwnerID: dealer, Holder: dealer, Quantity: 1}, true},
		{"dealer", "available", 0, ownership{Status: unitAvailable, Pending: "dealer_unmapped"}, true},
		{"trash", "used", 0, ownership{Status: unitUsed, OwnerType: ownerTrash, OwnerID: center, Holder: center, Quantity: 1}, true},
		{"trash", "used", dealer, ownership{Status: unitUsed, OwnerType: ownerTrash, OwnerID: dealer, Holder: dealer, Quantity: 1}, true},
		{"service", "used", dealer, ownership{Status: unitUsed, Pending: "service"}, true},
		{"dealer", "used", dealer, ownership{Status: unitUsed, Pending: "service"}, true},
		{"center", "used", 0, ownership{Status: unitUsed, Pending: "service"}, true},
		{"service", "available", dealer, ownership{}, false},
		{"moon", "used", 0, ownership{}, false},
	}
	for _, c := range cases {
		got, ok := hubOwnership(c.loc, c.status, c.dealer, center)
		if got != c.want || ok != c.ok {
			t.Errorf("hubOwnership(%s, %s, %d) = %+v, %v", c.loc, c.status, c.dealer, got, ok)
		}
	}
}

func TestWHOwnership(t *testing.T) {
	const center, loc = 1, 9
	cases := []struct {
		status string
		loc    int64
		want   ownership
		ok     bool
	}{
		{"reserved", 0, ownership{Status: unitReserved}, true},
		{"printed", loc, ownership{Status: unitPrinted}, true},
		{"placed", loc, ownership{Status: unitPlaced, OwnerType: ownerLocation, OwnerID: loc, Holder: center, Quantity: 3}, true},
		{"placed", 0, ownership{Status: unitAvailable, OwnerType: ownerOrg, OwnerID: center, Holder: center, Quantity: 3, Pending: "placed_without_location"}, true},
		{"void", 0, ownership{Status: unitVoid, OwnerType: ownerTrash, OwnerID: center, Holder: center, Quantity: 3}, true},
		{"used", 0, ownership{Status: unitUsed, Pending: "wh_used"}, true},
		{"in_transit", loc, ownership{Status: unitInTransit, Pending: "wh_in_transit"}, true},
		{"lost", 0, ownership{}, false},
	}
	for _, c := range cases {
		got, ok := whOwnership(c.status, c.loc, center, 3)
		if got != c.want || ok != c.ok {
			t.Errorf("whOwnership(%s, %d) = %+v, %v", c.status, c.loc, got, ok)
		}
	}
}

func TestUnitWinner(t *testing.T) {
	t0 := time.Date(2025, 3, 1, 9, 0, 0, 0, time.UTC)
	hub := &unitSide{UpdatedAt: t0}
	wh := &unitSide{UpdatedAt: t0}
	if w := (&unitCandidate{Hub: hub, WH: wh}).winner(); w != wh {
		t.Error("a tie must go to the warehouse")
	}
	hub.UpdatedAt = t0.Add(time.Hour)
	if w := (&unitCandidate{Hub: hub, WH: wh}).winner(); w != hub {
		t.Error("the newer hub row must win")
	}
	if w := (&unitCandidate{Hub: hub}).winner(); w != hub {
		t.Error("a single side wins")
	}
	early := sql.NullTime{Time: t0, Valid: true}
	hub.CreatedAt, wh.CreatedAt = sql.NullTime{Time: t0.Add(time.Hour), Valid: true}, early
	if got := earliest(&unitCandidate{Hub: hub, WH: wh}); got != early {
		t.Errorf("earliest = %v", got)
	}
}

func TestOrderLocations(t *testing.T) {
	parent := func(id string) sql.NullString { return sql.NullString{String: id, Valid: id != ""} }
	in := []legacyLocation{
		{ID: "bin", ParentID: parent("shelf")},
		{ID: "shelf", ParentID: parent("aisle")},
		{ID: "orphan", ParentID: parent("missing")},
		{ID: "aisle"},
	}
	var got []string
	for _, l := range orderLocations(in) {
		got = append(got, l.ID)
	}
	want := []string{"aisle", "shelf", "bin", "orphan"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}
