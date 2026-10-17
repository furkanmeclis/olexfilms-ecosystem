package usecase

// Shipping info and documents (TEC-768, Depo F4.3).
//
// ready -> shipped may carry the delivery method (one of the seller's active
// methods) and a tracking number; both are stored with SetOrderShipping. The
// shipping note PDF (sevk irsaliyesi: seller and buyer, lines with their
// barcodes, delivery method and tracking number) is rendered through
// Gotenberg on the seller's letterhead right after the shipment commits and
// stored under orders.shipping_document_key; a render failure does not undo
// the shipment, the seller regenerates it from the order. The buyer may
// attach a delivery receipt (teslim belgesi, PDF or image) when receiving;
// it is stored under orders.receipt_document_key.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Document errors; the handler maps them to 404/409/422/503.
var (
	// ErrDocumentNotFound: the order has no such document.
	ErrDocumentNotFound = errors.New("orders: document not found")
	// ErrDocumentUnavailable: storage or the PDF engine is not configured.
	ErrDocumentUnavailable = errors.New("orders: document storage or renderer unavailable")
	// ErrShippingDocumentNotShipped: the order has not shipped.
	ErrShippingDocumentNotShipped = errors.New("orders: the order has not shipped")
	// ErrReceiptNotAllowed: a receipt is attached while shipped or received.
	ErrReceiptNotAllowed = errors.New("orders: a delivery receipt needs a shipped or received order")
)

// Delivery modes (chk_orders_delivery_mode).
const (
	DeliveryModeHand      = "hand"
	DeliveryModeWarehouse = "warehouse"
)

// MaxReceiptBytes bounds an uploaded delivery receipt.
const MaxReceiptBytes = 10 << 20

// receiptTypes are the accepted receipt content types and their extensions.
var receiptTypes = map[string]string{
	"application/pdf": "pdf", "image/jpeg": "jpg", "image/png": "png", "image/webp": "webp",
}

// Code of a refused receipt file.
const CodeReceiptFileInvalid = "ORDER_RECEIPT_FILE_INVALID"

// PDFRenderer converts HTML to PDF (*pdfrender.Client).
type PDFRenderer interface {
	Convert(ctx context.Context, req pdfrender.Request) ([]byte, error)
}

// WithDocuments wires the object store and the PDF engine of the shipping
// documents; nil leaves them unavailable.
func (s *Service) WithDocuments(store storage.Driver, pdf PDFRenderer, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	s.store, s.pdf, s.log = store, pdf, log
	return s
}

// ShippingDocumentObjectKey builds orders/{seller org}/{order}/shipping-note.pdf.
func ShippingDocumentObjectKey(orgUUID, orderUUID uuid.UUID) string {
	return fmt.Sprintf("orders/%s/%s/shipping-note.pdf", orgUUID, orderUUID)
}

// ReceiptDocumentObjectKey builds orders/{seller org}/{order}/receipt-{doc}.{ext}.
func ReceiptDocumentObjectKey(orgUUID, orderUUID, docUUID uuid.UUID, ext string) string {
	return fmt.Sprintf("orders/%s/%s/receipt-%s.%s", orgUUID, orderUUID, docUUID, ext)
}

// shippingInput is the validated shipping info of a ship transition.
type shippingInput struct {
	method     *db.DeliveryMethod
	trackingNo pgtype.Text
}

func (in TransitionInput) shipping(ctx context.Context, q *db.Queries, o db.Order) (shippingInput, error) {
	var out shippingInput
	if in.TrackingNo != nil {
		t := strings.TrimSpace(*in.TrackingNo)
		if len(t) > MaxTrackingNo || strings.ContainsAny(t, "\r\n\t") {
			return out, invalid("tracking_no", fmt.Sprintf("at most %d characters on one line", MaxTrackingNo))
		}
		if t != "" {
			out.trackingNo = pgtype.Text{String: t, Valid: true}
		}
	}
	if in.DeliveryMethodUUID != nil && strings.TrimSpace(*in.DeliveryMethodUUID) != "" {
		m, err := resolveShippingMethod(ctx, q, o, *in.DeliveryMethodUUID)
		if err != nil {
			return out, err
		}
		out.method = &m
	}
	return out, nil
}

// setShipping stores the shipping info on the order.
func setShipping(ctx context.Context, q *db.Queries, o db.Order, in shippingInput) (db.Order, error) {
	if in.method == nil && !in.trackingNo.Valid {
		return o, nil
	}
	p := db.SetOrderShippingParams{
		ID: o.ID, TrackingNo: in.trackingNo, ShippingDocumentKey: o.ShippingDocumentKey, DeliveryMode: o.DeliveryMode,
		DeliveryMethodID: o.DeliveryMethodID,
	}
	if in.method != nil {
		p.DeliveryMethodID = pgtype.Int8{Int64: in.method.ID, Valid: true}
		p.DeliveryMode = pgtype.Text{}
		if in.method.IsSystem {
			p.DeliveryMode = pgtype.Text{String: DeliveryModeWarehouse, Valid: true}
		}
	}
	o, err := q.SetOrderShipping(ctx, p)
	if err != nil {
		return o, fmt.Errorf("orders: shipping info: %w", err)
	}
	return o, nil
}

// afterShip renders the shipping note of a freshly shipped order. Failures
// are logged; the seller regenerates the document from the order.
func (s *Service) afterShip(ctx context.Context, o db.Order) db.Order {
	if s.store == nil || s.pdf == nil {
		return o
	}
	updated, err := s.renderShippingDocument(ctx, o)
	if err != nil {
		s.log.Warn("order_shipping_document_failed", "order_uuid", o.Uuid.String(), "error", err)
		return o
	}
	return updated
}

// RegenerateShippingDocument renders the shipping note again (seller,
// orders.ship, after shipping).
func (s *Service) RegenerateShippingDocument(ctx context.Context, c Caller, orderUUID uuid.UUID) (OrderView, error) {
	o, err := s.sellerOrder(ctx, c, orderUUID)
	if err != nil {
		return OrderView{}, err
	}
	if !o.ShippedAt.Valid {
		return OrderView{}, ErrShippingDocumentNotShipped
	}
	if s.store == nil || s.pdf == nil {
		return OrderView{}, ErrDocumentUnavailable
	}
	if o, err = s.renderShippingDocument(ctx, o); err != nil {
		return OrderView{}, err
	}
	return s.view(ctx, s.q, c, o)
}

// renderShippingDocument renders, uploads and records the shipping note.
func (s *Service) renderShippingDocument(ctx context.Context, o db.Order) (db.Order, error) {
	doc, err := s.shippingDoc(ctx, o)
	if err != nil {
		return o, err
	}
	data, err := s.pdf.Convert(ctx, pdfrender.Request{
		HTML: ShippingDocumentHTML(doc), FooterHTML: pdfrender.FooterHTML(string(doc.Locale), doc.Seller.Name),
	})
	if err != nil {
		return o, fmt.Errorf("orders: render shipping note: %w", err)
	}
	seller, err := s.q.GetOrganizationByID(ctx, o.SellerOrgID)
	if err != nil {
		return o, fmt.Errorf("orders: seller: %w", err)
	}
	key := ShippingDocumentObjectKey(seller.Uuid, o.Uuid)
	sum := sha256.Sum256(data)
	if err := s.store.Upload(ctx, storage.File{
		Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: "application/pdf",
		Filename: shippingDocumentFilename(o),
		Metadata: map[string]string{"sha256": hex.EncodeToString(sum[:]), "order_uuid": o.Uuid.String()},
	}, key); err != nil {
		return o, fmt.Errorf("orders: upload shipping note: %w", err)
	}
	updated, err := s.q.SetOrderShippingDocument(ctx, db.SetOrderShippingDocumentParams{
		ID: o.ID, ShippingDocumentKey: pgtype.Text{String: key, Valid: true},
	})
	if err != nil {
		return o, fmt.Errorf("orders: shipping note key: %w", err)
	}
	return updated, nil
}

func shippingDocumentFilename(o db.Order) string { return "shipping-note-" + o.OrderNo + ".pdf" }

// DocumentFile is a stored document to stream.
type DocumentFile struct {
	Body        io.ReadCloser
	Size        int64
	ContentType string
	Filename    string
}

// Document kinds of GET /v1/orders/{uuid}/documents/{kind}.
const (
	DocumentShipping = "shipping"
	DocumentReceipt  = "receipt"
)

// OpenDocument streams a document of an order the caller sees (both sides).
func (s *Service) OpenDocument(ctx context.Context, c Caller, orderUUID uuid.UUID, kind string) (DocumentFile, error) {
	o, err := s.q.GetOrderByUUID(ctx, db.GetOrderByUUIDParams{Uuid: orderUUID, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return DocumentFile{}, ErrNotFound
	}
	if err != nil {
		return DocumentFile{}, fmt.Errorf("orders: get: %w", err)
	}
	if !visible(c, o) {
		return DocumentFile{}, ErrNotFound
	}
	var key pgtype.Text
	name := ""
	switch kind {
	case DocumentShipping:
		key, name = o.ShippingDocumentKey, shippingDocumentFilename(o)
	case DocumentReceipt:
		key, name = o.ReceiptDocumentKey, "delivery-receipt-"+o.OrderNo
	default:
		return DocumentFile{}, ErrDocumentNotFound
	}
	if !key.Valid || strings.TrimSpace(key.String) == "" {
		return DocumentFile{}, ErrDocumentNotFound
	}
	if s.store == nil {
		return DocumentFile{}, ErrDocumentUnavailable
	}
	rc, size, err := s.store.Download(ctx, key.String)
	if err != nil {
		return DocumentFile{}, fmt.Errorf("orders: download document: %w", err)
	}
	ct := contentTypeOfKey(key.String)
	if kind == DocumentReceipt {
		name += "." + receiptTypes[ct]
	}
	return DocumentFile{Body: rc, Size: size, ContentType: ct, Filename: name}, nil
}

func contentTypeOfKey(key string) string {
	ext := key[strings.LastIndex(key, ".")+1:]
	for ct, e := range receiptTypes {
		if e == ext {
			return ct
		}
	}
	return "application/octet-stream"
}

// ReceiptInput is an uploaded delivery receipt.
type ReceiptInput struct {
	Body io.Reader
	Size int64
}

// AttachReceipt stores the buyer's delivery receipt (buyer,
// orders.receive, shipped or received). A new upload replaces the old one.
func (s *Service) AttachReceipt(ctx context.Context, c Caller, orderUUID uuid.UUID, in ReceiptInput) (OrderView, error) {
	if s.store == nil {
		return OrderView{}, ErrDocumentUnavailable
	}
	if in.Size <= 0 || in.Size > MaxReceiptBytes {
		return OrderView{}, &ValidationError{Field: "file", Code: CodeReceiptFileInvalid,
			Message: fmt.Sprintf("the file must be 1 byte to %d MB", MaxReceiptBytes>>20)}
	}
	raw, err := io.ReadAll(io.LimitReader(in.Body, MaxReceiptBytes+1))
	if err != nil {
		return OrderView{}, invalid("file", "the file could not be read")
	}
	if len(raw) == 0 || len(raw) > MaxReceiptBytes {
		return OrderView{}, &ValidationError{Field: "file", Code: CodeReceiptFileInvalid, Message: "the file is empty or too large"}
	}
	ct := http.DetectContentType(raw)
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}
	ext, ok := receiptTypes[ct]
	if !ok {
		return OrderView{}, &ValidationError{Field: "file", Code: CodeReceiptFileInvalid, Message: "PDF, JPEG, PNG or WebP only"}
	}
	var result db.Order
	var old pgtype.Text
	var key string
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		o, err := s.lockVisible(ctx, q, c, orderUUID)
		if err != nil {
			return err
		}
		if partyOf(c, o) != PartyBuyer || !c.can(rbac.PermOrdersReceive) {
			return ErrForbidden
		}
		if o.Status != StatusShipped && o.Status != StatusReceived {
			return ErrReceiptNotAllowed
		}
		seller, err := q.GetOrganizationByID(ctx, o.SellerOrgID)
		if err != nil {
			return fmt.Errorf("orders: seller: %w", err)
		}
		key = ReceiptDocumentObjectKey(seller.Uuid, o.Uuid, uuid.New(), ext)
		if err := s.store.Upload(ctx, storage.File{
			Body: bytes.NewReader(raw), Size: int64(len(raw)), ContentType: ct,
			Filename: "delivery-receipt-" + o.OrderNo + "." + ext,
			Metadata: map[string]string{"order_uuid": o.Uuid.String()},
		}, key); err != nil {
			return fmt.Errorf("orders: upload receipt: %w", err)
		}
		old = o.ReceiptDocumentKey
		result, err = q.SetOrderReceiptDocument(ctx, db.SetOrderReceiptDocumentParams{
			ID: o.ID, ReceiptDocumentKey: pgtype.Text{String: key, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("orders: receipt key: %w", err)
		}
		return nil
	})
	if err != nil {
		if key != "" {
			_ = s.store.Delete(context.WithoutCancel(ctx), key)
		}
		return OrderView{}, err
	}
	if old.Valid && old.String != "" && old.String != key {
		if err := s.store.Delete(ctx, old.String); err != nil {
			s.log.Warn("order_receipt_old_delete_failed", "key", old.String, "error", err)
		}
	}
	return s.view(ctx, s.q, c, result)
}

// --- Shipping note document ------------------------------------------------------

// ShippingParty is a side of the shipping note.
type ShippingParty struct {
	Name    string
	Address string
	Phone   string
	Email   string
	TaxNo   string
}

// ShippingLine is one line of the shipping note.
type ShippingLine struct {
	// Stored amounts are also used by the order slip; the shipping note omits them.
	UnitPrice string
	LineTotal string
	SKU       string
	Name      string
	Amount    string
	Barcodes  []string
}

// ShippingDoc is the data of one shipping note.
type ShippingDoc struct {
	Locale         i18n.Locale
	OrderNo        string
	ShippedAt      time.Time
	Zone           *time.Location
	Seller         ShippingParty
	Buyer          ShippingParty
	DeliveryMethod string
	TrackingNo     string
	TrackingURL    string
	Lines          []ShippingLine
	Letterhead     ioengine.Letterhead
	GeneratedAt    time.Time
}

func shippingParty(o db.Organization) ShippingParty {
	p := ShippingParty{
		Name: strings.TrimSpace(o.Name), Phone: strings.TrimSpace(o.Phone), Email: strings.TrimSpace(o.Email),
		Address: ioengine.JoinNonEmpty(", ", strings.TrimSpace(o.Address), strings.TrimSpace(o.District), strings.TrimSpace(o.City)),
	}
	if o.InvoiceLegalName.Valid && strings.TrimSpace(o.InvoiceLegalName.String) != "" {
		p.Name = strings.TrimSpace(o.InvoiceLegalName.String)
	}
	tax := ""
	switch {
	case o.InvoiceVkn.Valid && o.InvoiceVkn.String != "":
		tax = o.InvoiceVkn.String
	case o.InvoiceTckn.Valid && o.InvoiceTckn.String != "":
		tax = o.InvoiceTckn.String
	}
	if tax != "" && o.InvoiceTaxOffice.Valid && o.InvoiceTaxOffice.String != "" {
		tax = o.InvoiceTaxOffice.String + " / " + tax
	}
	p.TaxNo = tax
	return p
}

// shippingDoc loads the shipping note data of an order. The language is
// the request's.
func (s *Service) shippingDoc(ctx context.Context, o db.Order) (ShippingDoc, error) {
	seller, err := s.q.GetOrganizationByID(ctx, o.SellerOrgID)
	if err != nil {
		return ShippingDoc{}, fmt.Errorf("orders: seller: %w", err)
	}
	buyer, err := s.q.GetOrganizationByID(ctx, o.BuyerOrgID)
	if err != nil {
		return ShippingDoc{}, fmt.Errorf("orders: buyer: %w", err)
	}
	loc := i18n.FromContext(ctx).Locale
	d := ShippingDoc{
		Locale: loc, OrderNo: o.OrderNo, Seller: shippingParty(seller), Buyer: shippingParty(buyer),
		Zone: pdfrender.Zone(seller.Timezone), GeneratedAt: s.now().UTC(), TrackingNo: o.TrackingNo.String,
	}
	if o.ShippedAt.Valid {
		d.ShippedAt = o.ShippedAt.Time
	}
	if o.DeliveryMethodID.Valid {
		m, err := s.q.GetDeliveryMethodByID(ctx, o.DeliveryMethodID.Int64)
		if err != nil {
			return ShippingDoc{}, fmt.Errorf("orders: delivery method: %w", err)
		}
		d.DeliveryMethod = deliveryMethodName(ctx, m)
		d.TrackingURL = TrackingURL(m.TrackingUrlTemplate.String, d.TrackingNo)
	}
	settings, err := s.q.GetAppSettings(ctx)
	if err != nil {
		return ShippingDoc{}, fmt.Errorf("orders: settings: %w", err)
	}
	d.Letterhead = ioengine.LetterheadFromOrganization(seller, settings)
	if lh, err := ioengine.LoadOrganizationLetterhead(ctx, s.store, seller, settings); err != nil {
		s.log.Warn("order_shipping_document_logo_failed", "error", err)
	} else {
		d.Letterhead = lh
	}
	items, err := s.q.ListOrderItems(ctx, o.ID)
	if err != nil {
		return ShippingDoc{}, fmt.Errorf("orders: lines: %w", err)
	}
	units, err := s.q.ListOrderItemUnitsByOrder(ctx, o.ID)
	if err != nil {
		return ShippingDoc{}, fmt.Errorf("orders: assigned units: %w", err)
	}
	byItem := map[int64][]string{}
	for _, u := range units {
		bc := u.Barcode
		switch {
		case u.Meters.Valid:
			bc += " (" + numericText(u.Meters, 2) + " m)"
		case u.UnitKind == "fixed" && u.Quantity.Valid:
			bc += " ×" + strconv.Itoa(int(u.Quantity.Int32))
		}
		byItem[u.OrderItemID] = append(byItem[u.OrderItemID], bc)
	}
	for _, it := range items {
		p, err := s.q.GetProduct(ctx, db.GetProductParams{ID: it.ProductID, BrandID: o.BrandID})
		if err != nil {
			return ShippingDoc{}, fmt.Errorf("orders: product: %w", err)
		}
		amount := strconv.Itoa(int(it.Quantity.Int32)) + " " + i18n.Translate(loc, "orders.pdf.unit_pcs")
		if it.Meters.Valid {
			amount = numericText(it.Meters, 2) + " m"
		}
		d.Lines = append(d.Lines, ShippingLine{SKU: p.Sku, Name: p.Name, Amount: amount, Barcodes: byItem[it.ID], UnitPrice: numericText(it.UnitPrice, 2), LineTotal: numericText(it.LineTotal, 2)})
	}
	return d, nil
}

// ShippingDocumentHTML renders the shipping note. Every value is escaped.
func ShippingDocumentHTML(d ShippingDoc) string {
	t := func(key string) string { return i18n.Translate(d.Locale, key) }
	esc := html.EscapeString
	var b strings.Builder
	b.Grow(8192 + 256*len(d.Lines))
	lh := d.Letterhead
	title := t("orders.pdf.shipping_note")
	b.WriteString(shippingDocCSS)
	b.WriteString(ioengine.LetterheadHeaderHTML(&lh))
	b.WriteString(`<h1 class="doc-title">` + esc(title) + `</h1><table class="doc-meta">`)
	meta := func(label, value string, ltr bool) {
		if strings.TrimSpace(value) == "" {
			return
		}
		dir := ""
		if ltr {
			dir = ` dir="ltr"`
		}
		b.WriteString(`<tr><th>` + esc(label) + `</th><td` + dir + `>` + esc(value) + `</td></tr>`)
	}
	meta(t("orders.pdf.order_no"), d.OrderNo, true)
	if !d.ShippedAt.IsZero() {
		meta(t("orders.pdf.shipped_at"), pdfrender.IssuedAt(d.ShippedAt, d.Zone), true)
	}
	meta(t("orders.pdf.delivery_method"), d.DeliveryMethod, false)
	meta(t("orders.pdf.tracking_no"), d.TrackingNo, true)
	meta(t("orders.pdf.tracking_url"), d.TrackingURL, true)
	b.WriteString(`</table><table class="parties"><tr>`)
	party := func(label string, p ShippingParty) {
		b.WriteString(`<td><h2>` + esc(label) + `</h2><p><strong>` + esc(p.Name) + `</strong>`)
		for _, v := range []string{p.Address, p.Phone, p.Email} {
			if v != "" {
				b.WriteString(`<br>` + esc(v))
			}
		}
		if p.TaxNo != "" {
			b.WriteString(`<br>` + esc(t("orders.pdf.tax_no")) + `: <span dir="ltr">` + esc(p.TaxNo) + `</span>`)
		}
		b.WriteString(`</p></td>`)
	}
	party(t("orders.pdf.seller"), d.Seller)
	party(t("orders.pdf.buyer"), d.Buyer)
	b.WriteString(`</tr></table>`)
	b.WriteString(`<table class="lines"><thead><tr><th class="num">#</th><th>` + esc(t("orders.pdf.product")) +
		`</th><th class="num">` + esc(t("orders.pdf.amount")) + `</th><th>` + esc(t("orders.pdf.barcodes")) + `</th></tr></thead><tbody>`)
	for i, l := range d.Lines {
		b.WriteString(`<tr><td class="num">` + strconv.Itoa(i+1) + `</td><td><strong>` + esc(l.Name) + `</strong>`)
		if l.SKU != "" {
			b.WriteString(`<br><span class="muted" dir="ltr">` + esc(l.SKU) + `</span>`)
		}
		b.WriteString(`</td><td class="num" dir="ltr">` + esc(l.Amount) + `</td><td class="codes" dir="ltr">`)
		for j, bc := range l.Barcodes {
			if j > 0 {
				b.WriteString(`<br>`)
			}
			b.WriteString(esc(bc))
		}
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</tbody></table>`)
	b.WriteString(`<table class="sign"><tr><td>` + esc(t("orders.pdf.delivered_by")) + `</td><td>` +
		esc(t("orders.pdf.received_by")) + `</td></tr></table>`)
	b.WriteString(`<p class="muted">` + esc(t("orders.pdf.generated_at")) + `: ` +
		esc(pdfrender.IssuedAt(d.GeneratedAt, d.Zone)) + `</p>`)
	b.WriteString(ioengine.LetterheadFooterHTML(&lh))
	return pdfrender.Document{Lang: string(d.Locale), Title: title + " " + d.OrderNo, Body: b.String(), PrimaryColor: lh.PrimaryColor}.HTML()
}

// shippingDocCSS uses logical properties, so the layout mirrors in RTL.
const shippingDocCSS = `<style>
table.parties{width:100%;border-collapse:collapse;margin-block:8pt}
table.parties td{width:50%;vertical-align:top;padding:4pt;border:1px solid #e5e7eb}
table.parties h2{font-size:10pt;margin:0 0 4pt}
table.lines{width:100%;border-collapse:collapse;font-size:9pt;margin-block-end:12pt}
table.lines th,table.lines td{border-block-end:1px solid #e5e7eb;padding:4pt;text-align:start;vertical-align:top}
table.lines thead th{background:#f3f4f6;border-block-end:2px solid var(--primary)}
table.lines .num{text-align:end;white-space:nowrap}
table.lines .codes{font-family:monospace;font-size:8pt}
table.sign{width:100%;margin-block-start:24pt}
table.sign td{width:50%;padding-block-start:36pt;border-block-start:1px solid #9ca3af;text-align:center}
</style>`
