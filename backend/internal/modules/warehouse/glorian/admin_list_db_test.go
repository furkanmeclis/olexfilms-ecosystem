package glorian_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-367: the Glorian admin list queries (sync runs and order outbounds)
// sort, filter, page and count in SQL (docs/list-contract.md), in the pull
// fixture's rolled-back transaction.

func tstz(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func TestAdminListSyncRunsSortFilterPage(t *testing.T) {
	f := newPullFixture(t, true)
	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	var ids []int64
	for i, row := range []struct {
		kind, status string
		at           time.Time
	}{
		{glorian.KindPullProducts, glorian.RunSucceeded, base},
		{glorian.KindReconcile, glorian.RunFailed, base.Add(time.Hour)},
		{glorian.KindPullStock, glorian.RunRunning, base.AddDate(0, 0, 1)},
	} {
		var id int64
		finished := pgtype.Timestamptz{}
		if row.status != glorian.RunRunning {
			finished = tstz(row.at.Add(time.Minute))
		}
		if err := f.tx.QueryRow(f.ctx, `INSERT INTO integration_sync_runs
			(organization_id, brand_id, connection_id, kind, status, started_at, finished_at, counts)
			VALUES ($1, $2, $3, $4, $5, $6, $7, '{}') RETURNING id`,
			f.conn.OrganizationID, f.conn.BrandID, f.conn.ID, row.kind, row.status, row.at, finished).Scan(&id); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		ids = append(ids, id)
	}
	list := func(p db.ListGlorianSyncRunsParams) []int64 {
		t.Helper()
		p.ConnectionID = f.conn.ID
		if p.RowLimit == 0 {
			p.RowLimit = 50
		}
		rows, err := f.q.ListGlorianSyncRuns(f.ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]int64, 0, len(rows))
		for _, r := range rows {
			out = append(out, r.ID)
		}
		return out
	}
	if got := list(db.ListGlorianSyncRunsParams{SortKey: "started_at", SortDesc: true}); !slices.Equal(got, []int64{ids[2], ids[1], ids[0]}) {
		t.Fatalf("-started_at = %v", got)
	}
	if got := list(db.ListGlorianSyncRunsParams{SortKey: "started_at"}); !slices.Equal(got, []int64{ids[0], ids[1], ids[2]}) {
		t.Fatalf("started_at = %v", got)
	}
	// finished_at: the running run (NULL) is last in both directions.
	if got := list(db.ListGlorianSyncRunsParams{SortKey: "finished_at", SortDesc: true}); !slices.Equal(got, []int64{ids[1], ids[0], ids[2]}) {
		t.Fatalf("-finished_at = %v", got)
	}
	if got := list(db.ListGlorianSyncRunsParams{SortKey: "kind"}); !slices.Equal(got, []int64{ids[0], ids[2], ids[1]}) {
		t.Fatalf("kind = %v", got)
	}
	multi := db.ListGlorianSyncRunsParams{Kinds: []string{glorian.KindPullProducts, glorian.KindReconcile}, SortKey: "started_at"}
	if got := list(multi); !slices.Equal(got, []int64{ids[0], ids[1]}) {
		t.Fatalf("kinds = %v", got)
	}
	day := db.ListGlorianSyncRunsParams{StartedFrom: tstz(base.Truncate(24 * time.Hour)), StartedBefore: tstz(base.Truncate(24*time.Hour).AddDate(0, 0, 1)), SortKey: "started_at"}
	if got := list(day); !slices.Equal(got, []int64{ids[0], ids[1]}) {
		t.Fatalf("started range = %v", got)
	}
	paged := db.ListGlorianSyncRunsParams{Statuses: []string{glorian.RunSucceeded, glorian.RunFailed, glorian.RunRunning}, SortKey: "started_at", RowLimit: 1, RowOffset: 1}
	if got := list(paged); !slices.Equal(got, []int64{ids[1]}) {
		t.Fatalf("offset page = %v", got)
	}
	total, err := f.q.CountGlorianSyncRuns(f.ctx, db.CountGlorianSyncRunsParams{
		ConnectionID: f.conn.ID, Statuses: []string{glorian.RunSucceeded, glorian.RunFailed},
	})
	if err != nil || total != 2 {
		t.Fatalf("count = %d, %v", total, err)
	}
}

func TestAdminListOutboundsAllStatesSearchSort(t *testing.T) {
	f := newPullFixture(t, true)
	buyer := f.distributor(t, f.glorian, f.phone(1))
	brand, err := f.q.GetBrandByID(f.ctx, f.glorian.BrandID)
	if err != nil {
		t.Fatal(err)
	}
	type ob struct {
		id      int64
		orderNo string
	}
	var obs []ob
	for i, row := range []struct {
		state, held string
		attempts    int
	}{
		{glorian.OutboundHeld, glorian.HeldMissingCustomer, 2},
		{glorian.OutboundSent, "", 1},
		{glorian.OutboundFailed, "", 5},
	} {
		o, err := f.q.CreateOrder(f.ctx, db.CreateOrderParams{
			SellerOrgID: f.glorian.ID, BrandID: f.glorian.BrandID, BuyerOrgID: buyer.ID, Currency: brand.Currency,
		})
		if err != nil {
			t.Fatalf("order %d: %v", i, err)
		}
		var id int64
		held := pgtype.Text{String: row.held, Valid: row.held != ""}
		if err := f.tx.QueryRow(f.ctx, `INSERT INTO order_outbounds
			(organization_id, brand_id, order_id, connection_id, external_reference, state, held_reason, attempts, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW() - make_interval(days => $9::int)) RETURNING id`,
			f.conn.OrganizationID, f.conn.BrandID, o.ID, f.conn.ID, fmt.Sprintf("T367-%s-%d", f.suffix, i),
			row.state, held, row.attempts, i).Scan(&id); err != nil {
			t.Fatalf("outbound %d: %v", i, err)
		}
		obs = append(obs, ob{id: id, orderNo: o.OrderNo})
	}
	list := func(p db.ListGlorianOutboundsParams) []int64 {
		t.Helper()
		p.ConnectionID, p.RowLimit = f.conn.ID, 50
		if !p.Q.Valid {
			p.Q = pgtype.Text{String: "T367-" + f.suffix, Valid: true}
		}
		rows, err := f.q.ListGlorianOutbounds(f.ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]int64, 0, len(rows))
		for _, r := range rows {
			out = append(out, r.ID)
		}
		return out
	}
	// No state: every state, replay order (created_at, id).
	if got := list(db.ListGlorianOutboundsParams{SortKey: "created_at"}); !slices.Equal(got, []int64{obs[0].id, obs[1].id, obs[2].id}) {
		t.Fatalf("all states = %v", got)
	}
	if got := list(db.ListGlorianOutboundsParams{States: []string{glorian.OutboundHeld, glorian.OutboundFailed}, SortKey: "attempts", SortDesc: true}); !slices.Equal(got, []int64{obs[2].id, obs[0].id}) {
		t.Fatalf("held+failed -attempts = %v", got)
	}
	if got := list(db.ListGlorianOutboundsParams{SortKey: "updated_at"}); !slices.Equal(got, []int64{obs[2].id, obs[1].id, obs[0].id}) {
		t.Fatalf("updated_at = %v", got)
	}
	// q matches the order number too.
	if got := list(db.ListGlorianOutboundsParams{Q: pgtype.Text{String: obs[1].orderNo, Valid: true}, SortKey: "created_at"}); !slices.Contains(got, obs[1].id) {
		t.Fatalf("q order_no = %v", got)
	}
	total, err := f.q.CountGlorianOutbounds(f.ctx, db.CountGlorianOutboundsParams{
		ConnectionID: f.conn.ID, Q: pgtype.Text{String: "T367-" + f.suffix, Valid: true},
		States: []string{glorian.OutboundSent},
	})
	if err != nil || total != 1 {
		t.Fatalf("count = %d, %v", total, err)
	}
}
