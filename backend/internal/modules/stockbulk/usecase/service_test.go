package usecase

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
)

func TestSplitBarcodeLine(t *testing.T) {
	for in, want := range map[string][2]string{
		"OLX1":         {"OLX1", ""},
		" OLX1;12,5 ":  {"OLX1", "12,5"},
		"OLX1\t30":     {"OLX1", "30"},
		"OLX1 30":      {"OLX1", "30"},
		"OLX1,30.5":    {"OLX1", "30.5"},
		"OLX1,ABC,DEF": {"OLX1,ABC,DEF", ""},
	} {
		a, b := splitBarcodeLine(in)
		if a != want[0] || b != want[1] {
			t.Errorf("%q -> %q %q, want %v", in, a, b, want)
		}
	}
}

func TestParseMeters(t *testing.T) {
	if cm, err := parseMeters("12,5"); err != nil || cm != 1250 {
		t.Fatalf("12,5 = %d %v", cm, err)
	}
	for _, bad := range []string{"0", "-1", "1.234", "x", "100001"} {
		if _, err := parseMeters(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// The protection rules: units at a dealer, used in a service, consumed,
// in transit, reserved or not in stock are excluded with their reason.
func TestClassify(t *testing.T) {
	const org = 1
	base := db.ListStockBulkCandidatesRow{
		ID: 1, Barcode: "B1", UnitKind: ledger.KindSerial, UnitStatus: "available",
		OwnerType: "warehouse_location", OwnerID: 9, HolderOrgID: org, StateStatus: "placed",
	}
	roll := pgtype.Numeric{}
	_ = roll.Scan("25")
	r := resolved{overrides: map[string]int64{}}
	cases := []struct {
		name   string
		action string
		mod    func(*db.ListStockBulkCandidatesRow)
		reason string
		cm     int64
	}{
		{"service", ActionConvertToRoll, func(u *db.ListStockBulkCandidatesRow) { u.InService = true }, ReasonUsedInService, 0},
		{"dealer", ActionConvertToRoll, func(u *db.ListStockBulkCandidatesRow) { u.HolderOrgID = 7 }, ReasonAtOtherOrg, 0},
		{"used", ActionSetLot, func(u *db.ListStockBulkCandidatesRow) { u.StateStatus = "used" }, ReasonConsumed, 0},
		{"transit", ActionConvertToRoll, func(u *db.ListStockBulkCandidatesRow) { u.StateStatus = "in_transit" }, ReasonInTransit, 0},
		{"reserved", ActionConvertToRoll, func(u *db.ListStockBulkCandidatesRow) { u.Reserved = true }, ReasonReserved, 0},
		{"label", ActionConvertToRoll, func(u *db.ListStockBulkCandidatesRow) {
			u.OwnerType, u.HolderOrgID, u.StateStatus, u.UnitStatus = "", 0, "", "printed"
		}, ReasonNotInStock, 0},
		{"label lot ok", ActionSetLot, func(u *db.ListStockBulkCandidatesRow) {
			u.OwnerType, u.HolderOrgID, u.StateStatus, u.UnitStatus = "", 0, "", "printed"
		}, "", 0},
		{"no meters", ActionConvertToRoll, func(*db.ListStockBulkCandidatesRow) {}, ReasonNoMeters, 0},
		{"product default", ActionConvertToRoll, func(u *db.ListStockBulkCandidatesRow) { u.DefaultRollMeters = roll }, "", 2500},
		{"already roll", ActionConvertToRoll, func(u *db.ListStockBulkCandidatesRow) { u.InitialMeters = roll }, ReasonAlreadyRoll, 0},
	}
	for _, c := range cases {
		u := base
		c.mod(&u)
		reason, cm := classify(c.action, org, u, r)
		if reason != c.reason || cm != c.cm {
			t.Errorf("%s: %q %d, want %q %d", c.name, reason, cm, c.reason, c.cm)
		}
	}
	// A per-barcode override wins over the job and product meters.
	r.meters, r.overrides["B1"] = 1000, 1250
	u := base
	u.DefaultRollMeters = roll
	if reason, cm := classify(ActionConvertToRoll, org, u, r); reason != "" || cm != 1250 {
		t.Errorf("override: %q %d", reason, cm)
	}
}
