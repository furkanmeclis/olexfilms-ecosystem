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

// TEC-506: the price_list kind has a published platform template in each of
// the 13 languages with only known variables, the price table block and
// translated headings; ar renders right to left.
func TestPriceListTemplatesEveryLanguage(t *testing.T) {
	e := newEnv(t, nil, 0)
	ctx := context.Background()
	spec, ok := model.Spec(model.KindPriceList)
	if !ok {
		t.Fatal("price_list kind is not registered")
	}
	headings := map[string]string{}
	for _, lang := range model.Languages {
		tpl, err := e.q.GetActiveDocumentTemplate(ctx, db.GetActiveDocumentTemplateParams{
			Kind: model.KindPriceList, Language: lang, BrandID: pgtype.Int8{},
		})
		if err != nil {
			t.Fatalf("%s: no platform template: %v", lang, err)
		}
		if strings.TrimSpace(tpl.Name) == "" {
			t.Fatalf("%s: empty name", lang)
		}
		if u := spec.Unknown(tpl.Html); len(u) > 0 {
			t.Errorf("%s: unknown variables %v", lang, u)
		}
		for _, k := range []string{"prices_table", "country_name", "currency", "effective_date"} {
			if !strings.Contains(tpl.Html, "{{"+k+"}}") {
				t.Errorf("%s: missing {{%s}}", lang, k)
			}
		}
		h := tpl.Html[strings.Index(tpl.Html, "<h1"):]
		if lang != model.FallbackLanguage && h == headings[model.FallbackLanguage] {
			t.Errorf("%s: headings are not translated", lang)
		}
		headings[lang] = h
	}
	ar, err := e.q.GetActiveDocumentTemplate(ctx, db.GetActiveDocumentTemplateParams{Kind: model.KindPriceList, Language: "ar"})
	if err != nil {
		t.Fatal(err)
	}
	html := BuildHTML(model.KindPriceList, ar.Html, ar.Language, map[string]string{"currency": "TRY"}, ar.Name, "", pdfrender.FontsEmbedded)
	if !strings.Contains(html, `<html lang="ar" dir="rtl">`) {
		t.Fatal("ar price list must be rtl")
	}
}
