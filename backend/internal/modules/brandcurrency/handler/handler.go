// Package handler serves the brand currency endpoints (TEC-1214).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/brandcurrency/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// CodeCurrencyChangeBlocked: the change has blockers (details list them).
const CodeCurrencyChangeBlocked = "CURRENCY_CHANGE_BLOCKED"

// Handler serves the brand currency endpoints.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New creates the handler; rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// List returns the brands with their currency (GET /v1/platform/brands).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, local, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items, "local_currency": local})
}

type changeBody struct {
	Currency           string `json:"currency"`
	DryRun             bool   `json:"dry_run"`
	OpenOrders         string `json:"open_orders"`
	AllowMissingPrices bool   `json:"allow_missing_prices"`
}

// ChangeCurrency previews or applies a brand currency change
// (PUT /v1/platform/brands/{uuid}/currency).
func (h *Handler) ChangeCurrency(w http.ResponseWriter, r *http.Request) {
	id, ok := brandUUID(w, r)
	if !ok {
		return
	}
	var body changeBody
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.ChangeCurrency(r.Context(), id, usecase.ChangeInput{
		Currency: body.Currency, DryRun: body.DryRun, OpenOrders: body.OpenOrders, AllowMissingPrices: body.AllowMissingPrices,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	if out.Applied {
		h.record(r, "brands.currency_changed", id, map[string]any{
			"from": out.From, "to": out.To, "open_orders": out.OpenOrdersOption, "repriced_orders": out.RepricedOrders,
			"missing_price_count": out.MissingPriceCount, "rate_to_local": out.RateToLocal, "conversion_rate": out.ConversionRate,
		})
	}
	response.JSON(w, r, http.StatusOK, out)
}

type convertBody struct {
	SourceCurrency string `json:"source_currency"`
	TargetCurrency string `json:"target_currency"`
	DryRun         bool   `json:"dry_run"`
	Overwrite      bool   `json:"overwrite"`
}

// ConvertPrices previews or writes the converted price list
// (POST /v1/platform/brands/{uuid}/price-conversion).
func (h *Handler) ConvertPrices(w http.ResponseWriter, r *http.Request) {
	id, ok := brandUUID(w, r)
	if !ok {
		return
	}
	var body convertBody
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.ConvertPrices(r.Context(), id, usecase.ConvertInput{
		SourceCurrency: body.SourceCurrency, TargetCurrency: body.TargetCurrency, DryRun: body.DryRun, Overwrite: body.Overwrite,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	if out.Applied {
		h.record(r, "brands.prices_converted", id, map[string]any{
			"source": out.Source, "target": out.Target, "rate": out.Rate, "overwrite": out.Overwrite,
			"created": out.Create, "updated": out.Update, "skipped": out.Skip,
		})
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) record(r *http.Request, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	var actor *int64
	if p, ok := authctx.PrincipalFrom(r.Context()); ok && p.UserInternal != 0 {
		v := p.UserInternal
		actor = &v
	}
	h.activity.Record(r.Context(), actor, action, "brands", &id, payload, r)
}

func brandUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue("uuid")))
	if err != nil {
		response.NotFound(w, r, "Brand was not found")
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var blocked *usecase.BlockedError
	switch {
	case errors.As(err, &blocked):
		details := make([]response.Detail, 0, len(blocked.Blockers))
		for _, b := range blocked.Blockers {
			details = append(details, response.Detail{Field: "currency", Code: b, Message: b})
		}
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeCurrencyChangeBlocked, err.Error(), details)
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Brand was not found")
	case errors.Is(err, usecase.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, usecase.ErrRateNotFound):
		response.Error(w, r, http.StatusUnprocessableEntity, response.CodeRateNotFound, "No exchange rate for the conversion")
	default:
		response.InternalErr(w, r, err, "Brand currency request failed")
	}
}
