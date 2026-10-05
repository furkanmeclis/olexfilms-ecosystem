package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

const CodeReviewQuestionLocked = "REVIEW_QUESTION_HAS_ANSWERS"

func writeQuestionError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, svcuc.ErrReviewQuestionLocked) {
		response.Conflict(w, r, CodeReviewQuestionLocked, "The review question already has answers")
		return
	}
	writeError(w, r, err)
}

func (h *Handler) ListReviewQuestions(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListReviewQuestions(r.Context(), caller(r), r.URL.Query().Get("active") == "true")
	if err != nil {
		writeQuestionError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) CreateReviewQuestion(w http.ResponseWriter, r *http.Request) {
	var in svcuc.ReviewQuestionInput
	if !decode(w, r, &in) {
		return
	}
	item, err := h.svc.CreateReviewQuestion(r.Context(), caller(r), in)
	if err != nil {
		writeQuestionError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

func (h *Handler) UpdateReviewQuestion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in svcuc.ReviewQuestionInput
	if !decode(w, r, &in) {
		return
	}
	item, err := h.svc.UpdateReviewQuestion(r.Context(), caller(r), id, in)
	if err != nil {
		writeQuestionError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) PutReviewQuestionLocale(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in svcuc.ReviewQuestionLocaleInput
	if !decode(w, r, &in) {
		return
	}
	item, err := h.svc.PutReviewQuestionLocale(r.Context(), caller(r), id, r.PathValue("locale"), in)
	if err != nil {
		writeQuestionError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) ListServiceReviews(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	f, ok := reviewFilter(w, r, q.Limit, q.Offset)
	if !ok {
		return
	}
	items, total, err := h.svc.ListServiceReviews(r.Context(), caller(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *Handler) ReviewDealerStats(w http.ResponseWriter, r *http.Request) {
	from, to, ok := reviewDateBounds(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ReviewDealerStats(r.Context(), caller(r), from, to)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) ReviewProductStats(w http.ResponseWriter, r *http.Request) {
	from, to, ok := reviewDateBounds(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ReviewProductStats(r.Context(), caller(r), from, to)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func reviewFilter(w http.ResponseWriter, r *http.Request, limit, offset int32) (svcuc.ServiceReviewFilter, bool) {
	v := r.URL.Query()
	from, to, ok := reviewDateBounds(w, r)
	if !ok {
		return svcuc.ServiceReviewFilter{}, false
	}
	f := svcuc.ServiceReviewFilter{CreatedFrom: from, CreatedTo: to, Limit: limit, Offset: offset}
	if raw := v.Get("dealer_uuid"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, r, &svcuc.ValidationError{Field: "dealer_uuid", Message: "must be a UUID"})
			return f, false
		}
		f.DealerUUID = &id
	}
	if raw := v.Get("product_uuid"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, r, &svcuc.ValidationError{Field: "product_uuid", Message: "must be a UUID"})
			return f, false
		}
		f.ProductUUID = &id
	}
	for key, dst := range map[string]**int{"min_rating": &f.MinRating, "max_rating": &f.MaxRating} {
		raw := v.Get(key)
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, r, &svcuc.ValidationError{Field: key, Message: "must be an integer"})
			return f, false
		}
		*dst = &n
	}
	return f, true
}

func reviewDateBounds(w http.ResponseWriter, r *http.Request) (*time.Time, *time.Time, bool) {
	v := r.URL.Query()
	from, err := ParseCreatedBound(v.Get("created_from"), false)
	if err != nil {
		writeError(w, r, &svcuc.ValidationError{Field: "created_from", Message: "invalid date"})
		return nil, nil, false
	}
	to, err := ParseCreatedBound(v.Get("created_to"), true)
	if err != nil {
		writeError(w, r, &svcuc.ValidationError{Field: "created_to", Message: "invalid date"})
		return nil, nil, false
	}
	return from, to, true
}
