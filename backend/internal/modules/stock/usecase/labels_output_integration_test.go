package usecase_test

import (
	"context"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/labels"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
)

// pageRenderer records the requests (page size) of the TEC-1230 outputs.
type pageRenderer struct{ reqs []pdfrender.Request }

func (p *pageRenderer) Convert(_ context.Context, req pdfrender.Request) ([]byte, error) {
	p.reqs = append(p.reqs, req)
	return []byte("%PDF-1.4\n% fake"), nil
}

// TEC-1230: a 100x50 single template renders one label per page on a
// label-sized page; a zpl template answers ZPL without the PDF engine; the
// output format round-trips through the template API and is validated.
func TestLabelOutputFormats(t *testing.T) {
	e := newLabelEnv(t)
	c := e.caller(e.center)
	pr := &pageRenderer{}
	lb := stockusecase.NewLabels(e.q, pr)
	w, h := "100", "50"
	single, err := e.tpl.Create(e.ctx, c, stockusecase.TemplateInput{
		Name: "t1230-single-" + e.suffix, WidthMm: &w, HeightMm: &h, OutputFormat: labels.OutputSingle,
	})
	if err != nil || single.OutputFormat != labels.OutputSingle {
		t.Fatalf("single template: %+v %v", single, err)
	}
	zpl, err := e.tpl.Create(e.ctx, c, stockusecase.TemplateInput{
		Name: "t1230-zpl-" + e.suffix, WidthMm: &w, HeightMm: &h, OutputFormat: labels.OutputZPL,
	})
	if err != nil || zpl.OutputFormat != labels.OutputZPL {
		t.Fatalf("zpl template: %+v %v", zpl, err)
	}
	if _, err := e.tpl.Create(e.ctx, c, stockusecase.TemplateInput{Name: "t1230-bad-" + e.suffix, OutputFormat: "png"}); err == nil {
		t.Fatal("unknown output format accepted")
	}
	// Update without output_format keeps it.
	upd, err := e.tpl.Update(e.ctx, c, zpl.UUID, stockusecase.TemplateInput{Name: zpl.Name})
	if err != nil || upd.OutputFormat != labels.OutputZPL {
		t.Fatalf("update keeps format: %+v %v", upd, err)
	}
	plain, err := e.tpl.Create(e.ctx, c, stockusecase.TemplateInput{Name: "t1230-sheet-" + e.suffix})
	if err != nil || plain.OutputFormat != labels.OutputSheet {
		t.Fatalf("default format: %+v %v", plain, err)
	}

	prefix := "Z" + e.suffix[len(e.suffix)-7:]
	b, err := e.bc.Create(e.ctx, c, stockusecase.BatchInput{ProductUUID: e.roll.Uuid.String(), Quantity: 2, Meters: "12.5", Prefix: prefix})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	codes := []string{b.Units[0].Barcode, b.Units[1].Barcode}

	pdf, err := lb.PrintUnits(e.ctx, c, codes, &single.UUID)
	if err != nil || labels.IsZPL(pdf) || !strings.HasPrefix(string(pdf), "%PDF-") {
		t.Fatalf("single pdf: %q %v", pdf, err)
	}
	req := pr.reqs[len(pr.reqs)-1]
	if req.PageWidthMm != 100 || req.PageHeightMm != 50 || strings.Count(req.HTML, `<div class="label">`) != 2 ||
		!strings.Contains(req.HTML, "break-after:page") || !strings.Contains(req.HTML, "12.50 m") {
		t.Fatalf("single request = %vx%v %.200s", req.PageWidthMm, req.PageHeightMm, req.HTML)
	}

	calls := len(pr.reqs)
	out, err := lb.PrintUnits(e.ctx, c, codes, &zpl.UUID)
	if err != nil || !labels.IsZPL(out) {
		t.Fatalf("zpl: %q %v", out, err)
	}
	if len(pr.reqs) != calls {
		t.Fatal("zpl must not call the PDF engine")
	}
	s := string(out)
	if strings.Count(s, "^XA") != 2 || !strings.Contains(s, "^PW800^LL400") || !strings.Contains(s, "^FD"+codes[0]+"^FS") {
		t.Fatalf("zpl body = %s", s)
	}
	// Location labels follow the template too (QR in ZPL).
	if _, err := e.tpl.Create(e.ctx, c, stockusecase.TemplateInput{
		Name: "t1230-loczpl-" + e.suffix, Kind: labels.KindLocation, OutputFormat: labels.OutputZPL,
	}); err != nil {
		t.Fatalf("location zpl template: %v", err)
	}
}
