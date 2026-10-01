package fxrates

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func rateMap(d Day) map[string]string {
	out := map[string]string{}
	for _, r := range d.Rates {
		out[r.Base+"/"+r.Quote] = r.Rate
	}
	return out
}

func TestParseTCMBFixture(t *testing.T) {
	f, err := os.Open("testdata/tcmb_today.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	day, err := ParseTCMB(f)
	if err != nil {
		t.Fatal(err)
	}
	if day.Source != SourceTCMB || day.Date.Format("2006-01-02") != "2026-09-30" {
		t.Fatalf("day = %s %s", day.Source, day.Date)
	}
	got := rateMap(day)
	want := map[string]string{
		"USD/TRY": "41.5769",
		"EUR/TRY": "48.7988",
		// JPY is quoted per 100 units: 28.1323 / 100.
		"JPY/TRY": "0.281323",
		"AZN/TRY": "24.5569",
	}
	if len(got) != len(want) {
		t.Fatalf("rates = %v (XDR without ForexSelling must be skipped)", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestParseTCMBLatin5(t *testing.T) {
	doc := "<?xml version=\"1.0\" encoding=\"ISO-8859-9\"?>\n" +
		"<Tarih_Date Tarih=\"02.01.2026\"><Currency CurrencyCode=\"USD\"><Unit>1</Unit>" +
		"<Isim>ABD DOLARI \xdd</Isim><ForexSelling>35.1</ForexSelling></Currency></Tarih_Date>"
	day, err := ParseTCMB(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if day.Date.Format("2006-01-02") != "2026-01-02" || rateMap(day)["USD/TRY"] != "35.1" {
		t.Fatalf("day = %+v", day)
	}
}

func TestParseTCMBRejectsGarbage(t *testing.T) {
	for _, doc := range []string{"not xml", `<Tarih_Date Date="09/30/2026"></Tarih_Date>`, `<Tarih_Date><Currency CurrencyCode="USD"><ForexSelling>1</ForexSelling></Currency></Tarih_Date>`} {
		if _, err := ParseTCMB(strings.NewReader(doc)); !errors.Is(err, ErrParse) {
			t.Errorf("ParseTCMB(%q) = %v, want ErrParse", doc, err)
		}
	}
}

func TestParseECBFixture(t *testing.T) {
	f, err := os.Open("testdata/ecb_daily.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	day, err := ParseECB(f)
	if err != nil {
		t.Fatal(err)
	}
	if day.Source != SourceECB || day.Date.Format("2006-01-02") != "2026-09-30" {
		t.Fatalf("day = %s %s", day.Source, day.Date)
	}
	got := rateMap(day)
	for k, v := range map[string]string{"EUR/USD": "1.1734", "EUR/JPY": "174.33", "EUR/GBP": "0.87295", "EUR/TRY": "48.7612"} {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestParseECBLatestDay(t *testing.T) {
	doc := `<Envelope><Cube>
		<Cube time="2026-09-29"><Cube currency="USD" rate="1.10"/></Cube>
		<Cube time="2026-09-30"><Cube currency="USD" rate="1.20"/></Cube>
	</Cube></Envelope>`
	day, err := ParseECB(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if day.Date.Format("2006-01-02") != "2026-09-30" || rateMap(day)["EUR/USD"] != "1.2" {
		t.Fatalf("day = %+v", day)
	}
}

func TestParseRate(t *testing.T) {
	if v, err := ParseRate(" 41.57690 "); err != nil || v != "41.5769" {
		t.Fatalf("ParseRate = %q %v", v, err)
	}
	for _, bad := range []string{"", "abc", "0", "-1", "0.00000000001"} {
		if _, err := ParseRate(bad); err == nil {
			t.Errorf("ParseRate(%q) accepted", bad)
		}
	}
}

// The fetcher talks to providers over HTTP; httptest serves the fixtures so
// no test leaves the machine.
func TestFetcherServesFixtures(t *testing.T) {
	tcmb, _ := os.ReadFile("testdata/tcmb_today.xml")
	ecb, _ := os.ReadFile("testdata/ecb_daily.xml")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tcmb":
			_, _ = w.Write(tcmb)
		case "/ecb":
			_, _ = w.Write(ecb)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	f := NewFetcher(srv.URL+"/tcmb", srv.URL+"/ecb")
	if d, err := f.TCMB(context.Background()); err != nil || len(d.Rates) != 4 {
		t.Fatalf("TCMB = %+v %v", d, err)
	}
	if d, err := f.ECB(context.Background()); err != nil || len(d.Rates) != 5 {
		t.Fatalf("ECB = %+v %v", d, err)
	}
	f.TCMBURL = srv.URL + "/missing"
	if _, err := f.TCMB(context.Background()); err == nil {
		t.Fatal("404 must fail")
	}
}
