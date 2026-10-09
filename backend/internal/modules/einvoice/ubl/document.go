// Package ubl turns a domain-independent invoice Document into a GİB UBL-TR
// 1.2.1 Invoice with go-ubltr, validates the XML (XSD + Schematron) and
// renders it to HTML with an XSLT. Totals, KDV, code lists, marshalling and
// validation come from go-ubltr; this package only maps and checks the
// inputs go-ubltr leaves to the caller (VKN/TCKN check digits, F5 TR/TRY
// scope, profile choice).
package ubl

import (
	"time"

	ubltr "github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/codelist"
)

// Profiles the F5 archive accepts (einvoices.profile CHECK).
const (
	ProfileEArchive   = codelist.ProfileEArsivFatura
	ProfileBasic      = codelist.ProfileTemelFatura
	ProfileCommercial = codelist.ProfileTicariFatura
)

// CountryTR / CurrencyTRY: the only buyer country and currency of F5
// (QUESTIONS S28).
const (
	CountryTR   = "TR"
	CurrencyTRY = codelist.TRY
)

// Party is a seller or buyer. Exactly one of VKN (company, 10 digits) or
// TCKN (person, 11 digits) is set; a TCKN party is written with Person.
type Party struct {
	Name string
	VKN  string
	TCKN string
	// FirstName/FamilyName are used for a TCKN party; empty → split Name.
	FirstName  string
	FamilyName string
	TaxOffice  string

	Street     string
	District   string
	City       string
	PostalZone string
	// CountryCode is ISO 3166-1 alpha-2; empty means TR.
	CountryCode string

	Email           string
	Phone           string
	Website         string
	MersisNo        string
	TradeRegistryNo string

	// EInvoiceRegistered: the buyer is a GİB e-Fatura taxpayer, so the
	// invoice goes out as e-Fatura instead of e-Arşiv.
	EInvoiceRegistered bool
}

// Line is one invoice line. Amounts are decimal strings, VAT-exclusive.
type Line struct {
	Name string
	// Quantity is a decimal ("2", "12.5").
	Quantity string
	// UnitCode is a GİB unit code (codelist.UnitC62, codelist.UnitMTR, ...);
	// empty means C62 (piece).
	UnitCode string
	// UnitPrice is the VAT-exclusive unit price.
	UnitPrice string
	// VATRate is the KDV percent ("20", "10", "1"). "0" needs a
	// TaxExemptionCode from the GİB exemption list.
	VATRate          string
	TaxExemptionCode string
	TaxExemptionText string
	// DiscountAmount (absolute) or DiscountPercent is the line iskonto; it
	// lowers the line extension before KDV.
	DiscountAmount  string
	DiscountPercent string
	DiscountReason  string
}

// Payment is the optional bank transfer block.
type Payment struct {
	IBAN    string
	DueDate time.Time
	Note    string
}

// Document is the mapper input of one invoice.
type Document struct {
	Seller Party
	Buyer  Party
	Lines  []Line
	// Currency is ISO 4217; empty means TRY.
	Currency string
	// IssueAt is converted to Türkiye time (UTC+3) for IssueDate/IssueTime.
	IssueAt time.Time
	Notes   []string
	Payment *Payment
	// PeriodStart/PeriodEnd fill InvoicePeriod (subscription periods).
	PeriodStart time.Time
	PeriodEnd   time.Time
	// OrderReference is printed as a note ("Sipariş No: ...").
	OrderReference string
}

// trTime is Türkiye time; the country has no DST since 2016, so a fixed
// zone avoids depending on tzdata in the container.
var trTime = time.FixedZone("TRT", 3*60*60)

// SelectProfile picks the UBL profile for the buyer: an e-Fatura taxpayer
// gets registeredProfile (TICARIFATURA by default, TEMELFATURA when the
// center prefers it), everybody else EARSIVFATURA.
func SelectProfile(buyer Party, registeredProfile string) string {
	if !buyer.EInvoiceRegistered {
		return ProfileEArchive
	}
	if registeredProfile == ProfileBasic {
		return ProfileBasic
	}
	return ProfileCommercial
}

// SchematronType is the go-ubltr Schematron type for a profile.
func SchematronType(profile string) string {
	if profile == ProfileEArchive {
		return ubltr.TypeEArchive
	}
	return ubltr.TypeEFatura
}
