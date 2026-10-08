// Package usecase holds the price list rules of TEC-146 (K8): who writes
// which price and the effective price view every organization type sees.
package usecase

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// Organization types that take part in the price chain.
const (
	OrgCenter      = "center"
	OrgDistributor = "distributor"
	OrgDealer      = "dealer"
)

// Viewer is the caller of a pricing request: the active organization and the
// pricing.* grants that reach it. A center grant must cover the brand (the
// list price is brand master data); distributor and dealer grants only need
// to cover their own organization.
type Viewer struct {
	// UserID is the internal user id (publisher of recommended prices).
	UserID  int64
	OrgID   int64
	OrgType string
	BrandID int64

	PurchaseRead     bool
	SaleRead         bool
	SaleWrite        bool
	RecommendedRead  bool
	RecommendedWrite bool
}

// ViewerFrom builds the viewer from the request principal and organization.
func ViewerFrom(p authctx.Principal, org orgctx.Scope) Viewer {
	need := rbac.ScopeManaged
	if org.OrgType == OrgCenter {
		need = rbac.ScopeBrand
	}
	return Viewer{
		UserID:           p.UserInternal,
		OrgID:            org.InternalID,
		OrgType:          org.OrgType,
		BrandID:          org.BrandID,
		PurchaseRead:     p.Can(rbac.PermPricingPurchaseRead, need),
		SaleRead:         p.Can(rbac.PermPricingSaleRead, need),
		SaleWrite:        p.Can(rbac.PermPricingSaleWrite, need),
		RecommendedRead:  p.Can(rbac.PermPricingRecommendedRead, need),
		RecommendedWrite: p.Can(rbac.PermPricingRecommendedWrite, need),
	}
}
