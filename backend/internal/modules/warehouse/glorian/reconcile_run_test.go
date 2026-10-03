package glorian_test

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TEC-273: the admin endpoint opens the run (StartRun) and the
// glorian:reconcile task finishes it; a redelivered task skips the
// finished run.

func (q *reconcileQuerier) GetIntegrationSyncRunByID(_ context.Context, id int64) (db.IntegrationSyncRun, error) {
	if id < 1 || int(id) > len(q.started) {
		return db.IntegrationSyncRun{}, pgx.ErrNoRows
	}
	status := glorian.RunRunning
	for _, f := range q.finished {
		if f.ID == id {
			status = f.Status
		}
	}
	return db.IntegrationSyncRun{ID: id, Uuid: uuid.New(), ConnectionID: q.conn.ID, Kind: q.started[id-1].Kind, Status: status}, nil
}

func (q *reconcileQuerier) GetIntegrationConnectionByID(_ context.Context, id int64) (db.IntegrationConnection, error) {
	if id != q.conn.ID {
		return db.IntegrationConnection{}, pgx.ErrNoRows
	}
	return q.conn, nil
}

func TestReconcileStartRunThenTask(t *testing.T) {
	e := newReconcileEnv(t, true)
	e.inSync()
	run, err := e.r.StartRun(context.Background(), e.q.conn)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if run.Status != glorian.RunRunning || len(e.q.finished) != 0 || len(e.srv.Requests()) != 0 {
		t.Fatalf("start must only open the run: %+v finished=%d requests=%d", run, len(e.q.finished), len(e.srv.Requests()))
	}
	if err := e.r.ReconcileTask(context.Background(), run.ID); err != nil {
		t.Fatalf("task: %v", err)
	}
	if c := e.runCounts(t); e.q.finished[0].ID != run.ID || e.q.finished[0].Status != glorian.RunSucceeded || c.Remote != 4 {
		t.Fatalf("finished = %+v counts = %+v", e.q.finished, c)
	}
	// Redelivery: the run is finished, nothing more happens.
	requests := len(e.srv.Requests())
	if err := e.r.ReconcileTask(context.Background(), run.ID); err != nil {
		t.Fatalf("redelivered task: %v", err)
	}
	if len(e.q.finished) != 1 || len(e.srv.Requests()) != requests {
		t.Fatal("a finished run must not be reconciled again")
	}
	if _, err := e.r.ReconcileRun(context.Background(), 99); !errors.Is(err, glorian.ErrRunNotRunnable) {
		t.Fatalf("missing run err = %v", err)
	}
}
