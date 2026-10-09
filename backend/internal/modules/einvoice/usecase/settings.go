package usecase

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // content address of the stylesheet (einvoice_settings.xslt_sha1), not a security hash
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/mapper"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/einvoice/ubl"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// SettingsView is the center's seller profile, series and stylesheet.
type SettingsView struct {
	// Configured is false until the seller profile is saved; archiving is
	// closed until then (QUESTIONS S31).
	Configured      bool       `json:"configured"`
	VKN             string     `json:"vkn"`
	TaxOffice       string     `json:"tax_office"`
	LegalName       string     `json:"legal_name"`
	Address         string     `json:"address"`
	City            string     `json:"city"`
	District        string     `json:"district"`
	Country         string     `json:"country"`
	IBAN            *string    `json:"iban"`
	Email           *string    `json:"email"`
	Phone           *string    `json:"phone"`
	Website         *string    `json:"website"`
	TradeRegistryNo *string    `json:"trade_registry_no"`
	MersisNo        *string    `json:"mersis_no"`
	DefaultNote     *string    `json:"default_note"`
	EArchiveSeries  string     `json:"earchive_series"`
	EFaturaSeries   string     `json:"efatura_series"`
	PDFEnabled      bool       `json:"pdf_enabled"`
	CustomXSLT      bool       `json:"custom_xslt"`
	XSLTSHA1        *string    `json:"xslt_sha1"`
	UpdatedAt       *time.Time `json:"updated_at"`
}

func settingsView(st db.EinvoiceSetting) SettingsView {
	v := SettingsView{
		Configured: true, VKN: st.Vkn, TaxOffice: st.TaxOffice, LegalName: st.LegalName, Address: st.Address,
		City: st.City, District: st.District, Country: st.Country, IBAN: textPtr(st.Iban), Email: textPtr(st.Email),
		Phone: textPtr(st.Phone), Website: textPtr(st.Website), TradeRegistryNo: textPtr(st.TradeRegistryNo),
		MersisNo: textPtr(st.MersisNo), DefaultNote: textPtr(st.DefaultNote), EArchiveSeries: st.EarchiveSeries,
		EFaturaSeries: st.EfaturaSeries, PDFEnabled: st.PdfEnabled, CustomXSLT: st.XsltStorageKey.Valid,
		XSLTSHA1: textPtr(st.XsltSha1),
	}
	if st.UpdatedAt.Valid {
		t := st.UpdatedAt.Time
		v.UpdatedAt = &t
	}
	return v
}

// GetSettings returns the settings, or the defaults with Configured false.
func (s *Service) GetSettings(ctx context.Context, c Caller) (SettingsView, error) {
	if err := c.center(); err != nil {
		return SettingsView{}, err
	}
	st, err := s.settings(ctx, s.q, c)
	if errors.Is(err, ErrSettingsRequired) {
		return SettingsView{Country: ubl.CountryTR, EArchiveSeries: "EAR", EFaturaSeries: "EFN", PDFEnabled: true}, nil
	}
	if err != nil {
		return SettingsView{}, err
	}
	return settingsView(st), nil
}

// SettingsInput is PUT /v1/einvoices/settings. The stylesheet has its own
// endpoints and is kept.
type SettingsInput struct {
	VKN             string
	TaxOffice       string
	LegalName       string
	Address         string
	City            string
	District        string
	IBAN            string
	Email           string
	Phone           string
	Website         string
	TradeRegistryNo string
	MersisNo        string
	DefaultNote     string
	EArchiveSeries  string
	EFaturaSeries   string
	PDFEnabled      bool
}

var (
	seriesRE  = regexp.MustCompile(`^[A-Z0-9]{3}$`)
	ibanRE    = regexp.MustCompile(`^[A-Z]{2}[0-9A-Z]{13,32}$`)
	emailRE   = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	phoneRE   = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
	websiteRE = regexp.MustCompile(`^https?://`)
	mersisRE  = regexp.MustCompile(`^[0-9]{16}$`)
)

func (in SettingsInput) normalize() SettingsInput {
	trim := strings.TrimSpace
	in.VKN, in.TaxOffice, in.LegalName = trim(in.VKN), trim(in.TaxOffice), trim(in.LegalName)
	in.Address, in.City, in.District = trim(in.Address), trim(in.City), trim(in.District)
	in.IBAN = strings.ToUpper(strings.ReplaceAll(trim(in.IBAN), " ", ""))
	in.Email, in.Phone, in.Website = trim(in.Email), trim(in.Phone), trim(in.Website)
	in.TradeRegistryNo, in.MersisNo, in.DefaultNote = trim(in.TradeRegistryNo), trim(in.MersisNo), trim(in.DefaultNote)
	in.EArchiveSeries, in.EFaturaSeries = strings.ToUpper(trim(in.EArchiveSeries)), strings.ToUpper(trim(in.EFaturaSeries))
	return in
}

func (in SettingsInput) validate() error {
	var issues []FieldIssue
	add := func(field, msg string) { issues = append(issues, FieldIssue{Field: field, Message: msg}) }
	if !ubl.ValidVKN(in.VKN) {
		add("vkn", "must be a valid 10 digit VKN")
	}
	for _, f := range []struct {
		name, v string
		max     int
	}{
		{"tax_office", in.TaxOffice, 120}, {"legal_name", in.LegalName, 255}, {"address", in.Address, 2000},
		{"city", in.City, 100}, {"district", in.District, 100},
	} {
		switch {
		case f.v == "":
			add(f.name, "is required")
		case len([]rune(f.v)) > f.max:
			add(f.name, fmt.Sprintf("must be at most %d characters", f.max))
		}
	}
	if in.IBAN != "" && !ibanRE.MatchString(in.IBAN) {
		add("iban", "must be a valid IBAN")
	}
	if in.Email != "" && !emailRE.MatchString(in.Email) {
		add("email", "must be a valid e-mail address")
	}
	if in.Phone != "" && !phoneRE.MatchString(in.Phone) {
		add("phone", "must be E.164")
	}
	if in.Website != "" && !websiteRE.MatchString(in.Website) {
		add("website", "must start with http:// or https://")
	}
	if in.MersisNo != "" && !mersisRE.MatchString(in.MersisNo) {
		add("mersis_no", "must be 16 digits")
	}
	if len([]rune(in.TradeRegistryNo)) > 64 {
		add("trade_registry_no", "must be at most 64 characters")
	}
	if len([]rune(in.DefaultNote)) > 4000 {
		add("default_note", "must be at most 4000 characters")
	}
	for _, f := range []struct{ name, v string }{{"earchive_series", in.EArchiveSeries}, {"efatura_series", in.EFaturaSeries}} {
		switch {
		case !seriesRE.MatchString(f.v):
			add(f.name, "must be 3 characters A-Z or 0-9")
		case f.v == DraftSeries:
			add(f.name, DraftSeries+" is reserved for drafts")
		}
	}
	if in.EArchiveSeries != "" && in.EArchiveSeries == in.EFaturaSeries {
		add("efatura_series", "must differ from earchive_series")
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// PutSettings saves the seller profile and series (einvoice.settings).
func (s *Service) PutSettings(ctx context.Context, c Caller, in SettingsInput) (SettingsView, error) {
	if err := c.center(); err != nil {
		return SettingsView{}, err
	}
	in = in.normalize()
	if err := in.validate(); err != nil {
		return SettingsView{}, err
	}
	tx, err := s.tx.Begin(ctx)
	if err != nil {
		return SettingsView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)
	var xsltKey, xsltSHA pgtype.Text
	if cur, err := s.settings(ctx, qtx, c); err == nil {
		xsltKey, xsltSHA = cur.XsltStorageKey, cur.XsltSha1
	} else if !errors.Is(err, ErrSettingsRequired) {
		return SettingsView{}, err
	}
	st, err := qtx.UpsertEinvoiceSettings(ctx, db.UpsertEinvoiceSettingsParams{
		OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, Vkn: in.VKN, TaxOffice: in.TaxOffice,
		LegalName: in.LegalName, Address: in.Address, City: in.City, District: in.District, Country: ubl.CountryTR,
		Iban: text(in.IBAN), Email: text(in.Email), Phone: text(in.Phone), Website: text(in.Website),
		TradeRegistryNo: text(in.TradeRegistryNo), MersisNo: text(in.MersisNo), DefaultNote: text(in.DefaultNote),
		EarchiveSeries: in.EArchiveSeries, EfaturaSeries: in.EFaturaSeries, XsltStorageKey: xsltKey, XsltSha1: xsltSHA,
		PdfEnabled: in.PDFEnabled,
	})
	if err != nil {
		return SettingsView{}, err
	}
	if err := activity.Write(ctx, qtx, c.actor(), ActionSettingsUpdated, activityResource, &st.Uuid, map[string]any{
		"earchive_series": st.EarchiveSeries, "efatura_series": st.EfaturaSeries, "pdf_enabled": st.PdfEnabled,
	}, c.Meta); err != nil {
		return SettingsView{}, err
	}
	return settingsView(st), tx.Commit(ctx)
}

// xslNamespace is the XSLT 1.0/2.0 namespace.
const xslNamespace = "http://www.w3.org/1999/XSL/Transform"

// checkStylesheetRoot requires an xsl:stylesheet root element.
func checkStylesheetRoot(data []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return ubl.StylesheetError("the file has no root element")
		}
		if err != nil {
			return ubl.StylesheetError("the file is not well-formed XML: " + err.Error())
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Space != xslNamespace || se.Name.Local != "stylesheet" {
				return ubl.StylesheetError("the root element must be xsl:stylesheet")
			}
			return nil
		}
	}
}

// sampleDocument is the invoice the uploaded stylesheet is tried on: the
// center as seller, a sample buyer and one line.
func sampleDocument(st db.EinvoiceSetting, now time.Time) ubl.Document {
	return ubl.Document{
		Seller: mapper.SellerParty(st),
		Buyer: ubl.Party{
			Name: "Örnek Alıcı A.Ş.", VKN: "1234567890", TaxOffice: "Konak", Street: "Örnek Cad. No:1",
			District: "Konak", City: "İzmir", CountryCode: ubl.CountryTR,
		},
		Currency: ubl.CurrencyTRY, IssueAt: now,
		Lines: []ubl.Line{{Name: "Örnek ürün", Quantity: "1", UnitPrice: "100", VATRate: "20"}},
	}
}

// UploadXSLT stores a custom stylesheet (≤ 1 MB, xsl:stylesheet root) after
// a trial render of a sample invoice; a broken one is 422
// EINVOICE_INVALID_STYLESHEET.
func (s *Service) UploadXSLT(ctx context.Context, c Caller, data []byte) (SettingsView, error) {
	if err := c.center(); err != nil {
		return SettingsView{}, err
	}
	if len(data) == 0 {
		return SettingsView{}, invalid("file", "is required")
	}
	if len(data) > MaxXSLTBytes {
		return SettingsView{}, invalid("file", "must be at most 1 MB")
	}
	if s.store == nil {
		return SettingsView{}, ErrStorageUnavailable
	}
	if err := checkStylesheetRoot(data); err != nil {
		return SettingsView{}, err
	}
	st, err := s.settings(ctx, s.q, c)
	if err != nil {
		return SettingsView{}, err
	}
	now := s.now()
	_, sample, err := compose(sampleDocument(st, now), ubl.ProfileEArchive,
		fmt.Sprintf("%s%d000000001", DraftSeries, now.In(trTime).Year()), uuid.New(), data)
	if err != nil {
		return SettingsView{}, err
	}
	if _, err := ubl.RenderHTML(ctx, sample, data); err != nil {
		// Any failure of the trial render (unknown function, bad XPath)
		// rejects the stylesheet.
		if errors.Is(err, ubl.ErrInvalidStylesheet) {
			return SettingsView{}, err
		}
		return SettingsView{}, ubl.StylesheetError(err.Error())
	}
	sum := sha1.Sum(data) //nolint:gosec // content address only
	digest := hex.EncodeToString(sum[:])
	key := fmt.Sprintf("einvoices/xslt/%s/%s.xslt", c.Org.UUID, digest)
	if err := s.store.Upload(ctx, storage.File{
		Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: "application/xml", Filename: "einvoice.xslt",
	}, key); err != nil {
		return SettingsView{}, err
	}
	return s.setXSLT(ctx, c, pgtype.Text{String: key, Valid: true}, pgtype.Text{String: digest, Valid: true}, ActionXSLTUploaded)
}

// ResetXSLT returns to the embedded GİB stylesheet.
func (s *Service) ResetXSLT(ctx context.Context, c Caller) (SettingsView, error) {
	if err := c.center(); err != nil {
		return SettingsView{}, err
	}
	return s.setXSLT(ctx, c, pgtype.Text{}, pgtype.Text{}, ActionXSLTReset)
}

func (s *Service) setXSLT(ctx context.Context, c Caller, key, digest pgtype.Text, action string) (SettingsView, error) {
	tx, err := s.tx.Begin(ctx)
	if err != nil {
		return SettingsView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)
	st, err := qtx.SetEinvoiceSettingsXSLT(ctx, db.SetEinvoiceSettingsXSLTParams{
		OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, XsltStorageKey: key, XsltSha1: digest,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return SettingsView{}, ErrSettingsRequired
	}
	if err != nil {
		return SettingsView{}, err
	}
	if err := activity.Write(ctx, qtx, c.actor(), action, activityResource, &st.Uuid, map[string]any{
		"xslt_sha1": digest.String,
	}, c.Meta); err != nil {
		return SettingsView{}, err
	}
	return settingsView(st), tx.Commit(ctx)
}

// BuyerProfileInput is PUT /v1/platform/organizations/{uuid}/invoice-profile.
type BuyerProfileInput struct {
	VKN                string
	TCKN               string
	TaxOffice          string
	LegalName          string
	EInvoiceRegistered bool
	EInvoiceAlias      string
	Email              string
}

// BuyerProfileView is an organization's invoice profile.
type BuyerProfileView struct {
	OrganizationUUID   uuid.UUID `json:"organization_uuid"`
	OrganizationName   string    `json:"organization_name"`
	OrganizationType   string    `json:"organization_type"`
	VKN                *string   `json:"invoice_vkn"`
	TCKN               *string   `json:"invoice_tckn"`
	TaxOffice          *string   `json:"invoice_tax_office"`
	LegalName          *string   `json:"invoice_legal_name"`
	EInvoiceRegistered bool      `json:"einvoice_registered"`
	EInvoiceAlias      *string   `json:"einvoice_alias"`
	Email              *string   `json:"invoice_email"`
	// Missing lists what the profile still lacks for an invoice (tax id,
	// tax office, address, city, district).
	Missing []string `json:"missing_fields"`
}

func buyerProfileView(o db.Organization) BuyerProfileView {
	v := BuyerProfileView{
		OrganizationUUID: o.Uuid, OrganizationName: o.Name, OrganizationType: o.Type,
		VKN: textPtr(o.InvoiceVkn), TCKN: textPtr(o.InvoiceTckn), TaxOffice: textPtr(o.InvoiceTaxOffice),
		LegalName: textPtr(o.InvoiceLegalName), EInvoiceRegistered: o.EinvoiceRegistered,
		EInvoiceAlias: textPtr(o.EinvoiceAlias), Email: textPtr(o.InvoiceEmail), Missing: []string{},
	}
	var pe *ProfileError
	if errors.As(checkBuyerProfile(o), &pe) {
		v.Missing = pe.Fields
	}
	return v
}

// UpdateBuyerProfile sets the invoice profile of a distributor or dealer of
// the center's brand.
func (s *Service) UpdateBuyerProfile(ctx context.Context, c Caller, orgID uuid.UUID, in BuyerProfileInput) (BuyerProfileView, error) {
	if err := c.center(); err != nil {
		return BuyerProfileView{}, err
	}
	in.VKN, in.TCKN = strings.TrimSpace(in.VKN), strings.TrimSpace(in.TCKN)
	in.TaxOffice, in.LegalName = strings.TrimSpace(in.TaxOffice), strings.TrimSpace(in.LegalName)
	in.EInvoiceAlias, in.Email = strings.TrimSpace(in.EInvoiceAlias), strings.TrimSpace(in.Email)
	var issues []FieldIssue
	add := func(field, msg string) { issues = append(issues, FieldIssue{Field: field, Message: msg}) }
	switch {
	case in.VKN != "" && in.TCKN != "":
		add("invoice_tckn", "set either invoice_vkn or invoice_tckn")
	case in.VKN != "" && !ubl.ValidVKN(in.VKN):
		add("invoice_vkn", "must be a valid 10 digit VKN")
	case in.TCKN != "" && !ubl.ValidTCKN(in.TCKN):
		add("invoice_tckn", "must be a valid 11 digit TCKN")
	}
	if len([]rune(in.TaxOffice)) > 120 {
		add("invoice_tax_office", "must be at most 120 characters")
	}
	if len([]rune(in.LegalName)) > 255 {
		add("invoice_legal_name", "must be at most 255 characters")
	}
	if len([]rune(in.EInvoiceAlias)) > 255 {
		add("einvoice_alias", "must be at most 255 characters")
	}
	if in.Email != "" && (!emailRE.MatchString(in.Email) || len(in.Email) > 255) {
		add("invoice_email", "must be a valid e-mail address")
	}
	if len(issues) > 0 {
		return BuyerProfileView{}, &ValidationError{Issues: issues}
	}
	org, err := s.q.GetOrganizationByUUID(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (org.BrandID != c.Org.BrandID ||
		(org.Type != "distributor" && org.Type != "dealer"))) {
		return BuyerProfileView{}, ErrNotFound
	}
	if err != nil {
		return BuyerProfileView{}, err
	}
	tx, err := s.tx.Begin(ctx)
	if err != nil {
		return BuyerProfileView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)
	updated, err := qtx.UpdateOrganizationInvoiceProfile(ctx, db.UpdateOrganizationInvoiceProfileParams{
		ID: org.ID, BrandID: org.BrandID, InvoiceVkn: text(in.VKN), InvoiceTckn: text(in.TCKN),
		InvoiceTaxOffice: text(in.TaxOffice), InvoiceLegalName: text(in.LegalName),
		EinvoiceRegistered: in.EInvoiceRegistered, EinvoiceAlias: text(in.EInvoiceAlias), InvoiceEmail: text(in.Email),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return BuyerProfileView{}, ErrNotFound
	}
	if err != nil {
		return BuyerProfileView{}, err
	}
	if err := activity.Write(ctx, qtx, c.actor(), ActionBuyerProfileUpdated, "organizations", &updated.Uuid, map[string]any{
		"has_vkn": in.VKN != "", "has_tckn": in.TCKN != "", "einvoice_registered": in.EInvoiceRegistered,
	}, c.Meta); err != nil {
		return BuyerProfileView{}, err
	}
	return buyerProfileView(updated), tx.Commit(ctx)
}
