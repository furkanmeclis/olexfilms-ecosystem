package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// PDFRequester is the measurement PDF use case (TEC-298).
type PDFRequester interface {
	RequestPDF(ctx context.Context, c usecase.PanelCaller, id uuid.UUID) (usecase.PDFFile, error)
}

// PDF serves GET /v1/measurements/{uuid}/pdf.
type PDF struct{ svc PDFRequester }

// NewPDF creates the PDF handler.
func NewPDF(svc PDFRequester) *PDF { return &PDF{svc: svc} }

// PDFPending is the 202 answer while worker-docs renders the PDF.
type PDFPending struct {
	UUID   uuid.UUID `json:"uuid"`
	Status string    `json:"status"`
}

// Get streams the stored PDF (200 application/pdf) or answers 202
// {uuid, status: "pending"} after queueing the first render; the client
// polls the same URL.
func (h *PDF) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "Measurement not found")
	if !ok {
		return
	}
	f, err := h.svc.RequestPDF(r.Context(), panelCaller(r), id)
	switch {
	case errors.Is(err, usecase.ErrPDFVINPending):
		response.Error(w, r, http.StatusUnprocessableEntity, "MEASUREMENT_VIN_PENDING", "The measurement needs a VIN before its PDF can be generated")
		return
	case errors.Is(err, usecase.ErrPDFNotNormalized):
		response.Error(w, r, http.StatusUnprocessableEntity, "MEASUREMENT_NOT_NORMALIZED", "The measurement readings are not normalized yet")
		return
	case errors.Is(err, usecase.ErrPDFUnavailable):
		response.Error(w, r, http.StatusServiceUnavailable, "PDF_UNAVAILABLE", "PDF renderer is unavailable")
		return
	case err != nil:
		writeError(w, r, err)
		return
	}
	if f.Pending {
		w.Header().Set("Retry-After", "2")
		response.JSON(w, r, http.StatusAccepted, PDFPending{UUID: f.UUID, Status: "pending"})
		return
	}
	defer func() { _ = f.Body.Close() }()
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+f.Filename+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = io.Copy(w, f.Body)
}
