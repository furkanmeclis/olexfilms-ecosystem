package handler

import (
	"errors"
	"net/http"

	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Error codes of the scan endpoint (404).
const (
	CodeScanNoMatch          = "SCAN_NO_MATCH"
	CodeScanLocationNotFound = "SCAN_LOCATION_NOT_FOUND"
)

// ScanHandler serves POST /v1/warehouse/scan (TEC-203).
type ScanHandler struct {
	svc *wh.Scanner
}

// NewScan builds the scan handler.
func NewScan(svc *wh.Scanner) *ScanHandler { return &ScanHandler{svc: svc} }

type scanBody struct {
	Code string `json:"code"`
}

// Scan (POST /v1/warehouse/scan) resolves one scanned string.
func (h *ScanHandler) Scan(w http.ResponseWriter, r *http.Request) {
	var body scanBody
	if !decode(w, r, &body) {
		return
	}
	c := wh.ScanCaller{Caller: caller(r), Principal: authctx.MustPrincipal(r.Context())}
	res, err := h.svc.Resolve(r.Context(), c, body.Code)
	switch {
	case err == nil:
		response.JSON(w, r, http.StatusOK, res)
	case errors.Is(err, wh.ErrScanNoMatch):
		response.Error(w, r, http.StatusNotFound, CodeScanNoMatch, "The scanned code matches no location, unit or product you can see")
	case errors.Is(err, wh.ErrScanLocationNotFound):
		response.Error(w, r, http.StatusNotFound, CodeScanLocationNotFound, "The scanned location QR is not a location of this organization")
	default:
		writeError(w, r, err)
	}
}
