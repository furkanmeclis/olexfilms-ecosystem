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

type fakeCreator struct {
	got usecase.Caller
	in  usecase.Input
	res usecase.Result
	err error
}

func (f *fakeCreator) Create(_ context.Context, c usecase.Caller, in usecase.Input) (usecase.Result, error) {
	f.got, f.in = c, in
	return f.res, f.err
}

func (f *fakeCreator) ListDevices(context.Context, usecase.PanelCaller) ([]usecase.DeviceView, error) {
	return nil, f.err
}

func (f *fakeCreator) CreateDevice(context.Context, usecase.PanelCaller, usecase.DeviceInput) (usecase.DeviceView, error) {
	return usecase.DeviceView{}, f.err
}

func (f *fakeCreator) UpdateDevice(context.Context, usecase.PanelCaller, uuid.UUID, usecase.DeviceInput) (usecase.DeviceView, error) {
	return usecase.DeviceView{}, f.err
}

func (f *fakeCreator) ListMeasurements(context.Context, usecase.PanelCaller, usecase.MeasurementFilter) ([]usecase.MeasurementSummary, int64, error) {
	return nil, 0, f.err
}

func (f *fakeCreator) GetMeasurement(context.Context, usecase.PanelCaller, uuid.UUID) (usecase.MeasurementDetail, error) {
	return usecase.MeasurementDetail{}, f.err
}

func call(t *testing.T, h *Handler, body, key string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/mobile/measurements", strings.NewReader(body))
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	ctx := authctx.WithPrincipal(req.Context(), authctx.Principal{UserInternal: 3})
	ctx = orgctx.WithScope(ctx, orgctx.Scope{InternalID: 10, BrandID: 1})
	rec := httptest.NewRecorder()
	h.Create(rec, req.WithContext(ctx))
	var env map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec.Code, env
}

func errCode(env map[string]any) string {
	e, _ := env["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

func TestCreateAnswers202WithUUIDAndStatus(t *testing.T) {
	id := uuid.New()
	f := &fakeCreator{res: usecase.Result{UUID: id, Status: usecase.StatusVINPending}}
	code, env := call(t, New(f), `{"raw":{}}`, "key-1")
	if code != http.StatusAccepted {
		t.Fatalf("status = %d %v", code, env)
	}
	data, _ := env["data"].(map[string]any)
	if data["uuid"] != id.String() || data["status"] != usecase.StatusVINPending {
		t.Fatalf("data = %v", data)
	}
	if f.got != (usecase.Caller{UserID: 3, OrganizationID: 10, BrandID: 1}) || f.in.IdempotencyKey != "key-1" ||
		string(f.in.Body) != `{"raw":{}}` {
		t.Fatalf("use case got %+v %+v", f.got, f.in)
	}

	// A replay answers the same 202.
	f.res.Replayed = true
	if code, _ := call(t, New(f), `{"raw":{}}`, "key-1"); code != http.StatusAccepted {
		t.Fatalf("replay = %d", code)
	}
}

func TestCreateErrors(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{&usecase.ValidationError{Field: "vin", Message: "bad"}, http.StatusBadRequest, "VALIDATION_ERROR"},
		{usecase.ErrServiceNotFound, http.StatusNotFound, "NOT_FOUND"},
	}
	for _, c := range cases {
		code, env := call(t, New(&fakeCreator{err: c.err}), `{}`, "")
		if code != c.status || errCode(env) != c.code {
			t.Fatalf("%v = %d %s", c.err, code, errCode(env))
		}
	}
	big := `{"raw":"` + strings.Repeat("a", MaxBodyBytes) + `"}`
	if code, env := call(t, New(&fakeCreator{}), big, ""); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("oversized body = %d %s", code, errCode(env))
	}
}
