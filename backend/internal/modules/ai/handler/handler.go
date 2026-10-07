// Package handler exposes the AI assistant chat (TEC-388, F4-01f): the
// conversations, the message stream and the action confirmations as
// server-sent events, the same for the panel (/v1/ai/...) and the portal
// (/v1/portal/ai/...).
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

const (
	// DefaultHeartbeat is the SSE keep-alive comment interval.
	DefaultHeartbeat = 15 * time.Second
	// StreamTimeout bounds one stream (the server write timeout is lifted
	// to it; the BFF allows 15 minutes too).
	StreamTimeout = 15 * time.Minute
	maxBodyBytes  = 64 << 10
)

// Handler serves one channel (panel or portal).
type Handler struct {
	chat      *usecase.Chat
	channel   string
	heartbeat time.Duration
}

// New creates the handler of a channel (model.ChannelPanel / ChannelPortal).
func New(chat *usecase.Chat, channel string) *Handler {
	return &Handler{chat: chat, channel: channel, heartbeat: DefaultHeartbeat}
}

// WithHeartbeat sets the keep-alive interval (tests).
func (h *Handler) WithHeartbeat(d time.Duration) *Handler {
	if d > 0 {
		h.heartbeat = d
	}
	return h
}

func (h *Handler) caller(r *http.Request) usecase.Caller {
	c := usecase.Caller{
		Auth: authctx.MustPrincipal(r.Context()), Channel: h.channel,
		AcceptLanguage: i18n.AcceptLanguage(r.Context()),
	}
	if h.channel == model.ChannelPanel {
		if s, ok := orgctx.ScopeFrom(r.Context()); ok {
			c.Org = &s
		}
	} else if b, ok := brandctx.From(r.Context()); ok {
		c.BrandID = b.ID
	}
	return c
}

// Status answers whether the assistant can be used and the quota left.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	out, err := h.chat.Status(r.Context(), h.caller(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// List lists the caller's conversations (list contract: sort
// updated_at | created_at | title, default -updated_at; q; created_from/_to).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	q := apiquery.Parse(values)
	created, err := apiquery.DateRange(values, "created")
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.chat.List(r.Context(), h.caller(r), usecase.ListFilter{
		Q: q.Q, Created: created, Sort: q.Sort, Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

type titleBody struct {
	Title *string `json:"title"`
}

// Create starts an empty conversation.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in titleBody
	if !decode(w, r, &in, true) {
		return
	}
	title := ""
	if in.Title != nil {
		title = *in.Title
	}
	out, err := h.chat.Create(r.Context(), h.caller(r), title)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// Get returns a conversation with its messages.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.chat.Get(r.Context(), h.caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Rename sets the title.
func (h *Handler) Rename(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in titleBody
	if !decode(w, r, &in, false) {
		return
	}
	if in.Title == nil {
		response.ValidationError(w, r, []response.Detail{{Field: "title", Message: "is required"}})
		return
	}
	out, err := h.chat.Rename(r.Context(), h.caller(r), id, *in.Title)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Delete soft-deletes a conversation.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if err := h.chat.Delete(r.Context(), h.caller(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type messageBody struct {
	Content string `json:"content"`
}

// SendMessage streams the answer to a new message as server-sent events:
// message_start, text_delta, tool_start, tool_result, confirm,
// quota_exceeded, error, message_done, title; ": ping" every 15 s.
func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in messageBody
	if !decode(w, r, &in, false) {
		return
	}
	turn, err := h.chat.PrepareMessage(r.Context(), h.caller(r), id, in.Content)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.stream(w, r, turn)
}

type confirmBody struct {
	Edits map[string]any `json:"edits"`
}

// ConfirmAction runs the caller's pending action (optionally with edited
// fields) and streams the continuation: action, then the turn events.
func (h *Handler) ConfirmAction(w http.ResponseWriter, r *http.Request) {
	h.action(w, r, true)
}

// CancelAction cancels the caller's pending action and streams the
// continuation like ConfirmAction.
func (h *Handler) CancelAction(w http.ResponseWriter, r *http.Request) {
	h.action(w, r, false)
}

func (h *Handler) action(w http.ResponseWriter, r *http.Request, confirm bool) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in confirmBody
	if confirm && !decode(w, r, &in, true) {
		return
	}
	turn, err := h.chat.PrepareAction(r.Context(), h.caller(r), id, confirm, in.Edits)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.stream(w, r, turn)
}

// sseWriter serializes events onto the response; after the first write
// error (client gone) it drops everything.
type sseWriter struct {
	mu  sync.Mutex
	w   http.ResponseWriter
	rc  *http.ResponseController
	err error
}

func (s *sseWriter) write(chunk string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return
	}
	if _, err := io.WriteString(s.w, chunk); err != nil {
		s.err = err
		return
	}
	if err := s.rc.Flush(); err != nil {
		s.err = err
	}
}

func (s *sseWriter) send(event string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		raw = []byte(`{}`)
	}
	s.write("event: " + event + "\ndata: " + string(raw) + "\n\n")
}

// stream opens the text/event-stream response with heartbeats and runs
// the turn on the request context: a client that disconnects cancels it.
func (h *Handler) stream(w http.ResponseWriter, r *http.Request, turn *usecase.Turn) {
	rc := http.NewResponseController(w)
	// Streams outlive the server's default write timeout.
	_ = rc.SetWriteDeadline(time.Now().Add(StreamTimeout))
	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream; charset=utf-8")
	hdr.Set("Cache-Control", "no-cache")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	sse := &sseWriter{w: w, rc: rc}
	sse.write(": stream open\n\n")

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(h.heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-r.Context().Done():
				return
			case <-ticker.C:
				sse.write(": ping\n\n")
			}
		}
	}()
	err := h.chat.Run(r.Context(), turn, sse.send)
	close(done)
	if err != nil {
		slog.ErrorContext(r.Context(), "ai_stream_failed", "err", err, "conversation", turn.ConversationUUID())
		sse.send(usecase.EventError, map[string]string{"code": "internal_error", "message": "unexpected error"})
	}
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Not found")
		return uuid.Nil, false
	}
	return id, true
}

// decode reads a JSON body; optional allows an empty body.
func decode(w http.ResponseWriter, r *http.Request, dst any, optional bool) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if optional && errors.Is(err, io.EOF) {
			return true
		}
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	if response.QueryValidation(w, r, err) {
		return
	}
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
		return
	}
	var ce *usecase.ConsentRequiredError
	if errors.As(err, &ce) {
		response.ErrorWithData(w, r, http.StatusPreconditionRequired, usecase.CodeConsentRequired,
			"Accept the AI assistant guidelines to continue", nil, map[string]any{"consent": ce.Text})
		return
	}
	var re *usecase.RateLimitedError
	if errors.As(err, &re) {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(re.RetryAfter.Seconds()))))
	}
	status, code, ok := usecase.ChatErrorStatus(err)
	if !ok {
		response.InternalErr(w, r, err, "ai chat request failed")
		return
	}
	response.Error(w, r, status, code, message(code, err))
}

func message(code string, err error) string {
	switch code {
	case "FEATURE_DISABLED":
		return "This feature is not enabled for your organization"
	case usecase.CodeQuotaExceeded:
		return "The monthly AI quota of your organization is used up"
	case usecase.CodeUnavailable:
		return "The AI assistant is not available right now"
	case usecase.CodeConversationLimit:
		return fmt.Sprintf("This conversation reached %d messages; start a new one", usecase.MaxConversationMessages)
	case "RATE_LIMITED":
		return "Too many messages; wait a moment"
	case "NOT_FOUND":
		return "Not found"
	case "FORBIDDEN":
		return "You are not allowed to use the AI assistant"
	}
	return err.Error()
}
