package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/glorianadmin"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/glorianadmin/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/glorianadmin/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian/fake"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-273: handler tests without a database. The querier stub implements
// only the reads and writes of the admin API (any other call panics on the
// nil embedded interface); the hub is the fake Inventory API server.

const (
	glorianBrandID = 2
	olexBrandID    = 1
	centerOrgID    = 21
	connID         = 9
	plainKey       = "plain-secret-key-should-never-leak"
)

type stubQ struct {
	db.Querier
	conn      *db.IntegrationConnection
	outbounds []db.GetGlorianOutboundByUUIDRow
	runs      []db.IntegrationSyncRun
	finished  []db.FinishIntegrationSyncRunParams
}

func (q *stubQ) GetBrandBySlug(_ context.Context, slug string) (db.Brand, error) {
	if slug != usecase.BrandSlug {
		return db.Brand{}, pgx.ErrNoRows
	}
	return db.Brand{ID: glorianBrandID, Slug: slug, Name: "Glorian", Status: "active"}, nil
}

func (q *stubQ) GetBrandCenter(_ context.Context, brandID int64) (db.Organization, error) {
	if brandID != glorianBrandID {
		return db.Organization{}, pgx.ErrNoRows
	}
	return db.Organization{ID: centerOrgID, BrandID: brandID}, nil
}

func (q *stubQ) GetIntegrationConnectionByKey(_ context.Context, arg db.GetIntegrationConnectionByKeyParams) (db.IntegrationConnection, error) {
	if q.conn == nil || arg.BrandID != q.conn.BrandID || arg.Key != q.conn.Key {
		return db.IntegrationConnection{}, pgx.ErrNoRows
	}
	return *q.conn, nil
}

func (q *stubQ) CreateIntegrationConnection(_ context.Context, arg db.CreateIntegrationConnectionParams) (db.IntegrationConnection, error) {
	q.conn = &db.IntegrationConnection{
		ID: connID, Uuid: uuid.New(), OrganizationID: arg.OrganizationID, BrandID: arg.BrandID, Key: arg.Key,
		BaseUrl: arg.BaseUrl, ApiKeyEnc: arg.ApiKeyEnc, DefaultWarehouseID: arg.DefaultWarehouseID,
		Active: arg.Active, ApiVersion: arg.ApiVersion,
	}
	return *q.conn, nil
}

func (q *stubQ) UpdateIntegrationConnection(_ context.Context, arg db.UpdateIntegrationConnectionParams) (db.IntegrationConnection, error) {
	q.conn.BaseUrl, q.conn.DefaultWarehouseID, q.conn.Active, q.conn.ApiVersion = arg.BaseUrl, arg.DefaultWarehouseID, arg.Active, arg.ApiVersion
	return *q.conn, nil
}

func (q *stubQ) SetIntegrationConnectionAPIKey(_ context.Context, arg db.SetIntegrationConnectionAPIKeyParams) (db.IntegrationConnection, error) {
	q.conn.ApiKeyEnc = arg.ApiKeyEnc
	return *q.conn, nil
}

func (q *stubQ) matchRun(r db.IntegrationSyncRun, conn int64, kinds, statuses []string) bool {
	return r.ConnectionID == conn && (len(kinds) == 0 || slices.Contains(kinds, r.Kind)) &&
		(len(statuses) == 0 || slices.Contains(statuses, r.Status))
}

func (q *stubQ) ListGlorianSyncRuns(_ context.Context, arg db.ListGlorianSyncRunsParams) ([]db.IntegrationSyncRun, error) {
	var out []db.IntegrationSyncRun
	for _, r := range q.runs {
		if q.matchRun(r, arg.ConnectionID, arg.Kinds, arg.Statuses) {
			out = append(out, r)
		}
	}
	if int(arg.RowOffset) >= len(out) {
		return nil, nil
	}
	out = out[arg.RowOffset:]
	if len(out) > int(arg.RowLimit) {
		out = out[:arg.RowLimit]
	}
	return out, nil
}

func (q *stubQ) CountGlorianSyncRuns(_ context.Context, arg db.CountGlorianSyncRunsParams) (int64, error) {
	var n int64
	for _, r := range q.runs {
		if q.matchRun(r, arg.ConnectionID, arg.Kinds, arg.Statuses) {
			n++
		}
	}
	return n, nil
}

func (q *stubQ) GetGlorianSyncRunByUUID(_ context.Context, arg db.GetGlorianSyncRunByUUIDParams) (db.IntegrationSyncRun, error) {
	for _, r := range q.runs {
		if r.Uuid == arg.Uuid && r.ConnectionID == arg.ConnectionID {
			return r, nil
		}
	}
	return db.IntegrationSyncRun{}, pgx.ErrNoRows
}

func (q *stubQ) StartIntegrationSyncRun(_ context.Context, arg db.StartIntegrationSyncRunParams) (db.IntegrationSyncRun, error) {
	r := db.IntegrationSyncRun{
		ID: int64(len(q.runs) + 1), Uuid: uuid.New(), OrganizationID: arg.OrganizationID, BrandID: arg.BrandID,
		ConnectionID: arg.ConnectionID, Kind: arg.Kind, Status: glorian.RunRunning,
		StartedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, Counts: []byte("{}"),
	}
	q.runs = append(q.runs, r)
	return r, nil
}

func (q *stubQ) FinishIntegrationSyncRun(_ context.Context, arg db.FinishIntegrationSyncRunParams) (db.IntegrationSyncRun, error) {
	q.finished = append(q.finished, arg)
	return db.IntegrationSyncRun{ID: arg.ID, Status: arg.Status}, nil
}

func (q *stubQ) ListGlorianOutbounds(_ context.Context, arg db.ListGlorianOutboundsParams) ([]db.ListGlorianOutboundsRow, error) {
	var out []db.ListGlorianOutboundsRow
	for _, ob := range q.outbounds {
		if len(arg.States) == 0 || slices.Contains(arg.States, ob.State) {
			out = append(out, db.ListGlorianOutboundsRow(ob))
		}
	}
	return out, nil
}

func (q *stubQ) CountGlorianOutbounds(_ context.Context, arg db.CountGlorianOutboundsParams) (int64, error) {
	var n int64
	for _, ob := range q.outbounds {
		if len(arg.States) == 0 || slices.Contains(arg.States, ob.State) {
			n++
		}
	}
	return n, nil
}

func (q *stubQ) GetGlorianOutboundByUUID(_ context.Context, arg db.GetGlorianOutboundByUUIDParams) (db.GetGlorianOutboundByUUIDRow, error) {
	for _, ob := range q.outbounds {
		if ob.Uuid == arg.Uuid && arg.ConnectionID == connID {
			return ob, nil
		}
	}
	return db.GetGlorianOutboundByUUIDRow{}, pgx.ErrNoRows
}

type recordingQueue struct {
	tasks []*asynq.Task
	err   error
}

func (q *recordingQueue) Enqueue(task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	if q.err != nil {
		return nil, q.err
	}
	q.tasks = append(q.tasks, task)
	return &asynq.TaskInfo{ID: "t", Type: task.Type()}, nil
}

type env struct {
	t     *testing.T
	q     *stubQ
	queue *recordingQueue
	srv   *fake.Server
	box   *crypto.SecretBox
	mux   *http.ServeMux
}

// principal is the caller of the next requests; requestBrand the brand of
// the domain (0: none).
var (
	principal    authctx.Principal
	requestBrand int64
)

func newEnv(t *testing.T) *env {
	t.Helper()
	box, err := crypto.NewSecretBox("test-encryption-key-32-bytes!!!!")
	if err != nil {
		t.Fatal(err)
	}
	srv := fake.New(t)
	q := &stubQ{}
	rq := &recordingQueue{}
	factory := glorian.HTTPClientFactory(glorian.Options{HTTPClient: srv.Client(), RetryBaseDelay: time.Millisecond})
	svc := usecase.New(q, box, factory, rq, nil)
	mux := http.NewServeMux()
	authn := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := authctx.WithPrincipal(r.Context(), principal)
			if requestBrand != 0 {
				ctx = brandctx.WithBrand(ctx, brandctx.Brand{ID: requestBrand})
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	glorianadmin.Mount(mux, handler.New(svc, nil), authn)
	principal = superAdmin()
	requestBrand = olexBrandID
	return &env{t: t, q: q, queue: rq, srv: srv, box: box, mux: mux}
}

func superAdmin() authctx.Principal {
	return authctx.Principal{UserID: uuid.New(), UserInternal: 1, IsSuperAdmin: true}
}

func granted(scope rbac.Scope, slugs ...string) authctx.Principal {
	p := authctx.Principal{UserID: uuid.New(), UserInternal: 2, PermissionScopes: map[string]rbac.Scope{}}
	for _, s := range slugs {
		p.Permissions = append(p.Permissions, s)
		p.PermissionScopes[s] = scope
	}
	return p
}

// withConnection stores a connection pointing at the fake hub.
func (e *env) withConnection(active bool, key string) {
	enc, err := e.box.Encrypt(key)
	if err != nil {
		e.t.Fatal(err)
	}
	e.q.conn = &db.IntegrationConnection{
		ID: connID, Uuid: uuid.New(), OrganizationID: centerOrgID, BrandID: glorianBrandID, Key: glorian.ConnectionKey,
		BaseUrl: e.srv.URL, ApiKeyEnc: enc, Active: active, ApiVersion: glorian.APIVersion,
	}
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Details []struct {
			Field string `json:"field"`
		} `json:"details"`
	} `json:"error"`
}

func (e *env) do(method, path string, body any) (int, envelope, string) {
	e.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, glorianadmin.Prefix+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	var env envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		e.t.Fatalf("%s %s: decode %q: %v", method, path, rec.Body.String(), err)
	}
	return rec.Code, env, rec.Body.String()
}

func TestUnauthorizedCallerGets403(t *testing.T) {
	e := newEnv(t)
	e.withConnection(true, fake.APIKey)
	held := uuid.New()
	e.q.outbounds = []db.GetGlorianOutboundByUUIDRow{{ID: 5, Uuid: held, State: glorian.OutboundHeld}}

	all := []struct{ method, path string }{
		{"GET", ""}, {"PUT", ""}, {"POST", "/test"}, {"GET", "/sync-runs"}, {"POST", "/sync-runs"},
		{"GET", "/sync-runs/" + uuid.NewString()}, {"GET", "/outbounds?state=held"},
		{"POST", "/outbounds/" + held.String() + "/replay"}, {"POST", "/reconcile"},
		{"POST", "/outbounds/replay"},
	}
	writes := map[string]bool{"PUT": true, "POST": true}

	cases := []struct {
		name    string
		p       authctx.Principal
		brand   int64
		allowed func(method string) bool
	}{
		{"no permission", granted(rbac.ScopeAll, rbac.PermWarehouseRead), olexBrandID, func(string) bool { return false }},
		{"view only", granted(rbac.ScopeAll, rbac.PermIntegrationsGlorianView), olexBrandID, func(m string) bool { return !writes[m] }},
		// A brand grant reaches the glorian connection only on the glorian domain.
		{"brand grant on olex domain", granted(rbac.ScopeBrand, rbac.PermIntegrationsGlorianView, rbac.PermIntegrationsGlorianManage), olexBrandID, func(string) bool { return false }},
		{"brand grant without domain brand", granted(rbac.ScopeBrand, rbac.PermIntegrationsGlorianView), 0, func(string) bool { return false }},
	}
	for _, tc := range cases {
		principal, requestBrand = tc.p, tc.brand
		for _, c := range all {
			code, env, body := e.do(c.method, c.path, nil)
			if !tc.allowed(c.method) {
				if code != http.StatusForbidden || env.Error == nil || env.Error.Code != "FORBIDDEN" {
					t.Fatalf("%s: %s %s = %d %s; want 403", tc.name, c.method, c.path, code, body)
				}
			} else if code == http.StatusForbidden {
				t.Fatalf("%s: %s %s = 403; want allowed", tc.name, c.method, c.path)
			}
		}
	}
	if len(e.queue.tasks) != 0 {
		t.Fatalf("forbidden calls queued %d tasks", len(e.queue.tasks))
	}

	// The brand grant on the glorian domain is allowed.
	principal, requestBrand = granted(rbac.ScopeBrand, rbac.PermIntegrationsGlorianView), glorianBrandID
	if code, _, body := e.do("GET", "", nil); code != http.StatusOK {
		t.Fatalf("brand grant on glorian domain = %d %s", code, body)
	}
}

func assertNoKey(t *testing.T, body string, enc string) {
	t.Helper()
	if strings.Contains(body, plainKey) {
		t.Fatalf("response leaks the plain key: %s", body)
	}
	if enc != "" && strings.Contains(body, enc) {
		t.Fatalf("response leaks the ciphertext: %s", body)
	}
	if strings.Contains(body, `"api_key"`) || strings.Contains(body, `"api_key_enc"`) {
		t.Fatalf("response has an api_key field: %s", body)
	}
}

func TestAPIKeyIsWriteOnly(t *testing.T) {
	e := newEnv(t)

	code, env, body := e.do("GET", "", nil)
	if code != http.StatusOK || !strings.Contains(string(env.Data), `"configured":false`) {
		t.Fatalf("unconfigured GET = %d %s", code, body)
	}

	code, env, body = e.do("PUT", "", map[string]any{
		"base_url": e.srv.URL, "active": true, "api_key": plainKey,
	})
	if code != http.StatusOK {
		t.Fatalf("PUT = %d %s", code, body)
	}
	if e.q.conn == nil || e.q.conn.ApiKeyEnc == "" || e.q.conn.ApiKeyEnc == plainKey {
		t.Fatalf("stored key = %+v; want ciphertext", e.q.conn)
	}
	if got, err := e.box.Decrypt(e.q.conn.ApiKeyEnc); err != nil || got != plainKey {
		t.Fatalf("decrypt = %q %v", got, err)
	}
	if e.q.conn.OrganizationID != centerOrgID || e.q.conn.BrandID != glorianBrandID || e.q.conn.Key != glorian.ConnectionKey {
		t.Fatalf("connection = %+v", e.q.conn)
	}
	assertNoKey(t, body, e.q.conn.ApiKeyEnc)
	var v usecase.ConnectionView
	if err := json.Unmarshal(env.Data, &v); err != nil {
		t.Fatal(err)
	}
	if !v.Configured || !v.APIKeySet || v.APIKeyMasked == nil || *v.APIKeyMasked != usecase.MaskedAPIKey || !v.Active {
		t.Fatalf("view = %+v", v)
	}

	code, _, body = e.do("GET", "", nil)
	if code != http.StatusOK {
		t.Fatalf("GET = %d %s", code, body)
	}
	assertNoKey(t, body, e.q.conn.ApiKeyEnc)

	// Without api_key the stored key is kept.
	enc := e.q.conn.ApiKeyEnc
	code, _, body = e.do("PUT", "", map[string]any{"base_url": e.srv.URL + "/", "active": false})
	if code != http.StatusOK || e.q.conn.ApiKeyEnc != enc || e.q.conn.Active {
		t.Fatalf("PUT without key = %d %s, key kept=%v", code, body, e.q.conn.ApiKeyEnc == enc)
	}
	assertNoKey(t, body, enc)
}

func TestPutValidation(t *testing.T) {
	e := newEnv(t)
	for _, tc := range []struct {
		body  map[string]any
		field string
	}{
		{map[string]any{"base_url": ""}, "base_url"},
		{map[string]any{"base_url": "ftp://hub"}, "base_url"},
		{map[string]any{"base_url": "https://hub.example", "api_version": "2"}, "api_version"},
		{map[string]any{"base_url": "https://hub.example", "active": true}, "api_key"},
		{map[string]any{"base_url": "https://hub.example", "unknown": 1}, "body"},
	} {
		code, env, body := e.do("PUT", "", tc.body)
		if code != http.StatusBadRequest || env.Error == nil || env.Error.Code != "VALIDATION_ERROR" ||
			len(env.Error.Details) == 0 || env.Error.Details[0].Field != tc.field {
			t.Fatalf("PUT %v = %d %s; want 400 on %s", tc.body, code, body, tc.field)
		}
	}
	if e.q.conn != nil {
		t.Fatal("invalid PUT created a connection")
	}
}

func TestPingHitsFakeHub(t *testing.T) {
	e := newEnv(t)
	if code, _, body := e.do("POST", "/test", nil); code != http.StatusNotFound {
		t.Fatalf("unconfigured test = %d %s", code, body)
	}

	// An inactive connection can still be tested before it is switched on.
	e.withConnection(false, fake.APIKey)
	code, env, body := e.do("POST", "/test", nil)
	if code != http.StatusOK {
		t.Fatalf("test = %d %s", code, body)
	}
	var res usecase.TestResult
	if err := json.Unmarshal(env.Data, &res); err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Code != nil {
		t.Fatalf("result = %+v", res)
	}
	reqs := e.srv.RequestsTo("GET", "/product-categories")
	if len(reqs) != 1 || reqs[0].Header.Get(glorian.HeaderAPIKey) != fake.APIKey {
		t.Fatalf("hub requests = %+v", e.srv.Requests())
	}
	if strings.Contains(body, fake.APIKey) {
		t.Fatal("test response leaks the key")
	}

	// A wrong key is a failed result, not an error.
	e.withConnection(true, plainKey)
	code, env, body = e.do("POST", "/test", nil)
	if code != http.StatusOK {
		t.Fatalf("test wrong key = %d %s", code, body)
	}
	res = usecase.TestResult{}
	if err := json.Unmarshal(env.Data, &res); err != nil {
		t.Fatal(err)
	}
	if res.OK || res.HTTPStatus == nil || *res.HTTPStatus != http.StatusUnauthorized || res.Code == nil || *res.Code != glorian.CodeUnauthorized {
		t.Fatalf("result = %+v", res)
	}
	assertNoKey(t, body, "")
}

func TestReplayQueuesHeldOutbound(t *testing.T) {
	e := newEnv(t)
	e.withConnection(true, fake.APIKey)
	held, sent := uuid.New(), uuid.New()
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	e.q.outbounds = []db.GetGlorianOutboundByUUIDRow{
		{ID: 41, Uuid: held, OrderUuid: uuid.New(), OrderNo: "ORD-41", State: glorian.OutboundHeld,
			HeldReason: pgtype.Text{String: glorian.HeldMissingCustomer, Valid: true}, CreatedAt: now, UpdatedAt: now},
		{ID: 42, Uuid: sent, OrderNo: "ORD-42", State: glorian.OutboundSent, CreatedAt: now, UpdatedAt: now},
	}

	code, env, body := e.do("GET", "/outbounds?state=held", nil)
	if code != http.StatusOK || !strings.Contains(string(env.Data), held.String()) || strings.Contains(string(env.Data), sent.String()) {
		t.Fatalf("held list = %d %s", code, body)
	}
	// TEC-367: no state lists every state, with total.
	code, env, body = e.do("GET", "/outbounds", nil)
	if code != http.StatusOK || !strings.Contains(string(env.Data), held.String()) || !strings.Contains(string(env.Data), sent.String()) ||
		!strings.Contains(string(env.Data), `"total":2`) {
		t.Fatalf("all states list = %d %s", code, body)
	}
	for _, q := range []string{"?state=bogus", "?sort=bogus", "?offset=-1", "?updated_from=nope"} {
		if code, env, body := e.do("GET", "/outbounds"+q, nil); code != http.StatusBadRequest || env.Error.Code != "VALIDATION_ERROR" {
			t.Fatalf("outbounds %s = %d %s", q, code, body)
		}
	}

	code, _, body = e.do("POST", "/outbounds/"+held.String()+"/replay", nil)
	if code != http.StatusAccepted {
		t.Fatalf("replay = %d %s", code, body)
	}
	if len(e.queue.tasks) != 1 || e.queue.tasks[0].Type() != queue.TaskGlorianOutboundReplayOne {
		t.Fatalf("tasks = %+v", e.queue.tasks)
	}
	var payload queue.GlorianOutboundReplayOnePayload
	if err := json.Unmarshal(e.queue.tasks[0].Payload(), &payload); err != nil || payload.OutboundID != 41 {
		t.Fatalf("payload = %+v %v", payload, err)
	}

	if code, env, body := e.do("POST", "/outbounds/"+sent.String()+"/replay", nil); code != http.StatusUnprocessableEntity ||
		env.Error.Code != handler.CodeNotReplayable {
		t.Fatalf("replay sent = %d %s", code, body)
	}
	if code, _, body := e.do("POST", "/outbounds/"+uuid.NewString()+"/replay", nil); code != http.StatusNotFound {
		t.Fatalf("replay unknown = %d %s", code, body)
	}
	if code, _, body := e.do("POST", "/outbounds/nope/replay", nil); code != http.StatusBadRequest {
		t.Fatalf("replay bad uuid = %d %s", code, body)
	}
	if len(e.queue.tasks) != 1 {
		t.Fatalf("rejected replays queued tasks: %d", len(e.queue.tasks))
	}

	// A double click finds the pending task: still accepted.
	e.queue.err = asynq.ErrTaskIDConflict
	if code, _, body := e.do("POST", "/outbounds/"+held.String()+"/replay", nil); code != http.StatusAccepted {
		t.Fatalf("duplicate replay = %d %s", code, body)
	}
	e.queue.err = errors.New("redis down")
	if code, _, _ := e.do("POST", "/outbounds/"+held.String()+"/replay", nil); code != http.StatusInternalServerError {
		t.Fatalf("enqueue failure = %d", code)
	}
}

func TestReconcileReturnsRunningRun(t *testing.T) {
	e := newEnv(t)
	e.withConnection(true, fake.APIKey)
	code, env, body := e.do("POST", "/reconcile", nil)
	if code != http.StatusAccepted {
		t.Fatalf("reconcile = %d %s", code, body)
	}
	var run usecase.SyncRunView
	if err := json.Unmarshal(env.Data, &run); err != nil {
		t.Fatal(err)
	}
	if run.Kind != glorian.KindReconcile || run.Status != glorian.RunRunning {
		t.Fatalf("run = %+v", run)
	}
	if len(e.queue.tasks) != 1 || e.queue.tasks[0].Type() != queue.TaskGlorianReconcile {
		t.Fatalf("tasks = %+v", e.queue.tasks)
	}
	if len(e.srv.Requests()) != 0 {
		t.Fatal("the endpoint must leave the hub calls to the worker")
	}
	code, env, body = e.do("GET", "/sync-runs/"+run.UUID.String(), nil)
	if code != http.StatusOK || !strings.Contains(string(env.Data), run.UUID.String()) {
		t.Fatalf("run detail = %d %s", code, body)
	}

	// An enqueue failure closes the run instead of leaving it running.
	e.queue.err = errors.New("redis down")
	if code, _, _ := e.do("POST", "/reconcile", nil); code != http.StatusInternalServerError {
		t.Fatalf("enqueue failure = %d", code)
	}
	if len(e.q.finished) != 1 || e.q.finished[0].Status != glorian.RunFailed {
		t.Fatalf("finished = %+v", e.q.finished)
	}
}

func TestSyncRunsListAndTrigger(t *testing.T) {
	e := newEnv(t)
	e.withConnection(false, fake.APIKey)
	e.q.runs = []db.IntegrationSyncRun{
		{ID: 1, Uuid: uuid.New(), ConnectionID: connID, Kind: glorian.KindPullProducts, Status: glorian.RunSucceeded, Counts: []byte(`{"fetched":3}`)},
		{ID: 2, Uuid: uuid.New(), ConnectionID: connID, Kind: glorian.KindReconcile, Status: glorian.RunFailed, Error: pgtype.Text{String: "held: inactive_connection", Valid: true}},
	}
	code, env, body := e.do("GET", "/sync-runs?kind=pull_products&status=succeeded", nil)
	if code != http.StatusOK {
		t.Fatalf("list = %d %s", code, body)
	}
	var list struct {
		Items []usecase.SyncRunView `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].Kind != glorian.KindPullProducts || string(list.Items[0].Counts) != `{"fetched":3}` {
		t.Fatalf("items = %+v", list.Items)
	}
	// TEC-367: multi-value filters, offset paging and total.
	code, env, body = e.do("GET", "/sync-runs?kind=pull_products,reconcile&limit=1&offset=1", nil)
	var page struct {
		Items  []usecase.SyncRunView `json:"items"`
		Total  int64                 `json:"total"`
		Offset int32                 `json:"offset"`
	}
	if code != http.StatusOK {
		t.Fatalf("paged list = %d %s", code, body)
	}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || page.Offset != 1 || len(page.Items) != 1 || page.Items[0].Kind != glorian.KindReconcile {
		t.Fatalf("page = %+v", page)
	}
	for _, q := range []string{"?kind=bogus", "?status=done", "?limit=0", "?limit=x", "?sort=nope", "?offset=x", "?started_from=2026-13-01"} {
		if code, env, body := e.do("GET", "/sync-runs"+q, nil); code != http.StatusBadRequest || env.Error.Code != "VALIDATION_ERROR" {
			t.Fatalf("list %s = %d %s", q, code, body)
		}
	}

	// Inactive: a manual sync would do nothing.
	if code, env, body := e.do("POST", "/sync-runs", map[string]any{"kind": "pull"}); code != http.StatusUnprocessableEntity || env.Error.Code != handler.CodeInactive {
		t.Fatalf("inactive trigger = %d %s", code, body)
	}
	e.q.conn.Active = true
	if code, _, body := e.do("POST", "/sync-runs", nil); code != http.StatusAccepted {
		t.Fatalf("trigger = %d %s", code, body)
	}
	if code, _, body := e.do("POST", "/sync-runs", map[string]any{"kind": "outbound_replay"}); code != http.StatusAccepted {
		t.Fatalf("trigger replay = %d %s", code, body)
	}
	if code, _, body := e.do("POST", "/sync-runs", map[string]any{"kind": "nope"}); code != http.StatusBadRequest {
		t.Fatalf("trigger bad kind = %d %s", code, body)
	}
	if len(e.queue.tasks) != 2 || e.queue.tasks[0].Type() != queue.TaskGlorianPullCatalog || e.queue.tasks[1].Type() != queue.TaskGlorianOrderReplay {
		t.Fatalf("tasks = %+v", e.queue.tasks)
	}
}

func TestReplayOutboundsBatch(t *testing.T) {
	e := newEnv(t)
	e.withConnection(true, fake.APIKey)
	held, failed, sent, unknown := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	e.q.outbounds = []db.GetGlorianOutboundByUUIDRow{
		{ID: 51, Uuid: held, OrderNo: "ORD-51", State: glorian.OutboundHeld,
			HeldReason: pgtype.Text{String: glorian.HeldMissingCustomer, Valid: true}, CreatedAt: now, UpdatedAt: now},
		{ID: 52, Uuid: failed, OrderNo: "ORD-52", State: glorian.OutboundFailed, CreatedAt: now, UpdatedAt: now},
		{ID: 53, Uuid: sent, OrderNo: "ORD-53", State: glorian.OutboundSent, CreatedAt: now, UpdatedAt: now},
	}
	code, env, body := e.do("POST", "/outbounds/replay", map[string]any{
		"uuids": []string{held.String(), failed.String(), sent.String(), unknown.String(), held.String()},
	})
	if code != http.StatusAccepted {
		t.Fatalf("batch replay = %d %s", code, body)
	}
	var out usecase.ReplayBatchResult
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Queued) != 2 || len(out.Skipped) != 2 || len(e.queue.tasks) != 2 {
		t.Fatalf("result = %+v tasks=%d", out, len(e.queue.tasks))
	}
	reasons := map[uuid.UUID]string{}
	for _, s := range out.Skipped {
		reasons[s.UUID] = s.Reason
	}
	if reasons[sent] != usecase.SkipNotReplayable || reasons[unknown] != usecase.SkipNotFound {
		t.Fatalf("skipped = %+v", out.Skipped)
	}
	if code, env, body := e.do("POST", "/outbounds/replay", map[string]any{"uuids": []string{}}); code != http.StatusBadRequest ||
		env.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("empty batch = %d %s", code, body)
	}
	if code, _, body := e.do("POST", "/outbounds/replay", map[string]any{"uuids": []string{"nope"}}); code != http.StatusBadRequest {
		t.Fatalf("bad uuid batch = %d %s", code, body)
	}
}
