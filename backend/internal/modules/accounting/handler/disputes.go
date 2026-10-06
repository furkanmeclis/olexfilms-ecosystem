package handler

import (
	"errors"
	"net/http"
	"strings"

	acc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the dispute endpoints (TEC-174).
const (
	CodeEntryNotDisputable    = "ENTRY_NOT_DISPUTABLE"
	CodeDisputeAlreadyOpen    = "DISPUTE_ALREADY_OPEN"
	CodeDisputeNotOpen        = "DISPUTE_NOT_OPEN"
	CodeDisputeNotResolvable  = "DISPUTE_NOT_RESOLVABLE"
	CodeDisputeSaleReturned   = "DISPUTE_SALE_RETURNED"
	disputeNotFoundMessage    = "Dispute not found"
	entryNotDisputableMessage = "Only an open entry the parent organization posted to this cari can be disputed"
)

// writeDisputeError maps the dispute errors and falls back to writeError.
func writeDisputeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, acc.ErrDisputeNotFound):
		response.NotFound(w, r, disputeNotFoundMessage)
	case errors.Is(err, acc.ErrNotDisputable):
		response.Conflict(w, r, CodeEntryNotDisputable, entryNotDisputableMessage)
	case errors.Is(err, acc.ErrDisputeAlreadyOpen):
		response.Conflict(w, r, CodeDisputeAlreadyOpen, "The entry already has an open dispute")
	case errors.Is(err, acc.ErrDisputeClosed):
		response.Conflict(w, r, CodeDisputeNotOpen, "The dispute is already resolved or rejected")
	case errors.Is(err, acc.ErrDisputeNotResolvable):
		response.Conflict(w, r, CodeDisputeNotResolvable, "The disputed entry is no longer open; reject or reverse the dispute")
	case errors.Is(err, acc.ErrDisputeSaleReturned):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeDisputeSaleReturned,
			"A return of this sale was already received; revise or reject the dispute instead of reversing it")
	default:
		writeError(w, r, err)
	}
}

// ListDisputes (GET /v1/accounting/disputes?status&organization_uuid&
// counterparty_organization_uuid&created_from&created_to&q&sort&limit&offset):
// disputes the accounting.read scope reaches (the disputing organization and
// the parent it addresses; brand scope: the whole domain brand). status and
// the organization filters are CSV lists (TEC-379).
func (h *Handler) ListDisputes(w http.ResponseWriter, r *http.Request) {
	f, err := acc.ParseDisputeFilter(r.URL.Query())
	if err != nil {
		writeDisputeError(w, r, err)
		return
	}
	items, total, err := h.svc.ListDisputes(r.Context(), caller(r), f)
	if err != nil {
		writeDisputeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// GetDispute (GET /v1/accounting/disputes/{uuid}).
func (h *Handler) GetDispute(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	d, err := h.svc.GetDispute(r.Context(), caller(r), id)
	if err != nil {
		writeDisputeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

type disputeBody struct {
	EntryUUID uuid.UUID `json:"entry_uuid"`
	Reason    string    `json:"reason"`
}

// OpenDispute (POST /v1/accounting/disputes, accounting.dispute): the active
// organization disputes a row its parent posted to its ledger (K24).
func (h *Handler) OpenDispute(w http.ResponseWriter, r *http.Request) {
	var b disputeBody
	if !decode(w, r, &b) {
		return
	}
	if b.EntryUUID == uuid.Nil {
		response.ValidationError(w, r, []response.Detail{{Field: "entry_uuid", Message: "is required"}})
		return
	}
	d, err := h.svc.OpenDispute(r.Context(), caller(r), acc.DisputeInput{EntryUUID: b.EntryUUID, Reason: b.Reason})
	if err != nil {
		writeDisputeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, d)
}

type resolveBody struct {
	Resolution      string `json:"resolution"`
	Note            string `json:"note"`
	CorrectedAmount string `json:"corrected_amount"`
}

// ResolveDispute (POST /v1/accounting/disputes/{uuid}/resolve,
// accounting.resolve): the parent reverses, revises or rejects.
func (h *Handler) ResolveDispute(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b resolveBody
	if !decode(w, r, &b) {
		return
	}
	d, err := h.svc.ResolveDispute(r.Context(), caller(r), id, acc.ResolveInput{
		Resolution: strings.TrimSpace(b.Resolution), Note: b.Note, CorrectedAmount: b.CorrectedAmount,
	})
	if err != nil {
		writeDisputeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}
