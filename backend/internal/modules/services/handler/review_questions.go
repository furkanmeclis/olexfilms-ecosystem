package handler

import (
	"errors"
	"net/http"

	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
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
	items, total, err := h.svc.ListServiceReviews(r.Context(), caller(r), q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items, "total": total})
}
