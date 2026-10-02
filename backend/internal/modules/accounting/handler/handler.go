// Package handler serves the /v1/accounting endpoints (TEC-172, F1-07b).
package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting"
	acc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes specific to accounting.
const (
	CodeEntryNotVoidable   = "ENTRY_NOT_VOIDABLE"
	CodeIdempotencyReused  = "IDEMPOTENCY_KEY_REUSED"
	CodeRateNotFound       = response.CodeRateNotFound
	CodeCounterpartyAbsent = "COUNTERPARTY_NOT_FOUND"
	CodeOpeningExists      = "OPENING_BALANCE_EXISTS"
)

// Handler serves accounting endpoints.
type Handler struct {
	svc     *acc.Service
	exports Exports
}

// New creates the handler.
func New(svc *acc.Service) *Handler { return &Handler{svc: svc} }

func caller(r *http.Request) acc.Caller {
	p := authctx.MustPrincipal(r.Context())
	f, _ := scopefilter.From(r.Context())
	return acc.Caller{UserID: p.UserInternal, Org: orgctx.MustScope(r.Context()), Filter: f}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *acc.ValidationError
	switch {
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.Is(err, acc.ErrForbidden):
		response.Forbidden(w, r, "This organization cannot write accounting entries")
	case errors.Is(err, acc.ErrBookNotFound):
		response.NotFound(w, r, "Organization not found")
	case errors.Is(err, acc.ErrAccountNotFound):
		response.NotFound(w, r, "Account not found")
	case errors.Is(err, acc.ErrCariNotFound):
		response.NotFound(w, r, "Cari account not found")
	case errors.Is(err, acc.ErrEntryNotFound):
		response.NotFound(w, r, "Entry not found")
	case errors.Is(err, acc.ErrCounterparty):
		response.Error(w, r, http.StatusNotFound, CodeCounterpartyAbsent,
			"Counterparty must be the parent or a direct child organization of the same brand")
	case errors.Is(err, acc.ErrNotVoidable):
		response.Conflict(w, r, CodeEntryNotVoidable, "Only an open manual entry or opening balance can be voided")
	case errors.Is(err, acc.ErrOpeningBalanceExists):
		response.Conflict(w, r, CodeOpeningExists, "An opening balance is already booked; reverse it first")
	case errors.Is(err, acc.ErrIdempotencyConflict):
		response.Conflict(w, r, CodeIdempotencyReused, "The idempotency key was used for a different entry")
	case errors.Is(err, acc.ErrRateNotFound):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeRateNotFound, "No exchange rate for this currency")
	default:
		response.InternalErr(w, r, err, "accounting request failed")
	}
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.NotFound(w, r, "Resource not found")
		return uuid.Nil, false
	}
	return id, true
}

func queryUUID(w http.ResponseWriter, r *http.Request, name string) (*uuid.UUID, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: "must be a UUID"}})
		return nil, false
	}
	return &id, true
}

func queryBool(w http.ResponseWriter, r *http.Request, name string) (*bool, bool) {
	switch strings.TrimSpace(r.URL.Query().Get(name)) {
	case "":
		return nil, true
	case "true":
		v := true
		return &v, true
	case "false":
		v := false
		return &v, true
	default:
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: "must be true or false"}})
		return nil, false
	}
}

func queryDate(w http.ResponseWriter, r *http.Request, name string) (*time.Time, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return nil, true
	}
	d, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: "must be a date (YYYY-MM-DD)"}})
		return nil, false
	}
	return &d, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

// --- Categories --------------------------------------------------------------

type categoryView struct {
	accounting.Category
	Label string `json:"label"`
}

// ListCategories returns the F1 category catalog with labels in the request
// language (GET /v1/accounting/categories?direction).
func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	dir := strings.TrimSpace(r.URL.Query().Get("direction"))
	loc := i18n.FromContext(r.Context()).Locale
	items := []categoryView{}
	for _, c := range accounting.Categories() {
		if dir != "" && c.Direction != dir {
			continue
		}
		items = append(items, categoryView{Category: c, Label: i18n.Translate(loc, c.LabelKey)})
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// --- Accounts ----------------------------------------------------------------

// ListAccounts (GET /v1/accounting/accounts?organization_uuid&active&type).
func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	org, ok := queryUUID(w, r, "organization_uuid")
	if !ok {
		return
	}
	active, ok := queryBool(w, r, "active")
	if !ok {
		return
	}
	items, err := h.svc.ListAccounts(r.Context(), caller(r), acc.AccountFilter{
		OrganizationUUID: org, Active: active, Type: strings.TrimSpace(r.URL.Query().Get("type")),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// GetAccount (GET /v1/accounting/accounts/{uuid}?organization_uuid).
func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	org, ok := queryUUID(w, r, "organization_uuid")
	if !ok {
		return
	}
	a, err := h.svc.GetAccount(r.Context(), caller(r), org, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, a)
}

type createAccountBody struct {
	Type string  `json:"type"`
	Name string  `json:"name"`
	IBAN *string `json:"iban"`
}

// CreateAccount (POST /v1/accounting/accounts).
func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var b createAccountBody
	if !decode(w, r, &b) {
		return
	}
	a, err := h.svc.CreateAccount(r.Context(), caller(r), acc.CreateAccountInput{
		Type: strings.TrimSpace(b.Type), Name: b.Name, IBAN: b.IBAN,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, a)
}

type updateAccountBody struct {
	Name   *string         `json:"name"`
	IBAN   json.RawMessage `json:"iban"`
	Active *bool           `json:"active"`
}

// UpdateAccount (PATCH /v1/accounting/accounts/{uuid}): absent fields keep
// the stored value, "iban": null clears it, "active": false deactivates.
func (h *Handler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b updateAccountBody
	if !decode(w, r, &b) {
		return
	}
	in := acc.UpdateAccountInput{Name: b.Name, Active: b.Active}
	if len(b.IBAN) > 0 {
		if bytes.Equal(bytes.TrimSpace(b.IBAN), []byte("null")) {
			in.ClearIBAN = true
		} else {
			var s string
			if err := json.Unmarshal(b.IBAN, &s); err != nil {
				response.ValidationError(w, r, []response.Detail{{Field: "iban", Message: "must be a string or null"}})
				return
			}
			if strings.TrimSpace(s) == "" {
				in.ClearIBAN = true
			} else {
				in.IBAN = &s
			}
		}
	}
	a, err := h.svc.UpdateAccount(r.Context(), caller(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, a)
}

// --- Cari --------------------------------------------------------------------

// ListCari (GET /v1/accounting/cari?organization_uuid&active&q&limit&offset).
func (h *Handler) ListCari(w http.ResponseWriter, r *http.Request) {
	org, ok := queryUUID(w, r, "organization_uuid")
	if !ok {
		return
	}
	active, ok := queryBool(w, r, "active")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.ListCari(r.Context(), caller(r), acc.CariFilter{
		OrganizationUUID: org, Active: active, Q: q.Q, Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetCari (GET /v1/accounting/cari/{uuid}?organization_uuid).
func (h *Handler) GetCari(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	org, ok := queryUUID(w, r, "organization_uuid")
	if !ok {
		return
	}
	c, err := h.svc.GetCari(r.Context(), caller(r), org, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, c)
}

// --- Entries -----------------------------------------------------------------

// ListEntries (GET /v1/accounting/entries?organization_uuid&account_uuid&
// cari_uuid&direction&category&source_type&date_from&date_to&limit&offset).
func (h *Handler) ListEntries(w http.ResponseWriter, r *http.Request) {
	f := acc.EntryFilter{}
	var ok bool
	if f.OrganizationUUID, ok = queryUUID(w, r, "organization_uuid"); !ok {
		return
	}
	if f.AccountUUID, ok = queryUUID(w, r, "account_uuid"); !ok {
		return
	}
	if f.CariUUID, ok = queryUUID(w, r, "cari_uuid"); !ok {
		return
	}
	if f.From, ok = queryDate(w, r, "date_from"); !ok {
		return
	}
	if f.To, ok = queryDate(w, r, "date_to"); !ok {
		return
	}
	v := r.URL.Query()
	f.Direction = strings.TrimSpace(v.Get("direction"))
	f.Category = strings.TrimSpace(v.Get("category"))
	f.SourceType = strings.TrimSpace(v.Get("source_type"))
	q := apiquery.Parse(v)
	f.Limit, f.Offset = q.Limit, q.Offset
	items, total, err := h.svc.ListEntries(r.Context(), caller(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetEntry (GET /v1/accounting/entries/{uuid}?organization_uuid).
func (h *Handler) GetEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	org, ok := queryUUID(w, r, "organization_uuid")
	if !ok {
		return
	}
	e, err := h.svc.GetEntry(r.Context(), caller(r), org, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, e)
}

type entryBody struct {
	Direction       string     `json:"direction"`
	Category        string     `json:"category"`
	Amount          string     `json:"amount"`
	Currency        string     `json:"currency"`
	AccountUUID     *uuid.UUID `json:"account_uuid"`
	CariUUID        *uuid.UUID `json:"cari_uuid"`
	CounterpartyOrg *uuid.UUID `json:"counterparty_organization_uuid"`
	Description     string     `json:"description"`
	IdempotencyKey  *uuid.UUID `json:"idempotency_key"`
}

func written(w http.ResponseWriter, r *http.Request, e acc.Entry, replayed bool, err error) {
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	response.JSON(w, r, status, e)
}

// CreateEntry books a manual income, expense or charge
// (POST /v1/accounting/entries). A repeated idempotency_key answers 200 with
// the earlier entry.
func (h *Handler) CreateEntry(w http.ResponseWriter, r *http.Request) {
	var b entryBody
	if !decode(w, r, &b) {
		return
	}
	e, replayed, err := h.svc.CreateEntry(r.Context(), caller(r), acc.EntryInput{
		Direction: strings.TrimSpace(b.Direction), Category: strings.TrimSpace(b.Category),
		Amount: b.Amount, Currency: b.Currency, AccountUUID: b.AccountUUID, CariUUID: b.CariUUID,
		CounterpartyOrg: b.CounterpartyOrg, Description: b.Description, IdempotencyKey: b.IdempotencyKey,
	})
	written(w, r, e, replayed, err)
}

type settlementBody struct {
	AccountUUID     *uuid.UUID `json:"account_uuid"`
	CariUUID        *uuid.UUID `json:"cari_uuid"`
	CounterpartyOrg *uuid.UUID `json:"counterparty_organization_uuid"`
	Amount          string     `json:"amount"`
	Currency        string     `json:"currency"`
	Description     string     `json:"description"`
	IdempotencyKey  *uuid.UUID `json:"idempotency_key"`
}

func (h *Handler) settle(w http.ResponseWriter, r *http.Request, direction string) {
	var b settlementBody
	if !decode(w, r, &b) {
		return
	}
	e, replayed, err := h.svc.Settle(r.Context(), caller(r), direction, acc.SettlementInput{
		AccountUUID: b.AccountUUID, CariUUID: b.CariUUID, CounterpartyOrg: b.CounterpartyOrg,
		Amount: b.Amount, Currency: b.Currency, Description: b.Description, IdempotencyKey: b.IdempotencyKey,
	})
	written(w, r, e, replayed, err)
}

// CreateCollection (POST /v1/accounting/collections): the counterparty paid.
func (h *Handler) CreateCollection(w http.ResponseWriter, r *http.Request) {
	h.settle(w, r, accounting.DirectionCollection)
}

// CreatePayment (POST /v1/accounting/payments): we paid the counterparty.
func (h *Handler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	h.settle(w, r, accounting.DirectionPayment)
}

type voidBody struct {
	Reason string `json:"reason"`
}

// VoidEntry reverses a manual entry (POST /v1/accounting/entries/{uuid}/void,
// step-up) and returns the reversal row.
func (h *Handler) VoidEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b voidBody
	if !decode(w, r, &b) {
		return
	}
	e, err := h.svc.Void(r.Context(), caller(r), id, b.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, e)
}

type openingBody struct {
	CariUUID        *uuid.UUID `json:"cari_uuid"`
	CounterpartyOrg *uuid.UUID `json:"counterparty_organization_uuid"`
	Side            string     `json:"side"`
	Amount          string     `json:"amount"`
	Currency        string     `json:"currency"`
	OpeningDate     string     `json:"opening_date"`
	Description     string     `json:"description"`
}

// CreateOpeningBalance books the one-off opening balance of a cari
// (POST /v1/accounting/opening-balances, step-up, TEC-177). The same
// opening balance again answers 200 with the earlier entry; other values
// while one is open answer 409 OPENING_BALANCE_EXISTS.
func (h *Handler) CreateOpeningBalance(w http.ResponseWriter, r *http.Request) {
	var b openingBody
	if !decode(w, r, &b) {
		return
	}
	e, replayed, err := h.svc.PostOpeningBalance(r.Context(), caller(r), acc.OpeningBalanceInput{
		CariUUID: b.CariUUID, CounterpartyOrg: b.CounterpartyOrg, Side: b.Side, Amount: b.Amount,
		Currency: b.Currency, Date: b.OpeningDate, Description: b.Description,
	})
	written(w, r, e, replayed, err)
}

type accountOpeningBody struct {
	Amount      string `json:"amount"`
	OpeningDate string `json:"opening_date"`
	Description string `json:"description"`
}

// CreateAccountOpening books the one-off opening balance of a cash/bank
// account (POST /v1/accounting/accounts/{uuid}/opening-balance, step-up,
// TEC-198). It moves the account balance only (no cari, no P&L). The same
// opening balance again answers 200 with the earlier entry; other values
// while one is open answer 409 OPENING_BALANCE_EXISTS.
func (h *Handler) CreateAccountOpening(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b accountOpeningBody
	if !decode(w, r, &b) {
		return
	}
	e, replayed, err := h.svc.PostAccountOpening(r.Context(), caller(r), id, acc.AccountOpeningInput{
		Amount: b.Amount, Date: b.OpeningDate, Description: b.Description,
	})
	written(w, r, e, replayed, err)
}
