package migrator

import (
	"database/sql"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
)

// TEC-260: the step queries pass the read-only guard.
func TestWarrantiesQueriesAreReadOnly(t *testing.T) {
	for _, q := range []string{legacyWarrantiesQuery, legacyTransfersQuery} {
		if err := source.CheckReadOnly(q); err != nil {
			t.Errorf("%v: %s", err, q)
		}
	}
}

func day(s string) sql.NullTime {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return sql.NullTime{Time: t, Valid: true}
}

// The duplicate rule: one warranty per (service, stock item), the earliest
// start kept (then the lowest id), the latest end, active when any is.
func TestGroupLegacyWarranties(t *testing.T) {
	list := []legacyWarranty{
		{ID: 1, ServiceID: 1, StockItemID: 3, Start: day("2025-03-10"), End: day("2030-03-10"), Active: false},
		{ID: 2, ServiceID: 1, StockItemID: 3, Start: day("2025-03-10"), End: day("2030-03-10"), Active: true},
		{ID: 3, ServiceID: 2, StockItemID: 5, Start: day("2025-03-15"), End: day("2030-03-15"), Active: true},
		// A later row with an earlier start wins; its end is extended.
		{ID: 4, ServiceID: 9, StockItemID: 7, Start: day("2025-05-01"), End: day("2027-05-01"), Active: true},
		{ID: 5, ServiceID: 9, StockItemID: 7, Start: day("2025-01-01"), End: day("2026-01-01"), Active: false},
		{ID: 6, ServiceID: 9, StockItemID: 8, Start: day("2024-01-01"), End: day("2025-01-01"), Active: false},
		{ID: 7, ServiceID: 9, StockItemID: 9, Start: sql.NullTime{}, End: day("2025-01-01")},
	}
	groups, undated := groupLegacyWarranties(list)
	if len(undated) != 1 || undated[0].ID != 7 {
		t.Fatalf("undated = %+v", undated)
	}
	if len(groups) != 4 {
		t.Fatalf("groups = %d, want 4", len(groups))
	}
	// Start date order: 6 (2024-01-01), 5 (2025-01-01), 1 (2025-03-10), 3.
	wantKept := []int64{6, 5, 1, 3}
	for i, g := range groups {
		if g.Kept.ID != wantKept[i] {
			t.Errorf("group %d kept %d, want %d", i, g.Kept.ID, wantKept[i])
		}
	}
	g := groups[2]
	if len(g.All) != 2 || !g.Active || dayNumber(g.Start) != 20250310 {
		t.Errorf("duplicate pair = %+v", g)
	}
	g = groups[1]
	if dayNumber(g.Start) != 20250101 || dayNumber(g.End) != 20270501 || !g.Active || len(g.All) != 2 {
		t.Errorf("merged group = start %v end %v active %v", g.Start, g.End, g.Active)
	}
}

func TestLegacyWarrantyPeriod(t *testing.T) {
	ist, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		t.Skip(err)
	}
	start, end := legacyWarrantyPeriod(day("2025-03-10").Time, day("2030-03-10").Time, ist)
	if want := time.Date(2025, 3, 10, 0, 0, 0, 0, ist); !start.Equal(want) {
		t.Errorf("start = %v, want %v", start, want)
	}
	if want := time.Date(2030, 3, 11, 0, 0, 0, 0, ist).Add(-time.Microsecond); !end.Equal(want) {
		t.Errorf("end = %v, want %v", end, want)
	}
	// A one-day warranty still has end > start.
	s, e := legacyWarrantyPeriod(day("2025-03-10").Time, day("2025-03-10").Time, ist)
	if !e.After(s) {
		t.Error("same-day period is empty")
	}
}

func TestLegacyWarrantyState(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	upd := sql.NullTime{Time: now.AddDate(0, -1, 0), Valid: true}
	cases := []struct {
		name         string
		active       bool
		end          time.Time
		status       string
		n30, n7, exp bool
	}{
		{"past end", true, now.Add(-time.Hour), warrantyExpired, false, false, true},
		{"inactive past end", false, now.AddDate(-1, 0, 0), warrantyExpired, false, false, true},
		{"deactivated early", false, now.AddDate(1, 0, 0), warrantyVoid, false, false, false},
		{"far end", true, now.AddDate(4, 0, 0), warrantyActive, false, false, false},
		{"in 30 day window", true, now.AddDate(0, 0, 20), warrantyActive, true, false, false},
		{"in 7 day window", true, now.AddDate(0, 0, 3), warrantyActive, true, true, false},
	}
	for _, tc := range cases {
		st := legacyWarrantyState(tc.active, tc.end, now, upd)
		if st.Status != tc.status || st.Notified30.Valid != tc.n30 || st.Notified7.Valid != tc.n7 || st.ExpiredAt.Valid != tc.exp {
			t.Errorf("%s: %+v", tc.name, st)
		}
		if (st.Status == warrantyVoid) != (st.VoidedAt.Valid && st.VoidReason.Valid) {
			t.Errorf("%s: void fields %+v", tc.name, st)
		}
		if st.Status == warrantyExpired && !st.ExpiredAt.Time.Equal(tc.end) {
			t.Errorf("%s: expired_at %v, want the end", tc.name, st.ExpiredAt.Time)
		}
	}
}

func TestLegacyTransferTimes(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	tr := legacyTransfer{CreatedAt: sql.NullTime{Time: now.Add(-2 * time.Hour), Valid: true},
		TransferredAt: sql.NullTime{Time: now.Add(-time.Hour), Valid: true}}
	created, completed, expires := legacyTransferTimes(tr, now)
	if !created.Equal(now.Add(-2*time.Hour)) || !completed.Equal(now.Add(-time.Hour)) || !expires.Equal(completed) {
		t.Errorf("times = %v %v %v", created, completed, expires)
	}
	created, completed, expires = legacyTransferTimes(legacyTransfer{}, now)
	if !created.Equal(now) || !completed.Equal(now) || !expires.After(created) {
		t.Errorf("no times = %v %v %v", created, completed, expires)
	}
	a := legacyTransfer{ID: 1, TransferredAt: sql.NullTime{Time: now, Valid: true}}
	b := legacyTransfer{ID: 2, TransferredAt: sql.NullTime{Time: now.Add(-time.Hour), Valid: true}}
	if !transferAfter(a, b) || transferAfter(b, a) {
		t.Error("transfer order by completion time")
	}
}
