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

	listCalls  int
	listFilter usecase.MeasurementFilter
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

func (f *fakeCreator) ListMeasurements(_ context.Context, _ usecase.PanelCaller, mf usecase.MeasurementFilter) ([]usecase.MeasurementSummary, int64, error) {
	f.listCalls++
	f.listFilter = mf
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

func listCall(t *testing.T, h *Handler, query string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("GET", "/v1/measurements?"+query, nil)
	ctx := orgctx.WithScope(req.Context(), orgctx.Scope{InternalID: 10, BrandID: 1})
	rec := httptest.NewRecorder()
	h.ListMeasurements(rec, req.WithContext(ctx))
	var env map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec.Code, env
}

func errField(env map[string]any) string {
	e, _ := env["error"].(map[string]any)
	details, _ := e["details"].([]any)
	if len(details) == 0 {
		return ""
	}
	d, _ := details[0].(map[string]any)
	s, _ := d["field"].(string)
	return s
}

// TEC-299: the list contract of GET /v1/measurements (docs/list-contract.md).
func TestListMeasurementsQueryContract(t *testing.T) {
	bad := []struct{ query, field string }{
		{"sort=bogus", "sort"},
		{"sort=-uuid", "sort"},
		{"status=accepted,nope", "status"},
		{"status=done", "status"},
		{"device_uuid=not-a-uuid", "device_uuid"},
		{"device_uuid=" + uuid.NewString() + ",x", "device_uuid"},
		{"linked=maybe", "linked"},
	}
	for _, c := range bad {
		f := &fakeCreator{}
		code, env := listCall(t, New(f), c.query)
		if code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" || errField(env) != c.field {
			t.Fatalf("%s = %d %s %s", c.query, code, errCode(env), errField(env))
		}
		if f.listCalls != 0 {
			t.Fatalf("%s reached the use case", c.query)
		}
	}

	f := &fakeCreator{}
	if code, env := listCall(t, New(f), ""); code != http.StatusOK {
		t.Fatalf("default = %d %v", code, env)
	}
	if f.listFilter.Sort.Key != "measured_at" || !f.listFilter.Sort.Desc || f.listFilter.Statuses != nil || f.listFilter.DeviceUUIDs != nil {
		t.Fatalf("default filter = %+v", f.listFilter)
	}

	d1, d2 := uuid.New(), uuid.New()
	q := "sort=plate&q=+34ab+&status=accepted,vin_pending&device_uuid=" + d1.String() + "&device_uuid=" + d2.String()
	if code, env := listCall(t, New(f), q); code != http.StatusOK {
		t.Fatalf("full = %d %v", code, env)
	}
	got := f.listFilter
	if got.Sort.Key != "plate" || got.Sort.Desc || got.Q != "34ab" ||
		strings.Join(got.Statuses, ",") != "accepted,vin_pending" ||
		len(got.DeviceUUIDs) != 2 || got.DeviceUUIDs[0] != d1 || got.DeviceUUIDs[1] != d2 {
		t.Fatalf("filter = %+v", got)
	}

	// Single values keep working.
	if code, _ := listCall(t, New(f), "status=vin_pending&device_uuid="+d1.String()+"&sort=-vin"); code != http.StatusOK {
		t.Fatalf("single = %d", code)
	}
	if strings.Join(f.listFilter.Statuses, ",") != "vin_pending" || len(f.listFilter.DeviceUUIDs) != 1 ||
		f.listFilter.Sort.Key != "vin" || !f.listFilter.Sort.Desc {
		t.Fatalf("single filter = %+v", f.listFilter)
	}
}

func TestPartMap(t *testing.T) {
	h := New(&fakeCreator{})
	get := func(bodyType string) (*httptest.ResponseRecorder, map[string]any) {
		req := httptest.NewRequest("GET", "/v1/measurement-part-maps/"+bodyType, nil)
		req.SetPathValue("body_type", bodyType)
		rec := httptest.NewRecorder()
		h.PartMap(rec, req)
		var env map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		return rec, env
	}
	rec, env := get("SEDAN")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "private, max-age=86400" {
		t.Fatalf("sedan = %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	data, _ := env["data"].(map[string]any)
	assets, _ := data["assets"].([]any)
	if data["id"] != "MUvi2UUvtDeNoPrWss2qpL" || data["point_radius"] != float64(22) || len(assets) == 0 {
		t.Fatalf("data = %v", data["id"])
	}
	first, _ := assets[0].(map[string]any)
	for _, k := range []string{"part", "place", "kind", "points", "svg"} {
		if _, ok := first[k]; !ok {
			t.Fatalf("asset lacks %s", k)
		}
	}
	if rec, env := get("NOPE"); rec.Code != http.StatusNotFound || errCode(env) != "NOT_FOUND" {
		t.Fatalf("unknown = %d %s", rec.Code, errCode(env))
	}
}
