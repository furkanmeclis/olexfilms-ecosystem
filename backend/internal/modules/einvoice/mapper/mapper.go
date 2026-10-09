// Package mapper turns olexfilms records into ubl.Document values: the
// center's sales (order → distributor, service catalog subscription period
// → dealer) and, backend only, a dealer's product sales and service income
// for a future integrator. Mappers are pure: callers load the rows (sqlc)
// and hand them in; nothing here reads the database or numbers invoices.
package mapper

import (
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/ubl"
)

// ErrUnsupportedBuyer is ubl.ErrUnsupportedBuyer: F5 invoices only TRY to
// buyers in Türkiye (QUESTIONS S28).
var ErrUnsupportedBuyer = ubl.ErrUnsupportedBuyer

// DefaultVATRate is the KDV percent used when neither the product nor its
// category has one (sysconfig einvoice.default_vat_rate default).
const DefaultVATRate = "20"

// VATRates resolves the KDV percent of a line: product first, then the
// product's category, then Default (sysconfig einvoice.default_vat_rate;
// empty means DefaultVATRate).
type VATRates struct {
	ByProduct  map[int64]string
	ByCategory map[int64]string
	Default    string
}

// For returns the KDV percent of a product.
func (v VATRates) For(productID, categoryID int64) string {
	if r := strings.TrimSpace(v.ByProduct[productID]); r != "" {
		return r
	}
	if r := strings.TrimSpace(v.ByCategory[categoryID]); r != "" {
		return r
	}
	return v.fallback()
}

func (v VATRates) fallback() string {
	if r := strings.TrimSpace(v.Default); r != "" {
		return r
	}
	return DefaultVATRate
}

// SellerParty maps the center's einvoice_settings row to the seller.
func SellerParty(s db.EinvoiceSetting) ubl.Party {
	return ubl.Party{
		Name:            s.LegalName,
		VKN:             s.Vkn,
		TaxOffice:       s.TaxOffice,
		Street:          s.Address,
		District:        s.District,
		City:            s.City,
		CountryCode:     s.Country,
		Email:           text(s.Email),
		Phone:           text(s.Phone),
		Website:         text(s.Website),
		MersisNo:        text(s.MersisNo),
		TradeRegistryNo: text(s.TradeRegistryNo),
	}
}

// SellerPayment is the bank transfer block of the center (nil without an
// IBAN); due is optional.
func SellerPayment(s db.EinvoiceSetting, due time.Time) *ubl.Payment {
	iban := text(s.Iban)
	if iban == "" {
		return nil
	}
	return &ubl.Payment{IBAN: iban, DueDate: due}
}

// OrganizationParty maps an organization's invoice profile (F5-08a columns)
// to a party. countryISO2 is the organization's country (countries.iso2;
// empty means TR); a foreign organization is ErrUnsupportedBuyer. A missing
// or broken VKN/TCKN is left to ubl.Build (422 EINVOICE_INVALID_TAX_ID).
func OrganizationParty(o db.Organization, countryISO2 string) (ubl.Party, error) {
	p := ubl.Party{
		Name:               firstNonEmpty(text(o.InvoiceLegalName), o.Name),
		VKN:                text(o.InvoiceVkn),
		TCKN:               text(o.InvoiceTckn),
		TaxOffice:          text(o.InvoiceTaxOffice),
		Street:             o.Address,
		District:           o.District,
		City:               o.City,
		CountryCode:        strings.ToUpper(strings.TrimSpace(countryISO2)),
		Email:              firstNonEmpty(text(o.InvoiceEmail), o.Email),
		Phone:              o.Phone,
		Website:            o.Website,
		EInvoiceRegistered: o.EinvoiceRegistered,
	}
	if err := ubl.CheckScope(ubl.Document{Buyer: p}); err != nil {
		return ubl.Party{}, err
	}
	return p, nil
}

// NumericString renders a NUMERIC exactly ("12.50" stays "12.5"); NULL is
// "".
func NumericString(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil || n.NaN {
		return ""
	}
	return decimal.NewFromBigInt(n.Int, n.Exp).String()
}

// unitFor is the GİB unit of a product: roll products are sold by the
// metre, everything else by the piece.
func unitFor(unitType string) string {
	if unitType == "roll_meter" {
		return codelist.UnitMTR
	}
	return codelist.UnitC62
}

func text(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return strings.TrimSpace(t.String)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func notes(vals ...string) []string {
	out := []string{}
	for _, n := range vals {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}
