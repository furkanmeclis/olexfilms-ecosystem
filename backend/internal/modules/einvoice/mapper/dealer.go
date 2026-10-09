package mapper

import (
	"time"

	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/shopspring/decimal"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/ubl"
)

// The dealer side is backend only (design §5 "E-fatura altyapısı"): no UI,
// no archive. An integrator issue will number, validate and send these.

// DealerSeller is the selling dealer: its organization invoice profile.
type DealerSeller struct {
	Org     db.Organization
	Country string
	// IBAN is printed as the payment block when set.
	IBAN string
}

// ProductSaleLine is one product_sale_lines row with its product.
type ProductSaleLine struct {
	Line    db.ProductSaleLine
	Product db.Product
}

// ProductSaleInput is a dealer's quick product sale to an end customer.
// Buyer is the customer as a party: users carry no tax identity, so the
// caller fills it (an unknown consumer is ubl.FinalConsumerTCKN).
type ProductSaleInput struct {
	Seller DealerSeller
	Sale   db.ProductSale
	Lines  []ProductSaleLine
	Buyer  ubl.Party
	VAT    VATRates
	// VATIncluded: the dealer's sale prices include KDV (retail); the unit
	// price is reduced to its VAT-exclusive amount before go-ubltr adds
	// KDV, so the payable can differ from the sale total by rounding.
	VATIncluded bool
	IssueAt     time.Time
}

// FromProductSale maps a dealer product sale.
func FromProductSale(in ProductSaleInput) (ubl.Document, error) {
	seller, err := OrganizationParty(in.Seller.Org, in.Seller.Country)
	if err != nil {
		return ubl.Document{}, err
	}
	seller.EInvoiceRegistered = false
	doc := ubl.Document{
		Seller:   seller,
		Buyer:    in.Buyer,
		Currency: in.Sale.Currency,
		IssueAt:  firstTime(in.IssueAt, in.Sale.SoldAt.Time),
		Notes:    notes(in.Sale.Note),
		Payment:  dealerPayment(in.Seller),
	}
	if err := ubl.CheckScope(doc); err != nil {
		return ubl.Document{}, err
	}
	for _, l := range in.Lines {
		rate := in.VAT.For(l.Product.ID, l.Product.CategoryID)
		price, err := netPrice(NumericString(l.Line.UnitPrice), rate, in.VATIncluded)
		if err != nil {
			return ubl.Document{}, err
		}
		doc.Lines = append(doc.Lines, ubl.Line{
			Name:      lineName(l.Product),
			Quantity:  NumericString(l.Line.Quantity),
			UnitCode:  unitFor(l.Product.UnitType),
			UnitPrice: price,
			VATRate:   rate,
		})
	}
	return doc, nil
}

// ServiceIncomeInput is the income of one completed dealer service
// (services.income_amount).
type ServiceIncomeInput struct {
	Seller  DealerSeller
	Service db.Service
	Buyer   ubl.Party
	// Description names the work ("Seramik kaplama — 34 ABC 123"); empty
	// means "Hizmet <service_no>".
	Description string
	Currency    string
	VATRate     string
	VATIncluded bool
	IssueAt     time.Time
}

// FromServiceIncome maps a service income as a single service line.
func FromServiceIncome(in ServiceIncomeInput) (ubl.Document, error) {
	seller, err := OrganizationParty(in.Seller.Org, in.Seller.Country)
	if err != nil {
		return ubl.Document{}, err
	}
	seller.EInvoiceRegistered = false
	currency := firstNonEmpty(in.Currency, in.Seller.Org.Currency)
	rate := VATRates{Default: in.VATRate}.fallback()
	doc := ubl.Document{
		Seller:   seller,
		Buyer:    in.Buyer,
		Currency: currency,
		IssueAt:  firstTime(in.IssueAt, in.Service.CompletedAt.Time),
		Payment:  dealerPayment(in.Seller),
	}
	if err := ubl.CheckScope(doc); err != nil {
		return ubl.Document{}, err
	}
	price, err := netPrice(NumericString(in.Service.IncomeAmount), rate, in.VATIncluded)
	if err != nil {
		return ubl.Document{}, err
	}
	doc.Lines = []ubl.Line{{
		Name:      firstNonEmpty(in.Description, "Hizmet "+in.Service.ServiceNo),
		Quantity:  "1",
		UnitCode:  codelist.UnitC62,
		UnitPrice: price,
		VATRate:   rate,
	}}
	return doc, nil
}

// netPrice turns a KDV-inclusive unit price into the VAT-exclusive one
// (4 decimals) when included is set.
func netPrice(price, rate string, included bool) (string, error) {
	if !included || price == "" {
		return price, nil
	}
	p, err := decimal.NewFromString(price)
	if err != nil {
		return "", err
	}
	r, err := decimal.NewFromString(rate)
	if err != nil {
		// Let ubl.Build report the rate as EINVOICE_INVALID_VAT_RATE.
		return price, nil
	}
	hundred := decimal.NewFromInt(100)
	return p.Mul(hundred).DivRound(hundred.Add(r), 4).String(), nil
}

func dealerPayment(s DealerSeller) *ubl.Payment {
	if s.IBAN == "" {
		return nil
	}
	return &ubl.Payment{IBAN: s.IBAN}
}

func firstTime(ts ...time.Time) time.Time {
	for _, t := range ts {
		if !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}
