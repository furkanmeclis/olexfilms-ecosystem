package usecase

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
)

func rat(s string) *big.Rat {
	r, _ := new(big.Rat).SetString(s)
	return r
}

// Debit/credit split and running balance from the opening balance; a
// reversal (negative signed amount of a debit row) lands in credit.
func TestRunStatement(t *testing.T) {
	got := runStatement(rat("100.00"), []lineInput{
		{signed: rat("1500.00")},  // charge
		{signed: rat("-400.50")},  // collection
		{signed: rat("-1500.00")}, // reversal of the charge
		{signed: rat("250.25")},   // payment
	})
	if got.opening != "100.00" || got.debit != "1750.25" || got.credit != "1900.50" || got.closing != "-50.25" {
		t.Fatalf("sums = %+v", got)
	}
	want := []lineMoney{
		{debit: "1500.00", credit: "0.00", balance: "1600.00"},
		{debit: "0.00", credit: "400.50", balance: "1199.50"},
		{debit: "0.00", credit: "1500.00", balance: "-300.50"},
		{debit: "250.25", credit: "0.00", balance: "-50.25"},
	}
	for i, w := range want {
		if got.lines[i] != w {
			t.Errorf("line %d = %+v, want %+v", i, got.lines[i], w)
		}
	}
	empty := runStatement(rat("-12.00"), nil)
	if empty.closing != "-12.00" || empty.debit != "0.00" || len(empty.lines) != 0 {
		t.Fatalf("empty period = %+v", empty)
	}
}

func sampleStatement() Statement {
	from, to := "2026-09-01", "2026-09-30"
	st := Statement{
		Organization: Ref{Name: "Olex <Center>"}, Currency: "TRY", From: &from, To: &to,
		OpeningBalance: "100.00", TotalDebit: "1500.00", TotalCredit: "0.00", ClosingBalance: "1600.00",
		GeneratedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		Lines: []StatementLine{{
			Date: time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC), Description: "Fatura & <b>x</b>",
			SourceLabel: "Manual entry", Debit: "1500.00", Credit: "0.00", Balance: "1600.00",
		}},
	}
	st.Cari.Counterparty.Name = "Dist A"
	return st
}

func TestStatementDatasetOpeningAndClosingRows(t *testing.T) {
	ds := StatementDataset(sampleStatement(), i18n.LocaleTR)
	if len(ds.Rows) != 3 {
		t.Fatalf("rows = %d", len(ds.Rows))
	}
	if ds.Rows[0]["description"] != "Açılış bakiyesi" || ds.Rows[0]["balance"] != "100.00" || ds.Rows[0]["date"] != "2026-09-01" {
		t.Fatalf("opening row = %v", ds.Rows[0])
	}
	if ds.Rows[1]["date"] != "2026-09-05" || ds.Rows[1]["debit"] != "1500.00" {
		t.Fatalf("line row = %v", ds.Rows[1])
	}
	last := ds.Rows[2]
	if last["description"] != "Kapanış bakiyesi" || last["balance"] != "1600.00" || last["debit"] != "1500.00" {
		t.Fatalf("closing row = %v", last)
	}
	csv, err := ioengine.EncodeCSV(ds, "tr")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(csv), "Tarih,Açıklama,Kaynak,Borç,Alacak,Bakiye") {
		t.Fatalf("csv header not in tr: %s", csv)
	}
	csvEN, _ := ioengine.EncodeCSV(StatementDataset(sampleStatement(), i18n.LocaleEN), "en")
	if !strings.Contains(string(csvEN), "Date,Description,Source,Debit,Credit,Balance") {
		t.Fatalf("csv header not in en: %s", csvEN)
	}
	if _, err := ioengine.EncodeXLSX(ds, "tr", &ioengine.Letterhead{CompanyName: "Olex"}); err != nil {
		t.Fatal(err)
	}
}

// The PDF document is RTL in Arabic, carries the letterhead and escapes
// every value.
func TestStatementDocumentHTML(t *testing.T) {
	a := NewStatementAdapter(nil)
	lh := &ioengine.Letterhead{CompanyName: "Olex Films İstanbul", Address: "Kadıköy", PrimaryColor: "#123456"}
	ds := StatementDataset(sampleStatement(), i18n.LocaleAR)
	out, err := a.DocumentHTML(ds, "ar", lh, ioengine.ExportTitle("ar", ResourceCariStatement))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`dir="rtl"`, `lang="ar"`, "Olex Films İstanbul", "كشف حساب", "مدين", "دائن", "1600.00 TRY", "--primary:#123456"} {
		if !strings.Contains(out, want) {
			t.Errorf("ar document lacks %q", want)
		}
	}
	if strings.Contains(out, "<b>x</b>") || strings.Contains(out, "Olex <Center>") {
		t.Error("values are not escaped")
	}
	en, _ := a.DocumentHTML(StatementDataset(sampleStatement(), i18n.LocaleEN), "en", nil, "Account statement")
	if !strings.Contains(en, `dir="ltr"`) || strings.Contains(en, `<header class="doc-header">`) {
		t.Error("en document without letterhead is wrong")
	}
}

func TestBalancesDataset(t *testing.T) {
	asOf := "2026-09-30"
	rep := BalanceReport{
		Organization: Ref{Name: "Olex"}, Currency: "TRY", AsOf: &asOf,
		Accounts: []AccountBalance{{Type: "cash", Name: "Kasa 1", Currency: "TRY", Balance: "10.00"}},
		Cari:     []CariBalance{{Counterparty: Counterparty{Name: "Dist"}, Currency: "TRY", Balance: "-5.00"}},
		Totals:   BalanceTotals{Cash: "10.00", Bank: "0.00", Receivable: "0.00", Payable: "5.00"},
	}
	ds := BalancesDataset(rep, i18n.LocaleEN)
	if len(ds.Rows) != 6 || ds.Rows[0]["kind"] != "Cash" || ds.Rows[1]["kind"] != "Current account" {
		t.Fatalf("rows = %v", ds.Rows)
	}
	html, err := NewBalancesAdapter(nil).DocumentHTML(ds, "en", nil, "Balance report")
	if err != nil || !strings.Contains(html, "Total payables") || !strings.Contains(html, "5.00 TRY") {
		t.Fatalf("balances html: %v", err)
	}
}

// TEC-521: the generated-at line of the statement and balance PDFs is
// printed in the requester's zone (export job), else the book
// organization's, else Europe/Istanbul; never UTC.
func TestAccountingPDFIssuedAtTimezone(t *testing.T) {
	at := time.Date(2026, 10, 9, 11, 32, 0, 0, time.UTC)
	for _, tc := range []struct{ name, job, org, want string }{
		{"default", "", "", "2026-10-09 14:32 (Europe/Istanbul)"},
		{"organization", "", "Asia/Baku", "2026-10-09 15:32 (Asia/Baku)"},
		{"requester", "America/New_York", "Asia/Baku", "2026-10-09 07:32 (America/New_York)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := sampleStatement()
			st.GeneratedAt, st.Timezone = at, tc.org
			ds := StatementDataset(st, i18n.LocaleTR)
			ds.Timezone = tc.job
			out, err := NewStatementAdapter(nil).DocumentHTML(ds, "tr", nil, "Ekstre")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tc.want) || strings.Contains(out, " UTC") {
				t.Fatalf("statement generated-at is not %q", tc.want)
			}
			rep := BalanceReport{Organization: Ref{Name: "Olex"}, Currency: "TRY", GeneratedAt: at, Timezone: tc.org}
			ds = BalancesDataset(rep, i18n.LocaleTR)
			ds.Timezone = tc.job
			out, err = NewBalancesAdapter(nil).DocumentHTML(ds, "tr", nil, "Bakiye")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tc.want) || strings.Contains(out, " UTC") {
				t.Fatalf("balances generated-at is not %q", tc.want)
			}
		})
	}
}
