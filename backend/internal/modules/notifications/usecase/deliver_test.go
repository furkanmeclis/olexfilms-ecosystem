package usecase

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/providers"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// deliverTestQuerier implements the methods Deliver uses; the embedded nil
// Querier panics on anything else.
type deliverTestQuerier struct {
	Querier
	mu         sync.Mutex
	row        db.Notification
	user       db.User
	claimErr   error
	sent       int
	failed     int
	stuck      []int64
	deliveries []db.MarkDeliveryResultParams
}

func (d *deliverTestQuerier) MarkNotificationProcessing(context.Context, int64) (db.Notification, error) {
	if d.claimErr != nil {
		return db.Notification{}, d.claimErr
	}
	return d.row, nil
}

func (d *deliverTestQuerier) GetNotificationByID(context.Context, int64) (db.Notification, error) {
	return d.row, nil
}

func (d *deliverTestQuerier) GetUserByID(context.Context, int64) (db.User, error) {
	return d.user, nil
}

func (d *deliverTestQuerier) MarkNotificationSent(context.Context, db.MarkNotificationSentParams) (db.Notification, error) {
	d.sent++
	d.row.Status = model.StatusDelivered
	return d.row, nil
}

func (d *deliverTestQuerier) MarkNotificationFailed(context.Context, db.MarkNotificationFailedParams) (db.Notification, error) {
	d.failed++
	return d.row, nil
}

func (d *deliverTestQuerier) InsertNotificationHistory(context.Context, db.InsertNotificationHistoryParams) (db.NotificationHistory, error) {
	return db.NotificationHistory{}, nil
}

func (d *deliverTestQuerier) MarkDeliveryProcessing(context.Context, int64) error { return nil }

func (d *deliverTestQuerier) MarkDeliveryResult(_ context.Context, arg db.MarkDeliveryResultParams) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.deliveries = append(d.deliveries, arg)
	return nil
}

func (d *deliverTestQuerier) ListStuckProcessingNotificationIDs(context.Context, int32) ([]int64, error) {
	return d.stuck, nil
}

// fakePublisher records Centrifugo publishes.
type fakePublisher struct {
	mu       sync.Mutex
	channels []string
	data     []any
}

func (f *fakePublisher) Publish(_ context.Context, channel string, data any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.channels = append(f.channels, channel)
	f.data = append(f.data, data)
	return nil
}

func TestDeliverResumesProcessingAfterClaimMiss(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		claimErr: pgx.ErrNoRows,
		row:      db.Notification{ID: 7, Channel: model.ChannelInapp, Status: model.StatusProcessing, Title: "Hello"},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	if err := svc.Deliver(context.Background(), 7); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if q.sent != 1 {
		t.Fatalf("MarkNotificationSent calls = %d, want 1", q.sent)
	}
}

func TestDeliverSkipsTerminalStatus(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		claimErr: pgx.ErrNoRows,
		row:      db.Notification{ID: 7, Channel: model.ChannelInapp, Status: model.StatusDelivered},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	if err := svc.Deliver(context.Background(), 7); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if q.sent != 0 {
		t.Fatalf("MarkNotificationSent calls = %d, want 0", q.sent)
	}
}

func TestReclaimStuckDeliversProcessingRows(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		claimErr: pgx.ErrNoRows,
		row:      db.Notification{ID: 3, Channel: model.ChannelInapp, Status: model.StatusProcessing},
		stuck:    []int64{3},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	n, err := svc.ReclaimStuck(context.Background(), 0)
	if err != nil {
		t.Fatalf("ReclaimStuck: %v", err)
	}
	if n != 1 || q.sent != 1 {
		t.Fatalf("reclaimed = %d, sent = %d, want 1/1", n, q.sent)
	}
}

// Acceptance 3: an in-app delivery is published to Centrifugo user:{uuid}
// and the delivery row is marked delivered.
func TestDeliverInappPublishesToUserChannel(t *testing.T) {
	t.Parallel()

	userUUID := uuid.New()
	nUUID := uuid.New()
	q := &deliverTestQuerier{
		row: db.Notification{
			ID: 9, Uuid: nUUID, Channel: model.ChannelInapp, Status: model.StatusProcessing,
			Title: "Modül talebi", Body: "Tech Oto stok modülünü istedi.",
			UserID:     pgtype.Int8{Int64: 42, Valid: true},
			DeliveryID: pgtype.Int8{Int64: 5, Valid: true},
		},
		user: db.User{ID: 42, Uuid: userUUID},
	}
	pub := &fakePublisher{}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{Pub: pub}}, slog.Default())
	if err := svc.Deliver(context.Background(), 9); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if len(pub.channels) != 1 || pub.channels[0] != "user:"+userUUID.String() {
		t.Fatalf("published channels = %v", pub.channels)
	}
	msg := pub.data[0].(map[string]any)
	if msg["type"] != "notification.created" || msg["notification"].(map[string]any)["uuid"] != nUUID {
		t.Fatalf("payload = %+v", msg)
	}
	if len(q.deliveries) != 1 || q.deliveries[0].ID != 5 || q.deliveries[0].Status != model.StatusDelivered {
		t.Fatalf("delivery updates = %+v", q.deliveries)
	}
}

// A channel without an address is recorded as skipped_no_recipient and not
// retried (no error back to Asynq).
func TestDeliverNoRecipientIsSkipped(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		row: db.Notification{
			ID: 11, Channel: model.ChannelEmail, Status: model.StatusProcessing,
			DeliveryID: pgtype.Int8{Int64: 6, Valid: true},
		},
	}
	svc := New(q, nil, []providers.Provider{providers.EmailProvider{}}, slog.Default())
	if err := svc.Deliver(context.Background(), 11); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if q.failed != 1 || len(q.deliveries) != 1 || q.deliveries[0].Status != model.DeliverySkippedNoRecipient {
		t.Fatalf("failed=%d deliveries=%+v", q.failed, q.deliveries)
	}
}

func TestPreferenceSetPrecedence(t *testing.T) {
	t.Parallel()

	ev := "features.module_requested"
	set := newPreferenceSet([]db.NotificationPreference{
		{Channel: model.ChannelEmail, Enabled: false},
		{EventCode: pgtype.Text{String: ev, Valid: true}, Channel: model.ChannelEmail, Enabled: true},
		{Channel: model.ChannelInapp, Enabled: false},
	})
	cases := []struct {
		code, ch  string
		def, want bool
	}{
		{ev, model.ChannelEmail, true, true},             // event row beats global
		{"other.event", model.ChannelEmail, true, false}, // global row
		{ev, model.ChannelInapp, true, false},            // global row
		{ev, model.ChannelSMS, true, true},               // default
		{ev, model.ChannelWebPush, false, false},         // default
	}
	for _, c := range cases {
		if got := set.allowed(c.code, c.ch, c.def); got != c.want {
			t.Errorf("allowed(%s,%s) = %v, want %v", c.code, c.ch, got, c.want)
		}
	}
	p := projectPreferences([]db.NotificationPreference{{Channel: model.ChannelWebPush, Enabled: true}})
	if !p.EmailEnabled || !p.InappEnabled || !p.RealtimeEnabled || !p.PushEnabled || len(p.Rules) != 1 {
		t.Fatalf("projection = %+v", p)
	}
}

func TestNormalizeChannel(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{"push": "webpush", "inapp": "inapp", "expo_push": "expo_push", "whatsapp": "whatsapp"} {
		if got, ok := normalizeChannel(raw); !ok || got != want {
			t.Errorf("normalizeChannel(%q) = %q,%v", raw, got, ok)
		}
	}
	for _, raw := range []string{"realtime", "", "fax"} {
		if _, ok := normalizeChannel(raw); ok {
			t.Errorf("normalizeChannel(%q) accepted", raw)
		}
	}
}
