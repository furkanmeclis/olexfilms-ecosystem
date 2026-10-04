package usecase

import (
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/model"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCheckCoverage(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	ok := CheckCoverage(CoverageInput{
		Now: now, WarrantyStatus: "active", StartAt: now.AddDate(-1, 0, 0), EndAt: now.AddDate(1, 0, 0),
		WarrantyDurationMonths: pgtype.Int4{Int32: 24, Valid: true},
		AppliedPartsJSON:       []byte(`["body_kaput","body_tavan"]`),
		Parts:                  []model.PartInput{{PartKey: "body_kaput"}},
	})
	if !ok.OK || len(ok.Reasons) != 0 {
		t.Fatalf("active coverage = %+v", ok)
	}

	expired := CheckCoverage(CoverageInput{
		Now: now, WarrantyStatus: "expired", StartAt: now.AddDate(-2, 0, 0), EndAt: now.Add(-time.Hour),
		WarrantyDurationMonths: pgtype.Int4{Int32: 24, Valid: true},
		AppliedPartsJSON:       []byte(`["body_kaput"]`),
		Parts:                  []model.PartInput{{PartKey: "body_tavan"}},
	})
	if expired.OK {
		t.Fatalf("expired coverage ok: %+v", expired)
	}
	want := map[string]bool{"warranty_not_active": true, "warranty_period_expired": true, "part_not_covered": true}
	for _, reason := range expired.Reasons {
		delete(want, reason)
	}
	if len(want) != 0 {
		t.Fatalf("missing reasons: %v in %+v", want, expired)
	}
}
