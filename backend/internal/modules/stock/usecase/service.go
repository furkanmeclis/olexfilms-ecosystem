// Package usecase serves the stock read API (TEC-155): the barcode history
// and the organization/bin product stock projections. Every read is narrowed
// by the stock.read filter on the holding organization (holder_org_id,
// TEC-94 decision 4): scope all (center warehouse, K20) sees everything,
// brand scope its brand, managed/subtree the listed organizations.
//
// Barcode history visibility (TEC-155 decision): a restricted viewer sees a
// unit only while it is held inside its reach. It then sees the whole
// history, but the names of organizations outside its reach and outside its
// own ancestor chain (its suppliers) are masked, together with their
// locations, references and reasons.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrNotFound: the record does not exist or is outside the viewer's reach.
var ErrNotFound = errors.New("stock: not found")

// Owner types (stock_movements / projections).
const (
	ownerLocation     = "warehouse_location"
	ownerOrganization = "organization"
	ownerService      = "service"
	ownerTrash        = "trash"
)

// Service reads the stock ledger.
type Service struct {
	q *db.Queries
}

// New builds the service.
func New(q *db.Queries) *Service { return &Service{q: q} }

// viewer is the resolved reach of one request.
type viewer struct {
	f         scopefilter.Filter
	ancestors map[int64]bool
}

func (s *Service) viewer(ctx context.Context, org orgctx.Scope, f scopefilter.Filter) (viewer, error) {
	v := viewer{f: f, ancestors: map[int64]bool{}}
	if f.Scope == rbac.ScopeAll {
		return v, nil
	}
	// The supplier chain of the active organization (distributor, center).
	id := org.InternalID
	for range 8 {
		o, err := s.q.GetOrganizationByID(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				break
			}
			return v, fmt.Errorf("stock: ancestors: %w", err)
		}
		if !o.ParentID.Valid {
			break
		}
		id = o.ParentID.Int64
		v.ancestors[id] = true
	}
	return v, nil
}

// reaches: the organization holds stock the viewer may read.
func (v viewer) reaches(orgID, brandID int64) bool { return v.f.AllowsOrg(orgID, brandID) }

// names: the viewer may see the organization's name in a history.
func (v viewer) names(o db.Organization) bool {
	return v.f.AllowsOrg(o.ID, o.BrandID) || v.ancestors[o.ID]
}

// UnitHistory serves GET /v1/stock/units/by-barcode/{barcode}.
func (s *Service) UnitHistory(ctx context.Context, org orgctx.Scope, f scopefilter.Filter, barcode string, limit, offset int32) (model.UnitHistory, error) {
	barcode = strings.TrimSpace(barcode)
	if barcode == "" {
		return model.UnitHistory{}, ErrNotFound
	}
	v, err := s.viewer(ctx, org, f)
	if err != nil {
		return model.UnitHistory{}, err
	}
	units, err := s.q.ListUnitsByBarcode(ctx, barcode)
	if err != nil {
		return model.UnitHistory{}, fmt.Errorf("stock: units: %w", err)
	}
	// Barcodes are unique per brand; the active brand wins over others.
	slices.SortStableFunc(units, func(a, b db.Unit) int {
		return boolRank(a.BrandID == org.BrandID) - boolRank(b.BrandID == org.BrandID)
	})
	for _, u := range units {
		state, holdings, visible, err := s.unitReach(ctx, v, u)
		if err != nil {
			return model.UnitHistory{}, err
		}
		if visible {
			return s.history(ctx, v, u, state, holdings, limit, offset)
		}
	}
	return model.UnitHistory{}, ErrNotFound
}

func boolRank(b bool) int {
	if b {
		return 0
	}
	return 1
}

// unitReach loads the current owner(s) of the unit and reports whether the
// viewer reaches it.
func (s *Service) unitReach(ctx context.Context, v viewer, u db.Unit) (*db.UnitCurrentState, []db.FixedBarcodeHolding, bool, error) {
	if u.UnitKind == "fixed" {
		all, err := s.q.ListFixedBarcodeHoldingsByUnit(ctx, u.ID)
		if err != nil {
			return nil, nil, false, fmt.Errorf("stock: holdings: %w", err)
		}
		var mine []db.FixedBarcodeHolding
		for _, h := range all {
			if h.QuantityOnHand > 0 && v.reaches(h.HolderOrgID, u.BrandID) {
				mine = append(mine, h)
			}
		}
		if len(mine) == 0 && v.f.Scope == rbac.ScopeAll {
			return nil, nil, true, nil
		}
		return nil, mine, len(mine) > 0, nil
	}
	st, err := s.q.GetUnitCurrentState(ctx, u.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Not in stock yet (reserved/printed label): only the issuer's reach.
		return nil, nil, v.reaches(u.OrganizationID, u.BrandID), nil
	}
	if err != nil {
		return nil, nil, false, fmt.Errorf("stock: unit state: %w", err)
	}
	return &st, nil, v.reaches(st.HolderOrgID, u.BrandID), nil
}

// refs resolves organizations and locations in batches.
type refs struct {
	orgs map[int64]db.Organization
	locs map[int64]db.WarehouseLocation
}

func (s *Service) loadRefs(ctx context.Context, orgIDs, locIDs []int64) (refs, error) {
	r := refs{orgs: map[int64]db.Organization{}, locs: map[int64]db.WarehouseLocation{}}
	if len(locIDs) > 0 {
		locs, err := s.q.ListWarehouseLocationsByIDs(ctx, uniq(locIDs))
		if err != nil {
			return r, fmt.Errorf("stock: locations: %w", err)
		}
		for _, l := range locs {
			r.locs[l.ID] = l
			orgIDs = append(orgIDs, l.OrganizationID)
		}
	}
	if len(orgIDs) > 0 {
		orgs, err := s.q.ListOrganizationsByIDs(ctx, uniq(orgIDs))
		if err != nil {
			return r, fmt.Errorf("stock: organizations: %w", err)
		}
		for _, o := range orgs {
			r.orgs[o.ID] = o
		}
	}
	return r, nil
}

func uniq(ids []int64) []int64 {
	out := slices.Clone(ids)
	slices.Sort(out)
	return slices.Compact(out)
}

func collectOwner(t pgtype.Text, id pgtype.Int8, orgIDs, locIDs *[]int64) {
	if !t.Valid || !id.Valid {
		return
	}
	switch t.String {
	case ownerLocation:
		*locIDs = append(*locIDs, id.Int64)
	case ownerOrganization, ownerTrash:
		*orgIDs = append(*orgIDs, id.Int64)
	}
}

func orgRef(o db.Organization) *model.OrgRef {
	return &model.OrgRef{UUID: o.Uuid, Name: o.Name, Type: o.Type}
}

// owner renders an owner; rowVisible decides service owners, which carry
// no organization of their own.
func (r refs) owner(v viewer, typ string, id int64, rowVisible bool) model.Owner {
	out := model.Owner{Type: typ}
	switch typ {
	case ownerLocation:
		loc, ok := r.locs[id]
		o, okOrg := r.orgs[loc.OrganizationID]
		if !ok || !okOrg || !v.names(o) {
			out.Masked = true
			return out
		}
		out.Location = &model.LocationRef{UUID: loc.Uuid, Code: loc.Code, Name: loc.Name}
		out.Organization = orgRef(o)
	case ownerOrganization, ownerTrash:
		o, ok := r.orgs[id]
		if !ok || !v.names(o) {
			out.Masked = true
			return out
		}
		out.Organization = orgRef(o)
	default: // service (TEC-97)
		out.Masked = !rowVisible
	}
	return out
}

func (s *Service) history(ctx context.Context, v viewer, u db.Unit, st *db.UnitCurrentState,
	holdings []db.FixedBarcodeHolding, limit, offset int32) (model.UnitHistory, error) {
	product, err := s.q.GetProduct(ctx, db.GetProductParams{ID: u.ProductID, BrandID: u.BrandID})
	if err != nil {
		return model.UnitHistory{}, fmt.Errorf("stock: product: %w", err)
	}
	total, err := s.q.CountStockMovementsByUnit(ctx, u.ID)
	if err != nil {
		return model.UnitHistory{}, fmt.Errorf("stock: count movements: %w", err)
	}
	rows, err := s.q.ListStockMovementsByUnitPage(ctx, db.ListStockMovementsByUnitPageParams{
		UnitID: u.ID, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return model.UnitHistory{}, fmt.Errorf("stock: movements: %w", err)
	}

	var orgIDs, locIDs []int64
	for _, m := range rows {
		orgIDs = append(orgIDs, m.OrganizationID)
		collectOwner(m.FromOwnerType, m.FromOwnerID, &orgIDs, &locIDs)
		collectOwner(m.ToOwnerType, m.ToOwnerID, &orgIDs, &locIDs)
	}
	if st != nil {
		orgIDs = append(orgIDs, st.HolderOrgID)
		collectOwner(pgtype.Text{String: st.OwnerType, Valid: true}, pgtype.Int8{Int64: st.OwnerID, Valid: true}, &orgIDs, &locIDs)
	}
	for _, h := range holdings {
		orgIDs = append(orgIDs, h.HolderOrgID)
		collectOwner(pgtype.Text{String: h.OwnerType, Valid: true}, pgtype.Int8{Int64: h.OwnerID, Valid: true}, &orgIDs, &locIDs)
	}
	r, err := s.loadRefs(ctx, orgIDs, locIDs)
	if err != nil {
		return model.UnitHistory{}, err
	}

	out := model.UnitHistory{
		Unit: model.Unit{
			UUID: u.Uuid, Barcode: u.Barcode, UnitKind: u.UnitKind, Source: u.Source, Status: u.Status,
			InitialMeters: numericPtr(u.InitialMeters), RemainingMeters: numericPtr(u.RemainingMeters),
			Product: model.ProductRef{
				UUID: product.Uuid, SKU: product.Sku, Name: product.Name,
				UnitType: product.UnitType, UsesFixedBarcode: product.UsesFixedBarcode,
			},
			CreatedAt: u.CreatedAt.Time,
		},
		Holdings: []model.Holding{},
		Movements: model.MovementPage{
			Items: make([]model.Movement, 0, len(rows)), Total: total, Limit: limit, Offset: offset,
		},
	}
	if st != nil {
		holder := r.orgs[st.HolderOrgID]
		out.Current = &model.CurrentState{
			Owner:  r.owner(v, st.OwnerType, st.OwnerID, true),
			Holder: *orgRef(holder), Status: st.Status, UpdatedAt: st.UpdatedAt.Time,
		}
	}
	for _, h := range holdings {
		out.Holdings = append(out.Holdings, model.Holding{
			Owner:  r.owner(v, h.OwnerType, h.OwnerID, true),
			Holder: *orgRef(r.orgs[h.HolderOrgID]), Quantity: h.QuantityOnHand,
		})
	}
	for _, m := range rows {
		o, ok := r.orgs[m.OrganizationID]
		visible := ok && v.names(o)
		mv := model.Movement{
			UUID: m.Uuid, Type: m.Type, QuantityDelta: m.QuantityDelta, MetersDelta: numericString(m.MetersDelta),
			FromStatus: textPtr(m.FromStatus), ToStatus: textPtr(m.ToStatus),
			OrganizationMasked: !visible, CreatedAt: m.CreatedAt.Time,
		}
		if visible {
			mv.Organization = orgRef(o)
			mv.ReferenceType = textPtr(m.ReferenceType)
			mv.Reason = textPtr(m.Reason)
		}
		if m.FromOwnerType.Valid {
			ow := r.owner(v, m.FromOwnerType.String, m.FromOwnerID.Int64, visible)
			mv.FromOwner = &ow
		}
		if m.ToOwnerType.Valid {
			ow := r.owner(v, m.ToOwnerType.String, m.ToOwnerID.Int64, visible)
			mv.ToOwner = &ow
		}
		out.Movements.Items = append(out.Movements.Items, mv)
	}
	return out, nil
}

// stockArgs are the shared filters of both product stock lists.
type stockArgs struct {
	brand    pgtype.Int8
	product  pgtype.Int8
	category pgtype.Int8
	inStock  pgtype.Bool
	q        pgtype.Text
}

// resolveFilter turns the UUID filters into ids; ok=false means a filter
// names nothing, so the list is empty.
func (s *Service) resolveFilter(ctx context.Context, f scopefilter.Filter, in model.StockFilter) (stockArgs, bool, error) {
	a := stockArgs{brand: f.BrandIDArg()}
	if in.ProductUUID != nil {
		id, err := s.q.GetProductIDByUUID(ctx, *in.ProductUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return a, false, nil
		}
		if err != nil {
			return a, false, fmt.Errorf("stock: product filter: %w", err)
		}
		a.product = pgtype.Int8{Int64: id, Valid: true}
	}
	if in.CategoryUUID != nil {
		id, err := s.q.GetProductCategoryIDByUUID(ctx, *in.CategoryUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return a, false, nil
		}
		if err != nil {
			return a, false, fmt.Errorf("stock: category filter: %w", err)
		}
		a.category = pgtype.Int8{Int64: id, Valid: true}
	}
	switch in.Status {
	case model.StatusInStock:
		a.inStock = pgtype.Bool{Bool: true, Valid: true}
	case model.StatusOutOfStock:
		a.inStock = pgtype.Bool{Bool: false, Valid: true}
	}
	if q := strings.TrimSpace(in.Q); q != "" {
		a.q = pgtype.Text{String: q, Valid: true}
	}
	return a, true, nil
}

// OrganizationStock serves GET /v1/stock/organizations/{uuid}/products.
func (s *Service) OrganizationStock(ctx context.Context, f scopefilter.Filter, orgUUID uuid.UUID, in model.StockFilter) ([]model.ProductStock, int64, error) {
	o, err := s.q.GetOrganizationByUUID(ctx, orgUUID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (o.DeletedAt.Valid || !f.AllowsOrg(o.ID, o.BrandID))) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("stock: organization: %w", err)
	}
	a, ok, err := s.resolveFilter(ctx, f, in)
	if err != nil || !ok {
		return []model.ProductStock{}, 0, err
	}
	total, err := s.q.CountOrganizationProductStockRows(ctx, db.CountOrganizationProductStockRowsParams{
		OrganizationID: o.ID, BrandID: a.brand, ProductID: a.product, CategoryID: a.category, InStock: a.inStock, Q: a.q,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stock: count organization stock: %w", err)
	}
	rows, err := s.q.ListOrganizationProductStockRows(ctx, db.ListOrganizationProductStockRowsParams{
		OrganizationID: o.ID, BrandID: a.brand, ProductID: a.product, CategoryID: a.category, InStock: a.inStock, Q: a.q,
		LimitCount: in.Limit, OffsetCount: in.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stock: organization stock: %w", err)
	}
	fixed, err := s.q.ListFixedBarcodeQuantitiesByHolder(ctx, db.ListFixedBarcodeQuantitiesByHolderParams{
		HolderOrgID: o.ID, ProductIds: fixedProductIDs(rows),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stock: fixed barcodes: %w", err)
	}
	return stockItems(rows, fixedByProduct(fixed)), total, nil
}

// LocationStock serves GET /v1/stock/locations/{uuid}/products.
func (s *Service) LocationStock(ctx context.Context, f scopefilter.Filter, locUUID uuid.UUID, in model.StockFilter) ([]model.ProductStock, int64, error) {
	loc, err := s.q.GetWarehouseLocationByUUID(ctx, locUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("stock: location: %w", err)
	}
	o, err := s.q.GetOrganizationByID(ctx, loc.OrganizationID)
	if err != nil {
		return nil, 0, fmt.Errorf("stock: location organization: %w", err)
	}
	if o.DeletedAt.Valid || !f.AllowsOrg(o.ID, o.BrandID) {
		return nil, 0, ErrNotFound
	}
	a, ok, err := s.resolveFilter(ctx, f, in)
	if err != nil || !ok {
		return []model.ProductStock{}, 0, err
	}
	total, err := s.q.CountBinProductStockRows(ctx, db.CountBinProductStockRowsParams{
		LocationID: loc.ID, BrandID: a.brand, ProductID: a.product, CategoryID: a.category, InStock: a.inStock, Q: a.q,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stock: count bin stock: %w", err)
	}
	binRows, err := s.q.ListBinProductStockRows(ctx, db.ListBinProductStockRowsParams{
		LocationID: loc.ID, BrandID: a.brand, ProductID: a.product, CategoryID: a.category, InStock: a.inStock, Q: a.q,
		LimitCount: in.Limit, OffsetCount: in.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stock: bin stock: %w", err)
	}
	rows := make([]db.ListOrganizationProductStockRowsRow, len(binRows))
	for i, r := range binRows {
		rows[i] = db.ListOrganizationProductStockRowsRow(r)
	}
	fixed, err := s.q.ListFixedBarcodeQuantitiesByLocation(ctx, db.ListFixedBarcodeQuantitiesByLocationParams{
		LocationID: loc.ID, ProductIds: fixedProductIDs(rows),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stock: fixed barcodes: %w", err)
	}
	held := make([]db.ListFixedBarcodeQuantitiesByHolderRow, len(fixed))
	for i, r := range fixed {
		held[i] = db.ListFixedBarcodeQuantitiesByHolderRow(r)
	}
	return stockItems(rows, fixedByProduct(held)), total, nil
}

func fixedProductIDs(rows []db.ListOrganizationProductStockRowsRow) []int64 {
	ids := []int64{}
	for _, r := range rows {
		if r.UsesFixedBarcode {
			ids = append(ids, r.ProductID)
		}
	}
	return ids
}

func fixedByProduct(rows []db.ListFixedBarcodeQuantitiesByHolderRow) map[int64][]model.FixedBarcodeQuantity {
	out := map[int64][]model.FixedBarcodeQuantity{}
	for _, r := range rows {
		out[r.ProductID] = append(out[r.ProductID], model.FixedBarcodeQuantity{
			UnitUUID: r.UnitUuid, Barcode: r.Barcode, Quantity: r.Quantity,
		})
	}
	return out
}

func stockItems(rows []db.ListOrganizationProductStockRowsRow, fixed map[int64][]model.FixedBarcodeQuantity) []model.ProductStock {
	out := make([]model.ProductStock, 0, len(rows))
	for _, r := range rows {
		fb := fixed[r.ProductID]
		if fb == nil {
			fb = []model.FixedBarcodeQuantity{}
		}
		out = append(out, model.ProductStock{
			Product: model.StockProduct{
				UUID: r.ProductUuid, SKU: r.Sku, Name: r.ProductName, UnitType: r.UnitType,
				UsesFixedBarcode: r.UsesFixedBarcode, Active: r.ProductActive,
				Category: model.CategoryRef{UUID: r.CategoryUuid, Name: r.CategoryName},
			},
			Quantity: r.Quantity, Meters: r.Meters, FixedBarcodes: fb, UpdatedAt: tsTime(r.UpdatedAt),
		})
	}
	return out
}

func tsTime(t pgtype.Timestamptz) time.Time { return t.Time }

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

// numericString renders a NUMERIC(…,2) with two decimals.
func numericString(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil {
		return "0.00"
	}
	r := new(big.Rat).SetInt(n.Int)
	if n.Exp != 0 {
		p := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs(n.Exp))), nil)
		if n.Exp > 0 {
			r.Mul(r, new(big.Rat).SetInt(p))
		} else {
			r.Quo(r, new(big.Rat).SetInt(p))
		}
	}
	return r.FloatString(2)
}

func numericPtr(n pgtype.Numeric) *string {
	if !n.Valid {
		return nil
	}
	s := numericString(n)
	return &s
}

func abs(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}
