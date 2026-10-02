package usecase

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

func months(n int32) pgtype.Int4 { return pgtype.Int4{Int32: n, Valid: true} }

// The shared rules are covered through the listener adapter in
// TestSkipReason (period_test.go); here the scan grouping.

func TestGroupRepairCandidates(t *testing.T) {
	row := func(sid, id int64, brand string, m int32) db.ListWarrantyRepairCandidatesRow {
		return db.ListWarrantyRepairCandidatesRow{
			ServiceID: sid, ID: id, ServiceBrandSlug: brand, UnitBrandSlug: brand,
			UnitSource: "generated", WarrantyDurationMonths: months(m),
		}
	}
	services, eligible := groupRepairCandidates([]db.ListWarrantyRepairCandidatesRow{
		row(3, 30, "olex", 0), row(3, 31, "olex", 12), // one eligible item is enough
		row(5, 50, "glorian", 12), // Glorian only
		row(7, 70, "olex", 0),     // no period only
	})
	if len(services) != 3 || services[0] != 3 || services[1] != 5 || services[2] != 7 {
		t.Fatalf("services = %v", services)
	}
	if !eligible[3] || eligible[5] || eligible[7] {
		t.Fatalf("eligible = %v", eligible)
	}
}

func TestNewRepairScannerDefaults(t *testing.T) {
	if d := NewRepairScanner(nil, 0, nil).Days(); d != DefaultRepairScanDays {
		t.Fatalf("days = %d", d)
	}
	if d := NewRepairScanner(nil, 7, nil).Days(); d != 7 {
		t.Fatalf("days = %d", d)
	}
}
