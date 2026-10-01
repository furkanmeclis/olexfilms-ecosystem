// Package handler exposes portal consents (/v1/portal/consents) and the admin
// legal text editor (/v1/platform/legal-texts/{kind}).
package handler

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legal/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// CodeStaleLegalText: the answered text version is no longer current.
const CodeStaleLegalText = "LEGAL_TEXT_STALE"

const maxRequestBytes = usecase.MaxBodyBytes + 4<<10

// Handler serves the legal routes.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New creates the handler.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func requestLocale(r *http.Request) i18n.Locale {
	if l, ok := i18n.Parse(r.URL.Query().Get("locale")); ok {
		return l
	}
	return i18n.FromContext(r.Context()).Locale
}

// PendingConsents lists the texts the portal user still has to answer.
func (h *Handler) PendingConsents(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	items, err := h.svc.Pending(r.Context(), p.UserInternal, requestLocale(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type decideBody struct {
	Kind     string `json:"kind"`
	Locale   string `json:"locale"`
	Version  int32  `json:"version"`
	Accepted *bool  `json:"accepted"`
}

// DecideConsent records accept / decline for the current text version.
func (h *Handler) DecideConsent(w http.ResponseWriter, r *http.Request) {
	var in decideBody
	if !decode(w, r, &in) {
		return
	}
	if in.Accepted == nil {
		response.BadRequest(w, r, response.CodeValidationError, "accepted is required")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	c, err := h.svc.Decide(r.Context(), p.UserInternal, usecase.DecideInput{
		Kind: strings.TrimSpace(in.Kind), Locale: in.Locale, Version: in.Version, Accepted: *in.Accepted,
		IP: clientIP(r), UserAgent: r.UserAgent(),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, c)
}

// AdminGet returns the latest texts per locale and the version history.
func (h *Handler) AdminGet(w http.ResponseWriter, r *http.Request) {
	view, err := h.svc.AdminGet(r.Context(), r.PathValue("kind"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, view)
}

type publishBody struct {
	Locale string `json:"locale"`
	Body   string `json:"body"`
}

// AdminPut publishes a new version of one locale's text.
func (h *Handler) AdminPut(w http.ResponseWriter, r *http.Request) {
	var in publishBody
	if !decode(w, r, &in) {
		return
	}
	kind := r.PathValue("kind")
	actor := actorID(r)
	t, created, err := h.svc.AdminPublish(r.Context(), kind, in.Locale, in.Body, actor)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if created && h.activity != nil {
		h.activity.Record(r.Context(), actor, "legal_texts.published", "platform.legal_texts", nil,
			map[string]any{"kind": t.Kind, "locale": t.Locale, "version": t.Version}, r)
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"text": t, "created": created})
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return false
	}
	return true
}

func actorID(r *http.Request) *int64 {
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok {
		return nil
	}
	id := p.UserInternal
	return &id
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Legal text was not found")
	case errors.Is(err, usecase.ErrStaleVersion):
		response.Conflict(w, r, CodeStaleLegalText, "The text was updated. Reload and answer the current version.")
	case errors.Is(err, usecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "legal text request failed")
	}
}
