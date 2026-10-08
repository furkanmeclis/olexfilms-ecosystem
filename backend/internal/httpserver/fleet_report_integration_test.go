package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// TEC-476 (HTTP): POST /v1/fleets/{uuid}/reports. A linked dealer asks for
// the report of a closed month: 202 pending and one generation task on the
// docs queue; the same period again keeps the single report; an open
// period is 400, an unlinked dealer 404, the module off 403.
func TestIntegrationFleetReportRequest(t *testing.T) {
	mr := miniredis.RunT(t)
	qc := queue.NewClient(config.RedisConfig{Addr: mr.Addr()})
	t.Cleanup(func() { _ = qc.Close() })
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = storage.NewMemory(); d.Queue = qc })
	ctx := context.Background()
	n := it.fleetNet("t476")
	it.enableFleet(n.dealerA, n.dealerB)
	opened := decodeData[fleetView](t, it.fleetDo("POST", "/v1/fleets", n.tokA, map[string]any{
		"legal_name": "T476 Rapor Filo", "tax_number": tecVKN(t, it.suffix), "report_locale": "en",
	}, http.StatusCreated))
	base := "/v1/fleets/" + opened.UUID + "/reports"
	ist, _ := time.LoadLocation("Europe/Istanbul")
	now := time.Now().In(ist)
	closed := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)

	type report struct {
		UUID        string `json:"uuid"`
		PeriodKind  string `json:"period_kind"`
		PeriodStart string `json:"period_start"`
		PeriodEnd   string `json:"period_end"`
		Locale      string `json:"locale"`
		Status      string `json:"status"`
	}
	body := map[string]any{"period_kind": "monthly", "period": closed.Format("2006-01")}
	r := decodeData[report](t, it.fleetDo("POST", base, n.tokA, body, http.StatusAccepted))
	if r.Status != "pending" || r.PeriodKind != "monthly" || r.Locale != "en" ||
		r.PeriodStart != closed.Format("2006-01")+"-01" || r.PeriodEnd != closed.Format(time.DateOnly) {
		t.Fatalf("report = %+v", r)
	}
	row, err := it.q.GetFleetReportByUUID(ctx, uuid.MustParse(r.UUID))
	if err != nil {
		t.Fatal(err)
	}
	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = insp.Close() })
	info, err := insp.GetTaskInfo(queue.QueueDocs, fmt.Sprintf("fleet-report-%d", row.ID))
	if err != nil || info.Type != queue.TaskFleetReportGenerate || info.MaxRetry != 3 {
		t.Fatalf("generation task = %+v, %v", info, err)
	}
	again := decodeData[report](t, it.fleetDo("POST", base, n.tokA, body, http.StatusAccepted))
	if again.UUID != r.UUID {
		t.Fatalf("same period opened a second report: %s", again.UUID)
	}

	it.fleetDo("POST", base, n.tokA, map[string]any{"period_kind": "monthly", "period": now.Format("2006-01")}, http.StatusBadRequest)
	it.fleetDo("POST", base, n.tokA, map[string]any{"period_kind": "yearly", "period": "2025"}, http.StatusBadRequest)
	it.fleetDo("POST", base, n.tokB, body, http.StatusNotFound)
	if _, err := it.srv.features.ClearByAdmin(ctx, n.dealerA.ID, "fleet"); err != nil {
		t.Fatal(err)
	}
	if env := it.fleetDo("POST", base, n.tokA, body, http.StatusForbidden); errCode(env) != "FEATURE_DISABLED" {
		t.Fatalf("module off: %s", errCode(env))
	}
}
