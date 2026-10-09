package mapper

import (
	"fmt"
	"time"

	"github.com/furkanmeclis/go-ubltr/codelist"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/ubl"
)

// OrderLine is one order_items row with its product.
type OrderLine struct {
	Item    db.OrderItem
	Product db.Product
}

// OrderInput is a center → distributor order (QUESTIONS S27: delivered
// orders of the center).
type OrderInput struct {
	Settings db.EinvoiceSetting
	Order    db.Order
	Lines    []OrderLine
	Buyer    db.Organization
	// BuyerCountry is the buyer organization's countries.iso2.
	BuyerCountry string
	VAT          VATRates
	IssueAt      time.Time
	// PaymentDue is the optional due date printed with the IBAN.
	PaymentDue time.Time
}

// FromOrder maps a center sales order. Lines are the order_items at their
// sale price (VAT-exclusive, as orders.subtotal); quantity is the piece
// count or the metres of a roll product. KDV comes from VAT.
func FromOrder(in OrderInput) (ubl.Document, error) {
	buyer, err := OrganizationParty(in.Buyer, in.BuyerCountry)
	if err != nil {
		return ubl.Document{}, err
	}
	doc := ubl.Document{
		Seller:         SellerParty(in.Settings),
		Buyer:          buyer,
		Currency:       in.Order.Currency,
		IssueAt:        in.IssueAt,
		Notes:          notes(text(in.Settings.DefaultNote)),
		Payment:        SellerPayment(in.Settings, in.PaymentDue),
		OrderReference: in.Order.OrderNo,
	}
	if err := ubl.CheckScope(doc); err != nil {
		return ubl.Document{}, err
	}
	for _, l := range in.Lines {
		qty := NumericString(l.Item.Meters)
		unit := codelist.UnitMTR
		if l.Item.Quantity.Valid {
			qty = fmt.Sprint(l.Item.Quantity.Int32)
			unit = unitFor(l.Product.UnitType)
			if unit == codelist.UnitMTR {
				// A roll ordered by count is a whole roll.
				unit = codelist.UnitC62
			}
		}
		doc.Lines = append(doc.Lines, ubl.Line{
			Name:      lineName(l.Product),
			Quantity:  qty,
			UnitCode:  unit,
			UnitPrice: NumericString(l.Item.UnitPrice),
			VATRate:   in.VAT.For(l.Product.ID, l.Product.CategoryID),
		})
	}
	return doc, nil
}

// SubscriptionInput is one posted service catalog subscription period of
// the center billed to a dealer.
type SubscriptionInput struct {
	Settings     db.EinvoiceSetting
	Period       db.ServiceSubscriptionPeriod
	Subscription db.ServiceSubscription
	Item         db.ServiceCatalogItem
	Buyer        db.Organization
	BuyerCountry string
	// VATRate is the KDV percent of the service (empty: DefaultVATRate).
	VATRate    string
	IssueAt    time.Time
	PaymentDue time.Time
}

// FromSubscriptionPeriod maps one subscription period: a single service
// line at the subscription price (VAT-exclusive) and the period as
// InvoicePeriod.
func FromSubscriptionPeriod(in SubscriptionInput) (ubl.Document, error) {
	buyer, err := OrganizationParty(in.Buyer, in.BuyerCountry)
	if err != nil {
		return ubl.Document{}, err
	}
	start, end := in.Period.PeriodStart.Time, in.Period.PeriodEnd.Time
	doc := ubl.Document{
		Seller:      SellerParty(in.Settings),
		Buyer:       buyer,
		Currency:    in.Subscription.Currency,
		IssueAt:     in.IssueAt,
		Notes:       notes(text(in.Settings.DefaultNote)),
		Payment:     SellerPayment(in.Settings, in.PaymentDue),
		PeriodStart: start,
		PeriodEnd:   end,
		Lines: []ubl.Line{{
			Name:      fmt.Sprintf("%s (%s – %s)", in.Item.Name, start.Format("02.01.2006"), end.Format("02.01.2006")),
			Quantity:  "1",
			UnitCode:  codelist.UnitC62,
			UnitPrice: NumericString(in.Subscription.Price),
			VATRate:   VATRates{Default: in.VATRate}.fallback(),
		}},
	}
	if err := ubl.CheckScope(doc); err != nil {
		return ubl.Document{}, err
	}
	return doc, nil
}

func lineName(p db.Product) string {
	if p.Sku == "" {
		return p.Name
	}
	return p.Name + " (" + p.Sku + ")"
}
