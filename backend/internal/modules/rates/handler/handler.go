// Package handler serves the exchange rate endpoints of TEC-84 (K7): the
// daily table, manual overrides, an on-demand fetch and the resolver.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Handler serves rate endpoints.
type Handler struct {
	svc      *fxrates.Service
	activity *activity.Recorder
}

// New creates the handler. rec may be nil.
func New(svc *fxrates.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, fxrates.ErrRateNotFound):
		response.Error(w, r, http.StatusNotFound, response.CodeRateNotFound, err.Error())
	case errors.Is(err, fxrates.ErrInvalid):
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, response.CodeValidationError, err.Error(), nil)
	default:
		response.InternalErr(w, r, err, "exchange rate request failed")
	}
}

func (h *Handler) record(r *http.Request, action string, payload map[string]any) {
	if h.activity == nil {
		return
	}
	var actor *int64
	if p, ok := authctx.PrincipalFrom(r.Context()); ok && p.UserInternal != 0 {
		id := p.UserInternal
		actor = &id
	}
	h.activity.Record(r.Context(), actor, action, "exchange_rates", nil, payload, r)
}

func actorID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func parseDate(w http.ResponseWriter, r *http.Request, raw string, required bool) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if required {
			response.BadRequest(w, r, response.CodeValidationError, "date is required (YYYY-MM-DD)")
			return time.Time{}, false
		}
		return time.Time{}, true
	}
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "date must be YYYY-MM-DD")
		return time.Time{}, false
	}
	return d, true
}

// Currencies lists the active currencies (GET /v1/currencies).
func (h *Handler) Currencies(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Currencies(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// List returns the stored rows of a day
// (GET /v1/platform/exchange-rates?date=YYYY-MM-DD&base=).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	on, ok := parseDate(w, r, r.URL.Query().Get("date"), false)
	if !ok {
		return
	}
	date, items, err := h.svc.DayRates(r.Context(), on, r.URL.Query().Get("base"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"date": date, "items": items})
}

// Resolve returns the rate a record would freeze
// (GET /v1/platform/exchange-rates/resolve?date&base&quote).
func (h *Handler) Resolve(w http.ResponseWriter, r *http.Request) {
	on, ok := parseDate(w, r, r.URL.Query().Get("date"), false)
	if !ok {
		return
	}
	if on.IsZero() {
		on = time.Now()
	}
	snap, err := h.svc.ResolveRate(r.Context(), on, r.URL.Query().Get("base"), r.URL.Query().Get("quote"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, snap)
}

type overrideBody struct {
	Date  string `json:"date"`
	Base  string `json:"base"`
	Quote string `json:"quote"`
	Rate  string `json:"rate"`
	Note  string `json:"note"`
}

// SetOverride stores a manual rate (PUT /v1/platform/exchange-rates/override).
func (h *Handler) SetOverride(w http.ResponseWriter, r *http.Request) {
	var in overrideBody
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	on, ok := parseDate(w, r, in.Date, true)
	if !ok {
		return
	}
	snap, err := h.svc.SetOverride(r.Context(), fxrates.OverrideInput{
		Date: on, Base: in.Base, Quote: in.Quote, Rate: in.Rate, Note: in.Note, ActorID: actorID(r),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "exchange_rates.override_set", map[string]any{
		"date": snap.RateDate, "base": snap.Base, "quote": snap.Quote, "rate": snap.Rate,
	})
	response.JSON(w, r, http.StatusOK, snap)
}

// ClearOverride removes a manual rate
// (DELETE /v1/platform/exchange-rates/override?date&base&quote).
func (h *Handler) ClearOverride(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	on, ok := parseDate(w, r, q.Get("date"), true)
	if !ok {
		return
	}
	if err := h.svc.ClearOverride(r.Context(), on, q.Get("base"), q.Get("quote")); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "exchange_rates.override_cleared", map[string]any{
		"date": on.Format("2006-01-02"), "base": strings.ToUpper(q.Get("base")), "quote": strings.ToUpper(q.Get("quote")),
	})
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// Fetch runs the TCMB/ECB fetch now (POST /v1/platform/exchange-rates/fetch).
// The report lists each source; a failing source does not fail the request.
func (h *Handler) Fetch(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	rep, err := h.svc.FetchLatest(ctx)
	payload := map[string]any{"sources": rep.Sources}
	if err != nil {
		payload["error"] = err.Error()
	}
	h.record(r, "exchange_rates.fetched", payload)
	response.JSON(w, r, http.StatusOK, rep)
}
