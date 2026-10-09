package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/mapper"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/ubl"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
)

// source is one billable center sale.
type source struct {
	Type         string
	UUID         uuid.UUID
	No           string
	Buyer        db.Organization
	BuyerCountry string
	Currency     string
	RateSnapshot []byte

	order      db.Order
	orderLines []mapper.OrderLine
	period     db.GetEinvoiceSubscriptionPeriodRow
}

// loadSource reads a billable sale of the caller's center. An unknown
// source or one of another seller is ErrNotFound; one that is not billable
// (yet) is ErrSourceNotBillable.
func (s *Service) loadSource(ctx context.Context, q *db.Queries, c Caller, sourceType string, id uuid.UUID) (source, error) {
	src := source{Type: sourceType, UUID: id}
	var buyerID int64
	switch sourceType {
	case SourceOrder:
		o, err := q.GetOrderByUUID(ctx, db.GetOrderByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && o.SellerOrgID != c.Org.InternalID) {
			return source{}, ErrNotFound
		}
		if err != nil {
			return source{}, err
		}
		if o.Status != "received" {
			return source{}, ErrSourceNotBillable
		}
		rows, err := q.ListEinvoiceOrderLines(ctx, o.ID)
		if err != nil {
			return source{}, err
		}
		for _, r := range rows {
			src.orderLines = append(src.orderLines, mapper.OrderLine{Item: r.OrderItem, Product: r.Product})
		}
		src.order, src.No, src.Currency, src.RateSnapshot = o, o.OrderNo, o.Currency, o.RateSnapshot
		buyerID = o.BuyerOrgID
	case SourceSubscription:
		p, err := q.GetEinvoiceSubscriptionPeriod(ctx, db.GetEinvoiceSubscriptionPeriodParams{Uuid: id, BrandID: c.Org.BrandID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && p.ServiceSubscription.SellerOrgID != c.Org.InternalID) {
			return source{}, ErrNotFound
		}
		if err != nil {
			return source{}, err
		}
		if !p.ServiceSubscriptionPeriod.PostedAt.Valid {
			return source{}, ErrSourceNotBillable
		}
		src.period, src.No = p, p.ServiceCatalogItem.Name
		src.Currency, src.RateSnapshot = p.ServiceSubscription.Currency, p.ServiceSubscription.RateSnapshot
		buyerID = p.ServiceSubscription.OrganizationID
	default:
		return source{}, invalid("source_type", "must be one of "+strings.Join(SourceTypes, ", "))
	}
	buyer, err := q.GetOrganizationByID(ctx, buyerID)
	if err != nil {
		return source{}, err
	}
	want := map[string]string{SourceOrder: "distributor", SourceSubscription: "dealer"}[sourceType]
	if buyer.Type != want {
		return source{}, ErrSourceNotBillable
	}
	country, err := q.GetOrganizationCountryISO2(ctx, buyer.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return source{}, err
	}
	src.Buyer, src.BuyerCountry = buyer, country
	return src, nil
}

// checkBuyerProfile lists the invoice profile fields the buyer misses. The
// UBL party needs a tax id (VKN with its tax office, or TCKN) and a postal
// address with city and district.
func checkBuyerProfile(o db.Organization) error {
	var missing []string
	vkn, tckn := strings.TrimSpace(o.InvoiceVkn.String), strings.TrimSpace(o.InvoiceTckn.String)
	if vkn == "" && tckn == "" {
		missing = append(missing, "invoice_vkn")
	}
	if vkn != "" && strings.TrimSpace(o.InvoiceTaxOffice.String) == "" {
		missing = append(missing, "invoice_tax_office")
	}
	if strings.TrimSpace(o.Address) == "" {
		missing = append(missing, "address")
	}
	if strings.TrimSpace(o.City) == "" {
		missing = append(missing, "city")
	}
	if strings.TrimSpace(o.District) == "" {
		missing = append(missing, "district")
	}
	if len(missing) > 0 {
		return &ProfileError{OrganizationUUID: o.Uuid, Fields: missing}
	}
	return nil
}

func (s *Service) vatDefault(ctx context.Context) string {
	if s.sys == nil {
		return ""
	}
	if v := s.sys.Int(ctx, sysconfig.KeyEinvoiceDefaultVATRate); v > 0 {
		return strconv.FormatInt(v, 10)
	}
	return ""
}

// document maps the source with the current seller settings and buyer
// profile; profile is the UBL profile the buyer gets.
func (s *Service) document(ctx context.Context, src source, st db.EinvoiceSetting, issueAt time.Time) (ubl.Document, string, error) {
	if err := checkBuyerProfile(src.Buyer); err != nil {
		return ubl.Document{}, "", err
	}
	vat := s.vatDefault(ctx)
	var (
		doc ubl.Document
		err error
	)
	switch src.Type {
	case SourceOrder:
		doc, err = mapper.FromOrder(mapper.OrderInput{
			Settings: st, Order: src.order, Lines: src.orderLines, Buyer: src.Buyer,
			BuyerCountry: src.BuyerCountry, VAT: mapper.VATRates{Default: vat}, IssueAt: issueAt,
		})
	case SourceSubscription:
		doc, err = mapper.FromSubscriptionPeriod(mapper.SubscriptionInput{
			Settings: st, Period: src.period.ServiceSubscriptionPeriod, Subscription: src.period.ServiceSubscription,
			Item: src.period.ServiceCatalogItem, Buyer: src.Buyer, BuyerCountry: src.BuyerCountry,
			VATRate: vat, IssueAt: issueAt,
		})
	}
	if err != nil {
		return ubl.Document{}, "", err
	}
	if len(doc.Lines) == 0 {
		return ubl.Document{}, "", ErrSourceNotBillable
	}
	return doc, ubl.SelectProfile(doc.Buyer, ""), nil
}

// compose builds the invoice with number and ETTN and, with a stylesheet,
// embeds it and marshals the XML.
func compose(doc ubl.Document, profile, number string, ettn uuid.UUID, xslt []byte) (*invoice.Invoice, []byte, error) {
	inv, err := ubl.Build(doc, profile, number, ettn.String())
	if err != nil {
		return nil, nil, err
	}
	if xslt == nil {
		return inv, nil, nil
	}
	if err := ubl.AttachXSLT(inv, xslt); err != nil {
		return nil, nil, err
	}
	out, err := ubl.Marshal(inv)
	if err != nil {
		return nil, nil, err
	}
	return inv, out, nil
}

// stylesheet is the center's uploaded XSLT, or the GİB default.
func (s *Service) stylesheet(ctx context.Context, st *db.EinvoiceSetting) []byte {
	if st == nil || !st.XsltStorageKey.Valid || s.store == nil {
		return ubl.DefaultXSLT()
	}
	rc, _, err := s.store.Download(ctx, st.XsltStorageKey.String)
	if err != nil {
		s.log.Warn("einvoice_xslt_missing", "key", st.XsltStorageKey.String, "error", err)
		return ubl.DefaultXSLT()
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, MaxXSLTBytes+1))
	if err != nil || len(data) == 0 {
		return ubl.DefaultXSLT()
	}
	return ubl.Stylesheet(data)
}

// snapshot is the frozen JSON view of an invoice (einvoices columns).
type snapshot struct {
	Buyer, Seller, Lines, TaxBreakdown []byte
	LineExtension, TaxExclusive        pgtype.Numeric
	TaxTotal, Payable                  pgtype.Numeric
}

// PartyJSON is the stored buyer / seller of an invoice.
type PartyJSON struct {
	Name               string `json:"name"`
	VKN                string `json:"vkn,omitempty"`
	TCKN               string `json:"tckn,omitempty"`
	TaxOffice          string `json:"tax_office,omitempty"`
	Street             string `json:"street,omitempty"`
	District           string `json:"district,omitempty"`
	City               string `json:"city,omitempty"`
	CountryCode        string `json:"country_code,omitempty"`
	Email              string `json:"email,omitempty"`
	Phone              string `json:"phone,omitempty"`
	EInvoiceRegistered bool   `json:"einvoice_registered"`
}

// LineJSON is one stored invoice line.
type LineJSON struct {
	Name          string `json:"name"`
	Quantity      string `json:"quantity"`
	UnitCode      string `json:"unit_code"`
	UnitPrice     string `json:"unit_price"`
	VATRate       string `json:"vat_rate"`
	LineExtension string `json:"line_extension"`
}

// TaxJSON is one KDV rate of the breakdown.
type TaxJSON struct {
	Percent       string `json:"percent"`
	TaxableAmount string `json:"taxable_amount"`
	TaxAmount     string `json:"tax_amount"`
}

func party(p ubl.Party) PartyJSON {
	return PartyJSON{
		Name: p.Name, VKN: p.VKN, TCKN: p.TCKN, TaxOffice: p.TaxOffice, Street: p.Street,
		District: p.District, City: p.City, CountryCode: p.CountryCode, Email: p.Email, Phone: p.Phone,
		EInvoiceRegistered: p.EInvoiceRegistered,
	}
}

func takeSnapshot(doc ubl.Document, inv *invoice.Invoice) (snapshot, error) {
	var sn snapshot
	var err error
	if sn.Buyer, err = json.Marshal(party(doc.Buyer)); err != nil {
		return snapshot{}, err
	}
	if sn.Seller, err = json.Marshal(party(doc.Seller)); err != nil {
		return snapshot{}, err
	}
	lines := make([]LineJSON, 0, len(doc.Lines))
	for i, l := range doc.Lines {
		ext := ""
		if i < len(inv.InvoiceLine) {
			ext = inv.InvoiceLine[i].LineExtensionAmount.Value
		}
		lines = append(lines, LineJSON{
			Name: l.Name, Quantity: l.Quantity, UnitCode: l.UnitCode, UnitPrice: l.UnitPrice,
			VATRate: l.VATRate, LineExtension: ext,
		})
	}
	if sn.Lines, err = json.Marshal(lines); err != nil {
		return snapshot{}, err
	}
	taxes := []TaxJSON{}
	taxTotal := "0"
	if len(inv.TaxTotal) > 0 {
		taxTotal = inv.TaxTotal[0].TaxAmount.Value
		for _, st := range inv.TaxTotal[0].TaxSubtotal {
			t := TaxJSON{TaxAmount: st.TaxAmount.Value}
			if st.Percent != nil {
				t.Percent = st.Percent.Value
			}
			if st.TaxableAmount != nil {
				t.TaxableAmount = st.TaxableAmount.Value
			}
			taxes = append(taxes, t)
		}
	}
	if sn.TaxBreakdown, err = json.Marshal(taxes); err != nil {
		return snapshot{}, err
	}
	m := inv.LegalMonetaryTotal
	for _, f := range []struct {
		dst *pgtype.Numeric
		v   string
	}{
		{&sn.LineExtension, m.LineExtensionAmount.Value}, {&sn.TaxExclusive, m.TaxExclusiveAmount.Value},
		{&sn.TaxTotal, taxTotal}, {&sn.Payable, m.PayableAmount.Value},
	} {
		if err := f.dst.Scan(strings.TrimSpace(f.v)); err != nil {
			return snapshot{}, fmt.Errorf("einvoice: amount %q: %w", f.v, err)
		}
	}
	return sn, nil
}

func date(t time.Time) pgtype.Date {
	t = t.In(trTime)
	return pgtype.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}
