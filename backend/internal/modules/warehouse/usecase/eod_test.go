package usecase

import (
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/google/uuid"
)

// Every postable ledger type (and reclassification) has an explicit group,
// so the report never hides a movement under the fallback.
func TestEODGroupCoversLedgerTypes(t *testing.T) {
	types := append([]ledger.MovementType{}, ledger.PostableTypes...)
	types = append(types, "reclassification")
	for _, mt := range types {
		if _, ok := eodGroupOf[string(mt)]; !ok {
			t.Errorf("movement type %s has no end-of-day group", mt)
		}
	}
	if EODGroupOf("brand_new_type") != EODGroupAdjustment {
		t.Fatal("unknown types fall back to adjustment")
	}
}

func TestEODMetersRoundTrip(t *testing.T) {
	for in, want := range map[string]int64{"": 0, "0": 0, "12.50": 1250, "3": 300, "0.05": 5, "-1.25": -125, "7.1": 710} {
		got, err := parseCm(in)
		if err != nil || got != want {
			t.Errorf("parseCm(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := parseCm("x.10"); err == nil {
		t.Fatal("garbage meters must fail")
	}
	for in, want := range map[int64]string{0: "0.00", 1250: "12.50", 5: "0.05", -125: "-1.25"} {
		if got := formatCm(in); got != want {
			t.Errorf("formatCm(%d) = %q, want %q", in, got, want)
		}
	}
}

// The day is the organization's calendar day: Istanbul (UTC+3) starts at
// 21:00 UTC of the day before.
func TestEODDayUsesTimezone(t *testing.T) {
	loc, tz := zoneOf("Europe/Istanbul")
	if tz != "Europe/Istanbul" {
		t.Fatalf("tz = %s", tz)
	}
	start, end := EODDay(time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), loc)
	if !start.UTC().Equal(time.Date(2026, 10, 1, 21, 0, 0, 0, time.UTC)) || end.Sub(start) != 24*time.Hour {
		t.Fatalf("day = %s .. %s", start.UTC(), end.UTC())
	}
	if _, tz := zoneOf("Not/AZone"); tz != "UTC" {
		t.Fatalf("unknown zone falls back to UTC, got %s", tz)
	}
}

// The grouped query rows fold into type, group and grand totals.
func TestBuildEODSummary(t *testing.T) {
	p1, p2 := uuid.New(), uuid.New()
	rows := []db.SummarizeEODMovementsRow{
		{Type: "consumption", ProductUuid: p1, Sku: "A", ProductName: "Film A", MovementCount: 2, UnitCount: 2, QuantityOut: 2, MetersIn: "0.00", MetersOut: "0.00"},
		{Type: "entry", ProductUuid: p1, Sku: "A", ProductName: "Film A", MovementCount: 3, UnitCount: 3, QuantityIn: 3, MetersIn: "0.00", MetersOut: "0.00"},
		{Type: "entry", ProductUuid: p2, Sku: "B", ProductName: "Roll B", MovementCount: 1, UnitCount: 1, QuantityIn: 1, MetersIn: "15.00", MetersOut: "0.00"},
		{Type: "partial_consumption", ProductUuid: p2, Sku: "B", ProductName: "Roll B", MovementCount: 2, UnitCount: 1, MetersIn: "0.00", MetersOut: "2.75"},
		{Type: "transfer_out", ProductUuid: p1, Sku: "A", ProductName: "Film A", MovementCount: 1, UnitCount: 1, QuantityOut: 1, MetersIn: "0", MetersOut: "0"},
	}
	s, err := BuildEODSummary(rows)
	if err != nil {
		t.Fatal(err)
	}
	if s.Totals.MovementCount != 9 || s.Totals.QuantityIn != 4 || s.Totals.QuantityOut != 3 ||
		s.Totals.MetersIn != "15.00" || s.Totals.MetersOut != "2.75" {
		t.Fatalf("totals = %+v", s.Totals)
	}
	if len(s.Groups) != len(EODGroups) {
		t.Fatalf("groups = %d", len(s.Groups))
	}
	g := map[string]EODGroupTotal{}
	for _, x := range s.Groups {
		g[x.Group] = x
	}
	if e := g[EODGroupEntry]; e.MovementCount != 4 || e.UnitCount != 4 || e.QuantityIn != 4 || e.MetersIn != "15.00" {
		t.Fatalf("entry group = %+v", e)
	}
	if c := g[EODGroupConsumption]; c.MovementCount != 4 || c.QuantityOut != 2 || c.MetersOut != "2.75" {
		t.Fatalf("consumption group = %+v", c)
	}
	if tr := g[EODGroupTransfer]; tr.MovementCount != 1 || tr.QuantityOut != 1 {
		t.Fatalf("transfer group = %+v", tr)
	}
	if o := g[EODGroupOrder]; o.MovementCount != 0 || o.MetersIn != "0.00" {
		t.Fatalf("empty group = %+v", o)
	}
	if len(s.Types) != 4 || s.Types[1].Type != "entry" || s.Types[1].MovementCount != 4 {
		t.Fatalf("types = %+v", s.Types)
	}
	if len(s.Products) != 5 || s.Products[2].ProductUUID != p2 || s.Products[2].Group != EODGroupEntry {
		t.Fatalf("products = %+v", s.Products)
	}
	empty, err := BuildEODSummary(nil)
	if err != nil || empty.Totals.MovementCount != 0 || len(empty.Groups) != len(EODGroups) || empty.Products == nil {
		t.Fatalf("empty = %+v, %v", empty, err)
	}
}

// The PDF body escapes data, translates groups and mirrors for Arabic.
func TestEODPDFHTML(t *testing.T) {
	s, err := BuildEODSummary([]db.SummarizeEODMovementsRow{
		{Type: "entry", ProductUuid: uuid.New(), Sku: "S1", ProductName: "<b>Film</b>", MovementCount: 1, UnitCount: 1, QuantityIn: 1, MetersIn: "0", MetersOut: "0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 1, 21, 0, 0, 0, time.UTC)
	d := EODPDFDoc{Organization: "Olex & Co", Report: EODReport{
		ReportDate: "2026-10-02", Timezone: "Europe/Istanbul", PeriodStart: start, PeriodEnd: start.Add(24 * time.Hour),
		Kind: EODKindAuto, Summary: s, Warehouse: &EODWarehouseRef{Code: "MAIN", Name: "Main"},
	}}
	out := EODPDFHTML(d, i18n.LocaleTR, "Gün sonu raporu")
	for _, want := range []string{"&lt;b&gt;Film&lt;/b&gt;", "Olex &amp; Co", "Stok girişleri", "MAIN · Main", "2026-10-02 00:00", "Otomatik"} {
		if !strings.Contains(out, want) {
			t.Errorf("tr html misses %q", want)
		}
	}
	if strings.Contains(out, "<b>Film</b>") {
		t.Fatal("product name must be escaped")
	}
	d.Report.Warehouse = nil
	d.Report.Summary = EODSummary{Groups: []EODGroupTotal{}}
	ar := EODPDFHTML(d, i18n.LocaleAR, "x")
	if !strings.Contains(ar, `dir="rtl"`) || !strings.Contains(ar, i18n.Translate(i18n.LocaleAR, "warehouse.eod.empty")) ||
		!strings.Contains(ar, i18n.Translate(i18n.LocaleAR, "warehouse.eod.scope_system")) {
		t.Fatal("ar html must be rtl with the empty / system labels")
	}
}

// TEC-521: the generated-at line is printed in the requester's zone (export
// job), else the report's, else Europe/Istanbul; never UTC.
func TestEODPDFIssuedAtTimezone(t *testing.T) {
	start := time.Date(2026, 10, 8, 21, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ name, job, report, want string }{
		{"report", "", "Asia/Baku", "2026-10-09 15:32 (Asia/Baku)"},
		{"invalid report zone", "", "", "2026-10-09 14:32 (Europe/Istanbul)"},
		{"requester", "America/New_York", "Asia/Baku", "2026-10-09 07:32 (America/New_York)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := EODPDFDoc{Organization: "Olex", Timezone: tc.report, Report: EODReport{
				ReportDate: "2026-10-09", Timezone: tc.report, PeriodStart: start, PeriodEnd: start.Add(24 * time.Hour),
				Kind: EODKindAuto, Summary: EODSummary{Groups: []EODGroupTotal{}},
				GeneratedAt: time.Date(2026, 10, 9, 11, 32, 0, 0, time.UTC),
			}}
			ds := EODPDFDataset(d, i18n.LocaleTR)
			ds.Timezone = tc.job
			out, err := NewEODPDFAdapter(nil).DocumentHTML(ds, "tr", nil, "Gün sonu raporu")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, i18n.Translate(i18n.LocaleTR, "warehouse.eod.generated_at")+": "+tc.want) {
				t.Fatalf("generated-at is not %q", tc.want)
			}
		})
	}
}
