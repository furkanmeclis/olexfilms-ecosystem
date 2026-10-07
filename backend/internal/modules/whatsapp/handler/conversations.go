package handler

// TEC-398 (F4-02f): the panel conversation API (/v1/conversations).

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Conversation error codes (422).
const (
	CodeContactNoPhone      = "CONTACT_PHONE_MISSING"
	CodeAssigneeNotAllowed  = "CONVERSATION_ASSIGNEE_NOT_ALLOWED"
	maxReplyBytes           = whatsapp.MaxMediaBytes + 1<<20 // file + form fields
	maxReplyMemory          = 1 << 20
	replyFileField          = "file"
	replyBodyField          = "body"
	startUserField          = "user_uuid"
	conversationsMetaSource = "conversations"
)

// Conversations serves /v1/conversations.
type Conversations struct {
	inbox *usecase.Inbox
	q     *db.Queries
}

// NewConversations creates the conversation handler.
func NewConversations(inbox *usecase.Inbox, q *db.Queries) *Conversations {
	return &Conversations{inbox: inbox, q: q}
}

func viewer(r *http.Request) usecase.Viewer {
	p := authctx.MustPrincipal(r.Context())
	return usecase.Viewer{
		UserID:        p.UserInternal,
		PlatformAdmin: p.IsSuperAdmin && p.HasPermission(rbac.PermConversationsRead),
	}
}

// ConversationsMeta is the list contract of GET /v1/conversations.
func ConversationsMeta() resourcemeta.ResourceMeta {
	enum, dt := resourcemeta.ColumnTypeEnum, resourcemeta.ColumnTypeDatetime
	faceted := resourcemeta.FilterVariantFaceted
	return resourcemeta.ResourceMeta{
		Resource:      conversationsMetaSource,
		DefaultSort:   repository.ConversationSort.DefaultString(),
		DefaultFields: []string{"contact_name", "contact_e164", "status", "ai_mode", "assigned_user", "unread_count", "last_message_at"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Search: true, Filter: true, Sort: true, Bulk: true,
		},
		SearchableFields: []string{"contact_name", "contact_e164", "identity_user"},
		SortableFields:   repository.ConversationSort.Fields(),
		FilterableFields: []string{"status", "identity_kind", "ai_mode", "assigned_user_uuid", "channel", "last_message_at", "unread"},
		Columns: []resourcemeta.Column{
			{Key: "contact_name", LabelKey: "conversations.contact_name", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "contact_e164", LabelKey: "conversations.contact_e164", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "status", LabelKey: "conversations.status", Type: enum, Filterable: true, FilterVariant: faceted, DefaultVisible: true},
			{Key: "ai_mode", LabelKey: "conversations.ai_mode", Type: enum, Filterable: true, FilterVariant: faceted, DefaultVisible: true},
			{Key: "identity_kind", LabelKey: "conversations.identity_kind", Type: enum, Filterable: true, FilterVariant: faceted, DefaultVisible: false},
			{Key: "assigned_user", LabelKey: "conversations.assigned_user", Type: resourcemeta.ColumnTypeString, Filterable: true, FilterVariant: faceted, DefaultVisible: true},
			{Key: "unread_count", LabelKey: "conversations.unread_count", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "last_message_at", LabelKey: "conversations.last_message_at", Type: dt, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantDateRange, DefaultVisible: true},
			{Key: "created_at", LabelKey: "conversations.created_at", Type: dt, Sortable: true, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "status", LabelKey: "conversations.status", Variant: faceted},
			{Key: "ai_mode", LabelKey: "conversations.ai_mode", Variant: faceted},
			{Key: "identity_kind", LabelKey: "conversations.identity_kind", Variant: faceted},
			{Key: "assigned_user_uuid", LabelKey: "conversations.assigned_user", Variant: faceted},
			{Key: "channel", LabelKey: "conversations.channel", Variant: faceted},
			{Key: "last_message", LabelKey: "conversations.last_message_at", Variant: resourcemeta.FilterVariantDateRange},
			{Key: "unread", LabelKey: "conversations.unread", Variant: resourcemeta.FilterVariantBoolean},
		},
		Includes:    []string{},
		BulkActions: usecase.NewBulkAdapter(nil).BulkActions(),
	}
}

// Meta returns the list contract (GET /v1/conversations/meta).
func (h *Conversations) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, ConversationsMeta())
}

// List pages the inbox (GET /v1/conversations).
func (h *Conversations) List(w http.ResponseWriter, r *http.Request) {
	f, err := usecase.ParseConversationFilter(r.Context(), h.q, r.URL.Query())
	if err != nil {
		writeConversationError(w, r, err)
		return
	}
	items, total, err := h.inbox.List(r.Context(), viewer(r), f)
	if err != nil {
		writeConversationError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

func pathUUID(w http.ResponseWriter, r *http.Request, key string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(key))
	if err != nil {
		response.NotFound(w, r, "conversation not found")
		return uuid.Nil, false
	}
	return id, true
}

// Get returns a conversation (GET /v1/conversations/{uuid}).
func (h *Conversations) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.inbox.Get(r.Context(), viewer(r), id)
	if err != nil {
		writeConversationError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Messages pages the timeline (GET /v1/conversations/{uuid}/messages).
func (h *Conversations) Messages(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var before *repository.MessageCursor
	if raw := strings.TrimSpace(r.URL.Query().Get("before")); raw != "" {
		c, err := usecase.DecodeCursor(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "before", Message: "invalid cursor", Code: "invalid"}})
			return
		}
		before = &c
	}
	limit := int32(usecase.DefaultMessagePage)
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > usecase.MaxMessagePage {
			response.ValidationError(w, r, []response.Detail{{Field: "limit", Message: "must be 1-100", Code: "invalid"}})
			return
		}
		limit = int32(n)
	}
	out, err := h.inbox.Messages(r.Context(), viewer(r), id, before, limit)
	if err != nil {
		writeConversationError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Media streams a stored attachment
// (GET /v1/conversations/{uuid}/messages/{message_uuid}/media).
func (h *Conversations) Media(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	msgID, ok := pathUUID(w, r, "message_uuid")
	if !ok {
		return
	}
	m, err := h.inbox.Media(r.Context(), viewer(r), id, msgID)
	if err != nil {
		writeConversationError(w, r, err)
		return
	}
	defer func() { _ = m.Body.Close() }()
	ct := m.Mime
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": m.FileName}))
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if m.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(m.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, m.Body)
}

// outgoing reads a reply: JSON {body, user_uuid} or multipart/form-data
// with body, user_uuid and file (≤ 16 MB).
type outgoing struct {
	Body     string
	UserUUID string
	Media    *usecase.OutgoingMedia
}

func readOutgoing(w http.ResponseWriter, r *http.Request) (outgoing, bool) {
	var out outgoing
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if ct != "multipart/form-data" {
		var in struct {
			Body     string `json:"body"`
			UserUUID string `json:"user_uuid"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "invalid body")
			return out, false
		}
		out.Body, out.UserUUID = in.Body, in.UserUUID
		return out, true
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxReplyBytes)
	if err := r.ParseMultipartForm(maxReplyMemory); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			response.ValidationError(w, r, []response.Detail{{Field: replyFileField, Message: "file is larger than 16 MB", Code: "too_large"}})
			return out, false
		}
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart body")
		return out, false
	}
	out.Body = r.FormValue(replyBodyField)
	out.UserUUID = r.FormValue(startUserField)
	file, hdr, err := r.FormFile(replyFileField)
	if errors.Is(err, http.ErrMissingFile) {
		return out, true
	}
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid file")
		return out, false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, whatsapp.MaxMediaBytes+1))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid file")
		return out, false
	}
	if len(data) > whatsapp.MaxMediaBytes {
		response.ValidationError(w, r, []response.Detail{{Field: replyFileField, Message: "file is larger than 16 MB", Code: "too_large"}})
		return out, false
	}
	out.Media = &usecase.OutgoingMedia{Data: data, FileName: hdr.Filename}
	return out, true
}

// Reply queues a staff message (POST /v1/conversations/{uuid}/messages).
func (h *Conversations) Reply(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	in, ok := readOutgoing(w, r)
	if !ok {
		return
	}
	out, err := h.inbox.Reply(r.Context(), viewer(r), id, in.Body, in.Media)
	if err != nil {
		writeConversationError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// Start opens a conversation with a user and queues the first message
// (POST /v1/conversations).
func (h *Conversations) Start(w http.ResponseWriter, r *http.Request) {
	in, ok := readOutgoing(w, r)
	if !ok {
		return
	}
	userID, err := uuid.Parse(strings.TrimSpace(in.UserUUID))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: startUserField, Message: "user_uuid is required", Code: "required"}})
		return
	}
	out, err := h.inbox.Start(r.Context(), viewer(r), usecase.StartInput{UserUUID: userID, Body: in.Body, Media: in.Media})
	if err != nil {
		writeConversationError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// Patch changes status, AI mode and assignment
// (PATCH /v1/conversations/{uuid}).
func (h *Conversations) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&raw); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid body")
		return
	}
	var p usecase.PatchInput
	var details []response.Detail
	field := func(key string, dst any) bool {
		v, ok := raw[key]
		if !ok {
			return false
		}
		if err := json.Unmarshal(v, dst); err != nil {
			details = append(details, response.Detail{Field: key, Message: "invalid value", Code: "invalid"})
		}
		return true
	}
	field("status", &p.Status)
	field("ai_mode", &p.AIMode)
	field("ai_paused_until", &p.AIPausedUntil)
	p.AssignUser = field("assigned_user_uuid", &p.AssignedUser)
	p.AssignOrg = field("assigned_org_uuid", &p.AssignedOrg)
	for k := range raw {
		switch k {
		case "status", "ai_mode", "ai_paused_until", "assigned_user_uuid", "assigned_org_uuid":
		default:
			details = append(details, response.Detail{Field: k, Message: "unknown field", Code: "unknown"})
		}
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	if p.Status == nil && p.AIMode == nil && p.AIPausedUntil == nil && !p.AssignUser && !p.AssignOrg {
		response.BadRequest(w, r, response.CodeValidationError, "nothing to change")
		return
	}
	out, err := h.inbox.Patch(r.Context(), viewer(r), id, p)
	if err != nil {
		writeConversationError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Read clears the unread counter (POST /v1/conversations/{uuid}/read).
func (h *Conversations) Read(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.inbox.MarkRead(r.Context(), viewer(r), id)
	if err != nil {
		writeConversationError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func writeConversationError(w http.ResponseWriter, r *http.Request, err error) {
	if response.QueryValidation(w, r, err) {
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "conversation not found")
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "Forbidden")
	case errors.Is(err, usecase.ErrContactNoPhone):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeContactNoPhone, "the user has no phone number")
	case errors.Is(err, usecase.ErrAssigneeNotAllowed):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeAssigneeNotAllowed, "the assignee cannot read conversations")
	default:
		writeError(w, r, err)
	}
}
