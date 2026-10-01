package pdfrender

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// Acceptance (TEC-88): 20 concurrent service PDFs in under 10 s against a
// real Gotenberg. Runs only with TEST_GOTENBERG_URL (e.g.
// http://127.0.0.1:3001 from compose.local.yml); CI has no Gotenberg.
func TestGotenbergConcurrentServicePDFs(t *testing.T) {
	url := os.Getenv("TEST_GOTENBERG_URL")
	if url == "" {
		t.Skip("TEST_GOTENBERG_URL not set; skipping real Gotenberg test")
	}
	for _, tc := range []struct {
		name  string
		lang  string
		fonts FontMode
	}{
		{"tr-embedded", "tr", FontsEmbedded},
		{"ar-embedded", "ar", FontsEmbedded},
		{"tr-system", "tr", FontsSystem},
		{"ar-system", "ar", FontsSystem},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewWithOptions(url, Options{MaxConnsPerHost: 32})
			body := sampleServiceBody(tc.lang)
			doc := Document{Lang: tc.lang, Title: "Service", Body: body, Fonts: tc.fonts}.HTML()
			req := Request{HTML: doc, FooterHTML: FooterHTML(tc.lang, "olexfilms.app")}
			// warm-up: Chromium start, font cache
			first, err := c.Convert(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if dir := os.Getenv("TEST_GOTENBERG_OUT"); dir != "" {
				_ = os.WriteFile(filepath.Join(dir, tc.name+".pdf"), first, 0o600)
			}
			var before runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			const n = 20
			durations := make([]time.Duration, n)
			sizes := make([]int, n)
			errs := make([]error, n)
			var wg sync.WaitGroup
			start := time.Now()
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					s := time.Now()
					pdf, err := c.Convert(context.Background(), req)
					durations[i], sizes[i], errs[i] = time.Since(s), len(pdf), err
				}(i)
			}
			wg.Wait()
			total := time.Since(start)
			var after runtime.MemStats
			runtime.ReadMemStats(&after)
			for _, err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			sort.Slice(durations, func(a, b int) bool { return durations[a] < durations[b] })
			t.Logf("%s: html=%d KB, 20 concurrent total=%v p50=%v p100=%v pdf=%d KB, client alloc=%.1f MB",
				tc.name, len(doc)/1024, total.Round(time.Millisecond), durations[n/2].Round(time.Millisecond),
				durations[n-1].Round(time.Millisecond), sizes[0]/1024,
				float64(after.TotalAlloc-before.TotalAlloc)/(1<<20))
			if total > 10*time.Second {
				t.Fatalf("20 concurrent PDFs took %v, want < 10s", total)
			}
		})
	}
}

func sampleServiceBody(lang string) string {
	name, plate, item := "Şükrü Öztürk", "34 ABC 123", "Ön cam PPF — İnce kaplama"
	if lang == "ar" {
		name, item = "محمد عبد الله", "حماية الطلاء — الواجهة الأمامية"
	}
	rows := make([][]string, 0, 12)
	for i := 1; i <= 12; i++ {
		rows = append(rows, []string{fmt.Sprintf("%s %d", item, i), "1", "1.250,00 TRY"})
	}
	tbl := Table([]Column{{Label: "Hizmet"}, {Label: "Adet", Numeric: true}, {Label: "Tutar", Numeric: true}}, rows)
	tpl := `<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong></div></header>
<h1 class="doc-title">Hizmet Formu</h1>
<table class="doc-meta"><tr><th>Müşteri</th><td>{{customer_name}}</td><th>Plaka</th><td>{{plate}}</td></tr></table>
{{items_table}}<p class="doc-total">Toplam: 15.000,00 TRY</p>` + strings.Repeat("<p>Garanti koşulları ve açıklamalar. ğüşıöç İĞÜŞÖÇ</p>", 10)
	return Fill(tpl, map[string]string{
		"company_name": "Olex Films", "customer_name": name, "plate": plate, "items_table": tbl,
	}, map[string]bool{"items_table": true})
}
