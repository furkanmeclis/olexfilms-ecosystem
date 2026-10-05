package usecase

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

const (
	ResourceReviewsExport = "services.reviews"
	reviewsQueryScope     = "scope"
	reviewsScopeBrand     = "brand"
	reviewsExportPage     = 500
)

// ReviewExportScope encodes the caller's reviews.read scope for export jobs.
func ReviewExportScope(c Caller) (string, error) {
	ids := reviewScopeOrgIDs(c, rbac.PermReviewsRead)
	if ids == nil {
		return reviewsScopeBrand, nil
	}
	if len(ids) == 0 {
		return "", ErrForbidden
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ","), nil
}

type ReviewsExportAdapter struct{ svc *Service }

func NewReviewsExportAdapter(svc *Service) *ReviewsExportAdapter {
	return &ReviewsExportAdapter{svc: svc}
}

func (a *ReviewsExportAdapter) Resource() string { return ResourceReviewsExport }

func (a *ReviewsExportAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "uuid", LabelKey: "reviews.export.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "service_no", LabelKey: "reviews.export.service_no", Type: ioengine.ColumnTypeString},
		{Key: "plate", LabelKey: "reviews.export.plate", Type: ioengine.ColumnTypeString},
		{Key: "platform_rating", LabelKey: "reviews.export.platform_rating", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "product_rating", LabelKey: "reviews.export.product_rating", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "anonymous", LabelKey: "reviews.export.anonymous", Type: ioengine.ColumnTypeBoolean},
		{Key: "customer", LabelKey: "reviews.export.customer", Type: ioengine.ColumnTypeString},
		{Key: "comment", LabelKey: "reviews.export.comment", Type: ioengine.ColumnTypeString, Weight: 1.8},
		{Key: "source", LabelKey: "reviews.export.source", Type: ioengine.ColumnTypeString},
		{Key: "created_at", LabelKey: "reviews.export.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *ReviewsExportAdapter) ImportSchema() []ioengine.ImportField { return nil }

func (a *ReviewsExportAdapter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "export only"}, nil
}

func (a *ReviewsExportAdapter) RevertRow(context.Context, string, string, map[string]any) error {
	return nil
}

func (a *ReviewsExportAdapter) Export(ctx context.Context, q ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	c, err := a.svc.reviewsExportCaller(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f, err := reviewsExportFilter(q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f.Limit = reviewsExportPage
	rows := []map[string]any{}
	for {
		items, total, err := a.svc.ListServiceReviews(ctx, c, f)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, item := range items {
			rows = append(rows, reviewExportRow(item))
		}
		f.Offset += int32(len(items))
		if len(items) == 0 || int64(f.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Resource: ResourceReviewsExport, Columns: a.ExportColumns(), Rows: rows}, nil
}

func (s *Service) reviewsExportCaller(ctx context.Context, q ioengine.ExportQuery) (Caller, error) {
	jobOrg, err := strconv.ParseInt(q[ioengine.QueryOrganizationID], 10, 64)
	if err != nil || jobOrg <= 0 {
		return Caller{}, errors.New("reviews export: organization is required")
	}
	org, err := s.q.GetOrganizationByID(ctx, jobOrg)
	if err != nil {
		return Caller{}, fmt.Errorf("reviews export: organization: %w", err)
	}
	c := Caller{
		Principal: authctx.Principal{PermissionScopes: map[string]rbac.Scope{rbac.PermReviewsRead: rbac.ScopeSubtree}},
		Org:       orgctx.Scope{InternalID: org.ID, UUID: org.Uuid, Name: org.Name, OrgType: org.Type, BrandID: org.BrandID},
	}
	raw := strings.TrimSpace(q[reviewsQueryScope])
	if raw == reviewsScopeBrand && org.Type == OrgCenter {
		c.Principal.PermissionScopes[rbac.PermReviewsRead] = rbac.ScopeBrand
		c.Filter = scopefilter.Filter{Scope: rbac.ScopeBrand, OrgID: org.ID, BrandID: org.BrandID}
		return c, nil
	}
	below, err := s.q.Descendants(ctx, org.ID)
	if err != nil {
		return Caller{}, fmt.Errorf("reviews export: descendants: %w", err)
	}
	covered := map[int64]bool{org.ID: true}
	tree := []int64{org.ID}
	for _, o := range below {
		covered[o.ID] = true
		tree = append(tree, o.ID)
	}
	ids := []int64{}
	if raw == reviewsScopeBrand {
		ids = tree
	} else {
		for _, part := range strings.Split(raw, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err == nil && covered[id] {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return Caller{}, errors.New("reviews export: scope is outside the job organization")
	}
	if len(ids) == 1 && ids[0] == org.ID {
		c.Principal.PermissionScopes[rbac.PermReviewsRead] = rbac.ScopeManaged
		c.Filter = scopefilter.Filter{Scope: rbac.ScopeManaged, OrgID: org.ID, OrgIDs: ids}
		return c, nil
	}
	c.Filter = scopefilter.Filter{Scope: rbac.ScopeSubtree, OrgID: org.ID, OrgIDs: ids}
	return c, nil
}

func reviewsExportFilter(q ioengine.ExportQuery) (ServiceReviewFilter, error) {
	var f ServiceReviewFilter
	if raw := q["dealer_uuid"]; raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return f, invalid("dealer_uuid", "must be a UUID")
		}
		f.DealerUUID = &id
	}
	if raw := q["product_uuid"]; raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return f, invalid("product_uuid", "must be a UUID")
		}
		f.ProductUUID = &id
	}
	for key, dst := range map[string]**int{"min_rating": &f.MinRating, "max_rating": &f.MaxRating} {
		if raw := q[key]; raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil {
				return f, invalid(key, "must be an integer")
			}
			*dst = &n
		}
	}
	if t, err := parseExportTime(q["created_from"]); err != nil {
		return f, invalid("created_from", "invalid date")
	} else {
		f.CreatedFrom = t
	}
	if t, err := parseExportTime(q["created_to"]); err != nil {
		return f, invalid("created_to", "invalid date")
	} else {
		f.CreatedTo = t
	}
	return f, nil
}

func parseExportTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func reviewExportRow(r ServiceReviewListItem) map[string]any {
	customer := ""
	if r.Customer != nil {
		customer = r.Customer.Name
	}
	return map[string]any{
		"uuid":            r.UUID.String(),
		"service_no":      r.ServiceNo,
		"plate":           ptrString(r.Plate),
		"platform_rating": strconv.Itoa(r.PlatformRating),
		"product_rating":  strconv.Itoa(r.ProductRating),
		"anonymous":       r.IsAnonymous,
		"customer":        customer,
		"comment":         ptrString(r.Comment),
		"source":          r.Source,
		"created_at":      r.CreatedAt,
	}
}

func ptrString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

var _ ioengine.ResourceAdapter = (*ReviewsExportAdapter)(nil)
