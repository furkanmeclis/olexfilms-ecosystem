package usecase

// TEC-395 (F4-02d): conversation messaging. Outgoing AI, staff and system
// messages are stored first (status queued) and sent by the whatsapp:send
// task; delivery receipts move them to delivered/read; inbound media is
// copied from the gateway to private object storage; every new message and
// status change is published to the inbox realtime channels.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SendMaxRetry is how many times a failed send is retried (whatsapp:send
// MaxRetry): one try plus three retries, then the message is failed.
const SendMaxRetry = 3

// MaxBodyLength bounds the text of an outgoing message (WhatsApp allows
// 4096 characters).
const MaxBodyLength = 4096

// Realtime event types of the conversation inbox.
const (
	EventMessageCreated = "conversations.message.created"
	EventMessageUpdated = "conversations.message.updated"
)

// Reasons an inbound media file was not stored (media.storage_skipped).
const (
	MediaSkipTooLarge      = "too_large"
	MediaSkipUnsupported   = "unsupported_type"
	MediaSkipMismatch      = "type_mismatch"
	MediaSkipNoDownload    = "no_download"
	MediaSkipDownloadError = "download_failed"
)

// Document kinds the send helpers render.
const (
	DocumentServicePDF          = "service_pdf"
	DocumentWarrantyCertificate = "warranty_certificate"
)

// ErrRateLimited is returned by ProcessSend while the recipient's
// per-minute budget is used up; the task is retried later.
var ErrRateLimited = errors.New("whatsapp: send rate limit reached")

// SendQueue enqueues the background tasks of a message.
type SendQueue interface {
	EnqueueSend(ctx context.Context, messageID int64, messageUUID uuid.UUID) error
	EnqueueMediaStore(ctx context.Context, messageID int64, messageUUID uuid.UUID) error
}

// SendLimiter is a fixed-window limiter (ratelimit.Limiter).
type SendLimiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// ObjectStore is the object storage surface (storage.Driver).
type ObjectStore interface {
	Upload(ctx context.Context, file storage.File, objectPath string) error
	Download(ctx context.Context, objectPath string) (io.ReadCloser, int64, error)
}

// DocumentRef names the document to render. OrganizationID is the caller's
// organization whose scope was already checked (the export job
// organization of the PDF adapters).
type DocumentRef struct {
	ServiceUUID    uuid.UUID
	BrandID        int64
	OrganizationID int64
	Locale         string
}

// RenderedDocument is a rendered PDF.
type RenderedDocument struct {
	Data     []byte
	FileName string
}

// DocumentRenderer renders the PDFs sent by the helpers (service PDF,
// warranty certificate) with the same adapters as the export pipeline.
type DocumentRenderer interface {
	Render(ctx context.Context, kind string, ref DocumentRef) (RenderedDocument, error)
}

// MessagingDeps wires Messaging. Queue, Limiter, Publisher, Downloader,
// Storage and Documents may be nil (the feature is then off).
type MessagingDeps struct {
	Queries    *db.Queries
	Tx         TxBeginner
	Provider   whatsapp.Provider
	Downloader whatsapp.MediaDownloader
	Storage    ObjectStore
	Queue      SendQueue
	Limiter    SendLimiter
	// SendPerMinute returns whatsapp.send_per_minute (nil = default 30).
	SendPerMinute func(ctx context.Context) int
	Publisher     realtime.Publisher
	Documents     DocumentRenderer
	Log           *slog.Logger
}

// Messaging is the outgoing queue, receipt, inbound media and realtime use
// case of WhatsApp conversations.
type Messaging struct {
	d   MessagingDeps
	log *slog.Logger
	now func() time.Time
}

// NewMessaging builds the messaging use case.
func NewMessaging(d MessagingDeps) *Messaging {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Messaging{d: d, log: log, now: time.Now}
}

// SetClock replaces the clock (tests).
func (m *Messaging) SetClock(now func() time.Time) { m.now = now }

// SetDocuments sets the PDF renderer of the document helpers.
func (m *Messaging) SetDocuments(r DocumentRenderer) { m.d.Documents = r }

// OutgoingMedia is a document or image attached to an outgoing message.
// The content is sniffed: images (jpeg/png/webp) go out as images, PDFs as
// documents; anything else, or a file whose extension promises another
// type, is rejected.
type OutgoingMedia struct {
	Data     []byte
	FileName string
}

// OutgoingMessage is a message to queue. Body is the text, or the caption
// of the media.
type OutgoingMessage struct {
	ConversationID int64
	SenderType     string
	SenderUserID   *int64
	AIRunID        *int64
	Body           string
	Media          *OutgoingMedia
}

// storedMedia is the messages.media JSON of an outgoing file.
type storedMedia struct {
	Type     string `json:"type"`
	MimeType string `json:"mime_type,omitempty"`
	FileName string `json:"file_name,omitempty"`
	Caption  string `json:"caption,omitempty"`
}

// ClientMessageID is the provider message id of an outgoing message: its
// uuid without dashes, upper case (the idempotency key at the provider).
func ClientMessageID(id uuid.UUID) string {
	return strings.ToUpper(strings.ReplaceAll(id.String(), "-", ""))
}

// Queue stores an outgoing message as queued and enqueues its send task.
// The message uuid is the idempotency key: the provider message id is
// derived from it and the send task runs once per message.
func (m *Messaging) Queue(ctx context.Context, in OutgoingMessage) (db.Message, error) {
	switch in.SenderType {
	case model.SenderAI, model.SenderStaff, model.SenderSystem:
	default:
		return db.Message{}, fmt.Errorf("%w: sender_type", ErrInvalidRequest)
	}
	if in.SenderUserID != nil && in.SenderType != model.SenderStaff {
		return db.Message{}, fmt.Errorf("%w: sender_user_id is only for staff messages", ErrInvalidRequest)
	}
	if in.AIRunID != nil && in.SenderType == model.SenderStaff {
		return db.Message{}, fmt.Errorf("%w: ai_run_id is only for ai/system messages", ErrInvalidRequest)
	}
	body := strings.TrimSpace(in.Body)
	if len([]rune(body)) > MaxBodyLength {
		return db.Message{}, fmt.Errorf("%w: body is longer than %d characters", ErrInvalidRequest, MaxBodyLength)
	}
	if body == "" && in.Media == nil {
		return db.Message{}, fmt.Errorf("%w: body or media is required", ErrInvalidRequest)
	}
	conv, err := m.d.Queries.GetConversationByID(ctx, in.ConversationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Message{}, fmt.Errorf("%w: conversation", ErrInvalidRequest)
	}
	if err != nil {
		return db.Message{}, err
	}
	if conv.Channel != whatsapp.ChannelWhatsApp {
		return db.Message{}, fmt.Errorf("%w: conversation channel", ErrInvalidRequest)
	}

	id := uuid.New()
	params := db.InsertQueuedMessageParams{
		Uuid: id, ConversationID: conv.ID, OrganizationID: conv.OrganizationID, BrandID: conv.BrandID,
		Channel: whatsapp.ChannelWhatsApp, SenderType: in.SenderType, ExternalID: ClientMessageID(id),
		Body: text(body), SenderUserID: int8Ptr(in.SenderUserID), AiRunID: int8Ptr(in.AIRunID),
	}
	if in.Media != nil {
		mime, err := whatsapp.ValidateMedia(in.Media.Data, in.Media.FileName)
		if err != nil {
			return db.Message{}, fmt.Errorf("%w: %w", ErrInvalidRequest, err)
		}
		if mime == whatsapp.MimeOGG {
			return db.Message{}, fmt.Errorf("%w: %w", ErrInvalidRequest, whatsapp.ErrMediaType)
		}
		if m.d.Storage == nil {
			return db.Message{}, ErrNotConfigured
		}
		kind := "document"
		if whatsapp.IsImageMime(mime) {
			kind = "image"
		}
		name := strings.TrimSpace(in.Media.FileName)
		key := storage.WhatsAppMediaObjectKey(conv.Uuid, id)
		if err := m.d.Storage.Upload(ctx, storage.File{
			Body: bytes.NewReader(in.Media.Data), Size: int64(len(in.Media.Data)), ContentType: mime, Filename: name,
		}, key); err != nil {
			return db.Message{}, fmt.Errorf("whatsapp: store media: %w", err)
		}
		params.Media, _ = json.Marshal(storedMedia{Type: kind, MimeType: mime, FileName: name, Caption: body})
		params.MediaStorageKey = text(key)
		params.MediaMime = text(mime)
		params.MediaSize = pgtype.Int8{Int64: int64(len(in.Media.Data)), Valid: true}
	}

	tx, err := m.d.Tx.Begin(ctx)
	if err != nil {
		return db.Message{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := m.d.Queries.WithTx(tx)
	msg, err := q.InsertQueuedMessage(ctx, params)
	if err != nil {
		return db.Message{}, err
	}
	conv, err = q.TouchConversationOutbound(ctx, db.TouchConversationOutboundParams{ID: conv.ID, At: ts(msg.CreatedAt.Time)})
	if err != nil {
		return db.Message{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Message{}, err
	}
	m.publish(ctx, EventMessageCreated, conv, msg)
	if m.d.Queue != nil {
		if err := m.d.Queue.EnqueueSend(ctx, msg.ID, msg.Uuid); err != nil {
			// The row is the source of truth; the stale queue sweep sends it.
			m.log.Warn("whatsapp_send_enqueue_failed", "message", msg.Uuid, "error", err)
		}
	}
	return msg, nil
}

// DocumentMessage queues a rendered PDF (service PDF, warranty
// certificate) for a conversation. Called by AI tools and staff replies
// after their own authorization of Ref.
type DocumentMessage struct {
	ConversationID int64
	SenderType     string
	SenderUserID   *int64
	AIRunID        *int64
	Kind           string
	Ref            DocumentRef
	Caption        string
}

// QueueDocument renders the document and queues it as a PDF message.
func (m *Messaging) QueueDocument(ctx context.Context, in DocumentMessage) (db.Message, error) {
	if in.Kind != DocumentServicePDF && in.Kind != DocumentWarrantyCertificate {
		return db.Message{}, fmt.Errorf("%w: document kind", ErrInvalidRequest)
	}
	if m.d.Documents == nil {
		return db.Message{}, ErrNotConfigured
	}
	doc, err := m.d.Documents.Render(ctx, in.Kind, in.Ref)
	if err != nil {
		return db.Message{}, err
	}
	name := doc.FileName
	if !strings.HasSuffix(strings.ToLower(name), ".pdf") {
		name += ".pdf"
	}
	return m.Queue(ctx, OutgoingMessage{
		ConversationID: in.ConversationID, SenderType: in.SenderType, SenderUserID: in.SenderUserID,
		AIRunID: in.AIRunID, Body: in.Caption, Media: &OutgoingMedia{Data: doc.Data, FileName: name},
	})
}

// ProcessSend runs the whatsapp:send task of one message. The queued row
// is locked while the provider is called, so a second run of the same task
// (or a concurrent one) sends nothing. final is true on the last retry: a
// failure then marks the message failed; earlier failures keep it queued
// with the reason and return the error so the task is retried.
func (m *Messaging) ProcessSend(ctx context.Context, messageID int64, final bool) error {
	tx, err := m.d.Tx.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := m.d.Queries.WithTx(tx)
	msg, err := q.LockQueuedMessage(ctx, messageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // already sent or failed, or another run holds it
	}
	if err != nil {
		return err
	}
	conv, err := q.GetConversationByID(ctx, msg.ConversationID)
	if err != nil {
		return err
	}
	if m.d.Limiter != nil {
		if ok, wait := m.d.Limiter.Allow(ctx, "whatsapp_send", conv.ContactE164, m.sendPerMinute(ctx), time.Minute); !ok {
			if !final {
				return fmt.Errorf("%w: retry in %s", ErrRateLimited, wait)
			}
			return m.sendFailed(ctx, tx, q, conv, msg, ErrRateLimited, true)
		}
	}
	ref, sendErr := m.send(ctx, conv.ContactE164, msg)
	if sendErr != nil {
		return m.sendFailed(ctx, tx, q, conv, msg, sendErr, final || permanentSendError(sendErr))
	}
	extID := ref.ID
	if extID == "" {
		extID = msg.ExternalID
	}
	at := ref.Timestamp
	if at.IsZero() {
		at = m.now()
	}
	sent, err := q.MarkMessageSent(ctx, db.MarkMessageSentParams{ID: msg.ID, ExternalID: extID, SentAt: ts(at)})
	if err != nil {
		// Rolled back: a retry sends again with the same client message id,
		// which the gateway/WhatsApp treats as the same message.
		m.log.Error("whatsapp_mark_sent_failed", "message", msg.Uuid, "error", err)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	m.publish(ctx, EventMessageUpdated, conv, sent)
	return nil
}

func (m *Messaging) sendFailed(ctx context.Context, tx pgx.Tx, q *db.Queries, conv db.Conversation, msg db.Message, sendErr error, failed bool) error {
	reason := sendErr.Error()
	if r := []rune(reason); len(r) > 1000 {
		reason = string(r[:1000])
	}
	upd, err := q.MarkMessageSendError(ctx, db.MarkMessageSendErrorParams{
		ID: msg.ID, Failed: failed, FailureReason: text(reason), At: ts(m.now()),
	})
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if failed {
		m.log.Warn("whatsapp_send_failed", "message", msg.Uuid, "attempts", upd.SendAttempts, "error", sendErr)
		m.publish(ctx, EventMessageUpdated, conv, upd)
		return nil // recorded as failed: nothing left to retry
	}
	return sendErr
}

// permanentSendError reports provider errors a retry cannot fix.
func permanentSendError(err error) bool {
	return errors.Is(err, whatsapp.ErrInvalidRecipient) || errors.Is(err, whatsapp.ErrMediaType) ||
		errors.Is(err, whatsapp.ErrMediaMismatch) || errors.Is(err, whatsapp.ErrMediaTooLarge)
}

func (m *Messaging) send(ctx context.Context, to string, msg db.Message) (whatsapp.MsgRef, error) {
	if m.d.Provider == nil {
		return whatsapp.MsgRef{}, ErrNotConfigured
	}
	opts := whatsapp.SendOptions{ID: msg.ExternalID}
	if !msg.MediaStorageKey.Valid {
		return m.d.Provider.SendText(ctx, to, msg.Body.String, opts)
	}
	if m.d.Storage == nil {
		return whatsapp.MsgRef{}, ErrNotConfigured
	}
	var meta storedMedia
	_ = json.Unmarshal(msg.Media, &meta)
	rc, _, err := m.d.Storage.Download(ctx, msg.MediaStorageKey.String)
	if err != nil {
		return whatsapp.MsgRef{}, fmt.Errorf("whatsapp: load media: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(rc, whatsapp.MaxMediaBytes+1))
	_ = rc.Close()
	if err != nil {
		return whatsapp.MsgRef{}, fmt.Errorf("whatsapp: load media: %w", err)
	}
	// Re-sniff what is actually sent (the object is the source).
	mime, err := whatsapp.ValidateMedia(data, meta.FileName)
	if err != nil {
		return whatsapp.MsgRef{}, err
	}
	media := whatsapp.Media{Data: data, MimeType: mime, FileName: meta.FileName, Caption: msg.Body.String}
	if whatsapp.IsImageMime(mime) {
		return m.d.Provider.SendImage(ctx, to, media, opts)
	}
	return m.d.Provider.SendDocument(ctx, to, media, opts)
}

func (m *Messaging) sendPerMinute(ctx context.Context) int {
	if m.d.SendPerMinute != nil {
		if n := m.d.SendPerMinute(ctx); n > 0 {
			return n
		}
	}
	return sysconfig.DefaultWhatsAppSendPerMinute
}

// RequeueStale enqueues the send task of messages queued longer than
// olderThan (a lost task after an enqueue error or a Redis flush). A
// message whose task is still pending is skipped by the task id dedup.
func (m *Messaging) RequeueStale(ctx context.Context, olderThan time.Duration) (int, error) {
	if m.d.Queue == nil {
		return 0, nil
	}
	rows, err := m.d.Queries.ListStaleQueuedMessages(ctx, db.ListStaleQueuedMessagesParams{
		Before: ts(m.now().Add(-olderThan)), LimitCount: 500,
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		if err := m.d.Queue.EnqueueSend(ctx, r.ID, r.Uuid); err != nil {
			m.log.Warn("whatsapp_send_requeue_failed", "message", r.Uuid, "error", err)
			continue
		}
		n++
	}
	return n, nil
}

// ApplyReceipt stores a delivery receipt (delivered | read) on the outgoing
// messages it names and publishes the changed rows.
func (m *Messaging) ApplyReceipt(ctx context.Context, ev whatsapp.InboundEvent) (int, error) {
	rows, err := applyReceipt(ctx, m.d.Queries, ev, m.now())
	if err != nil {
		return 0, err
	}
	for _, msg := range rows {
		if conv, err := m.d.Queries.GetConversationByID(ctx, msg.ConversationID); err == nil {
			m.publish(ctx, EventMessageUpdated, conv, msg)
		}
	}
	return len(rows), nil
}

type receiptStore interface {
	ApplyMessageReceipt(ctx context.Context, arg db.ApplyMessageReceiptParams) ([]db.Message, error)
}

// applyReceipt moves the named outgoing messages forward (sent → delivered
// → read) and stamps delivery_status_at with the receipt time.
func applyReceipt(ctx context.Context, q receiptStore, ev whatsapp.InboundEvent, now time.Time) ([]db.Message, error) {
	if ev.Status != "delivered" && ev.Status != "read" {
		return nil, nil
	}
	at := ev.Timestamp
	if at.IsZero() {
		at = now
	}
	return q.ApplyMessageReceipt(ctx, db.ApplyMessageReceiptParams{
		Status: ev.Status, At: ts(at), Channel: whatsapp.ChannelWhatsApp, ExternalIds: ev.MessageIDs,
	})
}

// AfterStored runs after a webhook stored a new message: it is published,
// and attached media is queued for storage.
func (m *Messaging) AfterStored(ctx context.Context, conv db.Conversation, msg db.Message) {
	m.publish(ctx, EventMessageCreated, conv, msg)
	if len(msg.Media) == 0 || msg.MediaStorageKey.Valid || m.d.Queue == nil {
		return
	}
	if err := m.d.Queue.EnqueueMediaStore(ctx, msg.ID, msg.Uuid); err != nil {
		m.log.Warn("whatsapp_media_enqueue_failed", "message", msg.Uuid, "error", err)
	}
}

// StoreInboundMedia copies the media of a stored message from the gateway
// to object storage (whatsapp/{conversation}/{message}). Media larger than
// 16 MB, of an unsupported type or whose content does not match its file
// type is not stored; media.storage_skipped records why. A download error
// is retried; on the final attempt it is recorded the same way.
func (m *Messaging) StoreInboundMedia(ctx context.Context, messageID int64, final bool) error {
	msg, err := m.d.Queries.GetMessageByID(ctx, messageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if msg.MediaStorageKey.Valid || len(msg.Media) == 0 {
		return nil
	}
	var meta whatsapp.InboundMedia
	if err := json.Unmarshal(msg.Media, &meta); err != nil {
		return m.skipMedia(ctx, msg, MediaSkipUnsupported)
	}
	if meta.Size > whatsapp.MaxMediaBytes {
		return m.skipMedia(ctx, msg, MediaSkipTooLarge)
	}
	if m.d.Downloader == nil || m.d.Storage == nil {
		return ErrNotConfigured
	}
	data, err := m.d.Downloader.DownloadMedia(ctx, meta, whatsapp.MaxMediaBytes)
	switch {
	case errors.Is(err, whatsapp.ErrMediaTooLarge):
		return m.skipMedia(ctx, msg, MediaSkipTooLarge)
	case errors.Is(err, whatsapp.ErrMediaType):
		return m.skipMedia(ctx, msg, MediaSkipUnsupported)
	case errors.Is(err, whatsapp.ErrMediaNoDownload):
		return m.skipMedia(ctx, msg, MediaSkipNoDownload)
	case err != nil:
		if final {
			return m.skipMedia(ctx, msg, MediaSkipDownloadError)
		}
		return err
	}
	mime, err := whatsapp.ValidateMedia(data, meta.FileName)
	switch {
	case errors.Is(err, whatsapp.ErrMediaTooLarge):
		return m.skipMedia(ctx, msg, MediaSkipTooLarge)
	case errors.Is(err, whatsapp.ErrMediaMismatch):
		return m.skipMedia(ctx, msg, MediaSkipMismatch)
	case err != nil:
		return m.skipMedia(ctx, msg, MediaSkipUnsupported)
	}
	conv, err := m.d.Queries.GetConversationByID(ctx, msg.ConversationID)
	if err != nil {
		return err
	}
	key := storage.WhatsAppMediaObjectKey(conv.Uuid, msg.Uuid)
	if err := m.d.Storage.Upload(ctx, storage.File{
		Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: mime, Filename: meta.FileName,
	}, key); err != nil {
		return fmt.Errorf("whatsapp: store media: %w", err)
	}
	upd, err := m.d.Queries.SetMessageMedia(ctx, db.SetMessageMediaParams{
		ID: msg.ID, MediaStorageKey: text(key), MediaMime: text(mime),
		MediaSize: pgtype.Int8{Int64: int64(len(data)), Valid: true},
	})
	if err != nil {
		return err
	}
	m.publish(ctx, EventMessageUpdated, conv, upd)
	return nil
}

func (m *Messaging) skipMedia(ctx context.Context, msg db.Message, reason string) error {
	upd, err := m.d.Queries.SetMessageMediaNote(ctx, db.SetMessageMediaNoteParams{ID: msg.ID, Reason: reason})
	if err != nil {
		return err
	}
	m.log.Info("whatsapp_media_not_stored", "message", msg.Uuid, "reason", reason)
	if conv, err := m.d.Queries.GetConversationByID(ctx, msg.ConversationID); err == nil {
		m.publish(ctx, EventMessageUpdated, conv, upd)
	}
	return nil
}

// MessageEvent is the realtime payload of one message.
type MessageEvent struct {
	UUID             uuid.UUID       `json:"uuid"`
	ConversationUUID uuid.UUID       `json:"conversation_uuid"`
	Direction        string          `json:"direction"`
	SenderType       string          `json:"sender_type"`
	Status           string          `json:"status"`
	Body             *string         `json:"body"`
	Media            json.RawMessage `json:"media,omitempty"`
	HasStoredMedia   bool            `json:"has_stored_media"`
	MediaMime        *string         `json:"media_mime,omitempty"`
	MediaSize        *int64          `json:"media_size,omitempty"`
	SendAttempts     int16           `json:"send_attempts"`
	FailureReason    *string         `json:"failure_reason,omitempty"`
	SentAt           *time.Time      `json:"sent_at,omitempty"`
	DeliveryStatusAt *time.Time      `json:"delivery_status_at,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
}

// NewMessageEvent maps a message row. The media download reference and the
// raw provider payload are left out.
func NewMessageEvent(conv db.Conversation, msg db.Message) MessageEvent {
	ev := MessageEvent{
		UUID: msg.Uuid, ConversationUUID: conv.Uuid, Direction: msg.Direction, SenderType: msg.SenderType,
		Status: msg.Status, Body: textPtr(msg.Body), HasStoredMedia: msg.MediaStorageKey.Valid,
		MediaMime: textPtr(msg.MediaMime), SendAttempts: msg.SendAttempts, FailureReason: textPtr(msg.FailureReason),
		SentAt: tsPtr(msg.SentAt), DeliveryStatusAt: tsPtr(msg.DeliveryStatusAt), CreatedAt: msg.CreatedAt.Time,
	}
	if msg.MediaSize.Valid {
		v := msg.MediaSize.Int64
		ev.MediaSize = &v
	}
	if len(msg.Media) > 0 {
		var media map[string]any
		if json.Unmarshal(msg.Media, &media) == nil {
			delete(media, "download")
			delete(media, "url")
			ev.Media, _ = json.Marshal(media)
		}
	}
	return ev
}

// publish sends the event to the platform inbox channel and the assigned
// user's channel. Failures are logged: the row is the source of truth and
// clients refetch.
func (m *Messaging) publish(ctx context.Context, typ string, conv db.Conversation, msg db.Message) {
	if m.d.Publisher == nil {
		return
	}
	payload := map[string]any{"type": typ, "conversation_uuid": conv.Uuid, "message": NewMessageEvent(conv, msg)}
	channels := []string{realtime.ChannelConversations}
	if conv.AssignedUserID.Valid {
		if u, err := m.d.Queries.GetUserByID(ctx, conv.AssignedUserID.Int64); err == nil {
			channels = append(channels, realtime.UserChannel(u.Uuid))
		}
	}
	for _, ch := range channels {
		if err := m.d.Publisher.Publish(ctx, ch, payload); err != nil {
			m.log.Warn("whatsapp_realtime_publish_failed", "channel", ch, "message", msg.Uuid, "error", err)
		}
	}
}

func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}
