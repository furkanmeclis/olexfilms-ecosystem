package tools

import (
	"context"
	"time"

	shorturlsuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/shorturls/usecase"
)

// LinkMaker stores a short URL for an internal frontend path and returns
// the absolute {frontend}/s/{token} link (shorturls.Linker).
type LinkMaker interface {
	Link(ctx context.Context, in shorturlsuc.CreateInput) (string, error)
}

// linkTTL is the lifetime of the short links the tools hand out.
const linkTTL = 30 * 24 * time.Hour

// shortLink shortens an internal path of the principal's brand. The
// organization is recorded for panel links.
func shortLink(ctx context.Context, links LinkMaker, p Principal, target string) (string, error) {
	in := shorturlsuc.CreateInput{BrandID: p.brandID(), Target: target, TTL: linkTTL}
	if p.Auth.UserInternal > 0 {
		uid := p.Auth.UserInternal
		in.CreatedBy = &uid
	}
	if p.Org != nil {
		oid := p.Org.InternalID
		in.OrganizationID = &oid
	}
	return links.Link(ctx, in)
}

// pdfLink is one warranty certificate PDF (the public pdfrender output of
// /garanti/{code}/pdf).
type pdfLink struct {
	PublicCode string `json:"public_code"`
	Product    string `json:"product"`
	Status     string `json:"status"`
	URL        string `json:"url"`
}

// certificateLinks shortens the certificate PDF of every warranty that is
// not void. Files are never generated or attached here: the links point at
// the existing PDF output.
func certificateLinks(ctx context.Context, links LinkMaker, p Principal, ws []certificate) ([]pdfLink, error) {
	out := []pdfLink{}
	for _, w := range ws {
		if w.Status == "void" {
			continue
		}
		u, err := shortLink(ctx, links, p, "/garanti/"+w.Code+"/pdf")
		if err != nil {
			return nil, err
		}
		out = append(out, pdfLink{PublicCode: w.Code, Product: dataText(w.Product, maxNameChars), Status: w.Status, URL: u})
	}
	return out, nil
}

type certificate struct {
	Code, Product, Status string
}
