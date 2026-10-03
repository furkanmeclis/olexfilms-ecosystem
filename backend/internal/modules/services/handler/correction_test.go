package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
)

// TEC-230: the correction refusals map to named codes (422 for the
// business rules, 409 for a second correction).
func TestWriteErrorCorrectionCodes(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{svcuc.ErrCorrectionWindowClosed, http.StatusUnprocessableEntity, CodeCorrectionWindowClosed},
		{svcuc.ErrItemWarrantyActive, http.StatusUnprocessableEntity, CodeItemWarrantyActive},
		{svcuc.ErrConsumptionNotReversible, http.StatusUnprocessableEntity, CodeConsumptionNotReversible},
		{svcuc.ErrItemAlreadyCorrected, http.StatusConflict, CodeItemAlreadyCorrected},
		{fmt.Errorf("wrapped: %w", svcuc.ErrUnitNotAvailable), http.StatusConflict, CodeUnitNotAvailable},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		writeError(rec, httptest.NewRequest(http.MethodPost, "/", nil), tc.err)
		if rec.Code != tc.status {
			t.Fatalf("%v: status %d, want %d", tc.err, rec.Code, tc.status)
		}
		var env struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("%v: %v", tc.err, err)
		}
		if env.Error.Code != tc.code {
			t.Fatalf("%v: code %q, want %q (%s)", tc.err, env.Error.Code, tc.code, rec.Body.String())
		}
	}
}
