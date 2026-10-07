package usecase_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp/fake"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Media fixtures (magic numbers only; the sniffer reads the header).
var (
	jpegBytes = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{1}, 64)...)
	pngBytes  = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{2}, 64)...)
	pdfBytes  = []byte("%PDF-1.7\n1 0 obj << >> endobj\n%%EOF\n")
)

type enqueued struct {
	kind string
	id   int64
}

// fakeQueue records enqueued tasks.
type fakeQueue struct {
	mu   sync.Mutex
	jobs []enqueued
}

func (q *fakeQueue) EnqueueSend(_ context.Context, id int64, _ uuid.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = append(q.jobs, enqueued{"send", id})
	return nil
}

func (q *fakeQueue) EnqueueMediaStore(_ context.Context, id int64, _ uuid.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = append(q.jobs, enqueued{"media", id})
	return nil
}

func (q *fakeQueue) count(kind string, id int64) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := 0
	for _, j := range q.jobs {
		if j.kind == kind && j.id == id {
			n++
		}
	}
	return n
}

type published struct {
	channel string
	typ     string
	message usecase.MessageEvent
}

// fakePublisher records realtime publishes.
type fakePublisher struct {
	mu  sync.Mutex
	got []published
}

func (p *fakePublisher) Publish(_ context.Context, channel string, data any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	m := data.(map[string]any)
	p.got = append(p.got, published{channel: channel, typ: m["type"].(string), message: m["message"].(usecase.MessageEvent)})
	return nil
}

// last returns the newest publish of typ on channel for message id.
func (p *fakePublisher) last(channel, typ string, id uuid.UUID) (usecase.MessageEvent, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := len(p.got) - 1; i >= 0; i-- {
		g := p.got[i]
		if g.channel == channel && g.typ == typ && g.message.UUID == id {
			return g.message, true
		}
	}
	return usecase.MessageEvent{}, false
}

// fakeLimiter allows while Deny is false and records the limit it saw.
// With Now set it is a fixed-window limiter on that clock instead.
type fakeLimiter struct {
	Deny    bool
	Now     func() time.Time
	limit   int
	subject string
	window  time.Time
	used    int
}

func (l *fakeLimiter) Allow(_ context.Context, _, subject string, limit int, window time.Duration) (bool, time.Duration) {
	l.limit, l.subject = limit, subject
	if l.Now == nil {
		return !l.Deny, 30 * time.Second
	}
	now := l.Now()
	if start := now.Truncate(window); !start.Equal(l.window) {
		l.window, l.used = start, 0
	}
	if l.used >= limit {
		return false, l.window.Add(window).Sub(now)
	}
	l.used++
	return true, 0
}

// fakeDocs renders a fixed "PDF".
type fakeDocs struct {
	data []byte
	kind string
	ref  usecase.DocumentRef
}

func (d *fakeDocs) Render(_ context.Context, kind string, ref usecase.DocumentRef) (usecase.RenderedDocument, error) {
	d.kind, d.ref = kind, ref
	return usecase.RenderedDocument{Data: d.data, FileName: "service_20261007.pdf"}, nil
}

// msgFixture runs every statement in one rolled back transaction (pgx.Tx
// opens savepoints for the use case's own transactions).
type msgFixture struct {
	ctx   context.Context
	pool  *pgxpool.Pool
	tx    pgx.Tx
	q     *db.Queries
	wa    *fake.Provider
	store *storage.Memory
	queue *fakeQueue
	pub   *fakePublisher
	lim   *fakeLimiter
	docs  *fakeDocs
	m     *usecase.Messaging
	phone string
	seq   int
}

func newMsgFixture(t *testing.T) *msgFixture {
	t.Helper()
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
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	f := &msgFixture{
		ctx: ctx, pool: pool, tx: tx, q: db.New(tx), wa: &fake.Provider{}, store: storage.NewMemory(),
		queue: &fakeQueue{}, pub: &fakePublisher{}, lim: &fakeLimiter{}, docs: &fakeDocs{data: pdfBytes},
		phone: fmt.Sprintf("+90%09d", time.Now().UnixNano()%1_000_000_000),
	}
	f.m = usecase.NewMessaging(usecase.MessagingDeps{
		Queries: f.q, Tx: tx, Provider: f.wa, Downloader: f.wa, Storage: f.store, Queue: f.queue,
		Limiter: f.lim, SendPerMinute: func(context.Context) int { return 30 }, Publisher: f.pub, Documents: f.docs,
	})
	return f
}

func (f *msgFixture) conversation(t *testing.T) db.Conversation {
	t.Helper()
	f.seq++
	c, err := f.q.UpsertConversation(f.ctx, db.UpsertConversationParams{
		Channel: whatsapp.ChannelWhatsApp, ContactE164: fmt.Sprintf("%s%02d", f.phone, f.seq),
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// inbound stores an inbound message with media metadata.
func (f *msgFixture) inbound(t *testing.T, conv db.Conversation, media whatsapp.InboundMedia) db.Message {
	t.Helper()
	f.seq++
	raw, _ := json.Marshal(media)
	m, err := f.q.InsertMessage(f.ctx, db.InsertMessageParams{
		ConversationID: conv.ID, Channel: whatsapp.ChannelWhatsApp, Direction: "in", SenderType: model.SenderContact,
		ExternalID: fmt.Sprintf("tec395-in-%s-%d", f.phone, f.seq), Media: raw, Status: "received",
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func (f *msgFixture) get(t *testing.T, id int64) db.Message {
	t.Helper()
	m, err := f.q.GetMessageByID(f.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func (f *msgFixture) queueText(t *testing.T, conv db.Conversation, body string) db.Message {
	t.Helper()
	msg, err := f.m.Queue(f.ctx, usecase.OutgoingMessage{ConversationID: conv.ID, SenderType: model.SenderAI, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

// An outgoing message is stored as queued, published and enqueued; the send
// task sends it once even when it runs twice.
func TestMessagingQueueSendsOnceWhenTaskRunsTwice(t *testing.T) {
	f := newMsgFixture(t)
	conv := f.conversation(t)
	msg := f.queueText(t, conv, "Merhaba")
	if msg.Status != "queued" || msg.Direction != "out" || msg.SendAttempts != 0 {
		t.Fatalf("queued row: %+v", msg)
	}
	if msg.ExternalID != usecase.ClientMessageID(msg.Uuid) {
		t.Fatalf("external id %q, want client id of uuid %s", msg.ExternalID, msg.Uuid)
	}
	if f.queue.count("send", msg.ID) != 1 {
		t.Fatalf("send task not enqueued: %+v", f.queue.jobs)
	}
	if _, ok := f.pub.last(realtime.ChannelConversations, usecase.EventMessageCreated, msg.Uuid); !ok {
		t.Fatal("created event not published")
	}

	for range 2 {
		if err := f.m.ProcessSend(f.ctx, msg.ID, false); err != nil {
			t.Fatal(err)
		}
	}
	sent := f.wa.Sent()
	if len(sent) != 1 || sent[0].Kind != "text" || sent[0].Body != "Merhaba" || sent[0].To != conv.ContactE164 {
		t.Fatalf("provider sends = %+v, want exactly one text", sent)
	}
	if sent[0].ID != msg.ExternalID {
		t.Fatalf("provider id %q, want idempotent client id %q", sent[0].ID, msg.ExternalID)
	}
	got := f.get(t, msg.ID)
	if got.Status != "sent" || got.SendAttempts != 1 || !got.SentAt.Valid || got.FailureReason.Valid {
		t.Fatalf("after send: status=%s attempts=%d sent_at=%v reason=%v", got.Status, got.SendAttempts, got.SentAt, got.FailureReason)
	}
	ev, ok := f.pub.last(realtime.ChannelConversations, usecase.EventMessageUpdated, msg.Uuid)
	if !ok || ev.Status != "sent" {
		t.Fatalf("updated event = %+v, %v", ev, ok)
	}
	if f.lim.subject != conv.ContactE164 || f.lim.limit != 30 {
		t.Fatalf("limiter saw subject=%q limit=%d", f.lim.subject, f.lim.limit)
	}
}

// A provider error keeps the message queued with the reason and returns the
// error (asynq retries); the last retry marks it failed with the attempt
// count. Three retries: four attempts in total.
func TestMessagingSendFailureRetriesThenFails(t *testing.T) {
	if usecase.SendMaxRetry != 3 || queue.WhatsAppSendMaxRetry != usecase.SendMaxRetry {
		t.Fatalf("retries: usecase %d, task %d; want 3", usecase.SendMaxRetry, queue.WhatsAppSendMaxRetry)
	}
	f := newMsgFixture(t)
	conv := f.conversation(t)
	msg := f.queueText(t, conv, "Deneme")
	f.wa.Err = errors.New("gateway timeout")

	for i := 1; i <= usecase.SendMaxRetry; i++ {
		if err := f.m.ProcessSend(f.ctx, msg.ID, false); err == nil {
			t.Fatalf("attempt %d: want the provider error for a retry", i)
		}
		got := f.get(t, msg.ID)
		if got.Status != "queued" || int(got.SendAttempts) != i || got.FailureReason.String != "gateway timeout" {
			t.Fatalf("attempt %d: status=%s attempts=%d reason=%q", i, got.Status, got.SendAttempts, got.FailureReason.String)
		}
	}
	// Last retry (asynq retry count == max).
	if err := f.m.ProcessSend(f.ctx, msg.ID, true); err != nil {
		t.Fatalf("final attempt is recorded, not retried: %v", err)
	}
	got := f.get(t, msg.ID)
	if got.Status != "failed" || int(got.SendAttempts) != usecase.SendMaxRetry+1 ||
		got.FailureReason.String != "gateway timeout" || !got.DeliveryStatusAt.Valid {
		t.Fatalf("failed row: status=%s attempts=%d reason=%q", got.Status, got.SendAttempts, got.FailureReason.String)
	}
	if ev, ok := f.pub.last(realtime.ChannelConversations, usecase.EventMessageUpdated, msg.Uuid); !ok || ev.Status != "failed" || ev.SendAttempts != 4 {
		t.Fatalf("failed event = %+v, %v", ev, ok)
	}
	// A failed message is never sent by a stray run.
	f.wa.Err = nil
	if err := f.m.ProcessSend(f.ctx, msg.ID, false); err != nil || len(f.wa.Sent()) != 0 {
		t.Fatalf("failed message resent: err=%v sends=%d", err, len(f.wa.Sent()))
	}

	// A permanent error (invalid recipient) fails at once.
	msg2 := f.queueText(t, conv, "x")
	f.wa.Err = whatsapp.ErrInvalidRecipient
	if err := f.m.ProcessSend(f.ctx, msg2.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := f.get(t, msg2.ID); got.Status != "failed" || got.SendAttempts != 1 {
		t.Fatalf("permanent error: status=%s attempts=%d", got.Status, got.SendAttempts)
	}
}

// Over the per-number limit the task is retried later without a provider
// call or a counted attempt.
func TestMessagingRateLimitDefersSend(t *testing.T) {
	f := newMsgFixture(t)
	conv := f.conversation(t)
	msg := f.queueText(t, conv, "limit")
	f.lim.Deny = true
	err := f.m.ProcessSend(f.ctx, msg.ID, false)
	var rl *usecase.RateLimitedError
	if !errors.Is(err, usecase.ErrRateLimited) || !errors.As(err, &rl) || rl.RetryAfter() != 30*time.Second {
		t.Fatalf("err = %v, want RateLimitedError (30s)", err)
	}
	if queue.IsTaskFailure(err) {
		t.Fatal("a rate limit deferral must not count as a task failure (retry)")
	}
	if got := f.get(t, msg.ID); got.Status != "queued" || got.SendAttempts != 0 || len(f.wa.Sent()) != 0 {
		t.Fatalf("rate limited: status=%s attempts=%d sends=%d", got.Status, got.SendAttempts, len(f.wa.Sent()))
	}
	f.lim.Deny = false
	if err := f.m.ProcessSend(f.ctx, msg.ID, false); err != nil || len(f.wa.Sent()) != 1 {
		t.Fatalf("after window: err=%v sends=%d", err, len(f.wa.Sent()))
	}

	// Deferred for longer than MaxRateLimitDeferral since queueing: failed
	// as rate_limited, nothing sent.
	late := f.queueText(t, conv, "late")
	f.lim.Deny = true
	f.m.SetClock(func() time.Time { return late.CreatedAt.Time.Add(usecase.MaxRateLimitDeferral) })
	if err := f.m.ProcessSend(f.ctx, late.ID, false); err != nil {
		t.Fatalf("expired deferral is recorded, not retried: %v", err)
	}
	if got := f.get(t, late.ID); got.Status != "failed" || got.FailureReason.String != usecase.FailureRateLimited || len(f.wa.Sent()) != 1 {
		t.Fatalf("expired: status=%s reason=%q sends=%d", got.Status, got.FailureReason.String, len(f.wa.Sent()))
	}
}

// A burst of 35 messages to one number with a limit of 30/min: the five
// over the limit are deferred to the next window without consuming a
// delivery retry, so none fails and each is sent exactly once. The loop
// mirrors asynq: the retry counter only grows when IsFailure says so and
// the next run waits TaskRetryDelay.
func TestMessagingRateLimitBurstNeverFails(t *testing.T) {
	f := newMsgFixture(t)
	conv := f.conversation(t)
	clock := time.Now().Truncate(time.Minute).Add(50 * time.Second)
	now := func() time.Time { return clock }
	f.m.SetClock(now)
	f.lim.Now = now

	type task struct {
		id      int64
		retried int
		runAt   time.Time
	}
	var tasks []*task
	var ids []int64
	for i := range 35 {
		msg := f.queueText(t, conv, fmt.Sprintf("burst %d", i))
		tasks = append(tasks, &task{id: msg.ID, runAt: clock})
		ids = append(ids, msg.ID)
	}
	deferrals := 0
	for runs := 0; len(tasks) > 0; runs++ {
		if runs > 200 {
			t.Fatalf("burst did not drain: %d tasks left", len(tasks))
		}
		next := 0
		for i, tk := range tasks {
			if tk.runAt.Before(tasks[next].runAt) {
				next = i
			}
		}
		tk := tasks[next]
		clock = tk.runAt
		err := f.m.ProcessSend(f.ctx, tk.id, tk.retried >= queue.WhatsAppSendMaxRetry)
		if err == nil {
			tasks = append(tasks[:next], tasks[next+1:]...)
			continue
		}
		if !errors.Is(err, usecase.ErrRateLimited) {
			t.Fatalf("message %d: %v", tk.id, err)
		}
		deferrals++
		if queue.IsTaskFailure(err) {
			t.Fatalf("message %d: a deferral consumed a delivery retry", tk.id)
		}
		tk.runAt = clock.Add(queue.TaskRetryDelay(tk.retried, err, nil))
	}
	if deferrals != 5 {
		t.Fatalf("deferrals = %d, want 5", deferrals)
	}
	sends := map[string]int{}
	for _, s := range f.wa.Sent() {
		sends[s.ID]++
	}
	if len(f.wa.Sent()) != 35 {
		t.Fatalf("sends = %d, want 35", len(f.wa.Sent()))
	}
	for _, id := range ids {
		got := f.get(t, id)
		if got.Status != "sent" || sends[got.ExternalID] != 1 || got.FailureReason.Valid {
			t.Fatalf("message %d: status=%s sends=%d reason=%q", id, got.Status, sends[got.ExternalID], got.FailureReason.String)
		}
	}
}

// Receipts move a sent message to delivered, then read (with the receipt
// time); a late "delivered" does not move it back.
func TestMessagingReceiptWritesRead(t *testing.T) {
	f := newMsgFixture(t)
	conv := f.conversation(t)
	msg := f.queueText(t, conv, "okundu mu")
	if err := f.m.ProcessSend(f.ctx, msg.ID, false); err != nil {
		t.Fatal(err)
	}
	deliveredAt := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	readAt := deliveredAt.Add(time.Minute)
	receipt := func(status string, at time.Time) int {
		n, err := f.m.ApplyReceipt(f.ctx, whatsapp.InboundEvent{
			Kind: whatsapp.KindStatus, Status: status, MessageIDs: []string{msg.ExternalID}, Timestamp: at,
		})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if receipt("delivered", deliveredAt) != 1 || f.get(t, msg.ID).Status != "delivered" {
		t.Fatal("delivered receipt not applied")
	}
	if receipt("read", readAt) != 1 {
		t.Fatal("read receipt not applied")
	}
	got := f.get(t, msg.ID)
	if got.Status != "read" || !got.DeliveryStatusAt.Time.Equal(readAt) {
		t.Fatalf("read: status=%s at=%v", got.Status, got.DeliveryStatusAt.Time)
	}
	if ev, ok := f.pub.last(realtime.ChannelConversations, usecase.EventMessageUpdated, msg.Uuid); !ok || ev.Status != "read" {
		t.Fatalf("read event = %+v, %v", ev, ok)
	}
	if receipt("delivered", readAt.Add(time.Minute)) != 0 || f.get(t, msg.ID).Status != "read" {
		t.Fatal("a late delivered receipt must not downgrade read")
	}
}

// Inbound media: a supported file is stored under whatsapp/{conv}/{msg}
// with its sniffed type; a 20 MB file is not stored and the message notes
// why.
func TestMessagingInboundMediaStorage(t *testing.T) {
	f := newMsgFixture(t)
	conv := f.conversation(t)

	f.wa.Media = jpegBytes
	ok := f.inbound(t, conv, whatsapp.InboundMedia{Type: "image", MimeType: "image/png", FileName: "foto.jpg"})
	if err := f.m.StoreInboundMedia(f.ctx, ok.ID, false); err != nil {
		t.Fatal(err)
	}
	got := f.get(t, ok.ID)
	wantKey := storage.WhatsAppMediaObjectKey(conv.Uuid, ok.Uuid)
	if got.MediaStorageKey.String != wantKey || got.MediaMime.String != whatsapp.MimeJPEG || got.MediaSize.Int64 != int64(len(jpegBytes)) {
		t.Fatalf("stored media: key=%q mime=%q size=%d", got.MediaStorageKey.String, got.MediaMime.String, got.MediaSize.Int64)
	}
	if ct, _, ok := f.store.GetMeta(wantKey); !ok || ct != whatsapp.MimeJPEG {
		t.Fatalf("object %s: content type %q, exists %v", wantKey, ct, ok)
	}
	if ev, ok := f.pub.last(realtime.ChannelConversations, usecase.EventMessageUpdated, ok.Uuid); !ok || !ev.HasStoredMedia {
		t.Fatalf("media stored event = %+v, %v", ev, ok)
	}

	// 20 MB: downloaded size over the limit.
	f.wa.Media = bytes.Repeat([]byte{0xFF, 0xD8, 0xFF, 0}, 5<<20)
	big := f.inbound(t, conv, whatsapp.InboundMedia{Type: "image", FileName: "big.jpg"})
	if err := f.m.StoreInboundMedia(f.ctx, big.ID, false); err != nil {
		t.Fatal(err)
	}
	assertSkipped(t, f, conv, big, usecase.MediaSkipTooLarge)

	// 20 MB declared by the sender: not even downloaded.
	f.wa.MediaErr = errors.New("must not download")
	declared := f.inbound(t, conv, whatsapp.InboundMedia{Type: "document", FileName: "big.pdf", Size: 20 << 20})
	if err := f.m.StoreInboundMedia(f.ctx, declared.ID, false); err != nil {
		t.Fatal(err)
	}
	assertSkipped(t, f, conv, declared, usecase.MediaSkipTooLarge)
}

// A "pdf" whose bytes are not a PDF is rejected: inbound it is not stored,
// outgoing it cannot be queued.
func TestMessagingRejectsFakePDF(t *testing.T) {
	f := newMsgFixture(t)
	conv := f.conversation(t)

	for _, data := range [][]byte{pngBytes, []byte("MZ\x90\x00 not a pdf")} {
		f.wa.Media = data
		in := f.inbound(t, conv, whatsapp.InboundMedia{Type: "document", MimeType: "application/pdf", FileName: "fatura.pdf"})
		if err := f.m.StoreInboundMedia(f.ctx, in.ID, false); err != nil {
			t.Fatal(err)
		}
		assertSkipped(t, f, conv, in, usecase.MediaSkipMismatch)

		_, err := f.m.Queue(f.ctx, usecase.OutgoingMessage{
			ConversationID: conv.ID, SenderType: model.SenderStaff,
			Media: &usecase.OutgoingMedia{Data: data, FileName: "fatura.pdf"},
		})
		if !errors.Is(err, usecase.ErrInvalidRequest) || !errors.Is(err, whatsapp.ErrMediaMismatch) {
			t.Fatalf("outgoing fake pdf: err = %v", err)
		}
	}
	// Unknown content without a known extension is unsupported.
	if _, err := f.m.Queue(f.ctx, usecase.OutgoingMessage{
		ConversationID: conv.ID, SenderType: model.SenderStaff,
		Media: &usecase.OutgoingMedia{Data: []byte("plain text"), FileName: "notes.txt"},
	}); !errors.Is(err, whatsapp.ErrMediaType) {
		t.Fatalf("unsupported type: err = %v", err)
	}
}

func assertSkipped(t *testing.T, f *msgFixture, conv db.Conversation, msg db.Message, reason string) {
	t.Helper()
	got := f.get(t, msg.ID)
	if got.MediaStorageKey.Valid {
		t.Fatalf("media of %d stored at %q, want skipped (%s)", msg.ID, got.MediaStorageKey.String, reason)
	}
	var media map[string]any
	_ = json.Unmarshal(got.Media, &media)
	if media["storage_skipped"] != reason {
		t.Fatalf("media note = %v, want %s", media["storage_skipped"], reason)
	}
	if ok, _ := f.store.Exists(f.ctx, storage.WhatsAppMediaObjectKey(conv.Uuid, msg.Uuid)); ok {
		t.Fatal("object written")
	}
}

// The service PDF helper renders, stores and queues a PDF document; the
// send task streams it from storage to SendDocument.
func TestMessagingQueueDocumentSendsPDF(t *testing.T) {
	f := newMsgFixture(t)
	conv := f.conversation(t)
	ref := usecase.DocumentRef{ServiceUUID: uuid.New(), BrandID: 1, OrganizationID: 1, Locale: "tr"}
	msg, err := f.m.QueueDocument(f.ctx, usecase.DocumentMessage{
		ConversationID: conv.ID, SenderType: model.SenderAI, Kind: usecase.DocumentServicePDF, Ref: ref, Caption: "Hizmet belgeniz",
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.docs.kind != usecase.DocumentServicePDF || f.docs.ref != ref {
		t.Fatalf("renderer got %s %+v", f.docs.kind, f.docs.ref)
	}
	if msg.MediaMime.String != whatsapp.MimePDF || msg.MediaStorageKey.String != storage.WhatsAppMediaObjectKey(conv.Uuid, msg.Uuid) {
		t.Fatalf("queued document: mime=%q key=%q", msg.MediaMime.String, msg.MediaStorageKey.String)
	}
	if err := f.m.ProcessSend(f.ctx, msg.ID, false); err != nil {
		t.Fatal(err)
	}
	last, _ := f.wa.Last()
	if last.Kind != "document" || last.MimeType != whatsapp.MimePDF || !bytes.Equal(last.Data, pdfBytes) ||
		last.FileName != "service_20261007.pdf" || last.Body != "Hizmet belgeniz" {
		t.Fatalf("sent document = kind %s mime %s name %s caption %q", last.Kind, last.MimeType, last.FileName, last.Body)
	}

	// An image goes out as an image.
	img, err := f.m.Queue(f.ctx, usecase.OutgoingMessage{
		ConversationID: conv.ID, SenderType: model.SenderSystem, Media: &usecase.OutgoingMedia{Data: pngBytes, FileName: "kart.png"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.m.ProcessSend(f.ctx, img.ID, false); err != nil {
		t.Fatal(err)
	}
	if last, _ := f.wa.Last(); last.Kind != "image" || last.MimeType != whatsapp.MimePNG {
		t.Fatalf("sent image = %+v", last)
	}

	// A renderer that returns no PDF is rejected.
	f.docs.data = []byte("<html>")
	if _, err := f.m.QueueDocument(f.ctx, usecase.DocumentMessage{
		ConversationID: conv.ID, SenderType: model.SenderAI, Kind: usecase.DocumentWarrantyCertificate, Ref: ref,
	}); !errors.Is(err, usecase.ErrInvalidRequest) {
		t.Fatalf("non-pdf document: err = %v", err)
	}
}

// Events go to the platform inbox channel and, once assigned, to the
// assigned user's channel.
func TestMessagingPublishesToAssignedUser(t *testing.T) {
	f := newMsgFixture(t)
	conv := f.conversation(t)
	var userID int64
	var userUUID uuid.UUID
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO users (email, password_hash, name, surname, status, phone_e164)
		VALUES ($1, 'x', 'Agent', 'T395', 'active', NULL) RETURNING id, uuid`,
		fmt.Sprintf("agent-%s@t395.example.com", f.phone[1:])).Scan(&userID, &userUUID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.AssignConversation(f.ctx, db.AssignConversationParams{ID: conv.ID, AssignedUserID: pgtype.Int8{Int64: userID, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	msg := f.queueText(t, conv, "atanmış")
	for _, ch := range []string{realtime.ChannelConversations, realtime.UserChannel(userUUID)} {
		if _, ok := f.pub.last(ch, usecase.EventMessageCreated, msg.Uuid); !ok {
			t.Fatalf("no created event on %s", ch)
		}
	}
}

// The webhook path: an inbound message with media is published and its
// media task enqueued; a read receipt from the gateway is stored.
func TestMessagingWebhookPublishesAndAppliesReceipt(t *testing.T) {
	f := newMsgFixture(t)
	// whatsapp_settings is shared with the httpserver integration tests.
	lockConn, err := f.pool.Acquire(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockConn.Exec(f.ctx, `SELECT pg_advisory_lock(920092)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = lockConn.Exec(context.Background(), `SELECT pg_advisory_unlock(920092)`)
		lockConn.Release()
	})
	box, _ := crypto.NewSecretBox("app-dev-encryption-key-32bytes!!")
	gw := &gateway{Provider: f.wa}
	// Settings writes (instance token) stay in the rolled back transaction.
	svc := usecase.New(f.q, f.tx, gw, box, nil, nil, nil)
	svc.AttachMessaging(f.m)

	conv := f.conversation(t)
	out := f.queueText(t, conv, "giden")
	if err := f.m.ProcessSend(f.ctx, out.ID, false); err != nil {
		t.Fatal(err)
	}
	extID := fmt.Sprintf("TEC395IN%s", f.phone[1:])
	f.wa.Events = []whatsapp.InboundEvent{
		{Kind: whatsapp.KindMessage, ExternalID: extID, From: conv.ContactE164, Timestamp: time.Now(),
			Media: &whatsapp.InboundMedia{Type: "image", MimeType: "image/jpeg"}},
		{Kind: whatsapp.KindStatus, Status: "read", MessageIDs: []string{out.ExternalID}, Timestamp: time.Now()},
	}
	res, err := svc.HandleWebhook(f.ctx, nil, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Messages != 1 || res.Statuses != 1 {
		t.Fatalf("webhook result = %+v", res)
	}
	if got := f.get(t, out.ID); got.Status != "read" || !got.DeliveryStatusAt.Valid {
		t.Fatalf("receipt via webhook: status=%s", got.Status)
	}
	var in db.Message
	if err := f.tx.QueryRow(f.ctx, `SELECT id, uuid FROM messages WHERE external_id = $1`, extID).Scan(&in.ID, &in.Uuid); err != nil {
		t.Fatal(err)
	}
	if f.queue.count("media", in.ID) != 1 {
		t.Fatalf("media task not enqueued: %+v", f.queue.jobs)
	}
	if _, ok := f.pub.last(realtime.ChannelConversations, usecase.EventMessageCreated, in.Uuid); !ok {
		t.Fatal("inbound message not published")
	}
}
