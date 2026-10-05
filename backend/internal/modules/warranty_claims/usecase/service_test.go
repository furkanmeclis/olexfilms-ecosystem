package usecase

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/xuri/excelize/v2"
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

func TestCanForwardDealerReviewToCenter(t *testing.T) {
	ok, err := canForwardDealerReviewToCenter(false, true, pgtype.Text{String: "center", Valid: true})
	if err != nil || !ok {
		t.Fatalf("direct dealer -> center_review = %v, %v", ok, err)
	}
	ok, err = canForwardDealerReviewToCenter(false, true, pgtype.Text{String: "distributor", Valid: true})
	if !errors.Is(err, ErrUnsupportedFlow) || ok {
		t.Fatalf("dealer under distributor bypass = %v, %v", ok, err)
	}
	ok, err = canForwardDealerReviewToCenter(false, false, pgtype.Text{String: "center", Valid: true})
	if !errors.Is(err, ErrForbidden) || ok {
		t.Fatalf("missing permission = %v, %v", ok, err)
	}
	ok, err = canForwardDealerReviewToCenter(true, false, pgtype.Text{String: "distributor", Valid: true})
	if err != nil || !ok {
		t.Fatalf("distributor review = %v, %v", ok, err)
	}
}

func TestClaimReports(t *testing.T) {
	productA := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	productB := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	lotA := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	lotB := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	dealerA := uuid.MustParse("dddddddd-dddd-dddd-dddd-ddddddddddda")
	dealerB := uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddb")
	store := &reportStore{
		productRows: []db.WarrantyClaimFailureRateByProductRow{
			{ProductUuid: productA, ProductSku: "PPF-A", ProductName: "PPF A", WarrantyCount: 6, ClaimCount: 2, ApprovedClaimCount: 1},
			{ProductUuid: productB, ProductSku: "PPF-B", ProductName: "PPF B", WarrantyCount: 4, ClaimCount: 1, ApprovedClaimCount: 1},
		},
		lotRows: []db.WarrantyClaimFailureRateByLotRow{
			{UnitUuid: lotA, LotCode: "LOT-A", ProductUuid: productA, ProductSku: "PPF-A", ProductName: "PPF A", WarrantyCount: 6, ClaimCount: 2, ApprovedClaimCount: 1},
			{UnitUuid: lotB, LotCode: "LOT-B", ProductUuid: productB, ProductSku: "PPF-B", ProductName: "PPF B", WarrantyCount: 4, ClaimCount: 1, ApprovedClaimCount: 1},
		},
		dealerRows: []db.WarrantyClaimsByDealerReportRow{
			{OrganizationUuid: dealerA, OrganizationName: "Dealer A", OrganizationType: "dealer", ClaimCount: 2, ApprovedClaimCount: 1, RejectedClaimCount: 1},
			{OrganizationUuid: dealerB, OrganizationName: "Dealer B", OrganizationType: "dealer", ClaimCount: 1, ApprovedClaimCount: 1},
		},
		partsRows: []db.WarrantyClaimPartsReportRow{
			{PartKey: "hood", ProductUuid: pgUUID(productA), ProductSku: "PPF-A", ProductName: "PPF A", PartCount: 2, ClaimCount: 2, ApprovedClaimCount: 1},
			{PartKey: "roof", ProductUuid: pgUUID(productB), ProductSku: "PPF-B", ProductName: "PPF B", PartCount: 1, ClaimCount: 1, ApprovedClaimCount: 1},
		},
	}
	svc := NewWithStore(store, nil, nil)
	c := Caller{BrandID: 10, Filter: scopefilter.Filter{Scope: rbac.ScopeBrand, BrandID: 10}}

	productReport, err := svc.FailureRateReport(context.Background(), c, model.ReportFilter{Group: "product"})
	if err != nil {
		t.Fatal(err)
	}
	if len(productReport.Items) != 2 || productReport.Items[0].ApprovedRate != 1.0/6.0 || productReport.Items[0].ClaimRate != 2.0/6.0 {
		t.Fatalf("product report = %+v", productReport.Items)
	}
	if productReport.Items[0].ApprovedClaimCount != 1 {
		t.Fatalf("rejected claim counted as approved: %+v", productReport.Items[0])
	}

	lotReport, err := svc.FailureRateReport(context.Background(), c, model.ReportFilter{Group: "lot"})
	if err != nil {
		t.Fatal(err)
	}
	if len(lotReport.Items) != 2 || lotReport.Items[0].LotCode == nil || *lotReport.Items[0].LotCode != "LOT-A" || lotReport.Items[1].ApprovedRate != 0.25 {
		t.Fatalf("lot report = %+v", lotReport.Items)
	}

	dealerReport, err := svc.ByDealerReport(context.Background(), c, model.ReportFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(dealerReport.Items) != 2 || dealerReport.Items[0].ApprovalRate != 0.5 || dealerReport.Items[0].RejectedClaimCount != 1 {
		t.Fatalf("dealer report = %+v", dealerReport.Items)
	}

	distCaller := Caller{BrandID: 10, Filter: scopefilter.Filter{Scope: rbac.ScopeSubtree, OrgIDs: []int64{200, 201}}}
	if _, err := svc.ByDealerReport(context.Background(), distCaller, model.ReportFilter{}); err != nil {
		t.Fatal(err)
	}
	if got := store.lastDealerOrgIDs; len(got) != 2 || got[0] != 200 || got[1] != 201 {
		t.Fatalf("distributor scope org ids = %v", got)
	}
}

func TestClaimReportXLSXRowsMatchReport(t *testing.T) {
	productA := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	productB := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	store := &reportStore{
		orgs: map[int64]db.Organization{100: {ID: 100, BrandID: 10, Type: "center"}},
		productRows: []db.WarrantyClaimFailureRateByProductRow{
			{ProductUuid: productA, ProductSku: "PPF-A", ProductName: "PPF A", WarrantyCount: 6, ClaimCount: 2, ApprovedClaimCount: 1},
			{ProductUuid: productB, ProductSku: "PPF-B", ProductName: "PPF B", WarrantyCount: 4, ClaimCount: 1, ApprovedClaimCount: 1},
		},
	}
	svc := NewWithStore(store, nil, nil)
	adapter := NewFailureRateAdapter(svc)
	ds, err := adapter.Export(context.Background(), ioengine.ExportQuery{
		ioengine.QueryOrganizationID: "100",
		QueryGroup:                   "product",
	}, i18n.LocaleEN)
	if err != nil {
		t.Fatal(err)
	}
	data, err := ioengine.EncodeExport(ioengine.ExportXLSX, ds, "en", nil, "Failure rate")
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(rows)-1, len(ds.Rows); got != want {
		t.Fatalf("xlsx rows = %d, report rows = %d", got, want)
	}
}

type reportStore struct {
	Store
	productRows      []db.WarrantyClaimFailureRateByProductRow
	lotRows          []db.WarrantyClaimFailureRateByLotRow
	dealerRows       []db.WarrantyClaimsByDealerReportRow
	partsRows        []db.WarrantyClaimPartsReportRow
	orgs             map[int64]db.Organization
	descendants      map[int64][]db.Organization
	lastDealerOrgIDs []int64
}

func (s *reportStore) WarrantyClaimFailureRateByProduct(_ context.Context, arg db.WarrantyClaimFailureRateByProductParams) ([]db.WarrantyClaimFailureRateByProductRow, error) {
	return s.productRows, nil
}

func (s *reportStore) WarrantyClaimFailureRateByLot(_ context.Context, arg db.WarrantyClaimFailureRateByLotParams) ([]db.WarrantyClaimFailureRateByLotRow, error) {
	return s.lotRows, nil
}

func (s *reportStore) WarrantyClaimsByDealerReport(_ context.Context, arg db.WarrantyClaimsByDealerReportParams) ([]db.WarrantyClaimsByDealerReportRow, error) {
	s.lastDealerOrgIDs = append([]int64(nil), arg.OrganizationIds...)
	return s.dealerRows, nil
}

func (s *reportStore) WarrantyClaimPartsReport(_ context.Context, arg db.WarrantyClaimPartsReportParams) ([]db.WarrantyClaimPartsReportRow, error) {
	return s.partsRows, nil
}

func (s *reportStore) GetOrganizationByID(_ context.Context, id int64) (db.Organization, error) {
	if org, ok := s.orgs[id]; ok {
		return org, nil
	}
	return db.Organization{}, errors.New("not found")
}

func (s *reportStore) Descendants(_ context.Context, id int64) ([]db.Organization, error) {
	return s.descendants[id], nil
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
