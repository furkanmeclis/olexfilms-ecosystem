package rebuild

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/jackc/pgx/v5/pgtype"
)

// Table names in a Diff.
const (
	TableUnits         = "units"
	TableUnitState     = "unit_current_state"
	TableFixedHoldings = "fixed_barcode_holdings"
	TableBinStocks     = "bin_product_stocks"
	TableOrgStocks     = "organization_product_stocks"
)

const (
	absent  = "absent"
	present = "present"
)

// scanResult holds the expected and the stored projections of one scope.
type scanResult struct {
	exp       *ledger.Projection
	units     int
	movements int
	diffs     []Diff
	anomalies []string

	// Repair work, derived with the diffs.
	states       []ledger.UnitState // with unitIDs[i]
	stateUnits   []db.Unit
	dropStates   []int64
	holdings     []holdingFix
	dropHoldings []int64
	unitStatus   []db.SetUnitStatusForRepairParams
	unitMeters   []db.SetUnitRemainingMetersForRepairParams
	binFixes     []db.UpsertBinProductStockForRepairParams
	orgFixes     []db.UpsertOrganizationProductStockForRepairParams
}

type holdingFix struct {
	unit db.Unit
	h    ledger.FixedHolding
}

func (sc *scanResult) fill(rep *Report) {
	rep.UnitsScanned, rep.MovementsReplayed = sc.units, sc.movements
	rep.DiffCount, rep.Diffs = len(sc.diffs), sc.diffs
	rep.Anomalies = sc.anomalies
	if rep.Diffs == nil {
		rep.Diffs = []Diff{}
	}
	if rep.Anomalies == nil {
		rep.Anomalies = []string{}
	}
}

func (sc *scanResult) diff(d Diff) { sc.diffs = append(sc.diffs, d) }

// scan replays every unit in scope batch by batch, compares the unit level
// projections per batch and the product stock rows at the end.
func scan(ctx context.Context, q *db.Queries, orgID int64) (*scanResult, error) {
	ids, err := unitIDs(ctx, q, orgID)
	if err != nil {
		return nil, err
	}
	sc := &scanResult{exp: ledger.NewProjection()}
	for b := range slices.Chunk(ids, batchSize) {
		if err := sc.batch(ctx, q, b); err != nil {
			return nil, err
		}
	}
	sc.anomalies = append(sc.anomalies, sc.exp.SplitAnomalies()...)
	if err := sc.stocks(ctx, q, orgID); err != nil {
		return nil, err
	}
	return sc, nil
}

func (sc *scanResult) batch(ctx context.Context, q *db.Queries, ids []int64) error {
	units, err := q.ListUnitsByIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("rebuild: units: %w", err)
	}
	mvs, err := q.ListStockMovementsByUnitIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("rebuild: movements: %w", err)
	}
	states, err := q.ListUnitCurrentStatesByUnitIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("rebuild: unit states: %w", err)
	}
	holdings, err := q.ListFixedBarcodeHoldingsByUnitIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("rebuild: holdings: %w", err)
	}
	byUnitMv := groupBy(mvs, func(m db.StockMovement) int64 { return m.UnitID })
	byUnitHold := groupBy(holdings, func(h db.FixedBarcodeHolding) int64 { return h.UnitID })
	stateOf := map[int64]db.UnitCurrentState{}
	for _, s := range states {
		stateOf[s.UnitID] = s
	}
	for _, u := range units {
		up, err := sc.exp.Replay(u, byUnitMv[u.ID])
		if err != nil {
			return err
		}
		sc.units++
		sc.movements += up.Movements
		for _, a := range up.Anomalies {
			sc.anomalies = append(sc.anomalies, fmt.Sprintf("unit %d (%s): %s", u.ID, u.Barcode, a))
		}
		st, hasState := stateOf[u.ID]
		sc.compareState(u, up, st, hasState)
		sc.compareHoldings(u, up, byUnitHold[u.ID])
		if err := sc.compareUnit(u, up); err != nil {
			return err
		}
	}
	return nil
}

func unitDiff(u db.Unit, table, field, exp, act string) Diff {
	return Diff{Table: table, Key: "unit:" + strconv.FormatInt(u.ID, 10), UnitID: u.ID, Barcode: u.Barcode,
		Field: field, Expected: exp, Actual: act}
}

func (sc *scanResult) compareState(u db.Unit, up *ledger.UnitProjection, st db.UnitCurrentState, has bool) {
	exp := up.State
	switch {
	case exp == nil && !has:
		return
	case exp == nil:
		sc.diff(unitDiff(u, TableUnitState, "row", absent, present))
		sc.dropStates = append(sc.dropStates, u.ID)
		return
	case !has:
		sc.diff(unitDiff(u, TableUnitState, "row", present, absent))
		sc.states, sc.stateUnits = append(sc.states, *exp), append(sc.stateUnits, u)
		return
	}
	n := len(sc.diffs)
	sc.field(u, TableUnitState, "owner_type", string(exp.Owner.Type), st.OwnerType)
	sc.field(u, TableUnitState, "owner_id", itoa(exp.Owner.ID), itoa(st.OwnerID))
	sc.field(u, TableUnitState, "holder_org_id", itoa(exp.Owner.OrgID), itoa(st.HolderOrgID))
	sc.field(u, TableUnitState, "status", string(exp.Status), st.Status)
	sc.field(u, TableUnitState, "last_movement_id", itoa(exp.LastMovementID), i8s(st.LastMovementID))
	if len(sc.diffs) > n {
		sc.states, sc.stateUnits = append(sc.states, *exp), append(sc.stateUnits, u)
	}
}

func (sc *scanResult) field(u db.Unit, table, field, exp, act string) {
	if exp != act {
		sc.diff(unitDiff(u, table, field, exp, act))
	}
}

func (sc *scanResult) compareHoldings(u db.Unit, up *ledger.UnitProjection, rows []db.FixedBarcodeHolding) {
	type key struct {
		t  string
		id int64
	}
	actual := map[key]db.FixedBarcodeHolding{}
	for _, r := range rows {
		actual[key{r.OwnerType, r.OwnerID}] = r
	}
	for _, h := range up.Holdings {
		k := key{string(h.Owner.Type), h.Owner.ID}
		r, ok := actual[k]
		delete(actual, k)
		prefix := fmt.Sprintf("%s:%d.", h.Owner.Type, h.Owner.ID)
		if !ok {
			sc.diff(unitDiff(u, TableFixedHoldings, prefix+"row", present, absent))
			sc.holdings = append(sc.holdings, holdingFix{u, h})
			continue
		}
		n := len(sc.diffs)
		sc.field(u, TableFixedHoldings, prefix+"quantity_on_hand", itoa(h.Quantity), itoa(int64(r.QuantityOnHand)))
		sc.field(u, TableFixedHoldings, prefix+"holder_org_id", itoa(h.Owner.OrgID), itoa(r.HolderOrgID))
		sc.field(u, TableFixedHoldings, prefix+"last_movement_id", itoa(h.LastMovementID), i8s(r.LastMovementID))
		if len(sc.diffs) > n {
			sc.holdings = append(sc.holdings, holdingFix{u, h})
		}
	}
	rest := make([]db.FixedBarcodeHolding, 0, len(actual))
	for _, r := range actual {
		rest = append(rest, r)
	}
	slices.SortFunc(rest, func(a, b db.FixedBarcodeHolding) int { return cmp.Compare(a.ID, b.ID) })
	for _, r := range rest {
		sc.diff(unitDiff(u, TableFixedHoldings, fmt.Sprintf("%s:%d.row", r.OwnerType, r.OwnerID), absent, present))
		sc.dropHoldings = append(sc.dropHoldings, r.ID)
	}
}

func (sc *scanResult) compareUnit(u db.Unit, up *ledger.UnitProjection) error {
	if up.Status != "" && string(up.Status) != u.Status {
		sc.diff(unitDiff(u, TableUnits, "status", string(up.Status), u.Status))
		sc.unitStatus = append(sc.unitStatus, db.SetUnitStatusForRepairParams{ID: u.ID, Status: string(up.Status)})
	}
	if up.RemainingCm != nil {
		act, err := ledger.NumericToCentimeters(u.RemainingMeters)
		if err != nil {
			return fmt.Errorf("rebuild: unit %d meters: %w", u.ID, err)
		}
		if act != *up.RemainingCm || !u.RemainingMeters.Valid {
			sc.diff(unitDiff(u, TableUnits, "remaining_meters", ledger.FormatMeters(*up.RemainingCm), meters(u.RemainingMeters)))
			sc.unitMeters = append(sc.unitMeters, db.SetUnitRemainingMetersForRepairParams{
				ID: u.ID, RemainingMeters: ledger.CentimetersToNumeric(*up.RemainingCm),
			})
		}
	}
	return nil
}

// stocks compares the product stock rows of the scope. A stored row with
// zero stock equals a missing one (Post creates rows and never removes
// them).
func (sc *scanResult) stocks(ctx context.Context, q *db.Queries, orgID int64) error {
	var org pgtype.Int8
	if orgID > 0 {
		org = i8(orgID)
	}
	bins, err := q.ListBinProductStocksForRebuild(ctx, org)
	if err != nil {
		return fmt.Errorf("rebuild: bin stocks: %w", err)
	}
	orgs, err := q.ListOrganizationProductStocksForRebuild(ctx, org)
	if err != nil {
		return fmt.Errorf("rebuild: organization stocks: %w", err)
	}
	inScope := func(o int64) bool { return orgID == 0 || o == orgID }

	type amount struct {
		qty, cm int64
		brand   int64
		org     int64
	}
	actualBin := map[ledger.BinStockKey]amount{}
	for _, r := range bins {
		cm, err := ledger.NumericToCentimeters(r.Meters)
		if err != nil {
			return fmt.Errorf("rebuild: bin %d meters: %w", r.LocationID, err)
		}
		actualBin[ledger.BinStockKey{LocationID: r.LocationID, ProductID: r.ProductID}] =
			amount{int64(r.Quantity), cm, r.BrandID, r.OrganizationID}
	}
	binKeys := keysUnion(sc.exp.BinStocks, actualBin)
	slices.SortFunc(binKeys, func(a, b ledger.BinStockKey) int {
		return cmp.Or(cmp.Compare(a.LocationID, b.LocationID), cmp.Compare(a.ProductID, b.ProductID))
	})
	for _, k := range binKeys {
		e, a := sc.exp.BinStocks[k], actualBin[k]
		if _, ok := sc.exp.BinStocks[k]; ok && !inScope(e.OrganizationID) {
			continue
		}
		if e.Quantity == a.qty && e.Centimeters == a.cm {
			continue
		}
		key := fmt.Sprintf("location:%d/product:%d", k.LocationID, k.ProductID)
		sc.stockDiffs(TableBinStocks, key, e.StockAmount, a.qty, a.cm)
		fix := db.UpsertBinProductStockForRepairParams{
			LocationID: k.LocationID, ProductID: k.ProductID,
			OrganizationID: cmp.Or(e.OrganizationID, a.org), BrandID: cmp.Or(e.BrandID, a.brand),
		}
		var err error
		if fix.Quantity, fix.Meters, err = stockValues(e.StockAmount); err != nil {
			return err
		}
		sc.binFixes = append(sc.binFixes, fix)
	}

	actualOrg := map[ledger.OrgStockKey]amount{}
	for _, r := range orgs {
		cm, err := ledger.NumericToCentimeters(r.Meters)
		if err != nil {
			return fmt.Errorf("rebuild: organization %d meters: %w", r.OrganizationID, err)
		}
		actualOrg[ledger.OrgStockKey{OrganizationID: r.OrganizationID, ProductID: r.ProductID}] =
			amount{int64(r.Quantity), cm, r.BrandID, r.OrganizationID}
	}
	orgKeys := keysUnion(sc.exp.OrgStocks, actualOrg)
	slices.SortFunc(orgKeys, func(a, b ledger.OrgStockKey) int {
		return cmp.Or(cmp.Compare(a.OrganizationID, b.OrganizationID), cmp.Compare(a.ProductID, b.ProductID))
	})
	for _, k := range orgKeys {
		if !inScope(k.OrganizationID) {
			continue
		}
		e, a := sc.exp.OrgStocks[k], actualOrg[k]
		if e.Quantity == a.qty && e.Centimeters == a.cm {
			continue
		}
		key := fmt.Sprintf("organization:%d/product:%d", k.OrganizationID, k.ProductID)
		sc.stockDiffs(TableOrgStocks, key, e, a.qty, a.cm)
		fix := db.UpsertOrganizationProductStockForRepairParams{
			OrganizationID: k.OrganizationID, ProductID: k.ProductID, BrandID: cmp.Or(e.BrandID, a.brand),
		}
		var err error
		if fix.Quantity, fix.Meters, err = stockValues(e); err != nil {
			return err
		}
		sc.orgFixes = append(sc.orgFixes, fix)
	}
	return nil
}

func (sc *scanResult) stockDiffs(table, key string, e ledger.StockAmount, qty, cm int64) {
	if e.Quantity != qty {
		sc.diff(Diff{Table: table, Key: key, Field: "quantity", Expected: itoa(e.Quantity), Actual: itoa(qty)})
	}
	if e.Centimeters != cm {
		sc.diff(Diff{Table: table, Key: key, Field: "meters",
			Expected: ledger.FormatMeters(e.Centimeters), Actual: ledger.FormatMeters(cm)})
	}
}

func stockValues(e ledger.StockAmount) (int32, pgtype.Numeric, error) {
	if e.Quantity < 0 || e.Centimeters < 0 || e.Quantity > 1<<31-1 {
		return 0, pgtype.Numeric{}, fmt.Errorf("rebuild: expected stock out of range (%d, %s m)",
			e.Quantity, ledger.FormatMeters(e.Centimeters))
	}
	return int32(e.Quantity), ledger.CentimetersToNumeric(e.Centimeters), nil
}

// apply writes the expected rows. Order: unit rows, unit states, holdings,
// then product stocks (Post's order).
func (sc *scanResult) apply(ctx context.Context, q *db.Queries) error {
	for _, p := range sc.unitStatus {
		if err := q.SetUnitStatusForRepair(ctx, p); err != nil {
			return fmt.Errorf("rebuild: unit %d status: %w", p.ID, err)
		}
	}
	for _, p := range sc.unitMeters {
		if err := q.SetUnitRemainingMetersForRepair(ctx, p); err != nil {
			return fmt.Errorf("rebuild: unit %d meters: %w", p.ID, err)
		}
	}
	for _, id := range sc.dropStates {
		if err := q.DeleteUnitCurrentStateForRepair(ctx, id); err != nil {
			return fmt.Errorf("rebuild: drop unit %d state: %w", id, err)
		}
	}
	for i, st := range sc.states {
		u := sc.stateUnits[i]
		if err := q.UpsertUnitCurrentStateForRepair(ctx, db.UpsertUnitCurrentStateForRepairParams{
			UnitID: u.ID, BrandID: u.BrandID, OwnerType: string(st.Owner.Type), OwnerID: st.Owner.ID,
			HolderOrgID: st.Owner.OrgID, Status: string(st.Status), LastMovementID: i8(st.LastMovementID),
		}); err != nil {
			return fmt.Errorf("rebuild: unit %d state: %w", u.ID, err)
		}
	}
	for _, id := range sc.dropHoldings {
		if err := q.DeleteFixedBarcodeHoldingForRepair(ctx, id); err != nil {
			return fmt.Errorf("rebuild: drop holding %d: %w", id, err)
		}
	}
	for _, f := range sc.holdings {
		if f.h.Quantity < 0 || f.h.Quantity > 1<<31-1 {
			return fmt.Errorf("rebuild: unit %d holding quantity %d out of range", f.unit.ID, f.h.Quantity)
		}
		if err := q.UpsertFixedBarcodeHoldingForRepair(ctx, db.UpsertFixedBarcodeHoldingForRepairParams{
			UnitID: f.unit.ID, BrandID: f.unit.BrandID, OwnerType: string(f.h.Owner.Type), OwnerID: f.h.Owner.ID,
			HolderOrgID: f.h.Owner.OrgID, QuantityOnHand: int32(f.h.Quantity), LastMovementID: i8(f.h.LastMovementID),
		}); err != nil {
			return fmt.Errorf("rebuild: unit %d holding: %w", f.unit.ID, err)
		}
	}
	for _, p := range sc.binFixes {
		if err := q.UpsertBinProductStockForRepair(ctx, p); err != nil {
			return fmt.Errorf("rebuild: bin %d product %d: %w", p.LocationID, p.ProductID, err)
		}
	}
	for _, p := range sc.orgFixes {
		if err := q.UpsertOrganizationProductStockForRepair(ctx, p); err != nil {
			return fmt.Errorf("rebuild: organization %d product %d: %w", p.OrganizationID, p.ProductID, err)
		}
	}
	return nil
}

func groupBy[T any](rows []T, key func(T) int64) map[int64][]T {
	out := map[int64][]T{}
	for _, r := range rows {
		k := key(r)
		out[k] = append(out[k], r)
	}
	return out
}

func keysUnion[K comparable, A, B any](a map[K]A, b map[K]B) []K {
	keys := make([]K, 0, len(a)+len(b))
	for k := range a {
		keys = append(keys, k)
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	return keys
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

func i8s(v pgtype.Int8) string {
	if !v.Valid {
		return "null"
	}
	return itoa(v.Int64)
}

func meters(n pgtype.Numeric) string {
	if !n.Valid {
		return "null"
	}
	cm, err := ledger.NumericToCentimeters(n)
	if err != nil {
		return "invalid"
	}
	return ledger.FormatMeters(cm)
}

func i8(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: v != 0} }

func jsonMarshal(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("rebuild: audit payload: %w", err)
	}
	return b, nil
}
