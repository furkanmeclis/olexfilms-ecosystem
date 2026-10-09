package ioengine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
)

type fakeConverter struct {
	reqs []pdfrender.Request
}

func (f *fakeConverter) Convert(_ context.Context, req pdfrender.Request) ([]byte, error) {
	f.reqs = append(f.reqs, req)
	return []byte("%PDF-1.7 fake"), nil
}

func exportPDFDataset() Dataset {
	return Dataset{
		Resource: "platform.users",
		Columns: []Column{
			{Key: "name", LabelKey: "users.name", Type: ColumnTypeString},
			{Key: "created_at", LabelKey: "users.created_at", Type: ColumnTypeDatetime},
			{Key: "total", LabelKey: "export.total", Type: ColumnTypeString, AlignRight: true},
		},
		Rows: []map[string]any{
			{"name": "Yıldız <b>", "created_at": time.Date(2026, 8, 23, 17, 47, 18, 0, time.UTC), "total": "12.50"},
			{"name": "محمد", "total": "1.00"},
			{"name": "王小明", "total": "3.00"},
		},
		Totals: map[string]any{"total": "16.50"},
		Info:   []InfoLine{{LabelKey: "users.name", Value: "Filtre & değer"}},
	}
}

// TEC-139: ar, zh_CN and tr exports go to Gotenberg as HTML with the right
// lang/dir and the Noto font stack (Arabic faces inlined for ar).
func TestEncodePDFLocalesGoThroughGotenberg(t *testing.T) {
	cases := []struct {
		locale, lang, dir string
		arabicFont        bool
		label             string
	}{
		{"ar", "ar", "rtl", true, "الاسم الأول"},
		{"zh_CN", "zh-CN", "ltr", true, "名"},
		{"tr", "tr", "ltr", true, "Ad"},
	}
	for _, tc := range cases {
		t.Run(tc.locale, func(t *testing.T) {
			conv := &fakeConverter{}
			lh := &Letterhead{CompanyName: "Olex", PrimaryColor: "#112233", FooterText: "Alt bilgi"}
			out, err := EncodePDF(context.Background(), conv, exportPDFDataset(), tc.locale, lh, "Başlık")
			if err != nil || string(out) != "%PDF-1.7 fake" {
				t.Fatalf("encode = %q, %v", out, err)
			}
			if len(conv.reqs) != 1 {
				t.Fatalf("gotenberg calls = %d", len(conv.reqs))
			}
			req := conv.reqs[0]
			h := req.HTML
			want := []string{
				`<html lang="` + tc.lang + `" dir="` + tc.dir + `">`,
				`font-family:"Noto Sans","Noto Sans Arabic","Noto Sans CJK SC"`,
				`@font-face{font-family:"Noto Sans"`,
				`--primary:#112233`,
				`<th>` + tc.label + `</th>`,
				`Yıldız &lt;b&gt;`, `محمد`, `王小明`,
				`<td class="num">16.50</td>`,
				`<th class="num">`,
				`Filtre &amp; değer`,
				`<strong>Olex</strong>`,
			}
			for _, w := range want {
				if !strings.Contains(h, w) {
					t.Errorf("html misses %q", w)
				}
			}
			// The body has Arabic text in every locale, so the Arabic faces
			// are inlined (shaping needs them, not only for ar).
			if got := strings.Contains(h, `font-family:"Noto Sans Arabic";font-style:normal`); got != tc.arabicFont {
				t.Errorf("arabic font inlined = %v", got)
			}
			if !strings.Contains(req.FooterHTML, `dir="`+tc.dir+`"`) || !strings.Contains(req.FooterHTML, "Alt bilgi") ||
				!strings.Contains(req.FooterHTML, `class="pageNumber"`) {
				t.Errorf("footer = %s", req.FooterHTML)
			}
			if req.Landscape {
				t.Error("3 columns rendered landscape")
			}
		})
	}
}

func TestExportTableHTMLArabicOnlyForRTLOrArabicText(t *testing.T) {
	ds := Dataset{
		Columns: []Column{{Key: "name", LabelKey: "users.name", Type: ColumnTypeString}},
		Rows:    []map[string]any{{"name": "Yıldız"}},
	}
	h := ExportTableHTML(ds, "tr", nil, "T")
	if strings.Contains(h, `font-family:"Noto Sans Arabic";font-style`) {
		t.Error("latin tr export inlined the Arabic faces")
	}
	if !strings.Contains(h, `lang="tr" dir="ltr"`) {
		t.Error("tr lang/dir missing")
	}
	h = ExportTableHTML(ds, "ar", nil, "T")
	if !strings.Contains(h, `font-family:"Noto Sans Arabic";font-style`) || !strings.Contains(h, `dir="rtl"`) {
		t.Error("ar export without rtl or Arabic faces")
	}
}

func TestExportPDFRequestLandscapeForWideTables(t *testing.T) {
	cols := make([]Column, pdfLandscapeColumns+1)
	for i := range cols {
		cols[i] = Column{Key: string(rune('a' + i)), LabelKey: "users.name"}
	}
	if !ExportPDFRequest(Dataset{Columns: cols}, "en", nil, "T").Landscape {
		t.Fatal("wide table not landscape")
	}
	if ExportPDFRequest(Dataset{Columns: cols[:pdfLandscapeColumns]}, "en", nil, "T").Landscape {
		t.Fatal("narrow table landscape")
	}
}

func TestEncodePDFNeedsRenderer(t *testing.T) {
	if _, err := EncodePDF(context.Background(), nil, exportPDFDataset(), "tr", nil, "T"); !errors.Is(err, ErrPDFRendererRequired) {
		t.Fatalf("err = %v", err)
	}
	if _, err := EncodeExport(ExportPDF, exportPDFDataset(), "tr", nil, "T"); !errors.Is(err, ErrPDFRendererRequired) {
		t.Fatalf("EncodeExport pdf err = %v", err)
	}
}

// CSV export is unchanged by the PDF engine switch.
func TestEncodeExportCSVUnchanged(t *testing.T) {
	out, err := EncodeExport(ExportCSV, exportPDFDataset(), "en", nil, "T")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "محمد") || !strings.Contains(string(out), "王小明") {
		t.Fatalf("csv = %s", out)
	}
}
