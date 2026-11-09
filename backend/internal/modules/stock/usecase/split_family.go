package usecase

// TEC-1078: the split family of a roll. A cut is a new unit coded by its
// meters (<roll>-3M, see ledger.MeterBarcode) and linked to the roll in
// stock_splits; the family is read through that link, recursively. Every
// part is shown only when the viewer's stock.read reach covers its holder
// (a part not in stock yet: its issuer), so a dealer never sees a sibling
// dealer's part; the order of a part only when the reach covers its buyer
// or seller.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
)

// familyStatus classifies a family member relative to organization orgID.
func familyStatus(r db.ListUnitSplitFamilyRow, orgID int64) string {
	switch {
	case r.Status == "used" || r.OwnerType.String == ownerService || r.OwnerType.String == ownerTrash:
		return model.FamilyUsed
	case r.Status == "in_transit":
		return model.FamilyInTransit
	case !r.HolderOrgID.Valid || (r.Status != "available" && r.Status != "placed"):
		return model.FamilyOther
	case r.HolderOrgID.Int64 == orgID:
		return model.FamilyOnHand
	default:
		return model.FamilySold
	}
}

// splitFamily reads the family of unitID (the unit itself at depth 0) and
// keeps the members f reaches.
func (s *Service) splitFamily(ctx context.Context, f scopefilter.Filter, unitID int64) ([]db.ListUnitSplitFamilyRow, error) {
	rows, err := s.q.ListUnitSplitFamily(ctx, unitID)
	if err != nil {
		return nil, fmt.Errorf("stock: split family: %w", err)
	}
	out := rows[:0]
	for _, r := range rows {
		holder := r.OrganizationID
		if r.HolderOrgID.Valid {
			holder = r.HolderOrgID.Int64
		}
		if f.AllowsOrg(holder, r.BrandID) {
			out = append(out, r)
		}
	}
	return out, nil
}

func familyHolder(r db.ListUnitSplitFamilyRow) *model.OrgRef {
	if !r.HolderUuid.Valid {
		return nil
	}
	return &model.OrgRef{UUID: uuid.UUID(r.HolderUuid.Bytes), Name: r.HolderName.String, Type: r.HolderType.String}
}

func familyOrder(f scopefilter.Filter, r db.ListUnitSplitFamilyRow) *model.OrderRef {
	if r.OrderUuid == "" || (!f.AllowsOrg(r.OrderBuyerID, r.BrandID) && !f.AllowsOrg(r.OrderSellerID, r.BrandID)) {
		return nil
	}
	id, err := uuid.Parse(r.OrderUuid)
	if err != nil {
		return nil
	}
	return &model.OrderRef{UUID: id, OrderNo: r.OrderNo}
}

// unitSplits builds UnitHistory.splits: the source roll of u (when
// reached) and its parts, classified for the active organization.
func (s *Service) unitSplits(ctx context.Context, v viewer, u db.Unit) (model.UnitSplits, error) {
	out := model.UnitSplits{Parts: []model.SplitPart{}}
	src, err := s.q.GetUnitSplitSource(ctx, u.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return out, fmt.Errorf("stock: split source: %w", err)
	default:
		holder := src.OrganizationID
		if src.HolderOrgID.Valid {
			holder = src.HolderOrgID.Int64
		}
		if v.reaches(holder, src.BrandID) {
			out.Source = &model.SplitSource{UUID: src.Uuid, Barcode: src.Barcode, Meters: numericString(src.Meters)}
		}
	}
	if u.UnitKind == "fixed" {
		return out, nil
	}
	rows, err := s.splitFamily(ctx, v.f, u.ID)
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		if r.Depth == 0 {
			continue
		}
		out.Parts = append(out.Parts, model.SplitPart{
			UUID: r.Uuid, Barcode: r.Barcode, Depth: r.Depth, SourceBarcode: r.SourceBarcode.String,
			Meters: numericString(r.SplitMeters), RemainingMeters: numericPtr(r.RemainingMeters),
			Status: r.Status, FamilyStatus: familyStatus(r, v.org), OwnerType: textPtr(r.OwnerType),
			Holder: familyHolder(r), Order: familyOrder(v.f, r), SplitAt: r.SplitAt.Time,
		})
	}
	return out, nil
}

// familyUnits serves the unit list with family=<barcode> (TEC-1078): the
// unit with that barcode in the organization's brand and every part cut
// from it, wherever they are now, within the viewer's reach; family_status
// is relative to the listed organization. Paged in memory (a family is
// capped at 500 rows); an unknown barcode is an empty page.
func (s *Service) familyUnits(ctx context.Context, v UnitViewer, o db.Organization, in model.UnitFilter) ([]model.StockUnitRow, int64, error) {
	var root db.Unit
	var err error
	for _, b := range []string{in.Family, strings.ToUpper(in.Family)} {
		root, err = s.q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: o.BrandID, Barcode: b})
		if !errors.Is(err, pgx.ErrNoRows) {
			break
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return []model.StockUnitRow{}, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("stock: family unit: %w", err)
	}
	rows, err := s.splitFamily(ctx, v.Filter, root.ID)
	if err != nil {
		return nil, 0, err
	}
	total := int64(len(rows))
	start := min(int(max(in.Offset, 0)), len(rows))
	end := len(rows)
	if in.Limit > 0 {
		end = min(start+int(in.Limit), len(rows))
	}
	out := make([]model.StockUnitRow, 0, end-start)
	for _, r := range rows[start:end] {
		depth := r.Depth
		row := model.StockUnitRow{
			UUID: r.Uuid, Barcode: r.Barcode, UnitKind: r.UnitKind, Status: r.Status,
			Quantity:      b2q(familyStatus(r, o.ID) == model.FamilyOnHand),
			InitialMeters: numericPtr(r.InitialMeters), RemainingMeters: numericPtr(r.RemainingMeters),
			Product: model.ProductRef{
				UUID: r.ProductUuid, SKU: r.Sku, Name: r.ProductName,
				UnitType: r.UnitType, UsesFixedBarcode: r.UsesFixedBarcode,
			},
			Category:     model.CategoryRef{UUID: r.CategoryUuid, Name: r.CategoryName},
			UpdatedAt:    r.UnitUpdatedAt.Time,
			Holder:       familyHolder(r),
			FamilyStatus: familyStatus(r, o.ID),
			FamilyDepth:  &depth,
			SplitMeters:  numericPtr(r.SplitMeters),
			Order:        familyOrder(v.Filter, r),
		}
		if r.StateUpdatedAt.Valid {
			row.UpdatedAt = r.StateUpdatedAt.Time
		}
		if r.SourceBarcode.Valid {
			sb := r.SourceBarcode.String
			row.SourceBarcode = &sb
		}
		// The bin only for a part of the listed organization.
		if r.LocationUuid.Valid && r.HolderOrgID.Int64 == o.ID {
			row.Location = &model.LocationRef{UUID: uuid.UUID(r.LocationUuid.Bytes), Code: r.LocationCode.String, Name: r.LocationName.String}
		}
		out = append(out, row)
	}
	return out, total, nil
}

func b2q(b bool) int32 {
	if b {
		return 1
	}
	return 0
}
