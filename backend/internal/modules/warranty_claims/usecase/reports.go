package usecase

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	ResourceFailureRateReport = "tenant.warranty_claims.failure_rate"
	ResourceByDealerReport    = "tenant.warranty_claims.by_dealer"
	ResourcePartsReport       = "tenant.warranty_claims.parts"

	QueryFrom  = "from"
	QueryTo    = "to"
	QueryGroup = "group"
)

var errExportScope = errors.New("warranty claims export: organization is outside brand")

func reportGroup(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "", "product":
		return "product", nil
	case "lot":
		return "lot", nil
	default:
		return "", invalid("group", "must be product or lot")
	}
}

func reportPeriod(f model.ReportFilter) (pgtype.Timestamptz, pgtype.Timestamptz) {
	return tsPtr(f.From), tsPtr(f.To)
}

func rate(num, den int64) float64 {
	if den <= 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func reportFilterFromQuery(q ioengine.ExportQuery) (model.ReportFilter, error) {
	from, err := queryTime(q, QueryFrom)
	if err != nil {
		return model.ReportFilter{}, err
	}
	to, err := queryTime(q, QueryTo)
	if err != nil {
		return model.ReportFilter{}, err
	}
	return model.ReportFilter{From: from, To: to, Group: q[QueryGroup]}, nil
}

func queryTime(q ioengine.ExportQuery, key string) (*time.Time, error) {
	raw := strings.TrimSpace(q[key])
	if raw == "" {
		return nil, nil
	}
	if d, err := time.Parse(time.DateOnly, raw); err == nil {
		if key == QueryTo {
			d = d.AddDate(0, 0, 1)
		}
		return &d, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, invalid(key, "must be a date (YYYY-MM-DD) or RFC3339 timestamp")
	}
	return &t, nil
}

func (s *Service) exportCaller(ctx context.Context, q ioengine.ExportQuery) (Caller, error) {
	orgID, err := strconv.ParseInt(strings.TrimSpace(q[ioengine.QueryOrganizationID]), 10, 64)
	if err != nil || orgID <= 0 {
		return Caller{}, errors.New("warranty claims export: organization is required")
	}
	org, err := s.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return Caller{}, err
	}
	f := scopefilter.Filter{Permission: rbac.PermWarrantyClaimsRead, OrgID: org.ID, BrandID: org.BrandID}
	switch org.Type {
	case "center":
		f.Scope = rbac.ScopeBrand
	case "distributor":
		f.Scope = rbac.ScopeSubtree
		f.OrgIDs = []int64{org.ID}
		below, err := s.q.Descendants(ctx, org.ID)
		if err != nil {
			return Caller{}, err
		}
		for _, o := range below {
			if o.BrandID != org.BrandID {
				return Caller{}, errExportScope
			}
			f.OrgIDs = append(f.OrgIDs, o.ID)
		}
	default:
		f.Scope = rbac.ScopeManaged
		f.OrgIDs = []int64{org.ID}
	}
	return Caller{OrganizationID: org.ID, BrandID: org.BrandID, OrgType: org.Type, Filter: f}, nil
}

type exportOnly struct{}

func (exportOnly) ImportSchema() []ioengine.ImportField { return nil }
func (exportOnly) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "export only"}, nil
}
func (exportOnly) RevertRow(context.Context, string, string, map[string]any) error { return nil }

type FailureRateAdapter struct {
	exportOnly
	svc *Service
}

func NewFailureRateAdapter(svc *Service) *FailureRateAdapter { return &FailureRateAdapter{svc: svc} }
func (a *FailureRateAdapter) Resource() string               { return ResourceFailureRateReport }
func (a *FailureRateAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "group", LabelKey: "warranty_claims.reports.group", Type: ioengine.ColumnTypeString},
		{Key: "product_sku", LabelKey: "warranty_claims.reports.product_sku", Type: ioengine.ColumnTypeString},
		{Key: "product_name", LabelKey: "warranty_claims.reports.product_name", Type: ioengine.ColumnTypeString, Weight: 1.6},
		{Key: "lot_code", LabelKey: "warranty_claims.reports.lot_code", Type: ioengine.ColumnTypeString},
		{Key: "warranty_count", LabelKey: "warranty_claims.reports.warranty_count", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "claim_count", LabelKey: "warranty_claims.reports.claim_count", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "approved_claim_count", LabelKey: "warranty_claims.reports.approved_claim_count", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "claim_rate", LabelKey: "warranty_claims.reports.claim_rate", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "approved_rate", LabelKey: "warranty_claims.reports.approved_rate", Type: ioengine.ColumnTypeString, AlignRight: true},
	}
}

func (a *FailureRateAdapter) Export(ctx context.Context, q ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	c, err := a.svc.exportCaller(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f, err := reportFilterFromQuery(q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rep, err := a.svc.FailureRateReport(ctx, c, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, len(rep.Items))
	for _, r := range rep.Items {
		lot := ""
		if r.LotCode != nil {
			lot = *r.LotCode
		}
		rows = append(rows, map[string]any{
			"group": r.Group, "product_sku": r.ProductSKU, "product_name": r.ProductName, "lot_code": lot,
			"warranty_count": r.WarrantyCount, "claim_count": r.ClaimCount, "approved_claim_count": r.ApprovedClaimCount,
			"claim_rate": fmt.Sprintf("%.4f", r.ClaimRate), "approved_rate": fmt.Sprintf("%.4f", r.ApprovedRate),
		})
	}
	return ioengine.Dataset{Resource: ResourceFailureRateReport, Columns: a.ExportColumns(), Rows: rows}, nil
}

type ByDealerAdapter struct {
	exportOnly
	svc *Service
}

func NewByDealerAdapter(svc *Service) *ByDealerAdapter { return &ByDealerAdapter{svc: svc} }
func (a *ByDealerAdapter) Resource() string            { return ResourceByDealerReport }
func (a *ByDealerAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "organization_name", LabelKey: "warranty_claims.reports.organization_name", Type: ioengine.ColumnTypeString, Weight: 1.8},
		{Key: "organization_type", LabelKey: "warranty_claims.reports.organization_type", Type: ioengine.ColumnTypeString},
		{Key: "claim_count", LabelKey: "warranty_claims.reports.claim_count", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "approved_claim_count", LabelKey: "warranty_claims.reports.approved_claim_count", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "rejected_claim_count", LabelKey: "warranty_claims.reports.rejected_claim_count", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "approval_rate", LabelKey: "warranty_claims.reports.approval_rate", Type: ioengine.ColumnTypeString, AlignRight: true},
	}
}

func (a *ByDealerAdapter) Export(ctx context.Context, q ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	c, err := a.svc.exportCaller(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f, err := reportFilterFromQuery(q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rep, err := a.svc.ByDealerReport(ctx, c, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, len(rep.Items))
	for _, r := range rep.Items {
		rows = append(rows, map[string]any{
			"organization_name": r.OrganizationName, "organization_type": r.OrganizationType,
			"claim_count": r.ClaimCount, "approved_claim_count": r.ApprovedClaimCount,
			"rejected_claim_count": r.RejectedClaimCount, "approval_rate": fmt.Sprintf("%.4f", r.ApprovalRate),
		})
	}
	return ioengine.Dataset{Resource: ResourceByDealerReport, Columns: a.ExportColumns(), Rows: rows}, nil
}

type PartsAdapter struct {
	exportOnly
	svc *Service
}

func NewPartsAdapter(svc *Service) *PartsAdapter { return &PartsAdapter{svc: svc} }
func (a *PartsAdapter) Resource() string         { return ResourcePartsReport }
func (a *PartsAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "part_key", LabelKey: "warranty_claims.reports.part_key", Type: ioengine.ColumnTypeString},
		{Key: "product_sku", LabelKey: "warranty_claims.reports.product_sku", Type: ioengine.ColumnTypeString},
		{Key: "product_name", LabelKey: "warranty_claims.reports.product_name", Type: ioengine.ColumnTypeString, Weight: 1.6},
		{Key: "part_count", LabelKey: "warranty_claims.reports.part_count", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "claim_count", LabelKey: "warranty_claims.reports.claim_count", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "approved_claim_count", LabelKey: "warranty_claims.reports.approved_claim_count", Type: ioengine.ColumnTypeString, AlignRight: true},
	}
}

func (a *PartsAdapter) Export(ctx context.Context, q ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	c, err := a.svc.exportCaller(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f, err := reportFilterFromQuery(q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rep, err := a.svc.PartsReport(ctx, c, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, len(rep.Items))
	for _, r := range rep.Items {
		rows = append(rows, map[string]any{
			"part_key": r.PartKey, "product_sku": r.ProductSKU, "product_name": r.ProductName,
			"part_count": r.PartCount, "claim_count": r.ClaimCount, "approved_claim_count": r.ApprovedClaimCount,
		})
	}
	return ioengine.Dataset{Resource: ResourcePartsReport, Columns: a.ExportColumns(), Rows: rows}, nil
}

var (
	_ ioengine.ResourceAdapter = (*FailureRateAdapter)(nil)
	_ ioengine.ResourceAdapter = (*ByDealerAdapter)(nil)
	_ ioengine.ResourceAdapter = (*PartsAdapter)(nil)
)
