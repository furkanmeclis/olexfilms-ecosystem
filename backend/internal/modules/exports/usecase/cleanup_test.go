package usecase

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-1259: the hourly cleanup deletes the file of an expired export job and
// marks it expired; a job that has not expired keeps its file; an expired
// file is not served before the sweep.
func TestCleanupExpiredDeletesFiles(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	q := db.New(tx)
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "Export", Surname: "T1259", Status: "active",
		PhoneE164: pgtype.Text{String: "+905551259001", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := storage.NewMemory()
	job := func(key string, expires time.Time) db.ExportJob {
		t.Helper()
		j, err := q.CreateExportJob(ctx, db.CreateExportJobParams{
			Resource: "customers.data_export", ActorID: user.ID, Format: "json", QueryJson: []byte("{}"), Locale: "tr",
			ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Upload(ctx, storage.File{Body: bytes.NewReader([]byte("{}")), Size: 2, ContentType: "application/json"}, key); err != nil {
			t.Fatal(err)
		}
		j, err = q.MarkExportJobCompleted(ctx, db.MarkExportJobCompletedParams{ID: j.ID, FileKey: pgtype.Text{String: key, Valid: true}, RowCount: 1})
		if err != nil {
			t.Fatal(err)
		}
		return j
	}
	old := job("exports/t1259-old.json", time.Now().Add(-time.Hour))
	fresh := job("exports/t1259-fresh.json", time.Now().Add(time.Hour))
	svc := New(q, store, nil, nil, nil, nil, nil)

	if _, _, _, err := svc.openExportFile(ctx, old); err != ErrNotFound {
		t.Fatalf("expired file served before cleanup: %v", err)
	}
	n, err := svc.CleanupExpired(ctx)
	if err != nil || n < 1 {
		t.Fatalf("cleanup = %d, %v", n, err)
	}
	if ok, _ := store.Exists(ctx, "exports/t1259-old.json"); ok {
		t.Fatal("expired export file still in storage")
	}
	got, err := q.GetExportJobByID(ctx, old.ID)
	if err != nil || got.Status != "expired" || got.FileKey.Valid {
		t.Fatalf("expired job = %s %v, %v", got.Status, got.FileKey, err)
	}
	if ok, _ := store.Exists(ctx, "exports/t1259-fresh.json"); !ok {
		t.Fatal("unexpired export file deleted")
	}
	if got, _ := q.GetExportJobByID(ctx, fresh.ID); got.Status != "completed" {
		t.Fatalf("unexpired job status = %s", got.Status)
	}
	// Idempotent: nothing of ours is left to expire.
	if _, err := svc.CleanupExpired(ctx); err != nil {
		t.Fatal(err)
	}
}
