package handler

// TEC-193: admin customer merge endpoints.

import (
	"errors"
	"net/http"

	cu "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Merge refusal codes (409).
const (
	CodeMergeSelf            = "CUSTOMER_MERGE_SELF"
	CodeAlreadyMerged        = "CUSTOMER_ALREADY_MERGED"
	CodeMergeAnonymized      = "CUSTOMER_MERGE_ANONYMIZED"
	CodeMergePendingTransfer = "CUSTOMER_MERGE_PENDING_TRANSFER"
)

type mergeBody struct {
	TargetUUID string `json:"target_uuid"`
}

func mergeTarget(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	source, ok := pathUUID(w, r, "uuid")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	var b mergeBody
	if !decode(w, r, &b) {
		return uuid.Nil, uuid.Nil, false
	}
	target, err := uuid.Parse(b.TargetUUID)
	if err != nil {
		writeError(w, r, &cu.ValidationError{Field: "target_uuid", Message: "must be a UUID"})
		return uuid.Nil, uuid.Nil, false
	}
	return source, target, true
}

// PreviewMerge (POST /v1/customers/{uuid}/merge/preview): dry run, writes
// nothing and reports the records that would move to target_uuid.
func (h *Handler) PreviewMerge(w http.ResponseWriter, r *http.Request) {
	source, target, ok := mergeTarget(w, r)
	if !ok {
		return
	}
	res, err := h.svc.PreviewMerge(r.Context(), caller(r), source, target)
	if err != nil {
		writeMergeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

// MergeCustomer (POST /v1/customers/{uuid}/merge): merges the customer
// (source) into target_uuid in one transaction; irreversible.
func (h *Handler) MergeCustomer(w http.ResponseWriter, r *http.Request) {
	source, target, ok := mergeTarget(w, r)
	if !ok {
		return
	}
	res, err := h.svc.MergeCustomer(r.Context(), caller(r), source, target, activity.MetaFromRequest(r))
	if err != nil {
		writeMergeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

func writeMergeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, cu.ErrMergeSelf):
		response.Conflict(w, r, CodeMergeSelf, "A customer cannot be merged into itself")
	case errors.Is(err, cu.ErrMergeChain):
		response.Conflict(w, r, CodeAlreadyMerged, "The customer was already merged into another customer")
	case errors.Is(err, cu.ErrMergeAnonymized):
		response.Conflict(w, r, CodeMergeAnonymized, "Anonymized customers cannot be merged")
	case errors.Is(err, cu.ErrMergePendingTransfer):
		response.Conflict(w, r, CodeMergePendingTransfer, "The customer has a pending vehicle transfer")
	default:
		writePrivacyError(w, r, err)
	}
}
