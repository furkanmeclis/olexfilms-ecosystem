package usecase_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp/fake"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp/wuzapi"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// gateway is a fake wuzapi: fake.Provider plus session calls.
type gateway struct {
	*fake.Provider
	connects int
	token    string
}

func (g *gateway) Connect(context.Context) error { g.connects++; return nil }
func (g *gateway) QR(context.Context) (string, error) {
	return "data:image/png;base64,QQ==", nil
}
func (g *gateway) PairPhone(context.Context, string) (string, error) { return "CODE", nil }
func (g *gateway) Logout(context.Context) error                      { return nil }
func (g *gateway) Configured() bool                                  { return true }
func (g *gateway) EnsureUser(context.Context, string) (wuzapi.User, bool, error) {
	return wuzapi.User{ID: "u", Token: "tok"}, true, nil
}
func (g *gateway) SetUserToken(t string)            { g.token = t }
func (g *gateway) UserToken() string                { return g.token }
func (g *gateway) SetWebhook(context.Context) error { return nil }
func (g *gateway) WebhookURL() string               { return "" }

type notes struct{ n []notifmodel.EnqueueInput }

func (n *notes) Enqueue(_ context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	n.n = append(n.n, in)
	return nil, nil
}

// PollStatus: a dropped websocket on a linked device reconnects; a session
// unlinked on the phone raises the logout alarm.
func TestPollStatus(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	// Singleton row shared with the httpserver integration tests.
	lockConn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockConn.Exec(ctx, `SELECT pg_advisory_lock(920092)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Do not leave this fake gateway's token for the httpserver tests.
		_, _ = lockConn.Exec(context.Background(), `UPDATE whatsapp_settings SET user_token_enc = NULL, status = 'unknown', jid = NULL WHERE id = 1`)
		_, _ = lockConn.Exec(context.Background(), `SELECT pg_advisory_unlock(920092)`)
		lockConn.Release()
	})
	if _, err := pool.Exec(ctx, `UPDATE whatsapp_settings SET user_token_enc = NULL, status = 'unknown', jid = NULL WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	box, _ := crypto.NewSecretBox("app-dev-encryption-key-32bytes!!")
	q := db.New(pool)
	gw := &gateway{Provider: &fake.Provider{}}
	n := &notes{}
	svc := usecase.New(q, pool, gw, box, nil, n, nil)

	gw.State = whatsapp.ConnState{Connected: true, LoggedIn: true, JID: "905320000009.0:1@s.whatsapp.net"}
	if err := svc.PollStatus(ctx); err != nil {
		t.Fatal(err)
	}
	st, _ := q.GetWhatsAppSettings(ctx)
	if st.Status != "connected" || st.PhoneE164.String != "+905320000009" || gw.token != "tok" {
		t.Fatalf("connected: %+v token=%q", st, gw.token)
	}

	gw.State = whatsapp.ConnState{}
	if err := svc.PollStatus(ctx); err != nil {
		t.Fatal(err)
	}
	st, _ = q.GetWhatsAppSettings(ctx)
	if st.Status != "disconnected" || gw.connects != 1 {
		t.Fatalf("websocket down: status=%s connects=%d", st.Status, gw.connects)
	}

	gw.State = whatsapp.ConnState{Connected: true, LoggedIn: true}
	_ = svc.PollStatus(ctx)
	gw.State = whatsapp.ConnState{Connected: true, LoggedIn: false}
	before := len(n.n)
	if err := svc.PollStatus(ctx); err != nil {
		t.Fatal(err)
	}
	st, _ = q.GetWhatsAppSettings(ctx)
	if st.Status != "logged_out" {
		t.Fatalf("unlinked: status=%s", st.Status)
	}
	recips, _ := q.ListWhatsAppAlarmRecipients(ctx)
	if len(n.n)-before != len(recips) {
		t.Fatalf("alarm notifications = %d, recipients = %d", len(n.n)-before, len(recips))
	}
}

// TEC-409: GET /v1/platform/whatsapp carries the AI pipeline health of the
// last 24 hours (run counts by status, error rate) for the cutover runbook.
// Runs are moved to a fixed far-future window inside a rolled-back
// transaction so parallel tests' runs never fall in it.
func TestOverviewAIPipelineHealth(t *testing.T) {
	f := newMsgFixture(t)
	now := time.Date(2100, 3, 10, 12, 0, 0, 0, time.UTC)
	svc := usecase.New(f.q, f.pool, nil, nil, nil, nil, nil)
	svc.SetClock(func() time.Time { return now })

	empty, err := svc.Overview(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := empty.AIPipeline; got.WindowHours != 24 || got.Runs != 0 || got.ErrorRate != 0 || got.LastRunAt != nil {
		t.Fatalf("empty window: %+v", got)
	}

	conv := f.conversation(t)
	run := func(status string, at time.Time) {
		t.Helper()
		msg := f.inbound(t, conv, whatsapp.InboundMedia{})
		r, err := f.q.CreateConversationAIRun(f.ctx, db.CreateConversationAIRunParams{
			ConversationID: conv.ID, TriggerMessageID: msg.ID, Model: "claude-sonnet-5-5",
		})
		if err != nil {
			t.Fatal(err)
		}
		if status != model.RunRunning {
			if _, err := f.q.FinishConversationAIRun(f.ctx, db.FinishConversationAIRunParams{
				ID: r.ID, Status: status, Stages: []byte("[]"), ToolCalls: []byte("[]"), Model: r.Model,
				FinishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := f.tx.Exec(f.ctx, `UPDATE conversation_ai_runs SET created_at = $2, started_at = $2 WHERE id = $1`, r.ID, at); err != nil {
			t.Fatal(err)
		}
	}
	for range 3 {
		run(model.RunCompleted, now.Add(-2*time.Hour))
	}
	run(model.RunFailed, now.Add(-90*time.Minute))
	run(model.RunSkipped, now.Add(-time.Hour))
	run(model.RunRunning, now.Add(-time.Minute))
	// Outside the window: older than 24 hours and in the future.
	run(model.RunFailed, now.Add(-25*time.Hour))
	run(model.RunFailed, now.Add(time.Hour))

	out, err := svc.Overview(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := out.AIPipeline
	if got.Runs != 6 || got.Completed != 3 || got.Failed != 1 || got.Skipped != 1 || got.Running != 1 {
		t.Fatalf("counts: %+v", got)
	}
	if got.ErrorRate != 0.25 {
		t.Fatalf("error rate = %v, want 0.25 (1 failed / 4 finished)", got.ErrorRate)
	}
	if got.LastRunAt == nil || !got.LastRunAt.Equal(now.Add(-time.Minute)) {
		t.Fatalf("last run at = %v", got.LastRunAt)
	}
	if got.LastFailedAt == nil || !got.LastFailedAt.Equal(now.Add(-90*time.Minute)) {
		t.Fatalf("last failed at = %v", got.LastFailedAt)
	}
}
