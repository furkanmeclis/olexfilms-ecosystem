package usecase

// TEC-398 (F4-02f): the panel conversation API. The inbox list (list
// contract), a conversation, its timeline (cursor), staff replies and new
// outgoing conversations, status / AI mode / assignment changes and read
// marks. Visibility follows the F4 user answer S2: only the platform admin
// (super admin holding conversations.read) reads conversations; anyone else
// gets 404 for a conversation and an empty list. assigned_org_id is kept as
// the future visibility owner ("hand over to a dealer") but grants nothing
// yet.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// StaffReplyAIPause is how long the AI stays silent after a staff reply.
const StaffReplyAIPause = 30 * time.Minute

// Timeline page size.
const (
	DefaultMessagePage = 50
	MaxMessagePage     = 100
)

// EventConversationUpdated is the realtime event of a changed conversation
// (status, AI mode, assignment, read mark).
const EventConversationUpdated = "conversations.conversation.updated"

// Inbox errors. ErrNotFound covers conversations the viewer may not see.
var (
	ErrNotFound  = errors.New("whatsapp: conversation not found")
	ErrForbidden = errors.New("whatsapp: forbidden")
	// ErrContactNoPhone: the chosen user has no E.164 phone (422).
	ErrContactNoPhone = errors.New("whatsapp: user has no phone number")
	// ErrAssigneeNotAllowed: the assignee cannot read conversations (S2:
	// platform admins only) (422).
	ErrAssigneeNotAllowed = errors.New("whatsapp: assignee cannot read conversations")
)

// Viewer is the caller of the inbox.
type Viewer struct {
	UserID int64
	// PlatformAdmin is a super admin holding conversations.read.
	PlatformAdmin bool
}

// canSee is the visibility rule of one conversation (S2: platform admin
// only, whatever the assignment).
func (v Viewer) canSee(db.Conversation) bool { return v.PlatformAdmin }

// InboxDeps wires Inbox; Outbox and Publisher may be nil.
type InboxDeps struct {
	Queries   *db.Queries
	Tx        TxBeginner
	Messaging *Messaging
	Storage   ObjectStore
	Outbox    Outbox
	Publisher realtime.Publisher
}

// Inbox is the panel conversation use case.
type Inbox struct {
	d   InboxDeps
	now func() time.Time
}

// NewInbox builds the inbox use case.
func NewInbox(d InboxDeps) *Inbox { return &Inbox{d: d, now: time.Now} }

// SetClock replaces the clock (tests).
func (in *Inbox) SetClock(now func() time.Time) { in.now = now }

// UserRef names a user in a view.
type UserRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// OrgRef names an organization in a view.
type OrgRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Type string    `json:"type"`
}

// ConversationView is the API shape of a conversation.
type ConversationView struct {
	UUID          uuid.UUID  `json:"uuid"`
	Channel       string     `json:"channel"`
	ContactE164   string     `json:"contact_e164"`
	ContactName   *string    `json:"contact_name"`
	Status        string     `json:"status"`
	AIMode        string     `json:"ai_mode"`
	AIPausedUntil *time.Time `json:"ai_paused_until"`
	AssignedUser  *UserRef   `json:"assigned_user"`
	AssignedOrg   *OrgRef    `json:"assigned_org"`
	IdentityKind  string     `json:"identity_kind"`
	IdentityUser  *UserRef   `json:"identity_user"`
	IdentityOrg   *OrgRef    `json:"identity_org"`
	Locale        *string    `json:"locale"`
	UnreadCount   int32      `json:"unread_count"`
	LastMessageAt *time.Time `json:"last_message_at"`
	LastInboundAt *time.Time `json:"last_inbound_at"`
	AIConsentAt   *time.Time `json:"ai_consent_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// MessageView is the API shape of a timeline message.
type MessageView struct {
	MessageEvent
	SenderUser *UserRef `json:"sender_user"`
}

// MessagePage is a timeline page, newest first. NextCursor pages older
// messages (before=); nil on the last page.
type MessagePage struct {
	Items      []MessageView `json:"items"`
	NextCursor *string       `json:"next_cursor"`
}

// AIRunView is a redacted AI pipeline run log for one conversation.
type AIRunView struct {
	UUID             uuid.UUID `json:"uuid"`
	Status           string    `json:"status"`
	Model            string    `json:"model"`
	Tokens           int64     `json:"tokens"`
	InputTokens      int64     `json:"input_tokens"`
	OutputTokens     int64     `json:"output_tokens"`
	CacheReadTokens  int64     `json:"cache_read_tokens"`
	CacheWriteTokens int64     `json:"cache_write_tokens"`
	DurationMS       *int32    `json:"duration_ms"`
	Error            *string   `json:"error"`
	CreatedAt        time.Time `json:"created_at"`
}

// AIRunPage is the newest-first run list of a conversation.
type AIRunPage struct {
	Items []AIRunView `json:"items"`
}

// ReplyResult is a queued staff message and the conversation after it.
type ReplyResult struct {
	Conversation ConversationView `json:"conversation"`
	Message      MessageView      `json:"message"`
}

// refs resolves user / organization names once per call.
type refs struct {
	q     *db.Queries
	users map[int64]*UserRef
	orgs  map[int64]*OrgRef
}

func newRefs(q *db.Queries) *refs {
	return &refs{q: q, users: map[int64]*UserRef{}, orgs: map[int64]*OrgRef{}}
}

func (r *refs) user(ctx context.Context, id pgtype.Int8) *UserRef {
	if !id.Valid {
		return nil
	}
	if u, ok := r.users[id.Int64]; ok {
		return u
	}
	var ref *UserRef
	if u, err := r.q.GetUserByID(ctx, id.Int64); err == nil {
		ref = &UserRef{UUID: u.Uuid, Name: strings.TrimSpace(u.Name + " " + u.Surname)}
	}
	r.users[id.Int64] = ref
	return ref
}

func (r *refs) org(ctx context.Context, id pgtype.Int8) *OrgRef {
	if !id.Valid {
		return nil
	}
	if o, ok := r.orgs[id.Int64]; ok {
		return o
	}
	var ref *OrgRef
	if o, err := r.q.GetOrganizationByID(ctx, id.Int64); err == nil {
		ref = &OrgRef{UUID: o.Uuid, Name: o.Name, Type: o.Type}
	}
	r.orgs[id.Int64] = ref
	return ref
}

func (r *refs) conversation(ctx context.Context, c db.Conversation) ConversationView {
	return ConversationView{
		UUID: c.Uuid, Channel: c.Channel, ContactE164: c.ContactE164, ContactName: textPtr(c.ContactName),
		Status: c.Status, AIMode: c.AiMode, AIPausedUntil: tsPtr(c.AiPausedUntil),
		AssignedUser: r.user(ctx, c.AssignedUserID), AssignedOrg: r.org(ctx, c.AssignedOrgID),
		IdentityKind: c.IdentityKind, IdentityUser: r.user(ctx, c.IdentityUserID), IdentityOrg: r.org(ctx, c.IdentityOrgID),
		Locale: textPtr(c.Locale), UnreadCount: c.UnreadCount, LastMessageAt: tsPtr(c.LastMessageAt),
		LastInboundAt: tsPtr(c.LastInboundAt), AIConsentAt: tsPtr(c.AiConsentAt),
		CreatedAt: c.CreatedAt.Time, UpdatedAt: c.UpdatedAt.Time,
	}
}

func (r *refs) message(ctx context.Context, c db.Conversation, m db.Message) MessageView {
	return MessageView{MessageEvent: NewMessageEvent(c, m), SenderUser: r.user(ctx, m.SenderUserID)}
}

func aiRunView(row db.ConversationAiRun) AIRunView {
	return AIRunView{
		UUID:             row.Uuid,
		Status:           row.Status,
		Model:            row.Model,
		Tokens:           row.InputTokens + row.OutputTokens + row.CacheReadTokens + row.CacheWriteTokens,
		InputTokens:      row.InputTokens,
		OutputTokens:     row.OutputTokens,
		CacheReadTokens:  row.CacheReadTokens,
		CacheWriteTokens: row.CacheWriteTokens,
		DurationMS:       int4Ptr(row.DurationMs),
		Error:            textPtr(row.Error),
		CreatedAt:        row.CreatedAt.Time,
	}
}

func int4Ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	return &v.Int32
}

// ParseConversationFilter reads the GET /v1/conversations filters (list
// contract): status, identity_kind, ai_mode (CSV), assigned_user_uuid (CSV
// of user uuids), channel, last_message_from/_to, unread, q, sort, limit,
// offset. Errors are *apiquery.ValidationError.
func ParseConversationFilter(ctx context.Context, q *db.Queries, values url.Values) (repository.ConversationFilter, error) {
	lq := apiquery.Parse(values)
	f := repository.ConversationFilter{Q: lq.Q, Sort: lq.Sort, Limit: lq.Limit, Offset: lq.Offset}
	if _, err := apiquery.ResolveSort(lq.Sort, repository.ConversationSort); err != nil {
		return f, err
	}
	var err error
	if f.Statuses, err = apiquery.EnumList(values, "status", model.Statuses...); err != nil {
		return f, err
	}
	if f.IdentityKinds, err = apiquery.EnumList(values, "identity_kind", model.IdentityKinds...); err != nil {
		return f, err
	}
	if f.AIModes, err = apiquery.EnumList(values, "ai_mode", model.AIModes...); err != nil {
		return f, err
	}
	channels, err := apiquery.EnumList(values, "channel", whatsapp.ChannelWhatsApp, "sms")
	if err != nil {
		return f, err
	}
	if len(channels) == 1 {
		f.Channel = channels[0]
	}
	if f.LastMessage, err = apiquery.DateRange(values, "last_message"); err != nil {
		return f, err
	}
	if f.Unread, err = apiquery.Bool(values, "unread"); err != nil {
		return f, err
	}
	for _, raw := range apiquery.CSVValues(values, "assigned_user_uuid") {
		id, err := uuid.Parse(raw)
		if err != nil {
			return f, &apiquery.ValidationError{Details: []apiquery.Detail{{
				Field: "assigned_user_uuid", Message: "must be a CSV of user uuids", Code: "invalid",
			}}}
		}
		u, err := q.GetUserByUUID(ctx, id)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			f.AssignedUserIDs = append(f.AssignedUserIDs, 0) // matches nobody
		case err != nil:
			return f, err
		default:
			f.AssignedUserIDs = append(f.AssignedUserIDs, u.ID)
		}
	}
	return f, nil
}

// List returns a page of the conversations the viewer may see.
func (in *Inbox) List(ctx context.Context, v Viewer, f repository.ConversationFilter) ([]ConversationView, int64, error) {
	if _, err := apiquery.ResolveSort(f.Sort, repository.ConversationSort); err != nil {
		return nil, 0, err
	}
	if !v.PlatformAdmin {
		return []ConversationView{}, 0, nil
	}
	rows, total, err := repository.FromQueries(in.d.Queries).ListConversations(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	r := newRefs(in.d.Queries)
	out := make([]ConversationView, 0, len(rows))
	for _, c := range rows {
		out = append(out, r.conversation(ctx, c))
	}
	return out, total, nil
}

// load returns a conversation the viewer may see.
func (in *Inbox) load(ctx context.Context, q *db.Queries, v Viewer, id uuid.UUID) (db.Conversation, error) {
	c, err := q.GetConversationByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !v.canSee(c)) {
		return db.Conversation{}, ErrNotFound
	}
	return c, err
}

// Get returns one conversation.
func (in *Inbox) Get(ctx context.Context, v Viewer, id uuid.UUID) (ConversationView, error) {
	c, err := in.load(ctx, in.d.Queries, v, id)
	if err != nil {
		return ConversationView{}, err
	}
	return newRefs(in.d.Queries).conversation(ctx, c), nil
}

// EncodeCursor is the opaque before= cursor of a timeline position.
func EncodeCursor(c repository.MessageCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(c.At.UnixMicro(), 10) + "." + strconv.FormatInt(c.ID, 10)))
}

// DecodeCursor parses a before= cursor.
func DecodeCursor(s string) (repository.MessageCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return repository.MessageCursor{}, fmt.Errorf("%w: cursor", ErrInvalidRequest)
	}
	at, id, ok := strings.Cut(string(raw), ".")
	micros, err1 := strconv.ParseInt(at, 10, 64)
	n, err2 := strconv.ParseInt(id, 10, 64)
	if !ok || err1 != nil || err2 != nil || n <= 0 {
		return repository.MessageCursor{}, fmt.Errorf("%w: cursor", ErrInvalidRequest)
	}
	return repository.MessageCursor{At: time.UnixMicro(micros).UTC(), ID: n}, nil
}

// Messages returns a timeline page, newest first, older than before (nil =
// the newest page).
func (in *Inbox) Messages(ctx context.Context, v Viewer, id uuid.UUID, before *repository.MessageCursor, limit int32) (MessagePage, error) {
	c, err := in.load(ctx, in.d.Queries, v, id)
	if err != nil {
		return MessagePage{}, err
	}
	if limit <= 0 {
		limit = DefaultMessagePage
	}
	limit = min(limit, MaxMessagePage)
	rows, next, err := repository.FromQueries(in.d.Queries).ListMessagesBefore(ctx, c.ID, before, limit)
	if err != nil {
		return MessagePage{}, err
	}
	r := newRefs(in.d.Queries)
	page := MessagePage{Items: make([]MessageView, 0, len(rows))}
	for _, m := range rows {
		page.Items = append(page.Items, r.message(ctx, c, m))
	}
	if next != nil {
		s := EncodeCursor(*next)
		page.NextCursor = &s
	}
	return page, nil
}

// AIRuns returns the newest AI pipeline runs of a visible conversation.
func (in *Inbox) AIRuns(ctx context.Context, v Viewer, id uuid.UUID, limit int32) (AIRunPage, error) {
	c, err := in.load(ctx, in.d.Queries, v, id)
	if err != nil {
		return AIRunPage{}, err
	}
	if limit <= 0 {
		limit = DefaultMessagePage
	}
	limit = min(limit, MaxMessagePage)
	rows, err := in.d.Queries.ListConversationAIRuns(ctx, db.ListConversationAIRunsParams{
		ConversationID: c.ID,
		LimitCount:     limit,
	})
	if err != nil {
		return AIRunPage{}, err
	}
	page := AIRunPage{Items: make([]AIRunView, 0, len(rows))}
	for _, row := range rows {
		page.Items = append(page.Items, aiRunView(row))
	}
	return page, nil
}

// StoredMedia is a stored message attachment.
type StoredMedia struct {
	Body     io.ReadCloser
	Size     int64
	Mime     string
	FileName string
}

// Media opens the stored attachment of a message of the conversation.
func (in *Inbox) Media(ctx context.Context, v Viewer, convID, msgID uuid.UUID) (StoredMedia, error) {
	c, err := in.load(ctx, in.d.Queries, v, convID)
	if err != nil {
		return StoredMedia{}, err
	}
	m, err := in.d.Queries.GetMessageByUUID(ctx, msgID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (m.ConversationID != c.ID || !m.MediaStorageKey.Valid)) {
		return StoredMedia{}, ErrNotFound
	}
	if err != nil {
		return StoredMedia{}, err
	}
	if in.d.Storage == nil {
		return StoredMedia{}, ErrNotConfigured
	}
	body, size, err := in.d.Storage.Download(ctx, m.MediaStorageKey.String)
	if err != nil {
		return StoredMedia{}, err
	}
	name := mediaFileName(m)
	return StoredMedia{Body: body, Size: size, Mime: m.MediaMime.String, FileName: name}, nil
}

func mediaFileName(m db.Message) string {
	ev := NewMessageEvent(db.Conversation{}, m)
	if len(ev.Media) > 0 {
		var meta struct {
			FileName string `json:"file_name"`
		}
		if json.Unmarshal(ev.Media, &meta) == nil && meta.FileName != "" {
			return meta.FileName
		}
	}
	return m.Uuid.String()
}

// Reply queues a staff message (sender_type staff, sender_user_id = the
// viewer) and pauses the AI for StaffReplyAIPause.
func (in *Inbox) Reply(ctx context.Context, v Viewer, id uuid.UUID, body string, media *OutgoingMedia) (ReplyResult, error) {
	c, err := in.load(ctx, in.d.Queries, v, id)
	if err != nil {
		return ReplyResult{}, err
	}
	return in.reply(ctx, v, c, body, media)
}

func (in *Inbox) reply(ctx context.Context, v Viewer, c db.Conversation, body string, media *OutgoingMedia) (ReplyResult, error) {
	if in.d.Messaging == nil {
		return ReplyResult{}, ErrNotConfigured
	}
	uid := v.UserID
	msg, err := in.d.Messaging.Queue(ctx, OutgoingMessage{
		ConversationID: c.ID, SenderType: model.SenderStaff, SenderUserID: &uid, Body: body, Media: media,
	})
	if err != nil {
		return ReplyResult{}, err
	}
	c, err = in.pauseForStaff(ctx, c.ID)
	if err != nil {
		return ReplyResult{}, err
	}
	r := newRefs(in.d.Queries)
	view := r.conversation(ctx, c)
	in.publish(ctx, c, view)
	return ReplyResult{Conversation: view, Message: r.message(ctx, c, msg)}, nil
}

// pauseForStaff pauses an auto conversation (or extends a shorter pause)
// until now + StaffReplyAIPause. AI off and an open-ended pause stay.
func (in *Inbox) pauseForStaff(ctx context.Context, convID int64) (db.Conversation, error) {
	c, err := in.d.Queries.GetConversationByID(ctx, convID)
	if err != nil {
		return c, err
	}
	until := in.now().Add(StaffReplyAIPause)
	switch c.AiMode {
	case model.AIModeOff:
		return c, nil
	case model.AIModePaused:
		if !c.AiPausedUntil.Valid || !c.AiPausedUntil.Time.Before(until) {
			return c, nil
		}
	}
	return in.d.Queries.SetConversationAIMode(ctx, db.SetConversationAIModeParams{
		ID: c.ID, AiMode: model.AIModePaused, AiPausedUntil: ts(until),
	})
}

// StartInput opens (or continues) the conversation with a user and sends
// the first staff message.
type StartInput struct {
	UserUUID uuid.UUID
	Body     string
	Media    *OutgoingMedia
}

// Start opens the WhatsApp conversation with a customer or staff user (the
// existing one when the number already has a conversation) and queues the
// first staff message. A one-to-one service message: the marketing opt-out
// does not apply.
func (in *Inbox) Start(ctx context.Context, v Viewer, s StartInput) (ReplyResult, error) {
	if !v.PlatformAdmin {
		return ReplyResult{}, ErrForbidden
	}
	// Checked before the conversation row is created (Queue checks again).
	if strings.TrimSpace(s.Body) == "" && s.Media == nil {
		return ReplyResult{}, fmt.Errorf("%w: body or file is required", ErrInvalidRequest)
	}
	if len([]rune(strings.TrimSpace(s.Body))) > MaxBodyLength {
		return ReplyResult{}, fmt.Errorf("%w: body is longer than %d characters", ErrInvalidRequest, MaxBodyLength)
	}
	if s.Media != nil {
		mime, err := whatsapp.ValidateMedia(s.Media.Data, s.Media.FileName)
		if err == nil && mime == whatsapp.MimeOGG {
			err = whatsapp.ErrMediaType
		}
		if err != nil {
			return ReplyResult{}, fmt.Errorf("%w: %w", ErrInvalidRequest, err)
		}
	}
	u, err := in.d.Queries.GetUserByUUID(ctx, s.UserUUID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && u.DeletedAt.Valid) {
		return ReplyResult{}, fmt.Errorf("%w: user_uuid", ErrInvalidRequest)
	}
	if err != nil {
		return ReplyResult{}, err
	}
	if !u.PhoneE164.Valid || !strings.HasPrefix(u.PhoneE164.String, "+") {
		return ReplyResult{}, ErrContactNoPhone
	}
	c, err := in.d.Queries.UpsertConversation(ctx, db.UpsertConversationParams{
		Channel: whatsapp.ChannelWhatsApp, ContactE164: u.PhoneE164.String,
		ContactName: text(strings.TrimSpace(u.Name + " " + u.Surname)),
		UserID:      pgtype.Int8{Int64: u.ID, Valid: true},
	})
	if err != nil {
		return ReplyResult{}, err
	}
	return in.reply(ctx, v, c, s.Body, s.Media)
}

// PatchInput changes a conversation; nil fields stay. The Set flags tell
// an explicit null (unassign) from an absent field.
type PatchInput struct {
	Status        *string
	AIMode        *string
	AIPausedUntil *time.Time
	AssignUser    bool
	AssignedUser  *uuid.UUID
	AssignOrg     bool
	AssignedOrg   *uuid.UUID
}

// Patch changes the status, the AI mode and/or the assignment. A new
// assignee (other than the viewer) is notified (conversations.assigned).
func (in *Inbox) Patch(ctx context.Context, v Viewer, id uuid.UUID, p PatchInput) (ConversationView, error) {
	if p.Status != nil && !slices.Contains(model.Statuses, *p.Status) {
		return ConversationView{}, fmt.Errorf("%w: status must be one of %s", ErrInvalidRequest, strings.Join(model.Statuses, ", "))
	}
	if p.AIMode != nil && !slices.Contains(model.AIModes, *p.AIMode) {
		return ConversationView{}, fmt.Errorf("%w: ai_mode must be one of %s", ErrInvalidRequest, strings.Join(model.AIModes, ", "))
	}
	if p.AIPausedUntil != nil {
		if p.AIMode == nil || *p.AIMode != model.AIModePaused {
			return ConversationView{}, fmt.Errorf("%w: ai_paused_until needs ai_mode paused", ErrInvalidRequest)
		}
		if !p.AIPausedUntil.After(in.now()) {
			return ConversationView{}, fmt.Errorf("%w: ai_paused_until must be in the future", ErrInvalidRequest)
		}
	}
	tx, err := in.d.Tx.Begin(ctx)
	if err != nil {
		return ConversationView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := in.d.Queries.WithTx(tx)
	c, err := in.load(ctx, q, v, id)
	if err != nil {
		return ConversationView{}, err
	}
	if p.Status != nil && *p.Status != c.Status {
		if c, err = q.SetConversationStatus(ctx, db.SetConversationStatusParams{ID: c.ID, Status: *p.Status}); err != nil {
			return ConversationView{}, err
		}
	}
	if p.AIMode != nil {
		until := pgtype.Timestamptz{}
		if p.AIPausedUntil != nil {
			until = ts(*p.AIPausedUntil)
		}
		if c, err = q.SetConversationAIMode(ctx, db.SetConversationAIModeParams{ID: c.ID, AiMode: *p.AIMode, AiPausedUntil: until}); err != nil {
			return ConversationView{}, err
		}
	}
	notify := int64(0)
	if p.AssignUser || p.AssignOrg {
		user, org := c.AssignedUserID, c.AssignedOrgID
		if p.AssignUser {
			if user, err = assignableUser(ctx, q, p.AssignedUser); err != nil {
				return ConversationView{}, err
			}
		}
		if p.AssignOrg {
			if org, err = assignableOrg(ctx, q, p.AssignedOrg); err != nil {
				return ConversationView{}, err
			}
		}
		if user.Valid && user != c.AssignedUserID && user.Int64 != v.UserID {
			notify = user.Int64
		}
		if c, err = q.AssignConversation(ctx, db.AssignConversationParams{ID: c.ID, AssignedUserID: user, AssignedOrgID: org}); err != nil {
			return ConversationView{}, err
		}
	}
	if notify != 0 && in.d.Outbox != nil {
		e := events.New(events.ConversationsAssigned)
		e.EntityType = "conversation"
		e.EntityID = &c.ID
		e.EntityUUID = &c.Uuid
		e.Payload = map[string]any{"conversation_uuid": c.Uuid.String(), "assigned_user_id": notify, "actor_user_id": v.UserID}
		if err := in.d.Outbox.Enqueue(ctx, tx, e); err != nil {
			return ConversationView{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ConversationView{}, err
	}
	view := newRefs(in.d.Queries).conversation(ctx, c)
	in.publish(ctx, c, view)
	return view, nil
}

// assignableUser resolves an assignee: nil unassigns; the user must be an
// active platform admin (S2: only they read conversations).
func assignableUser(ctx context.Context, q *db.Queries, id *uuid.UUID) (pgtype.Int8, error) {
	if id == nil {
		return pgtype.Int8{}, nil
	}
	u, err := q.GetUserByUUID(ctx, *id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && u.DeletedAt.Valid) {
		return pgtype.Int8{}, fmt.Errorf("%w: assigned_user_uuid", ErrInvalidRequest)
	}
	if err != nil {
		return pgtype.Int8{}, err
	}
	ok, err := q.UserHasRoleSlug(ctx, db.UserHasRoleSlugParams{UserID: u.ID, Slug: rbac.RoleSuperAdmin})
	if err != nil {
		return pgtype.Int8{}, err
	}
	if !ok || u.Status != "active" {
		return pgtype.Int8{}, ErrAssigneeNotAllowed
	}
	return pgtype.Int8{Int64: u.ID, Valid: true}, nil
}

// assignableOrg resolves the owner organization; nil is the center.
func assignableOrg(ctx context.Context, q *db.Queries, id *uuid.UUID) (pgtype.Int8, error) {
	if id == nil {
		return pgtype.Int8{}, nil
	}
	o, err := q.GetOrganizationByUUID(ctx, *id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && o.DeletedAt.Valid) {
		return pgtype.Int8{}, fmt.Errorf("%w: assigned_org_uuid", ErrInvalidRequest)
	}
	if err != nil {
		return pgtype.Int8{}, err
	}
	return pgtype.Int8{Int64: o.ID, Valid: true}, nil
}

// MarkRead clears the unread counter.
func (in *Inbox) MarkRead(ctx context.Context, v Viewer, id uuid.UUID) (ConversationView, error) {
	c, err := in.load(ctx, in.d.Queries, v, id)
	if err != nil {
		return ConversationView{}, err
	}
	if c.UnreadCount > 0 {
		if c, err = in.d.Queries.MarkConversationRead(ctx, c.ID); err != nil {
			return ConversationView{}, err
		}
	}
	view := newRefs(in.d.Queries).conversation(ctx, c)
	in.publish(ctx, c, view)
	return view, nil
}

// publish sends the changed conversation to the inbox channel and the
// assigned user's channel; failures only lose a live update.
func (in *Inbox) publish(ctx context.Context, c db.Conversation, view ConversationView) {
	if in.d.Publisher == nil {
		return
	}
	payload := map[string]any{"type": EventConversationUpdated, "conversation_uuid": c.Uuid, "conversation": view}
	channels := []string{realtime.ChannelConversations}
	if view.AssignedUser != nil {
		channels = append(channels, realtime.UserChannel(view.AssignedUser.UUID))
	}
	for _, ch := range channels {
		_ = in.d.Publisher.Publish(ctx, ch, payload)
	}
}
