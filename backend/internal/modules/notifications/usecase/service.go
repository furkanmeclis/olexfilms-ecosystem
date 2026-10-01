package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/actionlink"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/providers"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrForbidden      = errors.New("forbidden")
	ErrInvalidRequest = errors.New("invalid request")
)

// Querier is the persistence surface used by the notification service.
type Querier interface {
	CreateNotification(ctx context.Context, arg db.CreateNotificationParams) (db.Notification, error)
	GetNotificationByUUID(ctx context.Context, arg uuid.UUID) (db.Notification, error)
	GetNotificationByID(ctx context.Context, id int64) (db.Notification, error)
	ListNotificationsForUser(ctx context.Context, arg db.ListNotificationsForUserParams) ([]db.Notification, error)
	CountNotificationsForUser(ctx context.Context, arg db.CountNotificationsForUserParams) (int64, error)
	CountUnreadInappForUser(ctx context.Context, userID pgtype.Int8) (int64, error)
	ListPlatformNotifications(ctx context.Context, arg db.ListPlatformNotificationsParams) ([]db.Notification, error)
	CountPlatformNotifications(ctx context.Context, arg db.CountPlatformNotificationsParams) (int64, error)
	MarkNotificationProcessing(ctx context.Context, id int64) (db.Notification, error)
	ListStuckProcessingNotificationIDs(ctx context.Context, staleMinutes int32) ([]int64, error)
	MarkNotificationSent(ctx context.Context, arg db.MarkNotificationSentParams) (db.Notification, error)
	MarkNotificationFailed(ctx context.Context, arg db.MarkNotificationFailedParams) (db.Notification, error)
	MarkNotificationRead(ctx context.Context, arg db.MarkNotificationReadParams) (db.Notification, error)
	MarkAllNotificationsReadForUser(ctx context.Context, userID pgtype.Int8) (int64, error)
	InsertNotificationHistory(ctx context.Context, arg db.InsertNotificationHistoryParams) (db.NotificationHistory, error)
	GetUserByID(ctx context.Context, id int64) (db.User, error)
	GetUserByUUID(ctx context.Context, id uuid.UUID) (db.User, error)
	GetLocaleSources(ctx context.Context, arg db.GetLocaleSourcesParams) (db.GetLocaleSourcesRow, error)

	// Notification center (TEC-87).
	GetNotificationRecipient(ctx context.Context, arg db.GetNotificationRecipientParams) (db.GetNotificationRecipientRow, error)
	ListActiveTemplatesForEvent(ctx context.Context, arg db.ListActiveTemplatesForEventParams) ([]db.NotificationTemplate, error)
	ListNotificationPreferenceRows(ctx context.Context, userID int64) ([]db.NotificationPreference, error)
	UpsertNotificationPreferenceRow(ctx context.Context, arg db.UpsertNotificationPreferenceRowParams) (db.NotificationPreference, error)
	DeleteNotificationPreferenceRow(ctx context.Context, arg db.DeleteNotificationPreferenceRowParams) error
	ListNotificationChannelSettings(ctx context.Context) ([]db.NotificationChannelSetting, error)
	InsertNotificationDelivery(ctx context.Context, arg db.InsertNotificationDeliveryParams) (db.NotificationDelivery, error)
	AttachNotificationDelivery(ctx context.Context, arg db.AttachNotificationDeliveryParams) error
	MarkDeliveryProcessing(ctx context.Context, id int64) error
	MarkDeliveryResult(ctx context.Context, arg db.MarkDeliveryResultParams) error
}

// Enqueuer schedules background delivery tasks.
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Service coordinates enqueue + delivery.
type Service struct {
	q            Querier
	queue        Enqueuer
	providers    map[string]providers.Provider
	log          *slog.Logger
	syncMode     bool // when queue is nil, deliver inline
	vapid        *VAPIDConfig
	actionSecret []byte
	actionTTL    time.Duration
}

// New creates a notification service.
func New(q Querier, enq Enqueuer, provs []providers.Provider, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	m := make(map[string]providers.Provider, len(provs))
	for _, p := range provs {
		m[p.Channel()] = p
	}
	return &Service{
		q: q, queue: enq, providers: m, log: log, syncMode: enq == nil,
		actionTTL: actionlink.DefaultTTL,
	}
}

// WithActionSigner enables HMAC-signed action URLs (marks read on redeem).
func (s *Service) WithActionSigner(secret string, ttl time.Duration) *Service {
	if secret != "" {
		s.actionSecret = []byte(secret)
	}
	if ttl > 0 {
		s.actionTTL = ttl
	}
	return s
}

// Enqueue creates queued notification rows and schedules delivery.
// recipientLocale resolves a recipient's effective locale (user -> brand
// center -> tr) through i18n.Resolve. The requester's Accept-Language is not
// used: the recipient may be someone else. "" when the user is not found.
func (s *Service) recipientLocale(ctx context.Context, userID int64) string {
	params := db.GetLocaleSourcesParams{UserID: userID}
	if b, ok := brandctx.From(ctx); ok {
		params.BrandID = pgtype.Int8{Int64: b.ID, Valid: true}
	}
	row, err := s.q.GetLocaleSources(ctx, params)
	if err != nil {
		return ""
	}
	return string(i18n.Resolve(i18n.Sources{
		UserLocale: row.UserLocale, CenterLocale: row.CenterLocale,
	}).Locale)
}

func (s *Service) Enqueue(ctx context.Context, in model.EnqueueInput) ([]model.Notification, error) {
	if len(in.Channels) == 0 {
		return nil, fmt.Errorf("%w: channels required", ErrInvalidRequest)
	}
	lang := in.Language
	if l, ok := i18n.Parse(lang); ok {
		lang = string(l)
	}
	if lang == "" && in.UserID != nil {
		lang = s.recipientLocale(ctx, *in.UserID)
	}
	if lang == "" {
		lang = string(i18n.DefaultLocale)
	}
	priority := in.Priority
	if priority == "" {
		priority = model.PriorityNormal
	}

	var prefs preferenceSet
	if in.UserID != nil {
		prefs = s.loadPreferences(ctx, *in.UserID)
	}
	switches := s.channelSwitches(ctx)
	code := in.TemplateCode
	if code == "" {
		code = in.SourceEvent
	}

	var out []model.Notification
	seen := map[string]bool{}
	for _, raw := range in.Channels {
		ch, ok := normalizeChannel(raw)
		if !ok || seen[ch] {
			continue
		}
		seen[ch] = true
		if !switches[ch] {
			s.log.Debug("notification_channel_disabled", "channel", ch, "template", in.TemplateCode)
			continue
		}
		if (ch != model.ChannelEmail || !in.SecurityEmail) && !prefs.allowed(code, ch, defaultLegacyChannel(ch)) {
			s.log.Debug("notification_channel_skipped_by_preference", "channel", ch, "template", in.TemplateCode)
			continue
		}
		title, body := in.Title, in.Body
		rowLang := lang
		if in.TemplateCode != "" {
			tpl, ok, err := s.pickTemplate(ctx, in.TemplateCode, ch, catalog.RoleGeneric, nil,
				msgtemplate.LocaleChain(lang, "", ""))
			if err != nil {
				return nil, err
			}
			if ok {
				title = msgtemplate.Render(tpl.Subject, in.TemplateVars)
				body = msgtemplate.Render(tpl.Body, in.TemplateVars)
				rowLang = tpl.Language
			} else if title == "" {
				title = in.TemplateCode
			}
		}
		payload, err := json.Marshal(in.Payload)
		if err != nil || in.Payload == nil {
			payload = []byte("{}")
		}
		recipient := textPtr(in.Recipient)
		if !recipient.Valid && in.UserID != nil && needsAddress(ch) {
			if u, err := s.q.GetUserByID(ctx, *in.UserID); err == nil {
				switch {
				case ch == model.ChannelEmail && u.Email.Valid:
					recipient = pgtype.Text{String: u.Email.String, Valid: true}
				case ch != model.ChannelEmail && u.PhoneE164.Valid:
					recipient = u.PhoneE164
				}
			}
		}
		row, err := s.q.CreateNotification(ctx, db.CreateNotificationParams{
			UserID:         int8Ptr(in.UserID),
			Channel:        ch,
			Status:         model.StatusQueued,
			Priority:       priority,
			Title:          title,
			Body:           body,
			Payload:        payload,
			ActionUrl:      textPtr(in.ActionURL),
			Recipient:      recipient,
			TemplateCode:   pgtype.Text{String: in.TemplateCode, Valid: in.TemplateCode != ""},
			SourceEvent:    pgtype.Text{String: in.SourceEvent, Valid: in.SourceEvent != ""},
			MaxAttempts:    5,
			OrganizationID: int8Ptr(in.TenantID),
			Language:       pgtype.Text{String: rowLang, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("create notification: %w", err)
		}
		_, _ = s.q.InsertNotificationHistory(ctx, db.InsertNotificationHistoryParams{
			NotificationID: row.ID,
			Event:          "queued",
			Metadata:       []byte("{}"),
		})
		if err := s.scheduleDeliver(ctx, row.ID); err != nil {
			return nil, err
		}
		out = append(out, s.project(row))
	}
	return out, nil
}

// needsAddress reports channels that send to notifications.recipient.
func needsAddress(ch string) bool {
	return ch == model.ChannelEmail || ch == model.ChannelSMS || ch == model.ChannelWhatsApp
}

// normalizeChannel maps legacy channel names: push -> webpush; realtime is
// part of inapp now and is dropped (ok=false).
func normalizeChannel(raw string) (string, bool) {
	ch := strings.TrimSpace(raw)
	switch ch {
	case "push":
		return model.ChannelWebPush, true
	case "realtime", "":
		return "", false
	}
	return ch, catalog.IsChannel(ch)
}

// defaultLegacyChannel is the default of a channel without a preference row
// on the legacy Enqueue path: on for every channel the caller named, except
// webpush, which stays opt-in like the old push_enabled flag.
func defaultLegacyChannel(ch string) bool { return ch != model.ChannelWebPush }

func (s *Service) scheduleDeliver(ctx context.Context, id int64) error {
	if s.syncMode || s.queue == nil {
		return s.Deliver(ctx, id)
	}
	task, err := queue.NewNotificationDeliverTask(id)
	if err != nil {
		return err
	}
	// TaskID = notification id: a retried dispatch never enqueues twice.
	_, err = s.queue.Enqueue(task, asynq.Queue(queue.QueueNotifications),
		asynq.TaskID(fmt.Sprintf("notification:%d", id)))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

const DefaultStuckProcessingMinutes int32 = 2

// Deliver processes one queued notification.
func (s *Service) Deliver(ctx context.Context, id int64) error {
	row, err := s.q.MarkNotificationProcessing(ctx, id)
	resumed := false
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		existing, getErr := s.q.GetNotificationByID(ctx, id)
		if getErr != nil {
			if errors.Is(getErr, pgx.ErrNoRows) {
				return nil
			}
			return getErr
		}
		switch existing.Status {
		case model.StatusSent, model.StatusDelivered, model.StatusRead, model.StatusCancelled:
			return nil
		case model.StatusProcessing:
			row = existing
			resumed = true
		default:
			return nil
		}
	}
	if !resumed {
		_, _ = s.q.InsertNotificationHistory(ctx, db.InsertNotificationHistoryParams{
			NotificationID: row.ID, Event: "processing", Metadata: []byte("{}"),
		})
	}
	if row.DeliveryID.Valid {
		_ = s.q.MarkDeliveryProcessing(ctx, row.DeliveryID.Int64)
	}
	p, ok := s.providers[row.Channel]
	if !ok {
		p = providers.NoopProvider{Name: row.Channel, Log: s.log}
	}
	var userUUID *uuid.UUID
	if row.UserID.Valid {
		if u, err := s.q.GetUserByID(ctx, row.UserID.Int64); err == nil {
			uid := u.Uuid
			userUUID = &uid
		}
	}
	res, err := p.Deliver(ctx, row, userUUID)
	if errors.Is(err, providers.ErrNoRecipient) {
		// Nothing to send to (no address, subscription or token): no retry.
		_, _ = s.q.MarkNotificationFailed(ctx, db.MarkNotificationFailedParams{
			ID: id, LastError: pgtype.Text{String: err.Error(), Valid: true},
		})
		s.markDelivery(ctx, row, model.DeliverySkippedNoRecipient, providers.DeliveryResult{}, err)
		return nil
	}
	if err != nil {
		_, _ = s.q.MarkNotificationFailed(ctx, db.MarkNotificationFailedParams{
			ID: id, LastError: pgtype.Text{String: err.Error(), Valid: true},
		})
		_, _ = s.q.InsertNotificationHistory(ctx, db.InsertNotificationHistoryParams{
			NotificationID: id, Event: "failed",
			Metadata: mustJSON(map[string]string{"error": err.Error()}),
		})
		s.markDelivery(ctx, row, model.StatusFailed, providers.DeliveryResult{}, err)
		return err
	}
	status := res.Status
	if status == "" {
		status = model.StatusSent
	}
	_, err = s.q.MarkNotificationSent(ctx, db.MarkNotificationSentParams{
		ID: id, Status: status,
		Provider:          pgtype.Text{String: res.Provider, Valid: res.Provider != ""},
		ProviderReference: pgtype.Text{String: truncate(res.ProviderReference, 255), Valid: res.ProviderReference != ""},
	})
	if err != nil {
		return err
	}
	_, _ = s.q.InsertNotificationHistory(ctx, db.InsertNotificationHistoryParams{
		NotificationID: id, Event: status, Metadata: []byte("{}"),
	})
	s.markDelivery(ctx, row, status, res, nil)
	return nil
}

func (s *Service) markDelivery(ctx context.Context, row db.Notification, status string, res providers.DeliveryResult, cause error) {
	if !row.DeliveryID.Valid {
		return
	}
	params := db.MarkDeliveryResultParams{
		ID: row.DeliveryID.Int64, Status: status,
		Provider:    pgtype.Text{String: res.Provider, Valid: res.Provider != ""},
		ProviderRef: pgtype.Text{String: truncate(res.ProviderReference, 255), Valid: res.ProviderReference != ""},
	}
	if cause != nil {
		params.Error = pgtype.Text{String: cause.Error(), Valid: true}
	}
	if err := s.q.MarkDeliveryResult(ctx, params); err != nil {
		s.log.Warn("notification_delivery_update_failed", "delivery_id", row.DeliveryID.Int64, "error", err)
	}
}

func truncate(v string, n int) string {
	if len(v) <= n {
		return v
	}
	return v[:n]
}

// ReclaimStuck re-runs delivery for rows left in processing after a worker
// crash or a failed status write. Asynq may have already dropped the task.
func (s *Service) ReclaimStuck(ctx context.Context, staleMinutes int32) (int, error) {
	if staleMinutes < 0 {
		staleMinutes = 0
	}
	ids, err := s.q.ListStuckProcessingNotificationIDs(ctx, staleMinutes)
	if err != nil {
		return 0, fmt.Errorf("list stuck notifications: %w", err)
	}
	var n int
	for _, id := range ids {
		if err := s.Deliver(ctx, id); err != nil {
			s.log.Error("notification_reclaim_failed", "id", id, "error", err)
			continue
		}
		n++
	}
	return n, nil
}

// ListInbox returns the caller's notifications page.
func (s *Service) ListInbox(ctx context.Context, userID int64, q apiquery.Query, status, channel string, unread *bool) (apiquery.Page[model.Notification], error) {
	if err := apiquery.ValidateSort(q.Sort, apiquery.NotificationsSort); err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	params := db.ListNotificationsForUserParams{
		UserID:      pgtype.Int8{Int64: userID, Valid: true},
		Status:      optionalText(status),
		Channel:     optionalText(channel),
		Unread:      optionalBool(unread),
		Q:           optionalText(q.Q),
		LimitCount:  q.Limit,
		OffsetCount: q.Offset,
	}
	rows, err := s.q.ListNotificationsForUser(ctx, params)
	if err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	total, err := s.q.CountNotificationsForUser(ctx, db.CountNotificationsForUserParams{
		UserID: params.UserID, Status: params.Status, Channel: params.Channel,
		Unread: params.Unread, Q: params.Q,
	})
	if err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	items := make([]model.Notification, 0, len(rows))
	for _, r := range rows {
		items = append(items, s.project(r))
	}
	return apiquery.NewPage(items, total, q.Limit, q.Offset), nil
}

// UnreadCount returns unread in-app count.
func (s *Service) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	return s.q.CountUnreadInappForUser(ctx, pgtype.Int8{Int64: userID, Valid: true})
}

// GetOwned returns a notification owned by userID.
func (s *Service) GetOwned(ctx context.Context, userID int64, id uuid.UUID) (model.Notification, error) {
	row, err := s.q.GetNotificationByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Notification{}, ErrNotFound
		}
		return model.Notification{}, err
	}
	if !row.UserID.Valid || row.UserID.Int64 != userID {
		return model.Notification{}, ErrForbidden
	}
	return s.project(row), nil
}

// MarkRead marks one notification read.
func (s *Service) MarkRead(ctx context.Context, userID int64, id uuid.UUID) (model.Notification, error) {
	row, err := s.q.MarkNotificationRead(ctx, db.MarkNotificationReadParams{
		Uuid: id, UserID: pgtype.Int8{Int64: userID, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Notification{}, ErrNotFound
		}
		return model.Notification{}, err
	}
	return s.project(row), nil
}

// RedeemAction verifies a signed action link, marks the in-app row read, and
// returns the stored destination.
func (s *Service) RedeemAction(ctx context.Context, userID int64, id uuid.UUID, exp int64, sig string) (model.Notification, string, error) {
	row, err := s.q.GetNotificationByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Notification{}, "", ErrNotFound
		}
		return model.Notification{}, "", err
	}
	if !row.UserID.Valid || row.UserID.Int64 != userID {
		return model.Notification{}, "", ErrForbidden
	}
	if !row.ActionUrl.Valid || strings.TrimSpace(row.ActionUrl.String) == "" {
		return model.Notification{}, "", fmt.Errorf("%w: action url missing", ErrInvalidRequest)
	}
	dest := row.ActionUrl.String
	if !actionlink.Verify(s.actionSecret, id, dest, sig, exp) {
		return model.Notification{}, "", fmt.Errorf("%w: invalid or expired action link", ErrInvalidRequest)
	}
	if row.Channel == model.ChannelInapp && !row.ReadAt.Valid {
		marked, markErr := s.q.MarkNotificationRead(ctx, db.MarkNotificationReadParams{
			Uuid: id, UserID: pgtype.Int8{Int64: userID, Valid: true},
		})
		if markErr == nil {
			row = marked
		}
	}
	return s.project(row), dest, nil
}

// MarkAllRead marks all unread in-app as read.
func (s *Service) MarkAllRead(ctx context.Context, userID int64) error {
	_, err := s.q.MarkAllNotificationsReadForUser(ctx, pgtype.Int8{Int64: userID, Valid: true})
	return err
}

// ListPlatform returns notifications for platform operators.
// Without platform.notifications.read_all the page is always the caller's own rows.
func (s *Service) ListPlatform(
	ctx context.Context,
	actor authctx.Principal,
	q apiquery.Query,
	status, channel, scope, userUUID string,
) (apiquery.Page[model.Notification], error) {
	if err := apiquery.ValidateSort(q.Sort, apiquery.NotificationsSort); err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	audience, err := resolvePlatformAudience(actor, scope, userUUID, func(id uuid.UUID) (int64, error) {
		return s.ResolveUserID(ctx, id)
	})
	if err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	params := db.ListPlatformNotificationsParams{
		Status: optionalText(status), Channel: optionalText(channel), Q: optionalText(q.Q),
		UserID: audience.UserID, LimitCount: q.Limit, OffsetCount: q.Offset,
	}
	rows, err := s.q.ListPlatformNotifications(ctx, params)
	if err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	total, err := s.q.CountPlatformNotifications(ctx, db.CountPlatformNotificationsParams{
		Status: params.Status, Channel: params.Channel, Q: params.Q, UserID: params.UserID,
	})
	if err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	items := make([]model.Notification, 0, len(rows))
	users := map[int64]model.NotificationUser{}
	for _, r := range rows {
		n := s.project(r)
		if audience.IncludeUser && r.UserID.Valid {
			if u, ok := users[r.UserID.Int64]; ok {
				copied := u
				n.User = &copied
			} else if row, err := s.q.GetUserByID(ctx, r.UserID.Int64); err == nil {
				u := model.NotificationUser{
					UUID: row.Uuid, Name: row.Name, Surname: row.Surname, Email: row.Email.String,
				}
				users[r.UserID.Int64] = u
				n.User = &u
			}
		}
		items = append(items, n)
	}
	return apiquery.NewPage(items, total, q.Limit, q.Offset), nil
}

// ResolveUserID resolves public uuid to internal id.
func (s *Service) ResolveUserID(ctx context.Context, id uuid.UUID) (int64, error) {
	u, err := s.q.GetUserByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	return u.ID, nil
}

func (s *Service) project(row db.Notification) model.Notification {
	n := model.Notification{
		UUID: row.Uuid, Channel: row.Channel, Status: row.Status, Priority: row.Priority,
		Title: row.Title, Body: row.Body, Payload: map[string]any{},
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	_ = json.Unmarshal(row.Payload, &n.Payload)
	if row.ActionUrl.Valid {
		dest := row.ActionUrl.String
		n.ActionURL = &dest
		if signed := actionlink.URL(s.actionSecret, row.Uuid, dest, s.actionTTL); signed != "" {
			n.SignedActionURL = &signed
		}
	}
	if row.Recipient.Valid {
		s := row.Recipient.String
		n.Recipient = &s
	}
	if row.TemplateCode.Valid {
		s := row.TemplateCode.String
		n.TemplateCode = &s
	}
	if row.SourceEvent.Valid {
		s := row.SourceEvent.String
		n.SourceEvent = &s
	}
	if row.SentAt.Valid {
		t := row.SentAt.Time
		n.SentAt = &t
	}
	if row.ReadAt.Valid {
		t := row.ReadAt.Time
		n.ReadAt = &t
	}
	return n
}

func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func textPtr(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

func optionalText(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func optionalBool(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}
