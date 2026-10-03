package migrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/normalize"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
	shorturls "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/shorturls/usecase"
)

// ---------------------------------------------------------------- step 10

// ShortURLsStep imports the hub short_urls into short_urls (TEC-263, step
// 10). The legacy token is kept unchanged so old links keep resolving
// through /s/{token}; the hub's absolute target URL is mapped to an internal
// path of the new frontend (see MapLegacyShortTarget) and kept in
// legacy_target_url for audit. A target that cannot be mapped (an external
// site such as a Google Business review page, or an unknown hub route) is
// not imported and is reported as "unmapped:<id>".
type ShortURLsStep struct {
	// System is the migration_map source system; empty means SourceHub.
	System string
}

// Name implements Step.
func (ShortURLsStep) Name() string { return "short_urls" }

func (s ShortURLsStep) system() string {
	if s.System == "" {
		return SourceHub
	}
	return s.System
}

const legacyShortURLsQuery = `SELECT id, token, target_url, created_at, updated_at FROM short_urls`

type legacyShortURL struct {
	ID                   int64
	Token, TargetURL     string
	CreatedAt, UpdatedAt sql.NullTime
}

// Unmapped reasons of MapLegacyShortTarget.
const (
	ShortTargetInvalid  = "invalid_url"
	ShortTargetUnmapped = "unmapped"
)

// MapLegacyShortTarget maps a hub short URL target (an absolute URL) to an
// internal path of the new frontend, or returns the reason it cannot:
//
//   - /customer/{hash} (route customer.notify: the customer's panel behind a
//     non-expiring encrypted id) -> /portal, where the customer signs in
//     with an OTP; the hash itself is dropped.
//   - a path already under the new public areas (/portal, /garanti, /bayi)
//     is kept, query included.
//
// Anything else (external sites such as Google Business review links, the
// hub's /warranty/{serviceNo} whose service number is not a warranty
// public_code) is unmapped. The result always passes
// shorturls.NormalizeTarget, so the resolver can never become an open
// redirect.
func MapLegacyShortTarget(raw string) (string, string) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", ShortTargetInvalid
	}
	p := strings.TrimRight(u.Path, "/")
	segs := strings.Split(strings.TrimPrefix(p, "/"), "/")
	var target string
	switch {
	case len(segs) == 2 && segs[0] == "customer" && segs[1] != "":
		target = "/portal"
	case len(segs) >= 1 && (segs[0] == "portal" || segs[0] == "garanti" || segs[0] == "bayi"):
		target = p
		if u.RawQuery != "" {
			target += "?" + u.RawQuery
		}
	default:
		return "", ShortTargetUnmapped
	}
	norm, err := shorturls.NormalizeTarget(target)
	if err != nil {
		return "", ShortTargetUnmapped
	}
	return norm, ""
}

// Run implements Step.
func (s ShortURLsStep) Run(ctx context.Context, src Sources, dst *Target, m *Mapper) (StepResult, error) {
	c := counts{}
	hub, err := src.Get(SourceHub)
	if err != nil {
		return StepResult{}, err
	}
	brand, err := dst.Q.GetBrandBySlug(ctx, OlexBrandSlug)
	if err != nil {
		return StepResult{}, fmt.Errorf("brand %q: %w", OlexBrandSlug, err)
	}
	where, args := deltaFilter(dst)
	rows, err := hub.Query(ctx, legacyShortURLsQuery+where+" ORDER BY id", args...)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	var list []legacyShortURL
	for rows.Next() {
		var r legacyShortURL
		if err := rows.Scan(&r.ID, &r.Token, &r.TargetURL, &r.CreatedAt, &r.UpdatedAt); err != nil {
			_ = rows.Close()
			return StepResult{Counts: c}, fmt.Errorf("scan short url: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Close(); err != nil {
		return StepResult{Counts: c}, fmt.Errorf("read short urls: %w", err)
	}

	var watermark time.Time
	for _, r := range list {
		c.inc(cntRead)
		if ts := latest(r.CreatedAt, r.UpdatedAt); ts.After(watermark) {
			watermark = ts
		}
		if err := s.importShortURL(ctx, dst.Q, m, brand.ID, r, c); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("short url %d: %w", r.ID, err)
		}
	}
	return StepResult{Counts: c, Watermark: watermark}, nil
}

func (s ShortURLsStep) importShortURL(ctx context.Context, q *db.Queries, m *Mapper, brandID int64, r legacyShortURL, c counts) error {
	id := strconv.FormatInt(r.ID, 10)
	token := strings.TrimSpace(r.Token)
	if !shorturls.ValidToken(token) {
		c.inc("invalid_token:" + id)
		return nil
	}
	target, reason := MapLegacyShortTarget(r.TargetURL)
	if reason != "" {
		c.inc(reason)
		c.inc(reason + ":" + id)
		return nil
	}
	legacyURL := pgText(truncate(strings.TrimSpace(r.TargetURL), shorturls.MaxTargetLength))

	key := Key{System: s.system(), Table: "short_urls", ID: id, TargetTable: "short_urls"}
	sum := Checksum(token, r.TargetURL)
	_, mapped, err := m.Lookup(ctx, key.System, key.Table, key.ID)
	if err != nil {
		return err
	}
	if !mapped {
		// A token issued by the new app that equals a legacy one: the new
		// link wins, the legacy one is reported.
		taken, err := q.MigratorShortURLTokenTaken(ctx, token)
		if err != nil {
			return fmt.Errorf("token taken: %w", err)
		}
		if taken {
			c.inc("token_conflict:" + id)
			return nil
		}
	}
	res, err := m.Upsert(ctx, key, sum)
	if err != nil {
		return err
	}
	current, err := q.MigratorShortURLByUUID(ctx, res.UUID)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := q.MigratorInsertShortURL(ctx, db.MigratorInsertShortURLParams{
			Uuid: res.UUID, BrandID: brandID, Token: token, TargetPath: target,
			LegacyTargetUrl: legacyURL, CreatedAt: pgTime(r.CreatedAt),
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("token %s was taken meanwhile", token)
			}
			return fmt.Errorf("insert short url: %w", err)
		}
		c.inc(cntCreated)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read short url: %w", err)
	}
	if current.Token != token {
		// The hub never changes a token; a different one is reported, the
		// stored link is kept.
		c.inc("token_changed:" + id)
		return nil
	}
	if !res.Changed && current.TargetPath == target {
		c.inc(cntUnchanged)
		return nil
	}
	if err := q.MigratorUpdateShortURL(ctx, db.MigratorUpdateShortURLParams{
		ID: current.ID, TargetPath: target, LegacyTargetUrl: legacyURL,
	}); err != nil {
		return fmt.Errorf("update short url: %w", err)
	}
	c.inc(cntUpdated)
	return nil
}

// ---------------------------------------------------------------- step 11

// LegacyMessagesStep copies the hub's message logs into the read-only
// archive legacy_messages (TEC-263, step 11):
//
//   - sms_logs -> channel sms, or whatsapp for the rows the hub sent over
//     WhatsApp (sms_logs.channel = 'whatsapp'; the hub has no other WhatsApp
//     log table);
//   - notifications (Laravel database notifications) -> channel
//     notification.
//
// The archive is append-only: a row imported once is never rewritten, so a
// rerun adds only new legacy rows ("unchanged" counts the rest, "changed"
// the ones whose source changed since, which are reported and kept). The
// owning organization is the dealer of the notified customer / user (else of
// the sender), falling back to the Olex center; user_id is the migrated
// account of the notifiable, when an earlier step mapped it.
type LegacyMessagesStep struct {
	// System is the migration_map source system; empty means SourceHub.
	System string
}

// Name implements Step.
func (LegacyMessagesStep) Name() string { return "legacy_messages" }

func (s LegacyMessagesStep) system() string {
	if s.System == "" {
		return SourceHub
	}
	return s.System
}

// Legacy message channels (legacy_messages.channel).
const (
	ChannelSMS          = "sms"
	ChannelWhatsApp     = "whatsapp"
	ChannelNotification = "notification"
)

// Laravel morph types of the hub notifiables.
const (
	morphCustomer = `App\Models\Customer`
	morphUser     = `App\Models\User`
)

const legacySMSLogsQuery = `SELECT id, phone, message, sender, message_type, message_content_type, channel, status,
	response_id, quantity, amount, number_count, description, response_data, invalid_phones,
	notifiable_type, notifiable_id, bulk_sms_id, sent_by, sent_at, created_at, updated_at
FROM sms_logs`

const legacyNotificationsQuery = `SELECT id, type, notifiable_type, notifiable_id, data, read_at, created_at, updated_at
FROM notifications`

const (
	legacyCustomerDealersQuery = `SELECT id, dealer_id FROM customers WHERE dealer_id IS NOT NULL`
	legacyUserDealersQuery     = `SELECT id, dealer_id FROM users WHERE dealer_id IS NOT NULL`
)

type legacySMSLog struct {
	ID                                int64
	Phone, Message, Sender            string
	MessageType, ContentType, Channel string
	Status                            string
	ResponseID                        sql.NullInt64
	Quantity, NumberCount             sql.NullInt64
	Amount                            sql.NullString
	Description                       sql.NullString
	ResponseData, InvalidPhones       []byte
	NotifiableType                    sql.NullString
	NotifiableID, BulkSMSID, SentBy   sql.NullInt64
	SentAt, CreatedAt, UpdatedAt      sql.NullTime
}

type legacyNotification struct {
	ID, Type, NotifiableType, Data string
	NotifiableID                   int64
	ReadAt, CreatedAt, UpdatedAt   sql.NullTime
}

type messageRun struct {
	step     LegacyMessagesStep
	q        *db.Queries
	m        *Mapper
	c        counts
	brandID  int64
	centerID int64
	// legacy customer / user id -> legacy dealer id
	customerDealer, userDealer map[int64]int64
	orgs                       map[int64]int64 // legacy dealer id -> organization id (0: unmapped)
	users                      map[string]int64
}

// Run implements Step.
func (s LegacyMessagesStep) Run(ctx context.Context, src Sources, dst *Target, m *Mapper) (StepResult, error) {
	c := counts{}
	hub, err := src.Get(SourceHub)
	if err != nil {
		return StepResult{}, err
	}
	brand, err := dst.Q.GetBrandBySlug(ctx, OlexBrandSlug)
	if err != nil {
		return StepResult{}, fmt.Errorf("brand %q: %w", OlexBrandSlug, err)
	}
	center, err := dst.Q.GetBrandCenter(ctx, brand.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return StepResult{}, errors.New("olex center is missing; run the organizations step first")
	}
	if err != nil {
		return StepResult{}, fmt.Errorf("olex center: %w", err)
	}
	r := &messageRun{
		step: s, q: dst.Q, m: m, c: c, brandID: brand.ID, centerID: center.ID,
		orgs: map[int64]int64{}, users: map[string]int64{},
	}
	if r.customerDealer, err = readDealerLinks(ctx, hub, legacyCustomerDealersQuery); err != nil {
		return StepResult{Counts: c}, fmt.Errorf("customer dealers: %w", err)
	}
	if r.userDealer, err = readDealerLinks(ctx, hub, legacyUserDealersQuery); err != nil {
		return StepResult{Counts: c}, fmt.Errorf("user dealers: %w", err)
	}
	where, args := deltaFilter(dst)

	var watermark time.Time
	logs, err := readSMSLogs(ctx, hub, where, args)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	for _, l := range logs {
		c.inc("sms_logs_read")
		if ts := latest(l.CreatedAt, l.UpdatedAt, l.SentAt); ts.After(watermark) {
			watermark = ts
		}
		if err := r.importSMSLog(ctx, l); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("sms log %d: %w", l.ID, err)
		}
	}

	notes, err := readNotifications(ctx, hub, where, args)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	for _, n := range notes {
		c.inc("notifications_read")
		if ts := latest(n.CreatedAt, n.UpdatedAt, n.ReadAt); ts.After(watermark) {
			watermark = ts
		}
		if err := r.importNotification(ctx, n); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("notification %s: %w", n.ID, err)
		}
	}
	return StepResult{Counts: c, Watermark: watermark}, nil
}

func readDealerLinks(ctx context.Context, hub source.LegacySource, query string) (map[int64]int64, error) {
	rows, err := hub.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	out := map[int64]int64{}
	for rows.Next() {
		var id, dealer int64
		if err := rows.Scan(&id, &dealer); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out[id] = dealer
	}
	return out, rows.Close()
}

func readSMSLogs(ctx context.Context, hub source.LegacySource, where string, args []any) ([]legacySMSLog, error) {
	rows, err := hub.Query(ctx, legacySMSLogsQuery+where+" ORDER BY id", args...)
	if err != nil {
		return nil, err
	}
	var out []legacySMSLog
	for rows.Next() {
		var l legacySMSLog
		var respData, invalid []byte
		if err := rows.Scan(&l.ID, &l.Phone, &l.Message, &l.Sender, &l.MessageType, &l.ContentType, &l.Channel,
			&l.Status, &l.ResponseID, &l.Quantity, &l.Amount, &l.NumberCount, &l.Description, &respData, &invalid,
			&l.NotifiableType, &l.NotifiableID, &l.BulkSMSID, &l.SentBy, &l.SentAt, &l.CreatedAt, &l.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan sms log: %w", err)
		}
		l.ResponseData, l.InvalidPhones = jsonOrNil(respData), jsonOrNil(invalid)
		out = append(out, l)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read sms logs: %w", err)
	}
	return out, nil
}

func readNotifications(ctx context.Context, hub source.LegacySource, where string, args []any) ([]legacyNotification, error) {
	rows, err := hub.Query(ctx, legacyNotificationsQuery+where+" ORDER BY created_at, id", args...)
	if err != nil {
		return nil, err
	}
	var out []legacyNotification
	for rows.Next() {
		var n legacyNotification
		if err := rows.Scan(&n.ID, &n.Type, &n.NotifiableType, &n.NotifiableID, &n.Data, &n.ReadAt,
			&n.CreatedAt, &n.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read notifications: %w", err)
	}
	return out, nil
}

// jsonOrNil keeps a legacy JSON column only when it parses.
func jsonOrNil(v []byte) []byte {
	if len(v) == 0 || !json.Valid(v) {
		return nil
	}
	return append([]byte(nil), v...)
}

// SMSChannel maps sms_logs.channel to a legacy_messages channel: the hub
// writes 'sms' or 'whatsapp' (SmsChannelEnum); anything else is reported
// and archived as sms.
func SMSChannel(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "whatsapp":
		return ChannelWhatsApp, true
	case "sms", "":
		return ChannelSMS, true
	}
	return ChannelSMS, false
}

// NotificationBody is the readable text of a Laravel database notification:
// the title and body of its data (Filament notifications), else the raw
// data.
func NotificationBody(data string) string {
	var d map[string]any
	if err := json.Unmarshal([]byte(data), &d); err != nil {
		return data
	}
	var parts []string
	for _, k := range []string{"title", "body", "message"} {
		if v, ok := d[k].(string); ok && strings.TrimSpace(v) != "" {
			parts = append(parts, strings.TrimSpace(v))
		}
	}
	if len(parts) == 0 {
		return data
	}
	return strings.Join(parts, "\n")
}

func (r *messageRun) importSMSLog(ctx context.Context, l legacySMSLog) error {
	id := strconv.FormatInt(l.ID, 10)
	channel, known := SMSChannel(l.Channel)
	if !known {
		r.c.inc("channel_unknown:" + id)
	}
	payload := map[string]any{
		"sender": l.Sender, "message_type": l.MessageType, "message_content_type": l.ContentType,
		"legacy_channel": l.Channel, "status": l.Status,
	}
	putInt := func(k string, v sql.NullInt64) {
		if v.Valid {
			payload[k] = v.Int64
		}
	}
	putInt("response_id", l.ResponseID)
	putInt("quantity", l.Quantity)
	putInt("number_count", l.NumberCount)
	putInt("notifiable_id", l.NotifiableID)
	putInt("bulk_sms_id", l.BulkSMSID)
	putInt("sent_by", l.SentBy)
	if l.Amount.Valid {
		payload["amount"] = l.Amount.String
	}
	if l.Description.Valid {
		payload["description"] = l.Description.String
	}
	if l.NotifiableType.Valid {
		payload["notifiable_type"] = l.NotifiableType.String
	}
	if l.ResponseData != nil {
		payload["response_data"] = json.RawMessage(l.ResponseData)
	}
	if l.InvalidPhones != nil {
		payload["invalid_phones"] = json.RawMessage(l.InvalidPhones)
	}
	var recipient pgtype.Text
	if ph := normalize.NormalizePhone(l.Phone, TRCountry); ph.Verified {
		recipient = pgText(ph.E164)
	} else {
		r.c.inc("recipient_unresolved")
		if raw := strings.TrimSpace(l.Phone); raw != "" {
			payload["recipient_raw"] = truncate(raw, 255)
		}
	}

	notifType, notifID := "", int64(0)
	if l.NotifiableType.Valid && l.NotifiableID.Valid {
		notifType, notifID = l.NotifiableType.String, l.NotifiableID.Int64
	}
	userID, err := r.user(ctx, notifType, notifID)
	if err != nil {
		return err
	}
	orgID, err := r.org(ctx, notifType, notifID, l.SentBy)
	if err != nil {
		return err
	}
	sentAt := l.SentAt
	if !sentAt.Valid {
		sentAt = l.CreatedAt
	}
	return r.insert(ctx, "sms_logs", id, channel, recipient, userID, orgID, l.Message, payload, sentAt,
		Checksum(l.Phone, l.Message, l.Channel, l.Status, l.SentAt.Time.Unix(), l.SentAt.Valid, string(l.ResponseData)))
}

func (r *messageRun) importNotification(ctx context.Context, n legacyNotification) error {
	payload := map[string]any{
		"type": n.Type, "notifiable_type": n.NotifiableType, "notifiable_id": n.NotifiableID,
	}
	if json.Valid([]byte(n.Data)) {
		payload["data"] = json.RawMessage(n.Data)
	} else {
		payload["data_raw"] = n.Data
	}
	if n.ReadAt.Valid {
		payload["read_at"] = n.ReadAt.Time.UTC().Format(time.RFC3339)
	}
	userID, err := r.user(ctx, n.NotifiableType, n.NotifiableID)
	if err != nil {
		return err
	}
	orgID, err := r.org(ctx, n.NotifiableType, n.NotifiableID, sql.NullInt64{})
	if err != nil {
		return err
	}
	return r.insert(ctx, "notifications", strings.TrimSpace(n.ID), ChannelNotification, pgtype.Text{}, userID, orgID,
		NotificationBody(n.Data), payload, n.CreatedAt,
		Checksum(n.Type, n.NotifiableType, n.NotifiableID, n.Data, n.ReadAt.Valid))
}

func (r *messageRun) insert(ctx context.Context, table, id, channel string, recipient pgtype.Text, userID pgtype.Int8,
	orgID int64, body string, payload map[string]any, sentAt sql.NullTime, sum string) error {
	if id == "" {
		r.c.inc("skipped_no_id:" + table)
		return nil
	}
	key := Key{System: r.step.system(), Table: table, ID: id, TargetTable: "legacy_messages"}
	res, err := r.m.Upsert(ctx, key, sum)
	if err != nil {
		return err
	}
	present, err := r.q.MigratorLegacyMessageExists(ctx, db.MigratorLegacyMessageExistsParams{
		SourceSystem: key.System, SourceTable: table, SourceID: id,
	})
	if err != nil {
		return fmt.Errorf("read legacy message: %w", err)
	}
	if present {
		if res.Changed {
			// Append-only archive: the first copy stays, the change is
			// reported.
			r.c.inc("changed_kept:" + table + ":" + id)
			return nil
		}
		r.c.inc(cntUnchanged)
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode payload: %w", err)
	}
	if _, err := r.q.MigratorInsertLegacyMessage(ctx, db.MigratorInsertLegacyMessageParams{
		Uuid: res.UUID, OrganizationID: orgID, BrandID: r.brandID, Channel: channel, Recipient: recipient,
		UserID: userID, Body: body, Payload: raw, SentAt: pgTime(sentAt),
		SourceSystem: key.System, SourceTable: table, SourceID: id,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.c.inc(cntUnchanged)
			return nil
		}
		return fmt.Errorf("insert legacy message: %w", err)
	}
	r.c.inc(cntCreated)
	r.c.inc("created_" + channel)
	return nil
}

// user resolves the migrated account of a hub notifiable (a customer or a
// user an earlier step mapped); an unmapped notifiable stays NULL.
func (r *messageRun) user(ctx context.Context, morph string, id int64) (pgtype.Int8, error) {
	var table string
	switch morph {
	case morphCustomer:
		table = "customers"
	case morphUser:
		table = "users"
	default:
		return pgtype.Int8{}, nil
	}
	legacyID := strconv.FormatInt(id, 10)
	key := table + ":" + legacyID
	if v, ok := r.users[key]; ok {
		return pgtype.Int8{Int64: v, Valid: v != 0}, nil
	}
	target, found, err := r.m.Lookup(ctx, r.step.system(), table, legacyID)
	if err != nil {
		return pgtype.Int8{}, err
	}
	var userID int64
	if found {
		u, err := r.q.MigratorUserByUUID(ctx, target)
		switch {
		case err == nil:
			userID = u.ID
		case !errors.Is(err, pgx.ErrNoRows):
			return pgtype.Int8{}, fmt.Errorf("read user: %w", err)
		}
	}
	if userID == 0 {
		r.c.inc("user_unmapped:" + key)
	}
	r.users[key] = userID
	return pgtype.Int8{Int64: userID, Valid: userID != 0}, nil
}

// org is the dealer organization of the notifiable, else of the sender,
// else the Olex center.
func (r *messageRun) org(ctx context.Context, morph string, id int64, sentBy sql.NullInt64) (int64, error) {
	var dealers []int64
	switch morph {
	case morphCustomer:
		if d, ok := r.customerDealer[id]; ok {
			dealers = append(dealers, d)
		}
	case morphUser:
		if d, ok := r.userDealer[id]; ok {
			dealers = append(dealers, d)
		}
	}
	if sentBy.Valid {
		if d, ok := r.userDealer[sentBy.Int64]; ok {
			dealers = append(dealers, d)
		}
	}
	for _, d := range dealers {
		orgID, ok := r.orgs[d]
		if !ok {
			target, found, err := r.m.Lookup(ctx, r.step.system(), "dealers", strconv.FormatInt(d, 10))
			if err != nil {
				return 0, err
			}
			if found {
				orgID, err = r.q.MigratorOrganizationIDByUUID(ctx, target)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return 0, fmt.Errorf("read organization: %w", err)
				}
			}
			r.orgs[d] = orgID
		}
		if orgID != 0 {
			return orgID, nil
		}
	}
	r.c.inc("organization_center")
	return r.centerID, nil
}
