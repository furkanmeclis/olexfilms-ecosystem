package usecase

import (
	"context"
	"errors"
	"fmt"
	"html"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	pricinguc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	servicecataloguc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/servicecatalog/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	QuoteStatusDraft    = "draft"
	QuoteStatusSent     = "sent"
	QuoteStatusAccepted = "accepted"
	QuoteStatusRejected = "rejected"
	QuoteStatusExpired  = "expired"

	QuoteLineProduct        = "product"
	QuoteLineCatalogService = "catalog_service"
)

var (
	ErrQuoteNotFound  = errors.New("quotes: not found")
	ErrQuoteConflict  = errors.New("quotes: conflict")
	ErrQuoteForbidden = errors.New("quotes: forbidden")

	quantityRe = regexp.MustCompile(`^[0-9]{1,9}(\.[0-9]{1,3})?$`)
	moneyRe    = regexp.MustCompile(`^[0-9]{1,16}(\.[0-9]{1,2})?$`)
)

type Quote struct {
	UUID            uuid.UUID   `json:"uuid"`
	LeadUUID        uuid.UUID   `json:"lead_uuid"`
	QuoteNo         int64       `json:"quote_no"`
	DisplayNo       string      `json:"display_no"`
	Currency        string      `json:"currency"`
	Subtotal        string      `json:"subtotal"`
	DiscountTotal   string      `json:"discount_total"`
	TaxTotal        string      `json:"tax_total"`
	GrandTotal      string      `json:"grand_total"`
	ValidUntil      *time.Time  `json:"valid_until,omitempty"`
	Status          string      `json:"status"`
	CreatedByUserID *int64      `json:"created_by_user_id,omitempty"`
	Lines           []QuoteLine `json:"lines"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

type QuoteLine struct {
	LineType               string     `json:"line_type"`
	ProductUUID            *uuid.UUID `json:"product_uuid,omitempty"`
	ServiceCatalogItemUUID *uuid.UUID `json:"service_catalog_item_uuid,omitempty"`
	Description            string     `json:"description"`
	Quantity               string     `json:"quantity"`
	UnitPrice              string     `json:"unit_price"`
	DiscountAmount         string     `json:"discount_amount"`
	LineTotal              string     `json:"line_total"`
	SortOrder              int32      `json:"sort_order"`
}

type QuoteLineInput struct {
	LineType               string
	ProductUUID            *uuid.UUID
	ServiceCatalogItemUUID *uuid.UUID
	Description            *string
	Quantity               string
	UnitPrice              *string
	DiscountAmount         *string
}

type QuoteInput struct {
	ValidUntil *time.Time
	Lines      []QuoteLineInput
}

type QuotePatchInput struct {
	ValidUntil Field[time.Time]
}

type QuoteDecisionInput struct {
	Reason *string
}

func (s *Service) pricing() *pricinguc.Service {
	return pricinguc.New(s.q)
}

func (s *Service) serviceCatalog() *servicecataloguc.Service {
	return servicecataloguc.New(s.q)
}

func validQuoteStatus(v string) bool {
	switch v {
	case QuoteStatusDraft, QuoteStatusSent, QuoteStatusAccepted, QuoteStatusRejected, QuoteStatusExpired:
		return true
	}
	return false
}

func quoteDisplayNo(no int64) string {
	return fmt.Sprintf("Q-%06d", no)
}

func numericValue(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, err
	}
	return n, nil
}

func rat(s string) (*big.Rat, error) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	if !ok {
		return nil, fmt.Errorf("invalid decimal %q", s)
	}
	return r, nil
}

func numericRat(n pgtype.Numeric) *big.Rat {
	if !n.Valid || n.Int == nil {
		return nil
	}
	r := new(big.Rat).SetInt(n.Int)
	if n.Exp != 0 {
		e := n.Exp
		if e < 0 {
			e = -e
		}
		p := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(e)), nil))
		if n.Exp > 0 {
			r.Mul(r, p)
		} else {
			r.Quo(r, p)
		}
	}
	return r
}

func moneyText(n pgtype.Numeric) string {
	r := numericRat(n)
	if r == nil {
		return "0.00"
	}
	return r.FloatString(2)
}

func qtyText(n pgtype.Numeric) string {
	r := numericRat(n)
	if r == nil {
		return "0.000"
	}
	return strings.TrimRight(strings.TrimRight(r.FloatString(3), "0"), ".")
}

// round2 rounds to 2 decimals (half away from zero, as FloatString does).
func round2(r *big.Rat) *big.Rat {
	out, _ := new(big.Rat).SetString(r.FloatString(2))
	return out
}

func parseQuantity(raw string) (string, *big.Rat, error) {
	q := strings.TrimSpace(raw)
	if q == "" {
		q = "1"
	}
	if !quantityRe.MatchString(q) {
		return "", nil, invalid("quantity", "must be a positive decimal with at most 3 fractional digits")
	}
	r, err := rat(q)
	if err != nil || r.Sign() <= 0 {
		return "", nil, invalid("quantity", "must be greater than zero")
	}
	return r.FloatString(3), r, nil
}

func parseMoney(field string, raw *string, def string) (string, *big.Rat, error) {
	v := def
	if raw != nil {
		v = strings.TrimSpace(*raw)
	}
	if v == "" {
		v = "0"
	}
	if raw != nil && !moneyRe.MatchString(v) {
		return "", nil, invalid(field, "must be a non-negative decimal with at most 2 fractional digits")
	}
	r, err := rat(v)
	if err != nil || r.Sign() < 0 {
		return "", nil, invalid(field, "must be non-negative")
	}
	return r.FloatString(2), r, nil
}

func dateArg(v *time.Time) pgtype.Date {
	if v == nil {
		return pgtype.Date{}
	}
	t := time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, time.UTC)
	return pgtype.Date{Time: t, Valid: true}
}

func datePtr(v pgtype.Date) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func saleWriteScope(c Caller) rbac.Scope {
	if c.Org.OrgType == pricinguc.OrgCenter {
		return rbac.ScopeBrand
	}
	return rbac.ScopeManaged
}

func canOverridePrice(c Caller) bool {
	return c.Principal.Can(rbac.PermPricingSaleWrite, saleWriteScope(c))
}

func (s *Service) quoteRow(ctx context.Context, c Caller, id uuid.UUID) (db.Quote, error) {
	row, err := s.q.GetQuoteByUUID(ctx, db.GetQuoteByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Quote{}, ErrQuoteNotFound
	}
	if err != nil {
		return db.Quote{}, fmt.Errorf("quotes: get: %w", err)
	}
	if !c.Filter.AllowsOrg(row.OrganizationID, row.BrandID) {
		return db.Quote{}, ErrQuoteNotFound
	}
	return row, nil
}

func (s *Service) quoteOf(ctx context.Context, row db.Quote) (Quote, error) {
	lead, err := s.q.GetLeadByID(ctx, db.GetLeadByIDParams{ID: row.LeadID, BrandID: row.BrandID})
	if err != nil {
		return Quote{}, fmt.Errorf("quotes: lead: %w", err)
	}
	lines, err := s.q.ListQuoteLines(ctx, row.ID)
	if err != nil {
		return Quote{}, fmt.Errorf("quotes: lines: %w", err)
	}
	out := Quote{
		UUID: row.Uuid, LeadUUID: lead.Uuid, QuoteNo: row.QuoteNo, DisplayNo: quoteDisplayNo(row.QuoteNo),
		Currency: row.Currency, Subtotal: moneyText(row.Subtotal), DiscountTotal: moneyText(row.DiscountTotal),
		TaxTotal: moneyText(row.TaxTotal), GrandTotal: moneyText(row.GrandTotal), ValidUntil: datePtr(row.ValidUntil),
		Status: row.Status, CreatedByUserID: intPtr(row.CreatedByUserID), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	out.Lines = make([]QuoteLine, 0, len(lines))
	for _, l := range lines {
		ql := QuoteLine{
			LineType: l.LineType, Description: l.DescriptionSnapshot, Quantity: qtyText(l.Quantity),
			UnitPrice: moneyText(l.UnitPrice), DiscountAmount: moneyText(l.DiscountAmount),
			LineTotal: moneyText(l.LineTotal), SortOrder: l.SortOrder,
		}
		if l.ProductID.Valid {
			if p, err := s.q.GetProductByIDAnyBrand(ctx, l.ProductID.Int64); err == nil && p.BrandID == l.BrandID {
				ql.ProductUUID = &p.Uuid
			}
		}
		if l.ServiceCatalogItemID.Valid {
			if item, err := s.q.GetServiceCatalogItemByID(ctx, l.ServiceCatalogItemID.Int64); err == nil {
				ql.ServiceCatalogItemUUID = &item.Uuid
			}
		}
		out.Lines = append(out.Lines, ql)
	}
	return out, nil
}

func (s *Service) CreateQuote(ctx context.Context, c Caller, leadID uuid.UUID, in QuoteInput) (Quote, error) {
	lead, err := s.getRow(ctx, c, leadID)
	if err != nil {
		return Quote{}, err
	}
	org, err := s.q.GetOrganizationByID(ctx, lead.OrganizationID)
	if err != nil {
		return Quote{}, fmt.Errorf("quotes: organization: %w", err)
	}
	var row db.Quote
	err = s.inTx(ctx, func(q *db.Queries) error {
		// quote_no is MAX+1 per organization: serialize concurrent creates.
		if err := q.LockQuoteNumbering(ctx, lead.OrganizationID); err != nil {
			return fmt.Errorf("quotes: lock numbering: %w", err)
		}
		no, err := q.NextQuoteNo(ctx, lead.OrganizationID)
		if err != nil {
			return fmt.Errorf("quotes: next no: %w", err)
		}
		zero, _ := numericValue("0.00")
		row, err = q.CreateQuote(ctx, db.CreateQuoteParams{
			OrganizationID: lead.OrganizationID, BrandID: lead.BrandID, LeadID: lead.ID, QuoteNo: int64(no),
			Currency: org.Currency, Subtotal: zero, DiscountTotal: zero, TaxTotal: zero, GrandTotal: zero,
			ValidUntil: dateArg(in.ValidUntil), Status: QuoteStatusDraft, CreatedByUserID: c.actor(),
		})
		if err != nil {
			return fmt.Errorf("quotes: create: %w", err)
		}
		row, err = s.replaceQuoteLines(ctx, q, c, row, org, in.Lines)
		if err != nil {
			return err
		}
		return addEvent(ctx, q, lead, EventMessage, map[string]any{"kind": "quote_created", "quote_uuid": row.Uuid.String(), "quote_no": row.QuoteNo}, c.actor())
	})
	if err != nil {
		return Quote{}, err
	}
	return s.quoteOf(ctx, row)
}

func (s *Service) GetQuote(ctx context.Context, c Caller, id uuid.UUID) (Quote, error) {
	row, err := s.quoteRow(ctx, c, id)
	if err != nil {
		return Quote{}, err
	}
	return s.quoteOf(ctx, row)
}

func (s *Service) PatchQuote(ctx context.Context, c Caller, id uuid.UUID, in QuotePatchInput) (Quote, error) {
	cur, err := s.quoteRow(ctx, c, id)
	if err != nil {
		return Quote{}, err
	}
	if cur.Status != QuoteStatusDraft {
		return Quote{}, ErrQuoteConflict
	}
	if !in.ValidUntil.Set {
		return s.quoteOf(ctx, cur)
	}
	row, err := s.q.UpdateQuoteTotals(ctx, db.UpdateQuoteTotalsParams{
		ID: cur.ID, OrganizationID: cur.OrganizationID, Currency: cur.Currency,
		Subtotal: cur.Subtotal, DiscountTotal: cur.DiscountTotal, TaxTotal: cur.TaxTotal, GrandTotal: cur.GrandTotal,
		ValidUntil: dateArg(in.ValidUntil.Value),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Quote{}, ErrQuoteConflict
	}
	if err != nil {
		return Quote{}, fmt.Errorf("quotes: patch: %w", err)
	}
	return s.quoteOf(ctx, row)
}

func (s *Service) ReplaceQuoteLines(ctx context.Context, c Caller, id uuid.UUID, lines []QuoteLineInput) (Quote, error) {
	cur, err := s.quoteRow(ctx, c, id)
	if err != nil {
		return Quote{}, err
	}
	if cur.Status != QuoteStatusDraft {
		return Quote{}, ErrQuoteConflict
	}
	org, err := s.q.GetOrganizationByID(ctx, cur.OrganizationID)
	if err != nil {
		return Quote{}, fmt.Errorf("quotes: organization: %w", err)
	}
	var row db.Quote
	err = s.inTx(ctx, func(q *db.Queries) error {
		// Row lock: concurrent replaces would otherwise both delete only the
		// committed lines and leave the union of their inserts.
		locked, err := q.LockQuoteByID(ctx, cur.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrQuoteNotFound
		}
		if err != nil {
			return fmt.Errorf("quotes: lock: %w", err)
		}
		if locked.Status != QuoteStatusDraft {
			return ErrQuoteConflict
		}
		row, err = s.replaceQuoteLines(ctx, q, c, locked, org, lines)
		return err
	})
	if err != nil {
		return Quote{}, err
	}
	return s.quoteOf(ctx, row)
}

func (s *Service) replaceQuoteLines(ctx context.Context, q *db.Queries, c Caller, quote db.Quote, org db.Organization, inputs []QuoteLineInput) (db.Quote, error) {
	if _, err := q.DeleteQuoteLines(ctx, quote.ID); err != nil {
		return db.Quote{}, fmt.Errorf("quotes: delete lines: %w", err)
	}
	subtotal := new(big.Rat)
	discountTotal := new(big.Rat)
	for i, in := range inputs {
		p, gross, discount, err := s.buildLine(ctx, c, quote, org, in, int32((i+1)*10))
		if err != nil {
			return db.Quote{}, err
		}
		if _, err := q.CreateQuoteLine(ctx, p); err != nil {
			return db.Quote{}, fmt.Errorf("quotes: create line: %w", err)
		}
		subtotal.Add(subtotal, gross)
		discountTotal.Add(discountTotal, discount)
	}
	tax := "0.00"
	grand := new(big.Rat).Sub(subtotal, discountTotal).FloatString(2)
	sub, err := numericValue(subtotal.FloatString(2))
	if err != nil {
		return db.Quote{}, err
	}
	disc, err := numericValue(discountTotal.FloatString(2))
	if err != nil {
		return db.Quote{}, err
	}
	taxN, _ := numericValue(tax)
	grandN, err := numericValue(grand)
	if err != nil {
		return db.Quote{}, err
	}
	row, err := q.UpdateQuoteTotals(ctx, db.UpdateQuoteTotalsParams{
		ID: quote.ID, OrganizationID: quote.OrganizationID, Currency: org.Currency,
		Subtotal: sub, DiscountTotal: disc, TaxTotal: taxN, GrandTotal: grandN, ValidUntil: quote.ValidUntil,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Quote{}, ErrQuoteConflict
	}
	if err != nil {
		return db.Quote{}, fmt.Errorf("quotes: totals: %w", err)
	}
	return row, nil
}

func (s *Service) buildLine(ctx context.Context, c Caller, quote db.Quote, org db.Organization, in QuoteLineInput, sortOrder int32) (db.CreateQuoteLineParams, *big.Rat, *big.Rat, error) {
	typ := strings.TrimSpace(in.LineType)
	qtyS, qty, err := parseQuantity(in.Quantity)
	if err != nil {
		return db.CreateQuoteLineParams{}, nil, nil, err
	}
	defaultPrice, desc, productID, itemID, err := s.defaultLine(ctx, c, org, typ, in)
	if err != nil {
		return db.CreateQuoteLineParams{}, nil, nil, err
	}
	unitS, unit, err := parseMoney("unit_price", in.UnitPrice, defaultPrice)
	if err != nil {
		return db.CreateQuoteLineParams{}, nil, nil, err
	}
	// Echoing the default price back is not an override.
	if in.UnitPrice != nil && !canOverridePrice(c) {
		if def, derr := rat(defaultPrice); derr != nil || round2(def).Cmp(unit) != 0 {
			return db.CreateQuoteLineParams{}, nil, nil, ErrQuoteForbidden
		}
	}
	discS, discount, err := parseMoney("discount_amount", in.DiscountAmount, "0.00")
	if err != nil {
		return db.CreateQuoteLineParams{}, nil, nil, err
	}
	if in.Description != nil && strings.TrimSpace(*in.Description) != "" {
		desc = strings.TrimSpace(*in.Description)
	}
	if desc == "" || utf8.RuneCountInString(desc) > 500 {
		return db.CreateQuoteLineParams{}, nil, nil, invalid("description", "is required and must be at most 500 characters")
	}
	// Gross is rounded per line so subtotal - discount == sum(line totals).
	gross := round2(new(big.Rat).Mul(unit, qty))
	if discount.Cmp(gross) > 0 {
		return db.CreateQuoteLineParams{}, nil, nil, invalid("discount_amount", "cannot exceed the line gross amount")
	}
	totalS := new(big.Rat).Sub(gross, discount).FloatString(2)
	quantity, _ := numericValue(qtyS)
	unitPrice, _ := numericValue(unitS)
	discountAmount, _ := numericValue(discS)
	lineTotal, _ := numericValue(totalS)
	return db.CreateQuoteLineParams{
		QuoteID: quote.ID, OrganizationID: quote.OrganizationID, BrandID: quote.BrandID, LineType: typ,
		ProductID: productID, ServiceCatalogItemID: itemID, DescriptionSnapshot: desc,
		Quantity: quantity, UnitPrice: unitPrice, DiscountAmount: discountAmount, LineTotal: lineTotal, SortOrder: sortOrder,
	}, gross, discount, nil
}

func (s *Service) defaultLine(ctx context.Context, c Caller, org db.Organization, typ string, in QuoteLineInput) (string, string, pgtype.Int8, pgtype.Int8, error) {
	switch typ {
	case QuoteLineProduct:
		if in.ProductUUID == nil {
			return "", "", pgtype.Int8{}, pgtype.Int8{}, invalid("product_uuid", "is required")
		}
		viewer := pricinguc.Viewer{OrgID: org.ID, OrgType: org.Type, BrandID: org.BrandID, PurchaseRead: true, SaleRead: true, RecommendedRead: true}
		view, err := s.pricing().ProductView(ctx, viewer, *in.ProductUUID)
		if errors.Is(err, pricinguc.ErrProductNotFound) {
			return "", "", pgtype.Int8{}, pgtype.Int8{}, ErrQuoteNotFound
		}
		if err != nil {
			return "", "", pgtype.Int8{}, pgtype.Int8{}, err
		}
		price := ""
		for _, p := range view.Prices {
			if p.Currency != org.Currency {
				continue
			}
			switch {
			case p.SalePrice != nil:
				price = *p.SalePrice
			case p.PurchasePrice != nil:
				price = *p.PurchasePrice
			case p.RecommendedSalePrice != nil:
				price = *p.RecommendedSalePrice
			}
			if price != "" {
				break
			}
		}
		if price == "" {
			return "", "", pgtype.Int8{}, pgtype.Int8{}, invalid("product_uuid", "has no price in organization currency")
		}
		p, err := s.q.GetProductByUUID(ctx, db.GetProductByUUIDParams{Uuid: *in.ProductUUID, BrandID: c.Org.BrandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", pgtype.Int8{}, pgtype.Int8{}, ErrQuoteNotFound
		}
		if err != nil {
			return "", "", pgtype.Int8{}, pgtype.Int8{}, err
		}
		if !p.Active {
			return "", "", pgtype.Int8{}, pgtype.Int8{}, invalid("product_uuid", "is not active")
		}
		return price, view.Name, pgtype.Int8{Int64: p.ID, Valid: true}, pgtype.Int8{}, nil
	case QuoteLineCatalogService:
		if in.ServiceCatalogItemUUID == nil {
			return "", "", pgtype.Int8{}, pgtype.Int8{}, invalid("service_catalog_item_uuid", "is required")
		}
		item, err := s.q.GetServiceCatalogItemByUUID(ctx, db.GetServiceCatalogItemByUUIDParams{Uuid: *in.ServiceCatalogItemUUID, BrandID: c.Org.BrandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", pgtype.Int8{}, pgtype.Int8{}, ErrQuoteNotFound
		}
		if err != nil {
			return "", "", pgtype.Int8{}, pgtype.Int8{}, err
		}
		if !item.IsActive {
			return "", "", pgtype.Int8{}, pgtype.Int8{}, invalid("service_catalog_item_uuid", "is not active")
		}
		var price servicecataloguc.Price
		if org.Type == pricinguc.OrgCenter {
			// ResolvePrice only knows distributor/dealer buyers; the center
			// quotes its own catalog default.
			price = servicecataloguc.Price{Amount: moneyText(item.DefaultPrice), Currency: item.Currency, Source: "default"}
		} else {
			price, err = s.serviceCatalog().ResolvePrice(ctx, item, org)
			if errors.Is(err, servicecataloguc.ErrInvalidBuyerOrg) {
				return "", "", pgtype.Int8{}, pgtype.Int8{}, invalid("service_catalog_item_uuid", "has no price for this organization")
			}
			if err != nil {
				return "", "", pgtype.Int8{}, pgtype.Int8{}, err
			}
		}
		if price.Currency != org.Currency {
			return "", "", pgtype.Int8{}, pgtype.Int8{}, invalid("service_catalog_item_uuid", "has no price in organization currency")
		}
		return price.Amount, item.Name, pgtype.Int8{}, pgtype.Int8{Int64: item.ID, Valid: true}, nil
	default:
		return "", "", pgtype.Int8{}, pgtype.Int8{}, invalid("line_type", "must be product or catalog_service")
	}
}

func (s *Service) DecideQuote(ctx context.Context, c Caller, id uuid.UUID, status string, in QuoteDecisionInput) (Quote, error) {
	cur, err := s.quoteRow(ctx, c, id)
	if err != nil {
		return Quote{}, err
	}
	if !validQuoteStatus(status) || (status != QuoteStatusAccepted && status != QuoteStatusRejected) {
		return Quote{}, invalid("status", "must be accepted or rejected")
	}
	if cur.Status != QuoteStatusSent {
		return Quote{}, ErrQuoteConflict
	}
	reason := ""
	if in.Reason != nil {
		reason = strings.TrimSpace(*in.Reason)
	}
	if utf8.RuneCountInString(reason) > 1000 {
		return Quote{}, invalid("reason", "must be at most 1000 characters")
	}
	var row db.Quote
	err = s.inTx(ctx, func(q *db.Queries) error {
		// Row lock + re-check: accept/reject/expiry must not overwrite each other.
		locked, err := q.LockQuoteByID(ctx, cur.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrQuoteNotFound
		}
		if err != nil {
			return fmt.Errorf("quotes: lock: %w", err)
		}
		if locked.Status != QuoteStatusSent {
			return ErrQuoteConflict
		}
		row, err = q.SetQuoteStatus(ctx, db.SetQuoteStatusParams{ID: cur.ID, OrganizationID: cur.OrganizationID, Status: status})
		if err != nil {
			return fmt.Errorf("quotes: decide: %w", err)
		}
		lead, err := q.GetLeadByID(ctx, db.GetLeadByIDParams{ID: cur.LeadID, BrandID: cur.BrandID})
		if err != nil {
			return err
		}
		payload := map[string]any{"kind": "quote_" + status, "quote_uuid": row.Uuid.String(), "quote_no": row.QuoteNo}
		if reason != "" {
			payload["reason"] = reason
		}
		return addEvent(ctx, q, lead, EventMessage, payload, c.actor())
	})
	if err != nil {
		return Quote{}, err
	}
	return s.quoteOf(ctx, row)
}

func (s *Service) ExpireDueQuotesTask(ctx context.Context) error {
	today := s.nowFunc().UTC()
	// One transaction: a failed lead event rolls the status back, so the
	// retried task expires (and records) the same quotes again.
	return s.inTx(ctx, func(q *db.Queries) error {
		rows, err := q.ExpireDueQuotes(ctx, pgtype.Date{Time: today, Valid: true})
		if err != nil {
			return fmt.Errorf("quotes: expire due: %w", err)
		}
		for _, qt := range rows {
			lead, err := q.GetLeadByID(ctx, db.GetLeadByIDParams{ID: qt.LeadID, BrandID: qt.BrandID})
			if err != nil {
				return fmt.Errorf("quotes: expire lead: %w", err)
			}
			if err := addEvent(ctx, q, lead, EventMessage, map[string]any{"kind": "quote_expired", "quote_uuid": qt.Uuid.String(), "quote_no": qt.QuoteNo}, pgtype.Int8{}); err != nil {
				return fmt.Errorf("quotes: expire event: %w", err)
			}
		}
		return nil
	})
}

// QuoteOwner returns the organization and brand that own a quote visible to
// the caller (PDF renders run in the owner's scope, also for managed orgs).
func (s *Service) QuoteOwner(ctx context.Context, c Caller, id uuid.UUID) (int64, int64, error) {
	row, err := s.quoteRow(ctx, c, id)
	if err != nil {
		return 0, 0, err
	}
	return row.OrganizationID, row.BrandID, nil
}

func (s *Service) SourceType() string { return "quote" }

func (s *Service) Load(ctx context.Context, viewer docmodel.Viewer, sourceID string, locale string) (docmodel.Source, error) {
	id, err := uuid.Parse(strings.TrimSpace(sourceID))
	if err != nil {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	row, err := s.q.GetQuoteByUUID(ctx, db.GetQuoteByUUIDParams{Uuid: id, BrandID: viewer.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	if err != nil {
		return docmodel.Source{}, err
	}
	if !viewer.System && row.OrganizationID != viewer.OrganizationID {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	quote, err := s.quoteOf(ctx, row)
	if err != nil {
		return docmodel.Source{}, err
	}
	tr := docmodel.NormalizeLanguage(locale) == "tr"
	rows := make([][]string, 0, len(quote.Lines))
	for _, l := range quote.Lines {
		rows = append(rows, []string{l.Description, l.Quantity, formatMoney(l.UnitPrice, quote.Currency, tr), formatMoney(l.LineTotal, quote.Currency, tr)})
	}
	cols := []pdfrender.Column{{Label: "Service / Product"}, {Label: "Qty", Numeric: true}, {Label: "Unit price", Numeric: true}, {Label: "Amount", Numeric: true}}
	if tr {
		cols = []pdfrender.Column{{Label: "Hizmet / Ürün"}, {Label: "Adet", Numeric: true}, {Label: "Birim fiyat", Numeric: true}, {Label: "Tutar", Numeric: true}}
	}
	valid := ""
	if quote.ValidUntil != nil {
		valid = quote.ValidUntil.Format("2006-01-02")
	}
	vars := map[string]string{
		"document_number": quote.DisplayNo,
		"quote_number":    quote.DisplayNo,
		"document_date":   quote.CreatedAt.Format("2006-01-02"),
		"valid_until":     valid,
		"status":          quote.Status,
		"items_table":     pdfrender.Table(cols, rows),
		"subtotal":        formatMoney(quote.Subtotal, quote.Currency, tr),
		"discount_total":  formatMoney(quote.DiscountTotal, quote.Currency, tr),
		"tax_total":       formatMoney(quote.TaxTotal, quote.Currency, tr),
		"total_amount":    formatMoney(quote.GrandTotal, quote.Currency, tr),
		"currency":        quote.Currency,
	}
	lead, _ := s.q.GetLeadByID(ctx, db.GetLeadByIDParams{ID: row.LeadID, BrandID: row.BrandID})
	if name := textPtr(lead.CandidateContactName); name != nil {
		vars["customer_name"] = *name
	} else if company := textPtr(lead.CandidateCompanyName); company != nil {
		vars["customer_name"] = *company
	}
	if phone := textPtr(lead.CandidatePhoneE164); phone != nil {
		vars["customer_phone"] = *phone
	}
	if email := textPtr(lead.CandidateEmail); email != nil {
		vars["customer_email"] = *email
	}
	return docmodel.Source{
		OrganizationID: row.OrganizationID, BrandID: row.BrandID,
		Version: strconv.FormatInt(row.UpdatedAt.Time.UnixNano(), 10),
		Vars:    vars, Title: quote.DisplayNo,
	}, nil
}

func formatMoney(amount, currency string, tr bool) string {
	if tr {
		return html.EscapeString(amount + " " + currency)
	}
	return html.EscapeString(currency + " " + amount)
}
