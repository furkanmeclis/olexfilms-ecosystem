package handler

// TEC-401 (F4-03b): GET /oauth/authorize (public, browser) and the /v1
// session endpoints: consent screen data and decision (panel and portal),
// connected apps, platform client list.

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"strconv"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// ConsentService is the consent / connected apps usecase.
type ConsentService interface {
	Authorize(ctx context.Context, ip string, in model.AuthorizeInput) (string, error)
	Consent(ctx context.Context, a usecase.Actor, id uuid.UUID) (model.Consent, error)
	Decide(ctx context.Context, a usecase.Actor, id uuid.UUID, in model.DecideInput) (model.Decision, error)
	ListGrants(ctx context.Context, userID int64, f usecase.ListFilter) ([]model.Grant, int64, error)
	RevokeGrant(ctx context.Context, a usecase.Actor, id uuid.UUID) error
	ListClients(ctx context.Context, f usecase.ListFilter) ([]model.ClientSummary, int64, error)
	RevokeClient(ctx context.Context, a usecase.Actor, id uuid.UUID) error
}

// Consent serves the authorize endpoint and the session endpoints.
type Consent struct {
	svc ConsentService
	h   *Handler
}

// NewConsent builds the consent handler.
func NewConsent(svc ConsentService, h *Handler) *Consent {
	return &Consent{svc: svc, h: h}
}

// Authorize serves GET /oauth/authorize: 302 to the consent screen (or to
// the client with an error once client and redirect_uri are verified); an
// unknown client, an unregistered redirect_uri or the rate limit renders an
// error page and never redirects.
func (c *Consent) Authorize(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	to, err := c.svc.Authorize(r.Context(), clientIP(r), model.AuthorizeInput{
		ClientID: v.Get("client_id"), RedirectURI: v.Get("redirect_uri"), ResponseType: v.Get("response_type"),
		CodeChallenge: v.Get("code_challenge"), CodeChallengeMethod: v.Get("code_challenge_method"),
		State: v.Get("state"), Scope: v.Get("scope"), Resource: v.Get("resource"),
	})
	if err != nil {
		c.errorPage(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, to, http.StatusFound)
}

func (c *Consent) errorPage(w http.ResponseWriter, r *http.Request, err error) {
	var oe *model.Error
	if !errors.As(err, &oe) {
		oe = &model.Error{Status: http.StatusInternalServerError, Code: model.ErrServerError, Description: "internal server error", Cause: err}
	}
	if oe.Status >= http.StatusInternalServerError {
		c.h.log.ErrorContext(r.Context(), "oauth_authorize_failed", "error", oe.Cause)
	}
	if oe.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(oe.RetryAfter.Seconds())+1))
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(oe.Status)
	_, _ = w.Write([]byte("<!doctype html><html><head><meta charset=\"utf-8\"><title>Authorization error</title></head><body>" +
		"<h1>Authorization error</h1><p><code>" + html.EscapeString(oe.Code) + "</code></p><p>" +
		html.EscapeString(oe.Description) + "</p></body></html>"))
}

// actor builds the usecase actor of a /v1 session request.
func actor(r *http.Request) usecase.Actor {
	p := authctx.MustPrincipal(r.Context())
	a := usecase.Actor{
		UserID: p.UserInternal, Realm: p.Realm, Roles: p.Roles, IsSuperAdmin: p.IsSuperAdmin,
		Meta: activity.MetaFromRequest(r),
	}
	if b, ok := brandctx.From(r.Context()); ok {
		a.BrandID = b.ID
	}
	return a
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Not found")
		return uuid.Nil, false
	}
	return id, true
}

func (c *Consent) writeError(w http.ResponseWriter, r *http.Request, err error) {
	if response.QueryValidation(w, r, err) {
		return
	}
	var fe *usecase.ForbiddenError
	switch {
	case errors.As(err, &fe):
		response.Forbidden(w, r, fe.Message)
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Not found")
	default:
		response.InternalErr(w, r, err, "OAuth request failed")
	}
}

// ConsentInfo serves GET /v1/oauth/requests/{uuid} (and the portal twin).
func (c *Consent) ConsentInfo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := c.svc.Consent(r.Context(), actor(r), id)
	if err != nil {
		c.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Decide serves POST /v1/oauth/requests/{uuid}/decide (and the portal twin).
func (c *Consent) Decide(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.DecideInput
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	out, err := c.svc.Decide(r.Context(), actor(r), id, in)
	if err != nil {
		c.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListGrants serves GET /v1/oauth/grants (and the portal twin).
func (c *Consent) ListGrants(w http.ResponseWriter, r *http.Request) {
	f, err := usecase.ParseGrantListFilter(r.URL.Query())
	if err != nil {
		c.writeError(w, r, err)
		return
	}
	items, total, err := c.svc.ListGrants(r.Context(), authctx.MustPrincipal(r.Context()).UserInternal, f)
	if err != nil {
		c.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// RevokeGrant serves DELETE /v1/oauth/grants/{uuid} (and the portal twin).
func (c *Consent) RevokeGrant(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if err := c.svc.RevokeGrant(r.Context(), actor(r), id); err != nil {
		c.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListClients serves GET /v1/platform/oauth/clients.
func (c *Consent) ListClients(w http.ResponseWriter, r *http.Request) {
	f, err := usecase.ParseClientListFilter(r.URL.Query())
	if err != nil {
		c.writeError(w, r, err)
		return
	}
	items, total, err := c.svc.ListClients(r.Context(), f)
	if err != nil {
		c.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// RevokeClient serves DELETE /v1/platform/oauth/clients/{uuid}.
func (c *Consent) RevokeClient(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if err := c.svc.RevokeClient(r.Context(), actor(r), id); err != nil {
		c.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
