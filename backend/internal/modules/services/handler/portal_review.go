package handler

import (
	"errors"
	"net/http"

	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// TEC-244: the portal service review form.

// Error codes of the portal service review.
const (
	CodeAlreadyReviewed    = "SERVICE_ALREADY_REVIEWED"
	CodeReviewNotCompleted = "SERVICE_REVIEW_NOT_COMPLETED"
)

func writeReviewError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, svcuc.ErrAlreadyReviewed):
		response.Conflict(w, r, CodeAlreadyReviewed, "The service has already been reviewed")
	case errors.Is(err, svcuc.ErrReviewNotCompleted):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeReviewNotCompleted,
			"Only a completed service can be reviewed")
	default:
		writeError(w, r, err)
	}
}

// GetReview (GET /v1/portal/services/{uuid}/review): the review state of a
// service the user owns (review or null, can_review, google_business_url).
func (h *PortalDetail) GetReview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	v, err := h.svc.PortalGetReview(r.Context(), portalBrandID(r), p.UserInternal, id)
	if err != nil {
		writeReviewError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// CreateReview (POST /v1/portal/services/{uuid}/review): one review per
// service; 409 SERVICE_ALREADY_REVIEWED on a second one.
func (h *PortalDetail) CreateReview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in svcuc.ServiceReviewInput
	if !decode(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	v, err := h.svc.PortalCreateReview(r.Context(), portalBrandID(r), p.UserInternal, id, in)
	if err != nil {
		writeReviewError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}
