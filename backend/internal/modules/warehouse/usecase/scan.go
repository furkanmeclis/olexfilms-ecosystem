package usecase

// Universal scan resolver (TEC-203, F1-03c): one scanned string is resolved
// to a location, a unit or a product. Order:
//
//  1. OFW:LOC:<full_code> (location QR, TEC-202) -> a location of the
//     active organization; nothing else is tried.
//  2. OFW:UNIT:<barcode> (unit QR) -> the unit; nothing else is tried.
//  3. Unit barcode (generated <PREFIX>-<8 digits>, roll split
//     <barcode>-S<n> (TEC-184), legacy/import barcodes): exact, then
//     upper-cased.
//  4. Location full_code without prefix (scan.bare_location_code_enabled).
//  5. Product SKU of the active brand (scan.sku_enabled).
//  6. Short code: 1-8 digits (optionally -S<n>) expanded to
//     <PREFIX>-<8 digits> (scan.short_code_enabled, prefix from
//     scan.short_code_prefix or the brand's default prefix).
//
// Reach: the warehouse module rule (center and distributor, K12) gates the
// endpoint; locations resolve inside the active organization only; units
// follow the stock.read filter exactly like the barcode history (a
// distributor reads its dealers' units through the TEC-216 subtree grant;
// a unit held outside the reach is "no match", never leaked); products
// resolve inside the active brand (K1).

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/labels"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Scan errors.
var (
	// ErrScanNoMatch: the code resolves to nothing the caller may see.
	ErrScanNoMatch = errors.New("warehouse: scan matched nothing")
	// ErrScanLocationNotFound: a location QR whose full_code is not a
	// location of the active organization.
	ErrScanLocationNotFound = errors.New("warehouse: scanned location not found")
)

// Scan result types and match kinds.
const (
	ScanTypeLocation = "location"
	ScanTypeUnit     = "unit"
	ScanTypeProduct  = "product"

	MatchLocationQR   = "location_qr"
	MatchLocationCode = "location_code"
	MatchUnitQR       = "unit_qr"
	MatchBarcode      = "barcode"
	MatchSKU          = "sku"
	MatchShortCode    = "short_code"
)

// MaxScanLen bounds the scanned string.
const MaxScanLen = 200

var (
	shortCodePattern = regexp.MustCompile(`^([0-9]{1,8})(-S[0-9]{1,4})?$`)
	scanPrefixRe     = regexp.MustCompile(`^[A-Z0-9]{2,8}$`)
)

// ScanSettings reads the scan.* system settings (sysconfig.Service).
type ScanSettings interface {
	Scan(ctx context.Context) sysconfig.Scan
}

// ScanCaller is the scanning principal: the warehouse.read caller plus the
// principal from which the stock.read reach is resolved.
type ScanCaller struct {
	Caller
	Principal authctx.Principal
}

// ScanPathNode is one level of a location's tree path (warehouse, room,
// aisle, shelf, bin; "location" for an untyped legacy location).
type ScanPathNode struct {
	Level string    `json:"level"`
	UUID  uuid.UUID `json:"uuid"`
	Code  string    `json:"code"`
	Name  string    `json:"name"`
}

// ScanLocation summarizes a location with its tree path, root first.
type ScanLocation struct {
	UUID     uuid.UUID      `json:"uuid"`
	Type     *string        `json:"type"`
	Code     string         `json:"code"`
	FullCode string         `json:"full_code"`
	Name     string         `json:"name"`
	Active   bool           `json:"active"`
	Path     []ScanPathNode `json:"path"`
}

// ScanProduct summarizes a product.
type ScanProduct struct {
	UUID             uuid.UUID `json:"uuid"`
	SKU              string    `json:"sku"`
	Name             string    `json:"name"`
	UnitType         string    `json:"unit_type"`
	UsesFixedBarcode bool      `json:"uses_fixed_barcode"`
	Active           bool      `json:"active"`
}

// ScanOrganization is the holder of a unit.
type ScanOrganization struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Type string    `json:"type"`
}

// ScanUnit summarizes a unit: its status, meters, holder and location.
type ScanUnit struct {
	UUID            uuid.UUID         `json:"uuid"`
	Barcode         string            `json:"barcode"`
	UnitKind        string            `json:"unit_kind"`
	Status          string            `json:"status"`
	InitialMeters   *string           `json:"initial_meters"`
	RemainingMeters *string           `json:"remaining_meters"`
	OwnerType       *string           `json:"owner_type"`
	QuantityOnHand  *int32            `json:"quantity_on_hand"`
	Holder          *ScanOrganization `json:"holder"`
	Location        *ScanLocation     `json:"location"`
}

// ScanResult is the resolved entity: Type names which of Location, Unit
// or Product is set (a unit result also carries its Product).
type ScanResult struct {
	Type      string        `json:"type"`
	MatchedBy string        `json:"matched_by"`
	Code      string        `json:"code"`
	Location  *ScanLocation `json:"location"`
	Unit      *ScanUnit     `json:"unit"`
	Product   *ScanProduct  `json:"product"`
}

// Scanner implements the scan resolver.
type Scanner struct {
	q        *db.Queries
	stock    *stockusecase.Service
	settings ScanSettings
}

// NewScanner builds the resolver.
func NewScanner(q *db.Queries, settings ScanSettings) *Scanner {
	return &Scanner{q: q, stock: stockusecase.New(q), settings: settings}
}

// Resolve resolves one scanned string.
func (s *Scanner) Resolve(ctx context.Context, c ScanCaller, raw string) (ScanResult, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return ScanResult{}, err
	}
	code := strings.TrimSpace(raw)
	if code == "" {
		return ScanResult{}, invalid("code", "is required")
	}
	if utf8.RuneCountInString(code) > MaxScanLen || strings.ContainsAny(code, "\r\n\t") {
		return ScanResult{}, invalid("code", "must be a single line of at most 200 characters")
	}
	upper := strings.ToUpper(code)
	cfg := s.settings.Scan(ctx)

	// 1-2. Prefixed QR payloads resolve only as what they say they are.
	if rest, ok := strings.CutPrefix(upper, labels.LocationPrefix); ok {
		res, found, err := s.location(ctx, org, strings.TrimSpace(rest), MatchLocationQR)
		if err == nil && !found {
			err = ErrScanLocationNotFound
		}
		return res, err
	}
	if strings.HasPrefix(upper, labels.UnitPrefix) {
		barcode := strings.TrimSpace(code[len(labels.UnitPrefix):])
		res, found, err := s.unit(ctx, c, MatchUnitQR, barcode, strings.ToUpper(barcode))
		if err == nil && !found {
			err = ErrScanNoMatch
		}
		return res, err
	}

	// 3. Unit barcode.
	if res, found, err := s.unit(ctx, c, MatchBarcode, code, upper); err != nil || found {
		return res, err
	}
	// 4. Bare location full_code.
	if cfg.BareLocationCodeEnabled {
		if res, found, err := s.location(ctx, org, upper, MatchLocationCode); err != nil || found {
			return res, err
		}
	}
	// 5. Product SKU.
	if cfg.SKUEnabled {
		if res, found, err := s.product(ctx, c, code, upper); err != nil || found {
			return res, err
		}
	}
	// 6. Short code.
	if cfg.ShortCodeEnabled {
		if m := shortCodePattern.FindStringSubmatch(upper); m != nil {
			seq, _ := strconv.ParseInt(m[1], 10, 64)
			prefix := strings.ToUpper(strings.TrimSpace(cfg.ShortCodePrefix))
			if !scanPrefixRe.MatchString(prefix) {
				prefix = stockusecase.DefaultPrefix(c.Org.BrandSlug)
			}
			if seq > 0 {
				barcode := stockusecase.Barcode(prefix, seq) + m[2]
				if res, found, err := s.unit(ctx, c, MatchShortCode, barcode); err != nil || found {
					return res, err
				}
			}
		}
	}
	return ScanResult{}, ErrScanNoMatch
}

// location resolves a full_code of the active organization.
func (s *Scanner) location(ctx context.Context, org int64, fullCode, match string) (ScanResult, bool, error) {
	if fullCode == "" {
		return ScanResult{}, false, nil
	}
	l, err := s.q.GetWarehouseLocationByCode(ctx, db.GetWarehouseLocationByCodeParams{OrganizationID: org, Code: fullCode})
	if errors.Is(err, pgx.ErrNoRows) {
		return ScanResult{}, false, nil
	}
	if err != nil {
		return ScanResult{}, false, fmt.Errorf("warehouse: scan location: %w", err)
	}
	loc, err := s.locationSummary(ctx, l)
	if err != nil {
		return ScanResult{}, false, err
	}
	return ScanResult{Type: ScanTypeLocation, MatchedBy: match, Code: loc.FullCode, Location: &loc}, true, nil
}

// locationSummary builds the summary and the root-first tree path of l
// inside its own organization.
func (s *Scanner) locationSummary(ctx context.Context, l db.WarehouseLocation) (ScanLocation, error) {
	out := ScanLocation{UUID: l.Uuid, Type: textPtr(l.Type), Code: l.Code, FullCode: l.Code, Name: l.Name, Active: l.Active}
	if l.FullCode.Valid {
		out.FullCode = l.FullCode.String
	}
	level := func(x db.WarehouseLocation) string {
		if x.Type.Valid {
			return x.Type.String
		}
		return "location"
	}
	// Location chain, leaf first (the tree is at most aisle/shelf/bin deep;
	// the bound guards a corrupt parent cycle).
	chain := []ScanPathNode{{Level: level(l), UUID: l.Uuid, Code: l.Code, Name: l.Name}}
	cur := l
	for range 8 {
		if !cur.ParentID.Valid {
			break
		}
		p, err := s.q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: cur.ParentID.Int64, OrganizationID: l.OrganizationID})
		if err != nil {
			return ScanLocation{}, fmt.Errorf("warehouse: scan location parent: %w", err)
		}
		chain = append(chain, ScanPathNode{Level: level(p), UUID: p.Uuid, Code: p.Code, Name: p.Name})
		cur = p
	}
	if l.RoomID.Valid {
		r, err := s.q.GetRoomByID(ctx, db.GetRoomByIDParams{ID: l.RoomID.Int64, OrganizationID: l.OrganizationID})
		if err != nil {
			return ScanLocation{}, fmt.Errorf("warehouse: scan room: %w", err)
		}
		chain = append(chain, ScanPathNode{Level: "room", UUID: r.Uuid, Code: r.Code, Name: r.Name})
		w, err := s.q.GetWarehouseByID(ctx, db.GetWarehouseByIDParams{ID: r.WarehouseID, OrganizationID: l.OrganizationID})
		if err != nil {
			return ScanLocation{}, fmt.Errorf("warehouse: scan warehouse: %w", err)
		}
		chain = append(chain, ScanPathNode{Level: "warehouse", UUID: w.Uuid, Code: w.Code, Name: w.Name})
	}
	out.Path = make([]ScanPathNode, 0, len(chain))
	for i := len(chain) - 1; i >= 0; i-- {
		out.Path = append(out.Path, chain[i])
	}
	return out, nil
}

// stockFilter resolves the caller's stock.read reach; ok is false when the
// caller holds no stock.read (units then never resolve).
func (s *Scanner) stockFilter(ctx context.Context, c ScanCaller) (scopefilter.Filter, bool, error) {
	f, err := scopefilter.Resolve(ctx, s.q, c.Principal, &c.Org, rbac.PermStockRead)
	switch {
	case err == nil:
		return f, !f.UserOnly(), nil
	case errors.Is(err, scopefilter.ErrForbidden), errors.Is(err, scopefilter.ErrOrganizationRequired):
		return scopefilter.Filter{}, false, nil
	default:
		return scopefilter.Filter{}, false, fmt.Errorf("warehouse: scan stock scope: %w", err)
	}
}

// unit tries each candidate barcode in order.
func (s *Scanner) unit(ctx context.Context, c ScanCaller, match string, candidates ...string) (ScanResult, bool, error) {
	f, ok, err := s.stockFilter(ctx, c)
	if err != nil || !ok {
		return ScanResult{}, false, err
	}
	seen := map[string]bool{}
	for _, bc := range candidates {
		if bc == "" || seen[bc] {
			continue
		}
		seen[bc] = true
		ru, err := s.stock.ReachableUnit(ctx, c.Org, f, bc)
		if errors.Is(err, stockusecase.ErrNotFound) {
			continue
		}
		if err != nil {
			return ScanResult{}, false, err
		}
		res, err := s.unitResult(ctx, ru, match)
		return res, err == nil, err
	}
	return ScanResult{}, false, nil
}

func (s *Scanner) unitResult(ctx context.Context, ru stockusecase.ReachedUnit, match string) (ScanResult, error) {
	u := ru.Unit
	p, err := s.q.GetProductByIDAnyBrand(ctx, u.ProductID)
	if err != nil {
		return ScanResult{}, fmt.Errorf("warehouse: scan unit product: %w", err)
	}
	prod := productSummary(p)
	out := ScanUnit{
		UUID: u.Uuid, Barcode: u.Barcode, UnitKind: u.UnitKind, Status: u.Status,
		InitialMeters: stockusecase.MetersString(u.InitialMeters), RemainingMeters: stockusecase.MetersString(u.RemainingMeters),
	}
	var holderID, locID int64
	switch {
	case ru.State != nil:
		ot := ru.State.OwnerType
		out.OwnerType = &ot
		holderID = ru.State.HolderOrgID
		if ru.State.OwnerLocationID.Valid {
			locID = ru.State.OwnerLocationID.Int64
		}
	case len(ru.Holdings) > 0:
		var qty int32
		for _, h := range ru.Holdings {
			qty += h.QuantityOnHand
		}
		out.QuantityOnHand = &qty
		if len(ru.Holdings) == 1 {
			h := ru.Holdings[0]
			ot := h.OwnerType
			out.OwnerType = &ot
			holderID = h.HolderOrgID
			if h.OwnerLocationID.Valid {
				locID = h.OwnerLocationID.Int64
			}
		}
	}
	if holderID != 0 {
		o, err := s.q.GetOrganizationByID(ctx, holderID)
		if err != nil {
			return ScanResult{}, fmt.Errorf("warehouse: scan holder: %w", err)
		}
		out.Holder = &ScanOrganization{UUID: o.Uuid, Name: o.Name, Type: o.Type}
		if locID != 0 {
			l, err := s.q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: locID, OrganizationID: holderID})
			switch {
			case err == nil:
				loc, err := s.locationSummary(ctx, l)
				if err != nil {
					return ScanResult{}, err
				}
				out.Location = &loc
			case !errors.Is(err, pgx.ErrNoRows):
				return ScanResult{}, fmt.Errorf("warehouse: scan unit location: %w", err)
			}
		}
	}
	return ScanResult{Type: ScanTypeUnit, MatchedBy: match, Code: u.Barcode, Unit: &out, Product: &prod}, nil
}

// product resolves a SKU of the active brand (exact, then upper-cased).
func (s *Scanner) product(ctx context.Context, c ScanCaller, candidates ...string) (ScanResult, bool, error) {
	for i, sku := range candidates {
		if i > 0 && sku == candidates[0] {
			continue
		}
		p, err := s.q.GetProductBySKU(ctx, db.GetProductBySKUParams{BrandID: c.Org.BrandID, Sku: sku})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return ScanResult{}, false, fmt.Errorf("warehouse: scan product: %w", err)
		}
		prod := productSummary(p)
		return ScanResult{Type: ScanTypeProduct, MatchedBy: MatchSKU, Code: p.Sku, Product: &prod}, true, nil
	}
	return ScanResult{}, false, nil
}

func productSummary(p db.Product) ScanProduct {
	return ScanProduct{
		UUID: p.Uuid, SKU: p.Sku, Name: p.Name, UnitType: p.UnitType,
		UsesFixedBarcode: p.UsesFixedBarcode, Active: p.Active,
	}
}
