package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

func (f *fakeCreator) CompleteVIN(_ context.Context, _ usecase.PanelCaller, _ uuid.UUID, vin string) (usecase.MeasurementDetail, error) {
	if f.err != nil {
		return usecase.MeasurementDetail{}, f.err
	}
	if _, err := usecase.NormalizeVIN(vin); err != nil {
		return usecase.MeasurementDetail{}, err
	}
	v := vin
	return usecase.MeasurementDetail{MeasurementSummary: usecase.MeasurementSummary{VIN: &v, Status: usecase.StatusAccepted}}, nil
}

// TEC-294: the VIN completion answers 200, 400 VALIDATION_ERROR for an
// invalid VIN or body, 404 for a bad uuid and 422 for another VIN.
func TestCompleteVINHandler(t *testing.T) {
	call := func(f *fakeCreator, id, body string) (int, string) {
		t.Helper()
		req := httptest.NewRequest("PATCH", "/v1/measurements/"+id+"/vin", strings.NewReader(body))
		req.SetPathValue("uuid", id)
		ctx := orgctx.WithScope(req.Context(), orgctx.Scope{InternalID: 1, BrandID: 1})
		ctx = scopefilter.With(ctx, scopefilter.Filter{OrgIDs: []int64{1}})
		rec := httptest.NewRecorder()
		New(f).CompleteVIN(rec, req.WithContext(ctx))
		var env struct {
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		if env.Error == nil {
			return rec.Code, ""
		}
		return rec.Code, env.Error.Code
	}
	id := uuid.NewString()
	if st, _ := call(&fakeCreator{}, id, `{"vin":"WVWZZZ3CZKE012345"}`); st != http.StatusOK {
		t.Fatalf("valid = %d", st)
	}
	for _, body := range []string{`{"vin":"WVWZZZ3CZKE01234O"}`, `{"vin":"SHORT"}`, `{}`, `nope`} {
		if st, code := call(&fakeCreator{}, id, body); st != http.StatusBadRequest || code != "VALIDATION_ERROR" {
			t.Fatalf("%s = %d %s", body, st, code)
		}
	}
	if st, _ := call(&fakeCreator{}, "nope", `{"vin":"WVWZZZ3CZKE012345"}`); st != http.StatusNotFound {
		t.Fatalf("bad uuid = %d", st)
	}
	if st, code := call(&fakeCreator{err: usecase.ErrVINAlreadySet}, id, `{"vin":"WVWZZZ3CZKE012345"}`); st != http.StatusUnprocessableEntity ||
		code != "MEASUREMENT_VIN_ALREADY_SET" {
		t.Fatalf("already set = %d %s", st, code)
	}
	if st, _ := call(&fakeCreator{err: usecase.ErrNotFound}, id, `{"vin":"WVWZZZ3CZKE012345"}`); st != http.StatusNotFound {
		t.Fatalf("not found = %d", st)
	}
}
