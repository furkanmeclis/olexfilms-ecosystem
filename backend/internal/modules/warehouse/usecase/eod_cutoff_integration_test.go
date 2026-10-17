package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
)

// TEC-769 against a migrated PostgreSQL: the organization's end-of-day
// cutoff (warehouse_settings) moves the report period to
// [D-1 cutoff, D cutoff) in the organization's zone and lets the cron close
// day D once the cutoff passed; clearing it restores the calendar day.
func TestEODCutoffRun(t *testing.T) {
	e := newEntryEnv(t)
	dc := e.caller(e.dist)
	eod := wh.NewEOD(e.pool, e.q)

	// Settings: default none, HH:MM round trip, invalid refused, 00:00 clears.
	st, err := eod.Settings(e.ctx, dc.Caller)
	if err != nil || st.EODCutoff != nil || st.Timezone != "Europe/Istanbul" {
		t.Fatalf("settings = %+v, %v", st, err)
	}
	bad := "25:00"
	var ve *wh.ValidationError
	if _, err := eod.UpdateSettings(e.ctx, wh.EODCaller{Caller: dc.Caller}, wh.WarehouseSettingsInput{EODCutoff: &bad}); !errors.As(err, &ve) {
		t.Fatalf("invalid cutoff = %v", err)
	}
	cut := "18:00"
	if st, err = eod.UpdateSettings(e.ctx, wh.EODCaller{Caller: dc.Caller}, wh.WarehouseSettingsInput{EODCutoff: &cut}); err != nil || st.EODCutoff == nil || *st.EODCutoff != "18:00" {
		t.Fatalf("set cutoff = %+v, %v", st, err)
	}

	w, loc := e.location(t, e.center, "K")
	units := e.enterUnits(t, w, loc, 1, "K")
	from := ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: e.locationID(t, loc), OrgID: e.center.ID}
	dist := ledger.Owner{Type: ledger.OwnerOrganization, ID: e.dist.ID, OrgID: e.dist.ID}
	ref := time.Now().UnixNano()
	e.post(t, ledger.Movement{Type: ledger.TypeTransferOut, UnitID: units[0], From: &from, To: &dist,
		Source: "t769", RefType: "t769_transfer", RefID: ref, Reason: "t769 transfer"})
	e.post(t, ledger.Movement{Type: ledger.TypeTransferIn, UnitID: units[0], To: &dist,
		Source: "t769", RefType: "t769_transfer", RefID: ref, Reason: "t769 transfer in"})
	moved := time.Now()

	ist, _ := time.LoadLocation("Europe/Istanbul")
	cutoff := 18 * 60
	open := wh.EODOpenDay(moved, ist, &cutoff)
	_, closeAt := wh.EODPeriod(open, ist, &cutoff)

	// One second before the close the movement's day is still open: the
	// run writes the day before (no movements of this organization).
	eod.SetClock(func() time.Time { return closeAt.Add(-time.Second) })
	if n, err := eod.RunOrganization(e.ctx, e.dist.ID); err != nil || n != 1 {
		t.Fatalf("run before close: %d, %v (system report of the previous day)", n, err)
	}
	list, _, err := eod.List(e.ctx, dc.Caller, wh.EODListInput{Scope: wh.EODScopeSystem})
	if err != nil || len(list) != 1 || list[0].ReportDate != open.AddDate(0, 0, -1).Format("2006-01-02") ||
		list[0].Summary.Totals.MovementCount != 0 {
		t.Fatalf("previous day = %+v, %v", list, err)
	}

	// At the close the day is written with the 18:00 period.
	eod.SetClock(func() time.Time { return closeAt })
	if n, err := eod.RunOrganization(e.ctx, e.dist.ID); err != nil || n != 1 {
		t.Fatalf("run at close: %d, %v", n, err)
	}
	list, _, err = eod.List(e.ctx, dc.Caller, wh.EODListInput{Scope: wh.EODScopeSystem, DateFrom: open.Format("2006-01-02")})
	if err != nil || len(list) != 1 {
		t.Fatalf("closed day list = %+v, %v", list, err)
	}
	day := list[0]
	if day.ReportDate != open.Format("2006-01-02") || !day.PeriodEnd.Equal(closeAt) ||
		day.PeriodEnd.Sub(day.PeriodStart) < 23*time.Hour || day.PeriodEnd.In(ist).Hour() != 18 {
		t.Fatalf("closed day = %s %s..%s", day.ReportDate, day.PeriodStart, day.PeriodEnd)
	}
	if day.Summary.Totals.MovementCount != 2 {
		t.Fatalf("closed day totals = %+v", day.Summary.Totals)
	}

	// A manual run without a date reports the open business day: after
	// the close that is the next day.
	manual, err := eod.Generate(e.ctx, wh.EODCaller{Caller: dc.Caller}, wh.EODGenerateInput{})
	if err != nil || manual.ReportDate != open.AddDate(0, 0, 1).Format("2006-01-02") || !manual.PeriodStart.Equal(closeAt) {
		t.Fatalf("manual open day = %+v, %v", manual, err)
	}
	if _, err := eod.Generate(e.ctx, wh.EODCaller{Caller: dc.Caller}, wh.EODGenerateInput{Date: open.AddDate(0, 0, 2).Format("2006-01-02")}); !errors.As(err, &ve) {
		t.Fatalf("future business day = %v", err)
	}

	// Clearing the cutoff restores the calendar day.
	clear := "00:00"
	if st, err = eod.UpdateSettings(e.ctx, wh.EODCaller{Caller: dc.Caller}, wh.WarehouseSettingsInput{EODCutoff: &clear}); err != nil || st.EODCutoff != nil {
		t.Fatalf("clear cutoff = %+v, %v", st, err)
	}
	cal, err := eod.Generate(e.ctx, wh.EODCaller{Caller: dc.Caller}, wh.EODGenerateInput{Date: open.Format("2006-01-02")})
	if err != nil || cal.PeriodStart.In(ist).Hour() != 0 || cal.PeriodEnd.Sub(cal.PeriodStart) != 24*time.Hour {
		t.Fatalf("calendar day = %+v, %v", cal, err)
	}
}
