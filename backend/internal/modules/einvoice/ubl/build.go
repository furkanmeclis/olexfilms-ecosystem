package ubl

import (
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/go-ubltr/builder"
	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
	"github.com/shopspring/decimal"
)

// Build maps doc to a UBL-TR Invoice with the go-ubltr builder. profile is
// one of the Profile* constants (see SelectProfile); number is the 16
// character GİB invoice number and uuid the ETTN. Totals and KDV are computed
// by the builder; input errors are 422 *Error values, builder errors wrap
// ErrInvalidDocument.
func Build(doc Document, profile, number, uuid string) (*invoice.Invoice, error) {
	if err := checkDocument(doc, profile); err != nil {
		return nil, err
	}
	currency := docCurrency(doc)
	issue := doc.IssueAt.In(trTime)

	b := builder.NewInvoiceBuilder().
		Profile(profile).
		Type(codelist.InvoiceTypeSatis).
		ID(strings.TrimSpace(number)).
		UUID(strings.TrimSpace(uuid)).
		IssueDate(issue.Format(dateLayout)).
		IssueTime(issue.Format("15:04:05")).
		Currency(currency).
		SupplierBuilder(partyBuilder(doc.Seller)).
		CustomerBuilder(partyBuilder(doc.Buyer))

	for _, n := range doc.Notes {
		if n = strings.TrimSpace(n); n != "" {
			b.Note(n)
		}
	}
	if ref := strings.TrimSpace(doc.OrderReference); ref != "" {
		b.Note("Sipariş No: " + ref)
	}
	if !doc.PeriodStart.IsZero() && !doc.PeriodEnd.IsZero() {
		b.Period(doc.PeriodStart.Format(dateLayout), doc.PeriodEnd.Format(dateLayout))
	}
	for i, l := range doc.Lines {
		lb, err := lineBuilder(i, l, currency)
		if err != nil {
			return nil, err
		}
		b.Line(lb)
	}
	if p := doc.Payment; p != nil && (p.IBAN != "" || !p.DueDate.IsZero()) {
		pb := builder.NewPaymentBuilder().Code(codelist.PaymentMeansCreditTransfer)
		if p.IBAN != "" {
			pb.IBAN(strings.ReplaceAll(p.IBAN, " ", ""))
		}
		if !p.DueDate.IsZero() {
			pb.DueDate(p.DueDate.Format(dateLayout))
		}
		if p.Note != "" {
			pb.Note(p.Note)
		}
		b.Payment(pb)
	}

	inv, err := b.Build()
	if err != nil {
		var be *builder.BuilderError
		if errors.As(err, &be) {
			return nil, invalidDocument(be.Field, be.Msg)
		}
		return nil, invalidDocument("", err.Error())
	}
	return inv, nil
}

const dateLayout = "2006-01-02"

func docCurrency(doc Document) string {
	if c := strings.ToUpper(strings.TrimSpace(doc.Currency)); c != "" {
		return c
	}
	return CurrencyTRY
}

// CheckScope enforces the F5 scope (QUESTIONS S28): TRY invoices between
// parties in Türkiye. Anything else is ErrUnsupportedBuyer.
func CheckScope(doc Document) error {
	if docCurrency(doc) != CurrencyTRY {
		return unsupportedBuyer("currency", "only TRY invoices are supported")
	}
	if !isTR(doc.Seller.CountryCode) {
		return unsupportedBuyer("seller.country_code", "only sellers in Türkiye are supported")
	}
	if !isTR(doc.Buyer.CountryCode) {
		return unsupportedBuyer("buyer.country_code", "only buyers in Türkiye are supported")
	}
	return nil
}

// checkDocument covers what go-ubltr leaves to the caller: the F5 TR/TRY
// scope, VKN/TCKN check digits and the number/profile inputs.
func checkDocument(doc Document, profile string) error {
	if profile != ProfileEArchive && profile != ProfileBasic && profile != ProfileCommercial {
		return invalidDocument("profile", "must be EARSIVFATURA, TEMELFATURA or TICARIFATURA")
	}
	if err := CheckScope(doc); err != nil {
		return err
	}
	if err := checkParty("seller", doc.Seller); err != nil {
		return err
	}
	if doc.Seller.VKN == "" {
		return taxIDError("seller.vkn", "the seller needs a VKN")
	}
	if err := checkParty("buyer", doc.Buyer); err != nil {
		return err
	}
	if doc.Buyer.EInvoiceRegistered && profile == ProfileEArchive {
		return invalidDocument("profile", "an e-Fatura registered buyer cannot get an e-Arşiv invoice")
	}
	if !doc.Buyer.EInvoiceRegistered && profile != ProfileEArchive {
		return invalidDocument("profile", "a buyer outside e-Fatura gets an e-Arşiv invoice")
	}
	if doc.IssueAt.IsZero() {
		return invalidDocument("issue_at", "is required")
	}
	if len(doc.Lines) == 0 {
		return invalidDocument("lines", "must contain at least one line")
	}
	return nil
}

func isTR(code string) bool {
	c := strings.ToUpper(strings.TrimSpace(code))
	return c == "" || c == CountryTR
}

func checkParty(field string, p Party) error {
	if strings.TrimSpace(p.Name) == "" && p.TCKN == "" {
		return invalidDocument(field+".name", "is required")
	}
	switch {
	case p.VKN != "" && p.TCKN != "":
		return taxIDError(field, "set either a VKN or a TCKN, not both")
	case p.VKN != "":
		if len(p.VKN) != 10 || !ValidVKN(p.VKN) {
			return taxIDError(field+".vkn", "VKN must be 10 digits with a valid check digit")
		}
	case p.TCKN != "":
		if len(p.TCKN) != 11 || !ValidTCKN(p.TCKN) {
			return taxIDError(field+".tckn", "TCKN must be 11 digits with valid check digits")
		}
	default:
		return taxIDError(field, "a VKN or a TCKN is required")
	}
	if strings.TrimSpace(p.City) == "" {
		return invalidDocument(field+".city", "is required")
	}
	if strings.TrimSpace(p.District) == "" {
		return invalidDocument(field+".district", "is required")
	}
	return nil
}

func partyBuilder(p Party) *builder.PartyBuilder {
	addr := builder.NewAddressBuilder().
		District(strings.TrimSpace(p.District)).
		City(strings.TrimSpace(p.City)).
		Country("Türkiye").
		CountryCode(CountryTR)
	if s := strings.TrimSpace(p.Street); s != "" {
		addr.Street(s)
	}
	if s := strings.TrimSpace(p.PostalZone); s != "" {
		addr.PostalZone(s)
	}
	pb := builder.NewPartyBuilder().AddressBuilder(addr)
	if p.VKN != "" {
		pb.VKN(p.VKN).Name(strings.TrimSpace(p.Name))
	} else {
		first, family := personName(p)
		pb.TCKN(p.TCKN).Person(first, family)
		if n := strings.TrimSpace(p.Name); n != "" {
			pb.Name(n)
		}
	}
	if s := strings.TrimSpace(p.MersisNo); s != "" {
		pb.Identification(codelist.SchemeMERSISNO, s)
	}
	if s := strings.TrimSpace(p.TradeRegistryNo); s != "" {
		pb.Identification(codelist.SchemeTICARETSICILNO, s)
	}
	if s := strings.TrimSpace(p.TaxOffice); s != "" {
		pb.TaxOffice(s)
	}
	if s := strings.TrimSpace(p.Website); s != "" {
		pb.Website(s)
	}
	if s := strings.TrimSpace(p.Email); s != "" {
		pb.Email(s)
	}
	if s := strings.TrimSpace(p.Phone); s != "" {
		pb.Phone(s)
	}
	return pb
}

// personName returns FirstName/FamilyName, or splits Name at its last space
// ("Ayşe Nur Yılmaz" → "Ayşe Nur", "Yılmaz").
func personName(p Party) (string, string) {
	first, family := strings.TrimSpace(p.FirstName), strings.TrimSpace(p.FamilyName)
	if first != "" && family != "" {
		return first, family
	}
	name := strings.Join(strings.Fields(p.Name), " ")
	if i := strings.LastIndex(name, " "); i > 0 {
		return name[:i], name[i+1:]
	}
	return name, name
}

func lineBuilder(i int, l Line, currency string) (*builder.InvoiceLineBuilder, error) {
	field := fmt.Sprintf("lines[%d]", i)
	if strings.TrimSpace(l.Name) == "" {
		return nil, invalidDocument(field+".name", "is required")
	}
	unit := strings.TrimSpace(l.UnitCode)
	if unit == "" {
		unit = codelist.UnitC62
	}
	if !codelist.ValidUnit(unit) {
		return nil, invalidDocument(field+".unit_code", "is not a GİB unit code")
	}
	if !positive(l.Quantity) {
		return nil, invalidDocument(field+".quantity", "must be a positive decimal")
	}
	if !nonNegative(l.UnitPrice) {
		return nil, invalidDocument(field+".unit_price", "must be a non-negative decimal")
	}
	qty, err := builder.QuantityIn(unit, l.Quantity)
	if err != nil {
		return nil, invalidDocument(field+".quantity", err.Error())
	}
	price, err := builder.MoneyIn(currency, l.UnitPrice)
	if err != nil {
		return nil, invalidDocument(field+".unit_price", err.Error())
	}
	tax, err := taxFor(field, l)
	if err != nil {
		return nil, err
	}
	lb := builder.NewInvoiceLineBuilder().Name(strings.TrimSpace(l.Name)).Quantity(qty).Price(price).Tax(tax)
	reason := strings.TrimSpace(l.DiscountReason)
	if reason == "" {
		reason = "İskonto"
	}
	switch {
	case l.DiscountAmount != "" && l.DiscountPercent != "":
		return nil, invalidDocument(field+".discount", "set either an amount or a percent")
	case l.DiscountAmount != "":
		if !nonNegative(l.DiscountAmount) {
			return nil, invalidDocument(field+".discount_amount", "must be a non-negative decimal")
		}
		d, err := builder.MoneyIn(currency, l.DiscountAmount)
		if err != nil {
			return nil, invalidDocument(field+".discount_amount", err.Error())
		}
		if !d.IsZero() {
			lb.Allowance(d, reason)
		}
	case l.DiscountPercent != "":
		pct, err := decimal.NewFromString(l.DiscountPercent)
		if err != nil || pct.IsNegative() || pct.GreaterThan(decimal.NewFromInt(100)) {
			return nil, invalidDocument(field+".discount_percent", "must be between 0 and 100")
		}
		if !pct.IsZero() {
			lb.AllowancePercent(l.DiscountPercent, reason)
		}
	}
	return lb, nil
}

func taxFor(field string, l Line) (*builder.TaxBuilder, error) {
	rate, err := decimal.NewFromString(strings.TrimSpace(l.VATRate))
	if err != nil || rate.IsNegative() || rate.GreaterThan(decimal.NewFromInt(100)) {
		return nil, invalidVATRate(field+".vat_rate", "must be a percent between 0 and 100")
	}
	tb := builder.NewTaxBuilder().KDV(rate.String())
	if rate.IsZero() {
		code := strings.TrimSpace(l.TaxExemptionCode)
		if code == "" || !codelist.ValidExemption(code) {
			return nil, invalidVATRate(field+".tax_exemption_code", "a 0% KDV line needs a GİB exemption code")
		}
		tb.Exemption(code, strings.TrimSpace(l.TaxExemptionText))
	}
	return tb, nil
}

func positive(s string) bool {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	return err == nil && d.IsPositive()
}

func nonNegative(s string) bool {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	return err == nil && !d.IsNegative()
}
