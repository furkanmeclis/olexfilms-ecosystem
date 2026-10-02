package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrUnknownEvent is returned for an event code that is not in the catalog.
var ErrUnknownEvent = errors.New("unknown notification event")

// RetentionDays is how long notifications and deliveries are kept (design
// section 6: notification/activity logs are swept after 90 days).
const RetentionDays = 90

// --- preferences -------------------------------------------------------------

// preferenceSet holds a user's preference rows: event specific rows win over
// the global (event_code NULL) rows, which win over the default.
type preferenceSet struct {
	global map[string]bool
	event  map[string]map[string]bool
}

func (p preferenceSet) allowed(code, channel string, def bool) bool {
	if byCh, ok := p.event[code]; ok {
		if v, ok := byCh[channel]; ok {
			return v
		}
	}
	if v, ok := p.global[channel]; ok {
		return v
	}
	return def
}

func newPreferenceSet(rows []db.NotificationPreference) preferenceSet {
	p := preferenceSet{global: map[string]bool{}, event: map[string]map[string]bool{}}
	for _, r := range rows {
		if !r.EventCode.Valid {
			p.global[r.Channel] = r.Enabled
			continue
		}
		if p.event[r.EventCode.String] == nil {
			p.event[r.EventCode.String] = map[string]bool{}
		}
		p.event[r.EventCode.String][r.Channel] = r.Enabled
	}
	return p
}

func (s *Service) loadPreferences(ctx context.Context, userID int64) preferenceSet {
	rows, err := s.q.ListNotificationPreferenceRows(ctx, userID)
	if err != nil {
		s.log.Warn("notification_preferences_load_failed", "user_id", userID, "error", err)
	}
	return newPreferenceSet(rows)
}

// channelSwitches returns the admin channel switches. A channel without a
// row is on, except sms which needs an explicit admin opt-in (K21).
func (s *Service) channelSwitches(ctx context.Context) map[string]bool {
	out := map[string]bool{}
	for _, ch := range catalog.Channels {
		out[ch] = ch != model.ChannelSMS
	}
	rows, err := s.q.ListNotificationChannelSettings(ctx)
	if err != nil {
		s.log.Warn("notification_channel_settings_load_failed", "error", err)
		return out
	}
	for _, r := range rows {
		out[r.Channel] = r.Enabled
	}
	return out
}

// GetPreferences returns the caller's preference rows and the legacy bools.
func (s *Service) GetPreferences(ctx context.Context, userID int64) (model.Preferences, error) {
	rows, err := s.q.ListNotificationPreferenceRows(ctx, userID)
	if err != nil {
		return model.Preferences{}, err
	}
	return projectPreferences(rows), nil
}

func projectPreferences(rows []db.NotificationPreference) model.Preferences {
	set := newPreferenceSet(rows)
	out := model.Preferences{
		EmailEnabled: set.allowed("", model.ChannelEmail, true),
		InappEnabled: set.allowed("", model.ChannelInapp, true),
		PushEnabled:  set.allowed("", model.ChannelWebPush, false),
		Rules:        make([]model.PreferenceRule, 0, len(rows)),
	}
	out.RealtimeEnabled = out.InappEnabled
	for _, r := range rows {
		rule := model.PreferenceRule{Channel: r.Channel, Enabled: r.Enabled}
		if r.EventCode.Valid {
			code := r.EventCode.String
			rule.EventCode = &code
		}
		out.Rules = append(out.Rules, rule)
	}
	return out
}

// UpsertPreferences stores the legacy bools as global rows, then every rule
// (a rule overrides the bool of the same channel).
func (s *Service) UpsertPreferences(ctx context.Context, userID int64, p model.Preferences) (model.Preferences, error) {
	for ch, enabled := range map[string]bool{
		model.ChannelEmail: p.EmailEnabled, model.ChannelInapp: p.InappEnabled, model.ChannelWebPush: p.PushEnabled,
	} {
		if _, err := s.q.UpsertNotificationPreferenceRow(ctx, db.UpsertNotificationPreferenceRowParams{
			UserID: userID, Channel: ch, Enabled: enabled,
		}); err != nil {
			return model.Preferences{}, err
		}
	}
	for _, r := range p.Rules {
		if !catalog.IsChannel(r.Channel) {
			return model.Preferences{}, fmt.Errorf("%w: unknown channel %q", ErrInvalidRequest, r.Channel)
		}
		code := pgtype.Text{}
		if r.EventCode != nil && *r.EventCode != "" {
			if _, ok := catalog.Lookup(*r.EventCode); !ok {
				return model.Preferences{}, fmt.Errorf("%w: unknown event %q", ErrInvalidRequest, *r.EventCode)
			}
			code = pgtype.Text{String: *r.EventCode, Valid: true}
		}
		if _, err := s.q.UpsertNotificationPreferenceRow(ctx, db.UpsertNotificationPreferenceRowParams{
			UserID: userID, EventCode: code, Channel: r.Channel, Enabled: r.Enabled,
		}); err != nil {
			return model.Preferences{}, err
		}
	}
	return s.GetPreferences(ctx, userID)
}

// --- templates -----------------------------------------------------------------

// pickTemplate loads the active variants of code x channel and picks one
// with msgtemplate.Pick along chain (role -> generic, brand -> global).
func (s *Service) pickTemplate(ctx context.Context, code, channel, role string, brandID *int64, chain []i18n.Locale) (db.NotificationTemplate, bool, error) {
	rows, err := s.q.ListActiveTemplatesForEvent(ctx, db.ListActiveTemplatesForEventParams{
		Code: code, Channel: channel, BrandID: int8Ptr(brandID),
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return db.NotificationTemplate{}, false, err
	}
	variants := make([]msgtemplate.Variant, len(rows))
	for i, r := range rows {
		variants[i] = msgtemplate.Variant{Role: r.Role, Language: r.Language, Branded: r.BrandID.Valid}
	}
	i, ok := msgtemplate.Pick(variants, role, chain)
	if !ok {
		return db.NotificationTemplate{}, false, nil
	}
	return rows[i], true, nil
}

// recipientRole maps a recipient to a template role: platform staff and
// center members -> center, distributor/dealer members -> their type, users
// without a membership -> customer.
func recipientRole(r db.GetNotificationRecipientRow) string {
	if r.PlatformStaff {
		return catalog.RoleCenter
	}
	switch r.MemberOrgType {
	case "center":
		return catalog.RoleCenter
	case "distributor":
		return catalog.RoleDistributor
	case "dealer":
		return catalog.RoleDealer
	}
	return catalog.RoleCustomer
}

// --- dispatch --------------------------------------------------------------------

// Dispatch sends a catalog event to its recipients over every channel the
// catalog (or the caller) names. Per recipient x channel it writes one
// delivery row, keyed by (event_id, user, channel): a replayed event adds
// nothing. A channel the admin switched off, the user muted, without a
// template or without an address is recorded as skipped_*.
func (s *Service) Dispatch(ctx context.Context, in model.DispatchInput) (model.DispatchResult, error) {
	var res model.DispatchResult
	ev, ok := catalog.Lookup(in.EventCode)
	if !ok {
		return res, fmt.Errorf("%w: %s", ErrUnknownEvent, in.EventCode)
	}
	if in.EventID == uuid.Nil {
		return res, fmt.Errorf("%w: event_id required", ErrInvalidRequest)
	}
	channels := in.Channels
	if len(channels) == 0 {
		channels = ev.DefaultChannels
	}
	priority := in.Priority
	if priority == "" {
		priority = model.PriorityNormal
		if ev.Critical {
			priority = model.PriorityHigh
		}
	}
	payload, err := json.Marshal(in.Payload)
	if err != nil || in.Payload == nil {
		payload = []byte("{}")
	}
	switches := s.channelSwitches(ctx)

	seenUser := map[int64]bool{}
	var errs []error
	for _, userID := range in.UserIDs {
		if userID <= 0 || seenUser[userID] {
			continue
		}
		seenUser[userID] = true
		rcp, err := s.q.GetNotificationRecipient(ctx, db.GetNotificationRecipientParams{
			UserID: userID, OrganizationID: int8Ptr(in.OrganizationID), BrandID: int8Ptr(in.BrandID),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		role := recipientRole(rcp)
		chain := msgtemplate.LocaleChain(rcp.UserLocale, rcp.OrgLocale, rcp.CenterLocale)
		var brandID *int64
		if in.BrandID != nil {
			brandID = in.BrandID
		} else if rcp.BrandID > 0 {
			b := rcp.BrandID
			brandID = &b
		}
		prefs := s.loadPreferences(ctx, userID)

		seenCh := map[string]bool{}
		for _, raw := range channels {
			ch, ok := normalizeChannel(raw)
			if !ok || seenCh[ch] {
				continue
			}
			seenCh[ch] = true
			d := deliveryDraft{in: in, userID: userID, brandID: brandID, channel: ch, role: role}
			switch {
			case !switches[ch]:
				d.status = model.DeliverySkippedDisabled
			case !ev.Critical && !prefs.allowed(ev.Code, ch, true):
				d.status = model.DeliverySkippedPreference
			}
			if d.status != "" {
				s.recordSkip(ctx, d, &res, &errs)
				continue
			}
			tpl, found, err := s.pickTemplate(ctx, ev.Code, ch, role, brandID, chain)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if !found {
				d.status = model.DeliverySkippedNoTemplate
				d.language = string(chain[0])
				s.recordSkip(ctx, d, &res, &errs)
				continue
			}
			d.language, d.templateID = tpl.Language, &tpl.ID
			recipient := pgtype.Text{}
			switch ch {
			case model.ChannelEmail:
				recipient = pgtype.Text{String: rcp.Email, Valid: rcp.Email != ""}
			case model.ChannelSMS, model.ChannelWhatsApp:
				recipient = pgtype.Text{String: rcp.PhoneE164, Valid: rcp.PhoneE164 != ""}
			}
			if needsAddress(ch) && !recipient.Valid {
				d.status = model.DeliverySkippedNoRecipient
				s.recordSkip(ctx, d, &res, &errs)
				continue
			}
			delivery, err := s.q.InsertNotificationDelivery(ctx, d.params(model.StatusQueued))
			if errors.Is(err, pgx.ErrNoRows) {
				res.Duplicates++
				continue
			}
			if err != nil {
				errs = append(errs, err)
				continue
			}
			row, err := s.q.CreateNotification(ctx, db.CreateNotificationParams{
				UserID:         pgtype.Int8{Int64: userID, Valid: true},
				Channel:        ch,
				Status:         model.StatusQueued,
				Priority:       priority,
				Title:          msgtemplate.Render(tpl.Subject, in.Vars),
				Body:           msgtemplate.Render(tpl.Body, in.Vars),
				Payload:        payload,
				ActionUrl:      textPtr(in.ActionURL),
				Recipient:      recipient,
				TemplateCode:   pgtype.Text{String: ev.Code, Valid: true},
				SourceEvent:    pgtype.Text{String: ev.Code, Valid: true},
				MaxAttempts:    5,
				OrganizationID: int8Ptr(in.OrganizationID),
				BrandID:        int8Ptr(brandID),
				EventID:        pgtype.UUID{Bytes: in.EventID, Valid: true},
				Language:       pgtype.Text{String: tpl.Language, Valid: true},
			})
			if err != nil {
				_ = s.q.MarkDeliveryResult(ctx, db.MarkDeliveryResultParams{
					ID: delivery.ID, Status: model.StatusFailed, Error: pgtype.Text{String: err.Error(), Valid: true},
				})
				errs = append(errs, err)
				continue
			}
			if err := s.q.AttachNotificationDelivery(ctx, db.AttachNotificationDeliveryParams{ID: row.ID, DeliveryID: pgtype.Int8{Int64: delivery.ID, Valid: true}}); err != nil {
				errs = append(errs, err)
				continue
			}
			row.DeliveryID = pgtype.Int8{Int64: delivery.ID, Valid: true}
			_, _ = s.q.InsertNotificationHistory(ctx, db.InsertNotificationHistoryParams{
				NotificationID: row.ID, Event: "queued", Metadata: []byte("{}"),
			})
			res.Queued++
			if err := s.scheduleDeliver(ctx, row.ID); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return res, errors.Join(errs...)
}

type deliveryDraft struct {
	in         model.DispatchInput
	userID     int64
	brandID    *int64
	channel    string
	role       string
	language   string
	templateID *int64
	status     string
}

func (d deliveryDraft) params(status string) db.InsertNotificationDeliveryParams {
	return db.InsertNotificationDeliveryParams{
		EventID: d.in.EventID, EventCode: d.in.EventCode, UserID: d.userID,
		OrganizationID: int8Ptr(d.in.OrganizationID), BrandID: int8Ptr(d.brandID),
		Channel: d.channel, Role: optionalText(d.role), Language: optionalText(d.language),
		TemplateID: int8Ptr(d.templateID), Status: status,
	}
}

func (s *Service) recordSkip(ctx context.Context, d deliveryDraft, res *model.DispatchResult, errs *[]error) {
	_, err := s.q.InsertNotificationDelivery(ctx, d.params(d.status))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		res.Duplicates++
	case err != nil:
		*errs = append(*errs, err)
	default:
		res.Skipped++
	}
}

// --- maintenance ------------------------------------------------------------------

// PurgeStore is the persistence used by the retention sweep.
type PurgeStore interface {
	PurgeNotificationDeliveriesBefore(ctx context.Context, arg db.PurgeNotificationDeliveriesBeforeParams) (int64, error)
	PurgeNotificationsBefore(ctx context.Context, arg db.PurgeNotificationsBeforeParams) (int64, error)
}

// PurgeExpired deletes deliveries and notifications older than
// RetentionDays, in batches. It returns the number of rows removed.
func (s *Service) PurgeExpired(ctx context.Context) (int64, error) {
	return s.PurgeBefore(ctx, time.Now().Add(-RetentionDays*24*time.Hour))
}

// PurgeBefore deletes deliveries and notifications created before cutoff.
func (s *Service) PurgeBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	q, ok := s.q.(PurgeStore)
	if !ok {
		return 0, nil
	}
	const batch = 1000
	ts := pgtype.Timestamptz{Time: cutoff, Valid: true}
	var total int64
	for _, purge := range []func() (int64, error){
		func() (int64, error) {
			return q.PurgeNotificationDeliveriesBefore(ctx, db.PurgeNotificationDeliveriesBeforeParams{Cutoff: ts, BatchSize: batch})
		},
		func() (int64, error) {
			return q.PurgeNotificationsBefore(ctx, db.PurgeNotificationsBeforeParams{Cutoff: ts, BatchSize: batch})
		},
	} {
		for {
			n, err := purge()
			if err != nil {
				return total, err
			}
			total += n
			if n < batch || ctx.Err() != nil {
				break
			}
		}
	}
	if total > 0 {
		s.log.Info("notifications_purged", "rows", total, "cutoff", cutoff)
	}
	return total, nil
}

// CatalogStore mirrors the event catalog into notification_events and
// inserts the catalog's default templates.
type CatalogStore interface {
	UpsertNotificationEvent(ctx context.Context, arg db.UpsertNotificationEventParams) error
	InsertNotificationTemplateIfMissing(ctx context.Context, arg db.InsertNotificationTemplateIfMissingParams) (int64, error)
}

// SyncCatalog upserts every catalog event into notification_events and
// inserts its default templates (TEC-187) where no row exists yet, so an
// admin-edited template is never overwritten.
func SyncCatalog(ctx context.Context, q CatalogStore) error {
	for _, e := range catalog.All() {
		placeholders, _ := json.Marshal(e.Placeholders)
		if err := q.UpsertNotificationEvent(ctx, db.UpsertNotificationEventParams{
			Code: e.Code, Module: e.Module, DefaultChannels: nonNil(e.DefaultChannels),
			Critical: e.Critical, AudienceRoles: nonNil(e.AudienceRoles), Placeholders: placeholders,
			UserConfigurable: e.UserConfigurable,
		}); err != nil {
			return fmt.Errorf("sync notification event %s: %w", e.Code, err)
		}
		for _, t := range e.Templates {
			if _, err := q.InsertNotificationTemplateIfMissing(ctx, db.InsertNotificationTemplateIfMissingParams{
				Code: e.Code, Role: t.Role, Channel: t.Channel, Language: t.Language,
				Subject: t.Subject, Body: t.Body, Format: t.Format,
			}); err != nil {
				return fmt.Errorf("sync notification template %s/%s/%s: %w", e.Code, t.Channel, t.Language, err)
			}
		}
	}
	return nil
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// EventIDFor derives a stable event id for callers without an outbox event
// (e.g. an HTTP request): the same parts always give the same id, so a
// retried request is idempotent.
func EventIDFor(parts ...string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("notification:"+strings.Join(parts, "|")))
}
