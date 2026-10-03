package migrator

import (
	"database/sql"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
)

// The TEC-261 step queries stay inside the read-only guard.
func TestOrderQueriesPassGuard(t *testing.T) {
	for _, q := range []string{hubOrdersQuery, hubOrderItemsQuery, hubOrderStockQuery, hubDealerCodesQuery,
		whOrdersQuery, whOrderItemsQuery, whOrderBarcodesQuery} {
		if err := source.CheckReadOnly(q); err != nil {
			t.Errorf("guard refused %q: %v", q, err)
		}
	}
}

// Every legacy status (hub OrderStatusEnum, warehouse OrderStatus) maps to a
// status of chk_orders_status.
func TestOrderStatusMaps(t *testing.T) {
	valid := map[string]bool{
		"draft": true, "submitted": true, "approved": true, "preparing": true, "ready": true, "processing": true,
		"shipped": true, "delivered": true, "received": true, "cancelling": true, "cancelled": true,
	}
	for _, legacy := range []string{"pending", "processing", "shipped", "delivered", "cancelled"} {
		if got := mapOrderStatus(hubOrderStatuses, legacy); !valid[got] {
			t.Errorf("hub %s -> %q", legacy, got)
		}
	}
	for _, legacy := range []string{"draft", "preparing", "ready", "processing", "shipped", "delivered", "received", "cancelled"} {
		if got := mapOrderStatus(whOrderStatuses, legacy); !valid[got] {
			t.Errorf("wh %s -> %q", legacy, got)
		}
	}
	if got := mapOrderStatus(hubOrderStatuses, " Delivered "); got != orderReceived {
		t.Errorf("hub delivered -> %q, want received", got)
	}
	if got := mapOrderStatus(whOrderStatuses, "pending"); got != "" {
		t.Errorf("wh pending -> %q, want unknown", got)
	}
}

func nt(s string) sql.NullTime {
	tm, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		panic(err)
	}
	return sql.NullTime{Time: tm, Valid: true}
}

func TestResolveOrderState(t *testing.T) {
	// Hub only, delivered: received, rate frozen at creation.
	st, _, ok := resolveOrderState(&orderCandidate{Hub: &hubOrder{Status: "delivered",
		CreatedAt: nt("2025-03-04 09:00"), UpdatedAt: nt("2025-03-05 09:00")}})
	if !ok || st.Status != orderReceived || st.Approved != nt("2025-03-04 09:00") ||
		st.Received != nt("2025-03-05 09:00") || st.Shipped != nt("2025-03-05 09:00") || st.Cancelled.Valid {
		t.Errorf("hub delivered = %+v", st)
	}
	// Both sides: the more recently updated warehouse decides.
	cand := &orderCandidate{
		Hub: &hubOrder{Status: "processing", CreatedAt: nt("2026-07-24 07:30"), UpdatedAt: nt("2026-07-24 07:30")},
		WH: &whOrder{Status: "shipped", ConfirmedAt: nt("2026-07-24 08:00"), ShippedAt: nt("2026-07-24 09:00"),
			CreatedAt: nt("2026-07-24 07:00"), UpdatedAt: nt("2026-07-24 09:00")},
	}
	st, _, ok = resolveOrderState(cand)
	if !ok || st.Status != orderShipped || st.Created != nt("2026-07-24 07:00") ||
		st.Approved != nt("2026-07-24 08:00") || st.Shipped != nt("2026-07-24 09:00") {
		t.Errorf("merged = %+v", st)
	}
	// Hub newer: hub decides.
	cand.Hub.UpdatedAt = nt("2026-07-25 09:00")
	cand.Hub.Status = "cancelled"
	st, _, _ = resolveOrderState(cand)
	if st.Status != orderCancelled || st.Cancelled != nt("2026-07-25 09:00") || st.Received.Valid {
		t.Errorf("hub newer = %+v", st)
	}
	// Draft: nothing submitted or approved.
	st, _, _ = resolveOrderState(&orderCandidate{WH: &whOrder{Status: "draft", CreatedAt: nt("2026-07-25 07:00")}})
	if st.Status != orderDraft || st.Submitted.Valid || st.Approved.Valid {
		t.Errorf("draft = %+v", st)
	}
	if _, reason, ok := resolveOrderState(&orderCandidate{Hub: &hubOrder{Status: "lost"}}); ok || reason != "hub:lost" {
		t.Errorf("unknown status = %q, %v", reason, ok)
	}
}

func TestWHDealerID(t *testing.T) {
	codes := map[string]int64{"SYN00002": 2}
	if id := whDealerID(&whOrder{CustSource: "olexfilms", CustExternalID: "7"}, codes); id != 7 {
		t.Errorf("external id = %d", id)
	}
	if id := whDealerID(&whOrder{CustSource: "glorian", CustExternalID: "7", CustDealerCode: "syn00002"}, codes); id != 2 {
		t.Errorf("dealer code = %d", id)
	}
	if id := whDealerID(&whOrder{CustSource: "glorian", CustExternalID: "7"}, codes); id != 0 {
		t.Errorf("glorian customer = %d", id)
	}
}

func TestOrderTrackingAndNote(t *testing.T) {
	tr, note := orderTrackingAndNote(&orderCandidate{
		Hub: &hubOrder{Tracking: "H1", Notes: "Not", Cargo: "Kargo A"},
		WH:  &whOrder{Tracking: " ", Notes: "Not"},
	})
	if tr != "H1" || note != "Not\nKargo: Kargo A" {
		t.Errorf("tracking %q note %q", tr, note)
	}
}
