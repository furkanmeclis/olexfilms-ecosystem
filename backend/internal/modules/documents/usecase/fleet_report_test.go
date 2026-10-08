package usecase

import (
	"context"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/jackc/pgx/v5/pgtype"
)

// fleetReportBlocks are the tables the fleet loader fills.
var fleetReportBlocks = []string{
	"dealer_services_table", "vehicle_services_table", "parts_table", "products_table",
	"upcoming_expirations_table", "accounts_table",
}

// Acceptance (TEC-476): the fleet_report kind has a published platform
// template in each of the 13 languages: a name, a body with only known
// variables, every table block and its translated headings; ar is RTL.
func TestFleetReportTemplatesEveryLanguage(t *testing.T) {
	e := newEnv(t, nil, 0)
	ctx := context.Background()
	spec, ok := model.Spec(model.KindFleetReport)
	if !ok {
		t.Fatal("fleet_report kind is not registered")
	}
	if len(model.Languages) != 13 {
		t.Fatalf("languages = %d", len(model.Languages))
	}
	headings := map[string]string{}
	for _, lang := range model.Languages {
		tpl, err := e.q.GetActiveDocumentTemplate(ctx, db.GetActiveDocumentTemplateParams{
			Kind: model.KindFleetReport, Language: lang, BrandID: pgtype.Int8{},
		})
		if err != nil {
			t.Fatalf("%s: no platform template: %v", lang, err)
		}
		if strings.TrimSpace(tpl.Name) == "" || strings.TrimSpace(tpl.Html) == "" {
			t.Fatalf("%s: empty name or body", lang)
		}
		if u := spec.Unknown(tpl.Html); len(u) > 0 {
			t.Errorf("%s: unknown variables %v", lang, u)
		}
		for _, k := range append([]string{"period_label", "vehicle_count", "service_count", "warranty_active_count"}, fleetReportBlocks...) {
			if !strings.Contains(tpl.Html, "{{"+k+"}}") {
				t.Errorf("%s: missing {{%s}}", lang, k)
			}
		}
		// Every heading is filled (no empty <h2></h2> or <th></th>).
		if strings.Contains(tpl.Html, "<h2></h2>") || strings.Contains(tpl.Html, "<th></th>") {
			t.Errorf("%s: empty heading", lang)
		}
		h := tpl.Html[strings.Index(tpl.Html, "<h2>"):]
		if lang != model.FallbackLanguage && h == headings[model.FallbackLanguage] {
			t.Errorf("%s: headings are not translated", lang)
		}
		headings[lang] = h
	}

	ar, err := e.q.GetActiveDocumentTemplate(ctx, db.GetActiveDocumentTemplateParams{Kind: model.KindFleetReport, Language: "ar"})
	if err != nil {
		t.Fatal(err)
	}
	html := BuildHTML(model.KindFleetReport, ar.Html, ar.Language, map[string]string{"period_label": "سبتمبر 2026"}, ar.Name, "", pdfrender.FontsEmbedded)
	if !strings.Contains(html, `<html lang="ar" dir="rtl">`) || !strings.Contains(html, "تقرير الأسطول") {
		t.Fatal("ar fleet report must be rtl with the Arabic headings")
	}
}

// RenderSource renders a registered kind synchronously as the system: the
// fleet worker stores the PDF itself, no document_renders row is kept.
func TestRenderSourceFleetReport(t *testing.T) {
	e := newEnv(t, nil, 0)
	ctx := context.Background()
	if err := e.svc.RegisterLoader(model.KindFleetReport, e.loader); err != nil {
		t.Fatal(err)
	}
	e.loader.SetVar("parts_table", pdfrender.Table([]pdfrender.Column{{Label: "Parça"}}, [][]string{{"Kaput"}}))
	e.loader.SetVar("fleet_legal_name", "<b>Filo</b>")
	pdf, err := e.svc.RenderSource(ctx, model.KindFleetReport, "4001", "ar")
	if err != nil {
		t.Fatal(err)
	}
	if string(pdf) != string(fakePDF) {
		t.Fatalf("pdf = %q", pdf)
	}
	html := e.gotb.LastHTML()
	if !strings.Contains(html, `dir="rtl"`) || !strings.Contains(html, "<td>Kaput</td>") || !strings.Contains(html, "&lt;b&gt;Filo&lt;/b&gt;") {
		t.Fatal("render must use the ar template, raw table blocks and escaped text")
	}
	var n int
	if err := e.pool.QueryRow(ctx, "SELECT COUNT(*) FROM document_renders WHERE organization_id = $1 AND kind = $2", e.org.ID, model.KindFleetReport).Scan(&n); err != nil || n != 0 {
		t.Fatalf("render rows = %d, %v", n, err)
	}
	if _, err := e.svc.RenderSource(ctx, model.KindQuote, "1", "tr"); err == nil {
		t.Fatal("a kind without a loader must fail")
	}
}
