package usecase

// TEC-407 (F4-04d): campaign sending. The scheduler tick (every minute, on
// the worker-core leader) moves due scheduled campaigns to sending, takes
// the recipient snapshot (audience resolved at that moment: consent,
// opt-out, preferences, locale) and enqueues one task per recipient row.
// A recipient task sends on one channel; its result and the statistics
// projection (campaign_recipients triggers) are written in one transaction,
// and the last result finishes the campaign (sent / partially_failed).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Campaign lifecycle event types written by the sender.
const (
	EventStarted  = "started"
	EventFinished = "finished"
)

// Recipient statuses (chk_campaign_recipients_status).
const (
	RecipientPending = "pending"
	RecipientSent    = "sent"
	RecipientFailed  = "failed"
	RecipientSkipped = "skipped"
)

// Skip reasons of recipient rows.
const (
	SkipNoPushToken        = "no_push_token"
	SkipNoEmail            = "no_email"
	SkipNoPhone            = "no_phone"
	SkipChannelDisabled    = "channel_disabled"
	SkipOptedOut           = "opted_out"
	SkipLocaleMissing      = "locale_missing"
	SkipNoUnsubscribe      = "no_unsubscribe_address"
	SkipCampaignNotSending = "campaign_not_sending"
)

// FailQuietHoursExhausted: the last retry could not be deferred.
const FailDeferralExhausted = "deferral_exhausted"

const (
	// DefaultTimezone is used when neither the recipient nor the campaign
	// organization has a valid zone.
	DefaultTimezone = "Europe/Istanbul"
	// pushImageTTL is the lifetime of the presigned push image URL.
	pushImageTTL = 7 * 24 * time.Hour
	// staleRecipientAfter: a pending recipient untouched this long gets its
	// task enqueued again by the tick (lost task after an enqueue error).
	staleRecipientAfter = 10 * time.Minute
	tickBatch           = 50
	requeueBatch        = 500
	maxReasonLen        = 1000
	whatsAppLimitAction = "campaign_whatsapp"
	whatsAppLimitScope  = "all"
)

var (
	e164Re  = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
	emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+$`)
)

// ErrNoAddress is returned by a channel when the user has no address on it
// (no push token / subscription); the recipient is skipped.
var ErrNoAddress = errors.New("campaigns: recipient has no address on the channel")

// PermanentError marks a channel error a retry cannot fix.
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// DeferredError postpones a recipient task (quiet hours, WhatsApp rate
// limit) without consuming a retry (queue.RetryAfterError).
type DeferredError struct {
	Reason string
	Wait   time.Duration
}

func (e *DeferredError) Error() string {
	return fmt.Sprintf("campaigns: recipient deferred (%s) for %s", e.Reason, e.Wait)
}

// RetryAfter is the delay before the next run.
func (e *DeferredError) RetryAfter() time.Duration { return e.Wait }

// PushMessage is a campaign push to every device / browser of a user.
type PushMessage struct {
	UserID   int64
	BrandID  int64
	Locale   string
	Title    string
	Body     string
	Deeplink string
	ImageURL string
}

// EmailMessage is a campaign e-mail (markdown body).
type EmailMessage struct {
	To             string
	BrandID        int64
	Locale         string
	Subject        string
	Body           string
	Deeplink       string
	UnsubscribeURL string
}

// WhatsAppMedia is an image or PDF attached to a WhatsApp message.
type WhatsAppMedia struct {
	Data     []byte
	FileName string
}

// WhatsAppMessage is a campaign WhatsApp message (text + media).
type WhatsAppMessage struct {
	UserID int64
	Phone  string
	Name   string
	Body   string
	Media  []WhatsAppMedia
}

// PushSender sends a push; ErrNoAddress when the user has no token.
type PushSender interface {
	SendCampaignPush(ctx context.Context, m PushMessage) error
}

// EmailSender sends a campaign e-mail.
type EmailSender interface {
	SendCampaignEmail(ctx context.Context, m EmailMessage) error
}

// WhatsAppSender hands a message to the WhatsApp send path.
type WhatsAppSender interface {
	SendCampaignWhatsApp(ctx context.Context, m WhatsAppMessage) error
}

// MediaStore reads campaign media (storage.Driver).
type MediaStore interface {
	Download(ctx context.Context, objectPath string) (io.ReadCloser, int64, error)
	PresignGet(ctx context.Context, objectPath string, expiry time.Duration) (string, error)
}

// RecipientQueue enqueues the task of one recipient row (deduplicated by
// the row id).
type RecipientQueue interface {
	EnqueueRecipient(ctx context.Context, recipientID int64) error
}

// Limiter is a fixed-window limiter (ratelimit.Limiter).
type Limiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// SendSettings are the sysconfig campaign keys.
type SendSettings interface {
	CampaignsWhatsAppPerMinute(ctx context.Context) int
	CampaignsQuietHours(ctx context.Context) (int, int)
}

// SenderDeps wires the sender. A nil channel fails its recipients.
type SenderDeps struct {
	Push     PushSender
	Email    EmailSender
	WhatsApp WhatsAppSender
	Media    MediaStore
	Queue    RecipientQueue
	Limiter  Limiter
	Settings SendSettings
	// UnsubscribeSecret signs the e-mail unsubscribe links; FrontendURL is
	// their origin (/abonelik-iptal/{token}).
	UnsubscribeSecret []byte
	FrontendURL       string
	Log               *slog.Logger
}

// Sender runs the scheduler tick and the recipient tasks.
type Sender struct {
	svc *Service
	d   SenderDeps
	log *slog.Logger
}

// NewSender builds the sender over the campaign service (pool, queries,
// outbox and clock).
func NewSender(svc *Service, d SenderDeps) *Sender {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Sender{svc: svc, d: d, log: log}
}

// Tick starts due campaigns, finishes sending campaigns without pending
// recipients and re-enqueues stale pending recipients. Two ticks never
// start a campaign twice: the start locks the row and moves it from
// scheduled only once.
func (s *Sender) Tick(ctx context.Context) error {
	now := s.svc.now()
	due, err := s.svc.q.ListDueScheduledCampaigns(ctx, db.ListDueScheduledCampaignsParams{
		Now: ts(now), PageLimit: tickBatch,
	})
	if err != nil {
		return err
	}
	for _, row := range due {
		started, err := s.start(ctx, row.ID)
		if err != nil {
			s.log.Error("campaign_start_failed", "campaign", row.Uuid, "error", err)
			continue
		}
		if started {
			s.log.Info("campaign_started", "campaign", row.Uuid)
		}
	}
	sending, err := s.svc.q.ListSendingCampaigns(ctx, 200)
	if err != nil {
		return err
	}
	for _, row := range sending {
		var finished bool
		if err := s.svc.inTxOut(ctx, func(tx pgx.Tx, q *db.Queries) error {
			var err error
			finished, err = s.finishIfDone(ctx, tx, q, row.ID)
			return err
		}); err != nil {
			s.log.Error("campaign_finish_failed", "campaign", row.Uuid, "error", err)
			continue
		}
		if finished {
			continue
		}
		before := now.Add(-staleRecipientAfter)
		if row.StartedAt.Valid && row.StartedAt.Time.After(before) {
			// Just started: its tasks were enqueued by start.
			continue
		}
		s.enqueuePending(ctx, row.ID, &before)
	}
	return nil
}

// start moves a due campaign to sending and writes its snapshot. false:
// another tick (or a cancel) moved it meanwhile.
func (s *Sender) start(ctx context.Context, id int64) (bool, error) {
	now := s.svc.now()
	var started db.Campaign
	err := s.svc.inTxOut(ctx, func(tx pgx.Tx, q *db.Queries) error {
		cur, err := q.GetCampaignByID(ctx, id)
		if err != nil {
			return err
		}
		row, err := q.GetCampaignByIDForUpdate(ctx, db.GetCampaignByIDForUpdateParams{ID: id, BrandID: cur.BrandID})
		if err != nil {
			return err
		}
		if row.Status != StatusScheduled || !row.ScheduledAt.Valid || row.ScheduledAt.Time.After(now) {
			return nil
		}
		aud, err := s.svc.ResolveAudience(ctx, q, row)
		if err != nil {
			return err
		}
		contents, err := q.ListCampaignContents(ctx, row.ID)
		if err != nil {
			return err
		}
		rows, err := s.snapshotRows(ctx, q, row, aud, contents)
		if err != nil {
			return err
		}
		moved, err := q.SetCampaignStatus(ctx, db.SetCampaignStatusParams{
			ID: row.ID, FromStatus: StatusScheduled, Status: StatusSending,
			ApproverOrgID: row.ApproverOrgID, ScheduledAt: row.ScheduledAt,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		raw, err := json.Marshal(rows)
		if err != nil {
			return err
		}
		if _, err := q.InsertCampaignRecipientSnapshot(ctx, db.InsertCampaignRecipientSnapshotParams{
			CampaignID: row.ID, OrganizationID: row.OrganizationID, BrandID: row.BrandID, Rows: raw,
		}); err != nil {
			return err
		}
		// Counters were updated by the snapshot trigger.
		moved, err = q.GetCampaignByID(ctx, moved.ID)
		if err != nil {
			return err
		}
		if err := s.lifecycle(ctx, tx, q, row, moved, EventStarted, events.CampaignsStarted); err != nil {
			return err
		}
		started = moved
		return nil
	})
	if err != nil || started.ID == 0 {
		return false, err
	}
	s.enqueuePending(ctx, started.ID, nil)
	return true, nil
}

// snapshotRow is one element of InsertCampaignRecipientSnapshot.rows.
type snapshotRow struct {
	UserID         int64   `json:"user_id"`
	Channel        string  `json:"channel"`
	Locale         string  `json:"locale"`
	TargetAddress  *string `json:"target_address"`
	PushTokenCount *int32  `json:"push_token_count"`
	Status         string  `json:"status"`
	Reason         *string `json:"reason"`
}

// snapshotRows builds one row per (recipient, campaign channel). Rows that
// cannot be sent are stored as skipped with their reason.
func (s *Sender) snapshotRows(ctx context.Context, q *db.Queries, row db.Campaign, aud Audience,
	contents []db.CampaignContent) ([]snapshotRow, error) {
	byLocale := map[string]db.CampaignContent{}
	for _, ct := range contents {
		byLocale[ct.Locale] = ct
	}
	web := map[int64]int32{}
	if contains(row.Channels, ChannelPush) && len(aud.Recipients) > 0 {
		ids := make([]int64, 0, len(aud.Recipients))
		for _, r := range aud.Recipients {
			ids = append(ids, r.UserID)
		}
		counts, err := q.CountWebPushSubscriptionsByUsers(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, c := range counts {
			web[c.UserID] = c.Subscriptions
		}
	}
	out := make([]snapshotRow, 0, len(aud.Recipients)*len(row.Channels))
	for _, r := range aud.Recipients {
		locale := string(i18n.Normalize(r.Locale))
		phone := ""
		if e164Re.MatchString(r.PhoneE164) {
			phone = r.PhoneE164
		}
		email := ""
		if emailRe.MatchString(r.Email) && len(r.Email) <= 320 {
			email = r.Email
		}
		for _, ch := range row.Channels {
			sr := snapshotRow{UserID: r.UserID, Channel: ch, Locale: locale, Status: RecipientPending}
			reason := ""
			switch ch {
			case ChannelPush:
				n := r.PushTokens + web[r.UserID]
				sr.PushTokenCount = &n
				switch {
				case n == 0:
					reason = SkipNoPushToken
				case !r.PrefPush:
					reason = SkipChannelDisabled
				}
			case ChannelWhatsApp:
				switch {
				case phone == "":
					reason = SkipNoPhone
				case !r.PrefWhatsApp:
					reason = SkipChannelDisabled
				case r.MarketingOptedOut:
					reason = SkipOptedOut
				}
				if phone != "" {
					p := phone
					sr.TargetAddress = &p
				}
			case ChannelEmail:
				switch {
				case email == "":
					reason = SkipNoEmail
				case !r.PrefEmail:
					reason = SkipChannelDisabled
				case r.MarketingOptedOut:
					reason = SkipOptedOut
				case phone == "":
					// The unsubscribe link writes the marketing opt-out of
					// the phone number; without one it could not work.
					reason = SkipNoUnsubscribe
				}
				if email != "" {
					e := email
					sr.TargetAddress = &e
				}
			}
			if reason == "" {
				ct, ok := byLocale[locale]
				if !ok || !complete([]string{ch}, ct.Title, ct.Body) {
					reason = SkipLocaleMissing
				}
			}
			if reason != "" {
				sr.Status = RecipientSkipped
				sr.Reason = &reason
			}
			out = append(out, sr)
		}
	}
	return out, nil
}

// enqueuePending enqueues the tasks of pending recipients (not touched
// since before, when set); a task still queued is deduplicated by its id.
func (s *Sender) enqueuePending(ctx context.Context, campaignID int64, before *time.Time) {
	if s.d.Queue == nil {
		return
	}
	var after int64
	for {
		ids, err := s.svc.q.ListPendingCampaignRecipientIDs(ctx, db.ListPendingCampaignRecipientIDsParams{
			CampaignID: campaignID, Before: pgTime(before), AfterID: after, PageLimit: requeueBatch,
		})
		if err != nil {
			s.log.Error("campaign_recipients_list_failed", "campaign_id", campaignID, "error", err)
			return
		}
		for _, id := range ids {
			if err := s.d.Queue.EnqueueRecipient(ctx, id); err != nil {
				// The row stays pending; a later tick enqueues it again.
				s.log.Warn("campaign_recipient_enqueue_failed", "recipient_id", id, "error", err)
				return
			}
			after = id
		}
		if len(ids) < requeueBatch {
			return
		}
	}
}

// ProcessRecipient runs the task of one recipient row. A second run (or a
// concurrent one, blocked by the row lock) finds the row no longer pending
// and sends nothing. final is true on the last retry: a failure is then
// recorded as failed instead of being retried.
func (s *Sender) ProcessRecipient(ctx context.Context, recipientID int64, final bool) error {
	tx, err := s.svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.svc.q.WithTx(tx)
	r, err := q.LockCampaignRecipient(ctx, recipientID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if r.Status != RecipientPending {
		return nil
	}
	row, err := q.GetCampaignByID(ctx, r.CampaignID)
	if err != nil {
		return err
	}
	if row.Status != StatusSending {
		return s.end(ctx, tx, q, r, RecipientSkipped, SkipCampaignNotSending)
	}
	ct, err := q.GetCampaignContent(ctx, db.GetCampaignContentParams{CampaignID: row.ID, Locale: r.Locale})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !complete([]string{r.Channel}, ct.Title, ct.Body)) {
		return s.end(ctx, tx, q, r, RecipientSkipped, SkipLocaleMissing)
	}
	if err != nil {
		return err
	}
	sendErr := s.send(ctx, q, row, r, ct)
	var deferred *DeferredError
	switch {
	case sendErr == nil:
		return s.end(ctx, tx, q, r, RecipientSent, "")
	case errors.As(sendErr, &deferred):
		if final {
			return s.end(ctx, tx, q, r, RecipientFailed, FailDeferralExhausted)
		}
		return sendErr
	case errors.Is(sendErr, ErrNoAddress):
		return s.end(ctx, tx, q, r, RecipientSkipped, skipReasonOf(r.Channel))
	case errors.Is(sendErr, errSkip):
		var sk *skipError
		errors.As(sendErr, &sk)
		return s.end(ctx, tx, q, r, RecipientSkipped, sk.reason)
	}
	var perm *PermanentError
	if final || errors.As(sendErr, &perm) {
		s.log.Warn("campaign_recipient_failed", "recipient_id", r.ID, "channel", r.Channel, "error", sendErr)
		return s.end(ctx, tx, q, r, RecipientFailed, sendErr.Error())
	}
	if _, err := q.RecordCampaignRecipientAttempt(ctx, db.RecordCampaignRecipientAttemptParams{
		ID: r.ID, Reason: pgtype.Text{String: truncate(sendErr.Error()), Valid: true},
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return sendErr
}

var errSkip = errors.New("campaigns: recipient skipped")

// skipError skips a recipient with a reason found at send time.
type skipError struct{ reason string }

func (e *skipError) Error() string { return errSkip.Error() + ": " + e.reason }
func (e *skipError) Unwrap() error { return errSkip }

func skipReasonOf(channel string) string {
	switch channel {
	case ChannelWhatsApp:
		return SkipNoPhone
	case ChannelEmail:
		return SkipNoEmail
	default:
		return SkipNoPushToken
	}
}

// end writes the result of a recipient, finishes the campaign when it was
// the last pending one and commits.
func (s *Sender) end(ctx context.Context, tx pgx.Tx, q *db.Queries, r db.CampaignRecipient, status, reason string) error {
	var rs pgtype.Text
	if reason != "" {
		rs = pgtype.Text{String: truncate(reason), Valid: true}
	}
	if _, err := q.SetCampaignRecipientStatus(ctx, db.SetCampaignRecipientStatusParams{
		ID: r.ID, Status: status, Reason: rs,
	}); err != nil {
		return err
	}
	if _, err := s.finishIfDone(ctx, tx, q, r.CampaignID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// finishIfDone ends a sending campaign without pending recipients. The
// campaign row lock serializes the last results of concurrent tasks.
func (s *Sender) finishIfDone(ctx context.Context, tx pgx.Tx, q *db.Queries, campaignID int64) (bool, error) {
	cur, err := q.GetCampaignByID(ctx, campaignID)
	if err != nil {
		return false, err
	}
	row, err := q.GetCampaignByIDForUpdate(ctx, db.GetCampaignByIDForUpdateParams{ID: campaignID, BrandID: cur.BrandID})
	if err != nil {
		return false, err
	}
	if row.Status != StatusSending {
		return false, nil
	}
	n, err := q.CountPendingCampaignRecipients(ctx, row.ID)
	if err != nil || n > 0 {
		return false, err
	}
	to := StatusSent
	if row.RecipientsFailed > 0 {
		to = StatusPartiallyFailed
	}
	moved, err := q.SetCampaignStatus(ctx, db.SetCampaignStatusParams{
		ID: row.ID, FromStatus: StatusSending, Status: to,
		ApproverOrgID: row.ApproverOrgID, ScheduledAt: row.ScheduledAt,
	})
	if err != nil {
		return false, err
	}
	if err := s.lifecycle(ctx, tx, q, row, moved, EventFinished, events.CampaignsFinished); err != nil {
		return false, err
	}
	return true, nil
}

// lifecycle writes the campaign_events row and the outbox event of a
// system transition (no actor).
func (s *Sender) lifecycle(ctx context.Context, tx pgx.Tx, q *db.Queries, from, to db.Campaign, eventType, outboxName string) error {
	counters := map[string]any{
		"recipients_total": to.RecipientsTotal, "recipients_sent": to.RecipientsSent,
		"recipients_failed": to.RecipientsFailed, "recipients_skipped": to.RecipientsSkipped,
	}
	raw, err := json.Marshal(counters)
	if err != nil {
		return err
	}
	if _, err := q.InsertCampaignEvent(ctx, db.InsertCampaignEventParams{
		CampaignID: to.ID, OrganizationID: to.OrganizationID, BrandID: to.BrandID, EventType: eventType,
		FromStatus: pgtype.Text{String: from.Status, Valid: true}, ToStatus: pgtype.Text{String: to.Status, Valid: true},
		Payload: raw,
	}); err != nil {
		return err
	}
	if s.svc.out == nil {
		return nil
	}
	id, uid := to.ID, to.Uuid
	payload := map[string]any{
		"campaign_uuid": to.Uuid.String(), "campaign_name": to.Name, "brand_id": to.BrandID,
		"organization_id": to.OrganizationID, "status": to.Status,
	}
	for k, v := range counters {
		payload[k] = v
	}
	ev := events.New(outboxName).WithTenant(to.OrganizationID).WithEntity("campaign", &id, &uid).WithPayload(payload)
	if err := s.svc.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("campaigns: outbox: %w", err)
	}
	return nil
}

// send delivers one recipient on its channel.
func (s *Sender) send(ctx context.Context, q *db.Queries, row db.Campaign, r db.CampaignRecipient, ct db.CampaignContent) error {
	contact, err := q.GetCampaignRecipientContact(ctx, r.UserID)
	if err != nil {
		return err
	}
	deeplink := ""
	if ct.Deeplink.Valid {
		deeplink = ct.Deeplink.String
	}
	switch r.Channel {
	case ChannelPush:
		if s.d.Push == nil {
			return &PermanentError{Err: errors.New("campaigns: push is not configured")}
		}
		image, err := s.pushImage(ctx, q, row.ID, r.Locale)
		if err != nil {
			return err
		}
		return s.d.Push.SendCampaignPush(ctx, PushMessage{
			UserID: r.UserID, BrandID: row.BrandID, Locale: r.Locale, Title: ct.Title, Body: ct.Body,
			Deeplink: deeplink, ImageURL: image,
		})
	case ChannelEmail:
		if s.d.Email == nil {
			return &PermanentError{Err: errors.New("campaigns: e-mail is not configured")}
		}
		if !r.TargetAddress.Valid {
			return ErrNoAddress
		}
		if contact.PhoneE164 == "" {
			return &skipError{reason: SkipNoUnsubscribe}
		}
		return s.d.Email.SendCampaignEmail(ctx, EmailMessage{
			To: r.TargetAddress.String, BrandID: row.BrandID, Locale: r.Locale, Subject: ct.Title,
			Body: ct.Body, Deeplink: deeplink, UnsubscribeURL: s.UnsubscribeURL(contact.Uuid),
		})
	case ChannelWhatsApp:
		return s.sendWhatsApp(ctx, q, row, r, ct, contact)
	}
	return &PermanentError{Err: fmt.Errorf("campaigns: unknown channel %q", r.Channel)}
}

func (s *Sender) sendWhatsApp(ctx context.Context, q *db.Queries, row db.Campaign, r db.CampaignRecipient,
	ct db.CampaignContent, contact db.GetCampaignRecipientContactRow) error {
	if s.d.WhatsApp == nil {
		return &PermanentError{Err: errors.New("campaigns: whatsapp is not configured")}
	}
	if !r.TargetAddress.Valid {
		return ErrNoAddress
	}
	phone := r.TargetAddress.String
	// A DUR reply after the snapshot still stops the message.
	st, err := q.GetContactOptOutState(ctx, db.GetContactOptOutStateParams{ContactE164: phone, Scope: "marketing"})
	if err == nil && st.OptedOut {
		return &skipError{reason: SkipOptedOut}
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if wait := s.quietWait(ctx, q, row, contact); wait > 0 {
		return &DeferredError{Reason: "quiet_hours", Wait: wait}
	}
	if s.d.Limiter != nil {
		if ok, wait := s.d.Limiter.Allow(ctx, whatsAppLimitAction, whatsAppLimitScope, s.whatsAppPerMinute(ctx), time.Minute); !ok {
			if wait <= 0 {
				wait = time.Second
			}
			return &DeferredError{Reason: "rate_limited", Wait: wait}
		}
	}
	media, err := s.whatsAppMedia(ctx, q, row.ID, r.Locale)
	if err != nil {
		return err
	}
	return s.d.WhatsApp.SendCampaignWhatsApp(ctx, WhatsAppMessage{
		UserID: r.UserID, Phone: phone, Name: strings.TrimSpace(contact.Name + " " + contact.Surname),
		Body: whatsAppBody(ct), Media: media,
	})
}

// whatsAppBody is the message text: the body and the deeplink.
func whatsAppBody(ct db.CampaignContent) string {
	body := strings.TrimSpace(ct.Body)
	if ct.Deeplink.Valid && ct.Deeplink.String != "" && !strings.Contains(body, ct.Deeplink.String) {
		body += "\n\n" + ct.Deeplink.String
	}
	return body
}

func (s *Sender) whatsAppPerMinute(ctx context.Context) int {
	if s.d.Settings != nil {
		if n := s.d.Settings.CampaignsWhatsAppPerMinute(ctx); n > 0 {
			return n
		}
	}
	return 20
}

// quietWait is how long the recipient's quiet hours still last (0 =
// outside). Zone: the user's, else the campaign organization's.
func (s *Sender) quietWait(ctx context.Context, q *db.Queries, row db.Campaign, contact db.GetCampaignRecipientContactRow) time.Duration {
	start, end := 21, 9
	if s.d.Settings != nil {
		start, end = s.d.Settings.CampaignsQuietHours(ctx)
	}
	loc := loadZone(contact.Timezone)
	if loc == nil {
		if org, err := q.GetOrganizationByID(ctx, row.OrganizationID); err == nil {
			loc = loadZone(org.Timezone)
		}
	}
	if loc == nil {
		loc = loadZone(DefaultTimezone)
	}
	if loc == nil {
		loc = time.UTC
	}
	return QuietWait(s.svc.now(), loc, start, end)
}

func loadZone(name string) *time.Location {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil
	}
	return loc
}

// QuietWait returns how long until the quiet hours [start, end) (hours of
// day in loc) end, or 0 outside them. start == end disables them; start >
// end spans midnight (21 → 9).
func QuietWait(now time.Time, loc *time.Location, start, end int) time.Duration {
	if start == end || start < 0 || end < 0 || start > 23 || end > 23 {
		return 0
	}
	l := now.In(loc)
	h := l.Hour()
	quiet := h >= start && h < end
	if start > end {
		quiet = h >= start || h < end
	}
	if !quiet {
		return 0
	}
	next := time.Date(l.Year(), l.Month(), l.Day(), end, 0, 0, 0, loc)
	if !next.After(l) {
		next = time.Date(l.Year(), l.Month(), l.Day()+1, end, 0, 0, 0, loc)
	}
	return next.Sub(l)
}

// pushImage is a presigned URL of the first image of the locale, or "".
func (s *Sender) pushImage(ctx context.Context, q *db.Queries, campaignID int64, locale string) (string, error) {
	if s.d.Media == nil {
		return "", nil
	}
	media, err := q.ListCampaignMedia(ctx, campaignID)
	if err != nil {
		return "", err
	}
	for _, m := range media {
		if m.Locale == locale && m.Kind == MediaImage {
			url, err := s.d.Media.PresignGet(ctx, m.StorageKey, pushImageTTL)
			if err != nil {
				s.log.Warn("campaign_push_image_failed", "media", m.Uuid, "error", err)
				return "", nil
			}
			return url, nil
		}
	}
	return "", nil
}

// whatsAppMedia loads the images and PDFs of the locale in order.
func (s *Sender) whatsAppMedia(ctx context.Context, q *db.Queries, campaignID int64, locale string) ([]WhatsAppMedia, error) {
	media, err := q.ListCampaignMedia(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	var out []WhatsAppMedia
	for _, m := range media {
		if m.Locale != locale {
			continue
		}
		if s.d.Media == nil {
			return nil, &PermanentError{Err: errors.New("campaigns: media storage is not configured")}
		}
		rc, _, err := s.d.Media.Download(ctx, m.StorageKey)
		if err != nil {
			return nil, fmt.Errorf("campaigns: load media: %w", err)
		}
		data, err := io.ReadAll(io.LimitReader(rc, MaxDocumentBytes+1))
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("campaigns: load media: %w", err)
		}
		name := m.FileName.String
		if name == "" {
			name = m.Uuid.String() + extensionOf(m.MimeType)
		}
		out = append(out, WhatsAppMedia{Data: data, FileName: name})
	}
	return out, nil
}

func extensionOf(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "application/pdf":
		return ".pdf"
	}
	return ""
}

func truncate(s string) string {
	if r := []rune(s); len(r) > maxReasonLen {
		return string(r[:maxReasonLen])
	}
	return s
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
