package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/google/uuid"
)

// TEC-407 (F4-04d) sender tests: fake channels, fake clock and a fake
// fixed-window limiter driven by the same clock.

type fakeChannels struct {
	mu       sync.Mutex
	push     []PushMessage
	email    []EmailMessage
	whatsapp []WhatsAppMessage
	// errors by recipient user id (all channels)
	fail map[int64]error
	// users without a push address at send time
	noPush map[int64]bool
	// e-mail errors by address
	failEmail map[string]error
}

func newFakeChannels() *fakeChannels {
	return &fakeChannels{fail: map[int64]error{}, noPush: map[int64]bool{}, failEmail: map[string]error{}}
}

func (c *fakeChannels) SendCampaignPush(_ context.Context, m PushMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.noPush[m.UserID] {
		return ErrNoAddress
	}
	if err := c.fail[m.UserID]; err != nil {
		return err
	}
	c.push = append(c.push, m)
	return nil
}

type fakeEmail struct{ c *fakeChannels }

func (e fakeEmail) SendCampaignEmail(_ context.Context, m EmailMessage) error {
	e.c.mu.Lock()
	defer e.c.mu.Unlock()
	if err := e.c.failEmail[m.To]; err != nil {
		return err
	}
	e.c.email = append(e.c.email, m)
	return nil
}

type fakeWhatsApp struct{ c *fakeChannels }

func (w fakeWhatsApp) SendCampaignWhatsApp(_ context.Context, m WhatsAppMessage) error {
	w.c.mu.Lock()
	defer w.c.mu.Unlock()
	if err := w.c.fail[m.UserID]; err != nil {
		return err
	}
	w.c.whatsapp = append(w.c.whatsapp, m)
	return nil
}

type fakeQueue struct{ ids []int64 }

func (q *fakeQueue) EnqueueRecipient(_ context.Context, id int64) error {
	q.ids = append(q.ids, id)
	return nil
}

// clockLimiter is a fixed one-minute-window limiter on the test clock.
type clockLimiter struct {
	now    func() time.Time
	counts map[string]int
}

func (l *clockLimiter) Allow(_ context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration) {
	start := l.now().Truncate(window)
	key := fmt.Sprintf("%s|%s|%d", action, subject, start.Unix())
	l.counts[key]++
	if l.counts[key] > limit {
		return false, start.Add(window).Sub(l.now())
	}
	return true, 0
}

type fakeSettings struct{ perMinute, start, end int }

func (s fakeSettings) CampaignsWhatsAppPerMinute(context.Context) int  { return s.perMinute }
func (s fakeSettings) CampaignsQuietHours(context.Context) (int, int) { return s.start, s.end }

type senderEnv struct {
	*fixture
	now    time.Time
	ch     *fakeChannels
	queue  *fakeQueue
	out    *recordingOutbox
	sender *Sender
}

// noonIstanbul is 12:00 in Europe/Istanbul (UTC+3).
var noonIstanbul = time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)

func newSenderEnv(t *testing.T) *senderEnv {
	t.Helper()
	f := newFixture(t)
	e := &senderEnv{fixture: f, now: noonIstanbul, ch: newFakeChannels(), queue: &fakeQueue{}}
	e.out = f.withOutbox()
	f.svc.SetClock(func() time.Time { return e.now })
	e.sender = NewSender(f.svc, SenderDeps{
		Push: e.ch, Email: fakeEmail{e.ch}, WhatsApp: fakeWhatsApp{e.ch}, Queue: e.queue,
		Limiter:  &clockLimiter{now: func() time.Time { return e.now }, counts: map[string]int{}},
		Settings: fakeSettings{perMinute: 20, start: 21, end: 9},
		UnsubscribeSecret: []byte("test-secret"), FrontendURL: "https://app.example.test",
	})
	return e
}

// scheduled creates a dealer campaign on channels with complete tr content,
// due at e.now - 1 minute.
func (e *senderEnv) scheduled(t *testing.T, channels ...string) db.Campaign {
	t.Helper()
	camp := e.create(t, e.dealerC, AudienceFilter{AudienceType: AudienceCustomers}, channels...)
	if _, err := e.svc.PutContent(e.ctx, e.dealerC, camp.UUID, "tr", ContentInput{Title: "Kampanya", Body: "Merhaba"}); err != nil {
		t.Fatalf("content: %v", err)
	}
	e.exec(t, `UPDATE campaigns SET status = 'scheduled', scheduled_at = $2 WHERE uuid = $1`, camp.UUID, e.now.Add(-time.Minute))
	return e.campaignRow(t, camp.UUID)
}

func (e *senderEnv) customerWithConsent(t *testing.T, name string) db.User {
	t.Helper()
	return e.customer(t, e.dealer, name, "tr", ptr(true))
}

func (e *senderEnv) webPush(t *testing.T, u db.User) {
	t.Helper()
	e.exec(t, `INSERT INTO push_subscriptions (user_id, endpoint, key_p256dh, key_auth) VALUES ($1, $2, 'k', 'a')`,
		u.ID, "https://push.example.test/"+uuid.NewString())
}

func (e *senderEnv) tick(t *testing.T) {
	t.Helper()
	if err := e.sender.Tick(e.ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}
}

type recipientRow struct {
	ID       int64
	UserID   int64
	Channel  string
	Status   string
	Reason   string
	Attempts int32
}

func (e *senderEnv) recipients(t *testing.T, campaignID int64) []recipientRow {
	t.Helper()
	rows, err := e.tx.Query(e.ctx, `SELECT id, user_id, channel, status, COALESCE(reason, ''), attempts
		FROM campaign_recipients WHERE campaign_id = $1 ORDER BY id`, campaignID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []recipientRow
	for rows.Next() {
		var r recipientRow
		if err := rows.Scan(&r.ID, &r.UserID, &r.Channel, &r.Status, &r.Reason, &r.Attempts); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func (e *senderEnv) recipientOf(t *testing.T, campaignID, userID int64, channel string) recipientRow {
	t.Helper()
	for _, r := range e.recipients(t, campaignID) {
		if r.UserID == userID && r.Channel == channel {
			return r
		}
	}
	t.Fatalf("no %s recipient for user %d", channel, userID)
	return recipientRow{}
}

func (e *senderEnv) process(t *testing.T, id int64, final bool) error {
	t.Helper()
	return e.sender.ProcessRecipient(e.ctx, id, final)
}

func (e *senderEnv) mustProcess(t *testing.T, id int64) {
	t.Helper()
	if err := e.process(t, id, false); err != nil {
		t.Fatalf("process %d: %v", id, err)
	}
}

func (e *senderEnv) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.tx.QueryRow(e.ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func (e *senderEnv) outboxCount(name string) int {
	n := 0
	for _, ev := range e.out.events {
		if ev.Name == name {
			n++
		}
	}
	return n
}

// Acceptance: a due campaign starts once — two scheduler ticks write one
// started event, one snapshot and enqueue each recipient once; a campaign
// not yet due stays scheduled.
func TestCampaignTickStartsDueCampaignOnce(t *testing.T) {
	e := newSenderEnv(t)
	a := e.customerWithConsent(t, "a")
	b := e.customerWithConsent(t, "b")
	e.customer(t, e.dealer, "noconsent", "tr", nil)
	camp := e.scheduled(t, ChannelWhatsApp)
	later := e.scheduled(t, ChannelWhatsApp)
	e.exec(t, `UPDATE campaigns SET scheduled_at = $2 WHERE id = $1`, later.ID, e.now.Add(time.Hour))

	e.tick(t)
	e.tick(t)

	row := e.campaignRow(t, camp.Uuid)
	if row.Status != StatusSending || !row.StartedAt.Valid {
		t.Fatalf("status = %s started %v, want sending", row.Status, row.StartedAt)
	}
	started := 0
	for _, ev := range e.events(t, camp.Uuid) {
		if ev.EventType == EventStarted {
			started++
		}
	}
	if started != 1 || e.outboxCount(events.CampaignsStarted) != 1 {
		t.Fatalf("started events = %d, outbox = %d; want one each", started, e.outboxCount(events.CampaignsStarted))
	}
	rs := e.recipients(t, row.ID)
	if len(rs) != 2 || row.RecipientsTotal != 2 {
		t.Fatalf("recipients = %+v total %d, want a and b (no consent excluded)", rs, row.RecipientsTotal)
	}
	for _, r := range rs {
		if r.Status != RecipientPending || (r.UserID != a.ID && r.UserID != b.ID) {
			t.Fatalf("recipient %+v", r)
		}
	}
	// A re-enqueue is deduplicated by the task id (campaign-rcpt-<row>).
	unique := map[int64]bool{}
	for _, id := range e.queue.ids {
		unique[id] = true
	}
	if len(unique) != 2 {
		t.Fatalf("enqueued %v, want both recipients", e.queue.ids)
	}
	if st := e.campaignRow(t, later.Uuid).Status; st != StatusScheduled {
		t.Fatalf("not yet due campaign = %s, want scheduled", st)
	}
}

// Acceptance: running a recipient task twice sends once.
func TestCampaignRecipientTaskSendsOnce(t *testing.T) {
	e := newSenderEnv(t)
	u := e.customerWithConsent(t, "once")
	camp := e.scheduled(t, ChannelWhatsApp)
	e.tick(t)
	r := e.recipientOf(t, camp.ID, u.ID, ChannelWhatsApp)

	e.mustProcess(t, r.ID)
	e.mustProcess(t, r.ID)

	if len(e.ch.whatsapp) != 1 {
		t.Fatalf("whatsapp sends = %d, want 1", len(e.ch.whatsapp))
	}
	got := e.recipientOf(t, camp.ID, u.ID, ChannelWhatsApp)
	if got.Status != RecipientSent || got.Attempts != 1 {
		t.Fatalf("recipient = %+v, want sent after one attempt", got)
	}
	if m := e.ch.whatsapp[0]; m.Phone != u.PhoneE164.String || m.Body != "Merhaba" {
		t.Fatalf("message = %+v", m)
	}
	// The last recipient finished the campaign.
	if st := e.campaignRow(t, camp.Uuid).Status; st != StatusSent {
		t.Fatalf("campaign = %s, want sent", st)
	}
	if e.outboxCount(events.CampaignsFinished) != 1 {
		t.Fatal("campaigns.finished not written")
	}
}

// Acceptance: a recipient in quiet hours (21:00-09:00 in the recipient's
// zone) is deferred until 09:00; another zone at the same instant is sent.
func TestCampaignQuietHoursDefer(t *testing.T) {
	e := newSenderEnv(t)
	ist := e.customerWithConsent(t, "istanbul")
	utc := e.customerWithConsent(t, "london")
	e.exec(t, `UPDATE users SET timezone = 'UTC' WHERE id = $1`, utc.ID)
	camp := e.scheduled(t, ChannelWhatsApp)
	e.tick(t)
	e.now = time.Date(2026, 10, 7, 19, 0, 0, 0, time.UTC) // 22:00 Istanbul, 19:00 UTC

	err := e.process(t, e.recipientOf(t, camp.ID, ist.ID, ChannelWhatsApp).ID, false)
	var d *DeferredError
	if !errors.As(err, &d) || d.RetryAfter() != 11*time.Hour {
		t.Fatalf("err = %v, want deferral of 11h (until 09:00 Istanbul)", err)
	}
	if r := e.recipientOf(t, camp.ID, ist.ID, ChannelWhatsApp); r.Status != RecipientPending || r.Attempts != 0 {
		t.Fatalf("deferred recipient = %+v, want pending without attempt", r)
	}
	e.mustProcess(t, e.recipientOf(t, camp.ID, utc.ID, ChannelWhatsApp).ID)
	if len(e.ch.whatsapp) != 1 || e.ch.whatsapp[0].UserID != utc.ID {
		t.Fatalf("sends = %+v, want only the UTC recipient", e.ch.whatsapp)
	}

	e.now = time.Date(2026, 10, 8, 6, 0, 0, 0, time.UTC) // 09:00 Istanbul
	e.mustProcess(t, e.recipientOf(t, camp.ID, ist.ID, ChannelWhatsApp).ID)
	if len(e.ch.whatsapp) != 2 {
		t.Fatalf("sends after quiet hours = %d, want 2", len(e.ch.whatsapp))
	}
}

// Acceptance: at most 20 campaign WhatsApp messages per minute (fake
// clock); the rest wait for the next window without consuming a retry.
func TestCampaignWhatsAppPerMinuteLimit(t *testing.T) {
	e := newSenderEnv(t)
	for i := 0; i < 25; i++ {
		e.customerWithConsent(t, fmt.Sprintf("rate%d", i))
	}
	camp := e.scheduled(t, ChannelWhatsApp)
	e.tick(t)
	rs := e.recipients(t, camp.ID)
	if len(rs) != 25 {
		t.Fatalf("recipients = %d", len(rs))
	}
	deferred := 0
	for _, r := range rs {
		err := e.process(t, r.ID, false)
		var d *DeferredError
		switch {
		case err == nil:
		case errors.As(err, &d) && d.RetryAfter() > 0 && d.RetryAfter() <= time.Minute:
			deferred++
		default:
			t.Fatalf("process: %v", err)
		}
	}
	if len(e.ch.whatsapp) != 20 || deferred != 5 {
		t.Fatalf("first minute: sent %d deferred %d, want 20 / 5", len(e.ch.whatsapp), deferred)
	}
	e.now = e.now.Add(time.Minute)
	for _, r := range e.recipients(t, camp.ID) {
		if r.Status == RecipientPending {
			e.mustProcess(t, r.ID)
		}
	}
	if len(e.ch.whatsapp) != 25 {
		t.Fatalf("after a minute sent %d, want 25", len(e.ch.whatsapp))
	}
	if st := e.campaignRow(t, camp.Uuid).Status; st != StatusSent {
		t.Fatalf("campaign = %s", st)
	}
}

// Acceptance: a recipient without a push token is skipped with its reason
// (in the snapshot, and at send time when the token is gone).
func TestCampaignPushWithoutTokenSkipped(t *testing.T) {
	e := newSenderEnv(t)
	withToken := e.customerWithConsent(t, "token")
	noToken := e.customerWithConsent(t, "notoken")
	gone := e.customerWithConsent(t, "gone")
	e.webPush(t, withToken)
	e.webPush(t, gone)
	camp := e.scheduled(t, ChannelPush)
	e.tick(t)

	if r := e.recipientOf(t, camp.ID, noToken.ID, ChannelPush); r.Status != RecipientSkipped || r.Reason != SkipNoPushToken {
		t.Fatalf("no token recipient = %+v, want skipped no_push_token", r)
	}
	e.ch.noPush[gone.ID] = true
	e.mustProcess(t, e.recipientOf(t, camp.ID, withToken.ID, ChannelPush).ID)
	e.mustProcess(t, e.recipientOf(t, camp.ID, gone.ID, ChannelPush).ID)
	if r := e.recipientOf(t, camp.ID, gone.ID, ChannelPush); r.Status != RecipientSkipped || r.Reason != SkipNoPushToken {
		t.Fatalf("token gone recipient = %+v, want skipped", r)
	}
	if len(e.ch.push) != 1 || e.ch.push[0].UserID != withToken.ID || e.ch.push[0].Title != "Kampanya" {
		t.Fatalf("pushes = %+v", e.ch.push)
	}
	row := e.campaignRow(t, camp.Uuid)
	if row.Status != StatusSent || row.RecipientsSkipped != 2 || row.RecipientsSent != 1 {
		t.Fatalf("campaign = %s sent %d skipped %d", row.Status, row.RecipientsSent, row.RecipientsSkipped)
	}
}

// Acceptance: after a cancel the remaining recipients are not sent.
func TestCampaignCancelStopsRemainingRecipients(t *testing.T) {
	e := newSenderEnv(t)
	first := e.customerWithConsent(t, "first")
	rest := []db.User{e.customerWithConsent(t, "r1"), e.customerWithConsent(t, "r2")}
	camp := e.scheduled(t, ChannelWhatsApp)
	e.tick(t)
	e.mustProcess(t, e.recipientOf(t, camp.ID, first.ID, ChannelWhatsApp).ID)

	if _, err := e.svc.Cancel(e.ctx, e.dealerC, camp.Uuid); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	for _, u := range rest {
		e.mustProcess(t, e.recipientOf(t, camp.ID, u.ID, ChannelWhatsApp).ID)
		if r := e.recipientOf(t, camp.ID, u.ID, ChannelWhatsApp); r.Status != RecipientSkipped {
			t.Fatalf("recipient after cancel = %+v, want skipped", r)
		}
	}
	if len(e.ch.whatsapp) != 1 {
		t.Fatalf("sends = %d, want only the one before the cancel", len(e.ch.whatsapp))
	}
	e.tick(t)
	if st := e.campaignRow(t, camp.Uuid).Status; st != StatusCancelled {
		t.Fatalf("campaign = %s, want cancelled", st)
	}
}

// Acceptance: the statistics projection equals the recipient table over
// sent, failed (permanent error, retries exhausted) and skipped rows; a
// failed recipient makes the campaign partially_failed. Transient errors
// keep the row pending and count the attempt.
func TestCampaignStatisticsMatchRecipients(t *testing.T) {
	e := newSenderEnv(t)
	ok := e.customerWithConsent(t, "ok")
	perm := e.customerWithConsent(t, "perm")
	flaky := e.customerWithConsent(t, "flaky")
	noPhone := e.customerWithConsent(t, "nophone")
	e.exec(t, `UPDATE users SET phone_e164 = NULL WHERE id = $1`, noPhone.ID)
	for _, u := range []db.User{ok, perm, flaky} {
		e.webPush(t, u)
	}
	camp := e.scheduled(t, ChannelPush, ChannelWhatsApp, ChannelEmail)
	e.tick(t)
	e.ch.fail[perm.ID] = &PermanentError{Err: errors.New("invalid recipient")}
	e.ch.fail[flaky.ID] = errors.New("gateway timeout")
	e.ch.failEmail[perm.Email.String] = e.ch.fail[perm.ID]
	e.ch.failEmail[flaky.Email.String] = e.ch.fail[flaky.ID]

	for _, r := range e.recipients(t, camp.ID) {
		if r.Status != RecipientPending {
			continue
		}
		if r.UserID == flaky.ID {
			if err := e.process(t, r.ID, false); err == nil {
				t.Fatal("transient error must be returned for a retry")
			}
			if got := e.recipientOf(t, camp.ID, r.UserID, r.Channel); got.Status != RecipientPending || got.Attempts != 1 {
				t.Fatalf("after a transient error = %+v, want pending with one attempt", got)
			}
			if err := e.process(t, r.ID, true); err != nil {
				t.Fatalf("final attempt: %v", err)
			}
			continue
		}
		e.mustProcess(t, r.ID)
	}

	row := e.campaignRow(t, camp.Uuid)
	var total, sent, failed, skipped int
	if err := e.tx.QueryRow(e.ctx, `SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'sent'),
		COUNT(*) FILTER (WHERE status = 'failed'), COUNT(*) FILTER (WHERE status = 'skipped')
		FROM campaign_recipients WHERE campaign_id = $1`, row.ID).Scan(&total, &sent, &failed, &skipped); err != nil {
		t.Fatal(err)
	}
	if int(row.RecipientsTotal) != total || int(row.RecipientsSent) != sent ||
		int(row.RecipientsFailed) != failed || int(row.RecipientsSkipped) != skipped {
		t.Fatalf("projection %d/%d/%d/%d != table %d/%d/%d/%d", row.RecipientsTotal, row.RecipientsSent,
			row.RecipientsFailed, row.RecipientsSkipped, total, sent, failed, skipped)
	}
	// 4 users x 3 channels; ok: 3 sent; perm + flaky: push/whatsapp failed
	// (e-mail fails too: same user errors); nophone: push skipped (no
	// token), whatsapp skipped (no phone), e-mail skipped (no unsubscribe).
	if total != 12 || sent != 3 || failed != 6 || skipped != 3 {
		t.Fatalf("table = %d/%d/%d/%d", total, sent, failed, skipped)
	}
	if r := e.recipientOf(t, camp.ID, noPhone.ID, ChannelEmail); r.Reason != SkipNoUnsubscribe {
		t.Fatalf("no phone e-mail = %+v", r)
	}
	if row.Status != StatusPartiallyFailed || !row.FinishedAt.Valid {
		t.Fatalf("campaign = %s, want partially_failed", row.Status)
	}
	finished := e.out.last(t, events.CampaignsFinished)
	if finished.Payload["recipients_failed"] != int32(6) {
		t.Fatalf("finished payload = %v", finished.Payload)
	}
	if len(e.ch.email) != 1 || !strings.Contains(e.ch.email[0].UnsubscribeURL, "https://app.example.test"+UnsubscribePath) {
		t.Fatalf("e-mails = %+v", e.ch.email)
	}
}

// A WhatsApp DUR / e-mail unsubscribe opt-out: later campaigns leave the
// customer out, and an opt-out after the snapshot stops the send.
func TestCampaignMarketingOptOut(t *testing.T) {
	e := newSenderEnv(t)
	stay := e.customerWithConsent(t, "stay")
	leave := e.customerWithConsent(t, "leave")
	late := e.customerWithConsent(t, "late")

	unsub := NewUnsubscriber(e.q, []byte("test-secret"))
	token := strings.TrimPrefix(e.sender.UnsubscribeURL(leave.Uuid), "https://app.example.test"+UnsubscribePath)
	if err := unsub.Unsubscribe(e.ctx, token); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	if err := unsub.Unsubscribe(e.ctx, token); err != nil {
		t.Fatalf("repeated unsubscribe: %v", err)
	}
	if err := unsub.Unsubscribe(e.ctx, token[:len(token)-2]+"AA"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tampered token err = %v, want not found", err)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM contact_opt_outs WHERE contact_e164 = $1 AND scope = 'marketing' AND source = 'campaign'`,
		leave.PhoneE164.String); n != 1 {
		t.Fatalf("opt-out rows = %d, want 1", n)
	}

	camp := e.scheduled(t, ChannelWhatsApp)
	e.tick(t)
	rs := e.recipients(t, camp.ID)
	if len(rs) != 2 {
		t.Fatalf("recipients = %+v, want stay and late only", rs)
	}
	// DUR after the snapshot (same opt-out ledger as F4-02c).
	e.exec(t, `INSERT INTO contact_opt_outs (contact_e164, scope, action, source) VALUES ($1, 'marketing', 'out', 'whatsapp')`,
		late.PhoneE164.String)
	for _, r := range rs {
		e.mustProcess(t, r.ID)
	}
	if r := e.recipientOf(t, camp.ID, late.ID, ChannelWhatsApp); r.Status != RecipientSkipped || r.Reason != SkipOptedOut {
		t.Fatalf("late opt-out = %+v, want skipped opted_out", r)
	}
	if len(e.ch.whatsapp) != 1 || e.ch.whatsapp[0].UserID != stay.ID {
		t.Fatalf("sends = %+v", e.ch.whatsapp)
	}
}

func TestQuietWait(t *testing.T) {
	ist, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		t.Skip("tzdata missing")
	}
	at := func(h, m int) time.Time { return time.Date(2026, 10, 7, h, m, 0, 0, ist) }
	cases := []struct {
		now        time.Time
		start, end int
		want       time.Duration
	}{
		{at(12, 0), 21, 9, 0},
		{at(21, 0), 21, 9, 12 * time.Hour},
		{at(23, 30), 21, 9, 9*time.Hour + 30*time.Minute},
		{at(8, 59), 21, 9, time.Minute},
		{at(9, 0), 21, 9, 0},
		{at(13, 0), 12, 14, time.Hour},
		{at(3, 0), 9, 9, 0},
	}
	for _, c := range cases {
		if got := QuietWait(c.now, ist, c.start, c.end); got != c.want {
			t.Errorf("QuietWait(%s, %d-%d) = %s, want %s", c.now.Format("15:04"), c.start, c.end, got, c.want)
		}
	}
}

func TestUnsubscribeToken(t *testing.T) {
	u := uuid.New()
	tok := UnsubscribeToken([]byte("s1"), u)
	if got, ok := ParseUnsubscribeToken([]byte("s1"), tok); !ok || got != u {
		t.Fatalf("parse = %v %v", got, ok)
	}
	if _, ok := ParseUnsubscribeToken([]byte("s2"), tok); ok {
		t.Fatal("token verified with another secret")
	}
	if _, ok := ParseUnsubscribeToken([]byte("s1"), "garbage"); ok {
		t.Fatal("garbage verified")
	}
}
