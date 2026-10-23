package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/google/uuid"
)

type fakeCalibrations struct {
	caller   usecase.Caller
	in       usecase.CalibrationInput
	org      int64
	serial   int
	replayed bool
	err      error
}

func (f *fakeCalibrations) Record(_ context.Context, c usecase.Caller, in usecase.CalibrationInput) (usecase.CalibrationView, bool, error) {
	f.caller, f.in = c, in
	return usecase.CalibrationView{UUID: uuid.New(), Serial: in.Serial, TargetUm: in.TargetUm}, f.replayed, f.err
}

func (f *fakeCalibrations) Latest(_ context.Context, org int64, serial int) (usecase.CalibrationView, error) {
	f.org, f.serial = org, serial
	if f.err != nil {
		return usecase.CalibrationView{}, f.err
	}
	return usecase.CalibrationView{Serial: serial, TargetUm: 90}, nil
}

func calibrationCall(h http.HandlerFunc, method, path, body string) (*httptest.ResponseRecorder, string) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if i := strings.LastIndex(path, "/"); method == http.MethodGet {
		req.SetPathValue("serial", path[i+1:])
	}
	ctx := authctx.WithPrincipal(req.Context(), authctx.Principal{UserInternal: 7})
	ctx = orgctx.WithScope(ctx, orgctx.Scope{InternalID: 10, BrandID: 2})
	rec := httptest.NewRecorder()
	h(rec, req.WithContext(ctx))
	var env struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error == nil {
		return rec, ""
	}
	return rec, env.Error.Code
}

// TEC-939: POST records with the active organization and the user; a
// replayed client_id answers 200; bad JSON and usecase validation are 400/422.
func TestCalibrationCreate(t *testing.T) {
	f := &fakeCalibrations{}
	h := NewCalibrations(f)
	body := `{"serial":32475,"fe_offset_hz":-35,"al_offset_hz":12,"target_um":90,"samples":{"fe":[18275]}}`
	rec, _ := calibrationCall(h.Create, "POST", "/v1/measurements/devices/smartptg/calibrations", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	if f.caller != (usecase.Caller{UserID: 7, OrganizationID: 10, BrandID: 2}) {
		t.Fatalf("caller = %+v", f.caller)
	}
	if f.in.Serial != 32475 || f.in.FeOffsetHz == nil || *f.in.FeOffsetHz != -35 || f.in.Samples == nil || f.in.Samples.Fe[0] != 18275 {
		t.Fatalf("input = %+v", f.in)
	}

	f.replayed = true
	if rec, _ := calibrationCall(h.Create, "POST", "/x", body); rec.Code != http.StatusOK {
		t.Fatalf("replay = %d", rec.Code)
	}

	if rec, code := calibrationCall(h.Create, "POST", "/x", `{"serial":`); rec.Code != http.StatusBadRequest || code != "VALIDATION_ERROR" {
		t.Fatalf("bad json = %d %s", rec.Code, code)
	}

	f.err = &usecase.ValidationError{Field: "target_um", Message: "must be 90 or 150"}
	if rec, code := calibrationCall(h.Create, "POST", "/x", body); rec.Code != http.StatusBadRequest || code != "VALIDATION_ERROR" {
		t.Fatalf("invalid = %d %s", rec.Code, code)
	}
}

// TEC-939: GET answers the organization's latest row; 404 without one,
// 400 INVALID_SERIAL for a bad path serial (the use case is not called).
func TestCalibrationLatest(t *testing.T) {
	f := &fakeCalibrations{}
	h := NewCalibrations(f)
	rec, _ := calibrationCall(h.Latest, "GET", "/v1/measurements/devices/smartptg/calibrations/0032475", "")
	if rec.Code != http.StatusOK || f.org != 10 || f.serial != 32475 {
		t.Fatalf("latest = %d org=%d serial=%d", rec.Code, f.org, f.serial)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache-control = %q", rec.Header().Get("Cache-Control"))
	}

	f.serial = 0
	for _, bad := range []string{"abc", "0", "1234567890", "-5"} {
		rec, code := calibrationCall(h.Latest, "GET", "/v1/measurements/devices/smartptg/calibrations/"+bad, "")
		if rec.Code != http.StatusBadRequest || code != "INVALID_SERIAL" || f.serial != 0 {
			t.Fatalf("%q = %d %s (serial %d)", bad, rec.Code, code, f.serial)
		}
	}

	f.err = usecase.ErrCalibrationNotFound
	if rec, _ := calibrationCall(h.Latest, "GET", "/v1/measurements/devices/smartptg/calibrations/1", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing = %d", rec.Code)
	}
}
