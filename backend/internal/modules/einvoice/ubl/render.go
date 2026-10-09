package ubl

import (
	"context"
	_ "embed"
	"errors"
	"strings"

	ubltr "github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/render"
	"github.com/furkanmeclis/go-ubltr/render/xslt/helium"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

// defaultXSLT is the GİB general.xslt viewer (assets/NOTICE.md).
//
//go:embed assets/general.xslt
var defaultXSLT []byte

// DefaultXSLT returns a copy of the embedded GİB general.xslt.
func DefaultXSLT() []byte {
	return append([]byte(nil), defaultXSLT...)
}

// Stylesheet returns the admin-uploaded XSLT when there is one, the GİB
// default otherwise.
func Stylesheet(uploaded []byte) []byte {
	if len(uploaded) > 0 {
		return uploaded
	}
	return DefaultXSLT()
}

// AttachXSLT embeds xslt in inv as an AdditionalDocumentReference of
// type XSLT, so receivers render the invoice with the seller's view. The
// reference ID is the invoice number (GİB convention).
func AttachXSLT(inv *invoice.Invoice, xslt []byte) error {
	if inv == nil {
		return invalidDocument("", "nil invoice")
	}
	if len(xslt) == 0 {
		return stylesheetError("xslt is empty")
	}
	err := ubltr.AddAttachment(inv, ubltr.Attachment{
		ID:       inv.ID.Value,
		Type:     "XSLT",
		Filename: inv.ID.Value + ".xslt",
		MIME:     "application/xml",
		Data:     xslt,
	})
	if err != nil {
		return stylesheetError(err.Error())
	}
	return nil
}

// RenderHTML transforms invoice XML to HTML with xslt (Stylesheet picks the
// uploaded or default one) through the pure Go helium XSLT engine. A broken
// stylesheet is a 422 ErrInvalidStylesheet.
func RenderHTML(ctx context.Context, xml, xslt []byte) ([]byte, error) {
	r, err := render.NewHTML(helium.New(), render.Style{Name: "einvoice", Stylesheet: xslt})
	if err != nil {
		return nil, stylesheetError(err.Error())
	}
	out, err := r.Render(ctx, xml)
	if errors.Is(err, render.ErrStylesheet) {
		return nil, stylesheetError(err.Error())
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(out)) == "" {
		return nil, stylesheetError("the stylesheet produced no output")
	}
	return out, nil
}
