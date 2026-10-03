// Package usecase is the Glorian admin API (TEC-273, F2-02h): the hub
// connection of the glorian brand (K2), its sync runs, the order
// outbounds and the on-demand pull, replay and reconcile triggers.
//
// The connection's API key is write only: it is stored encrypted
// (glorian.Store, SecretBox) and no view carries it, not even masked
// characters; ConnectionView only says whether one is set.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// BrandSlug is the brand that owns the Glorian connection (K2).
const BrandSlug = "glorian"

// MaskedAPIKey stands in for a stored key in ConnectionView.
const MaskedAPIKey = "********"

// Limits of the list endpoints.
const (
	DefaultListLimit = 50
	MaxListLimit     = 200
	maxBaseURL       = 500
	maxAPIKey        = 500
)

// Manual sync kinds of POST /sync-runs.
const (
	SyncPull           = "pull"
	SyncPushBarcodes   = "push_barcodes"
	SyncOutboundReplay = "outbound_replay"
)

// Errors mapped by the handler.
var (
	// ErrBrandNotFound: there is no glorian brand (404).
	ErrBrandNotFound = errors.New("glorian admin: glorian brand not found")
	// ErrNotConfigured: the brand has no glorian connection yet (404).
	ErrNotConfigured = errors.New("glorian admin: connection not configured")
	// ErrNotFound: the sync run or outbound is not one of the connection (404).
	ErrNotFound = errors.New("glorian admin: not found")
	// ErrCenterMissing: the brand has no center organization to own a new
	// connection (422 GLORIAN_CENTER_MISSING).
	ErrCenterMissing = errors.New("glorian admin: brand has no center organization")
	// ErrInactive: the connection is inactive, a manual sync would do
	// nothing (422 GLORIAN_CONNECTION_INACTIVE).
	ErrInactive = errors.New("glorian admin: connection is inactive")
	// ErrNotReplayable: only held and failed outbounds are replayed (422
	// GLORIAN_OUTBOUND_NOT_REPLAYABLE).
	ErrNotReplayable = errors.New("glorian admin: outbound is not held or failed")
	// ErrQueueUnavailable: the task queue is off (503).
	ErrQueueUnavailable = errors.New("glorian admin: task queue unavailable")
)

// ValidationError is a 400 VALIDATION_ERROR with field details.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string { return "glorian admin: validation failed" }

func invalid(field, msg string) *ValidationError {
	return &ValidationError{Fields: map[string]string{field: msg}}
}

// Service runs the admin use cases.
type Service struct {
	q          db.Querier
	store      *glorian.Store
	factory    glorian.ClientFactory
	reconciler *glorian.Reconciler
	queue      glorian.Enqueuer
	now        func() time.Time
}

// New wires the service. factory builds hub clients (HTTPClientFactory in
// production); queue may be nil when the task queue is off.
func New(q db.Querier, box glorian.SecretBox, factory glorian.ClientFactory, queue glorian.Enqueuer, log *slog.Logger) *Service {
	return &Service{
		q:          q,
		store:      glorian.NewStore(q, box),
		factory:    factory,
		reconciler: glorian.NewReconciler(q, box, factory, log),
		queue:      queue,
		now:        time.Now,
	}
}

// Brand returns the glorian brand.
func (s *Service) Brand(ctx context.Context) (db.Brand, error) {
	b, err := s.q.GetBrandBySlug(ctx, BrandSlug)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Brand{}, ErrBrandNotFound
	}
	return b, err
}

// Allowed reports whether p may use slug on the brand's connection: an
// `all` grant everywhere, a `brand` grant only on the brand's own domain
// (requestBrandID, 0 when the request has none).
func Allowed(p authctx.Principal, slug string, brandID, requestBrandID int64) bool {
	if p.Can(slug, rbac.ScopeAll) {
		return true
	}
	return requestBrandID != 0 && requestBrandID == brandID && p.Can(slug, rbac.ScopeBrand)
}

func (s *Service) connection(ctx context.Context, brandID int64) (db.IntegrationConnection, error) {
	conn, err := s.q.GetIntegrationConnectionByKey(ctx, db.GetIntegrationConnectionByKeyParams{
		BrandID: brandID, Key: glorian.ConnectionKey,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.IntegrationConnection{}, ErrNotConfigured
	}
	return conn, err
}

// ConnectionView is the connection as the API shows it. It has no api_key
// field: APIKeySet and APIKeyMasked are all that is said about the key.
type ConnectionView struct {
	Configured           bool       `json:"configured"`
	UUID                 *uuid.UUID `json:"uuid"`
	Key                  string     `json:"key"`
	BaseURL              string     `json:"base_url"`
	Active               bool       `json:"active"`
	APIVersion           string     `json:"api_version"`
	DefaultWarehouseUUID *uuid.UUID `json:"default_warehouse_uuid"`
	APIKeySet            bool       `json:"api_key_set"`
	APIKeyMasked         *string    `json:"api_key_masked"`
	CreatedAt            *time.Time `json:"created_at"`
	UpdatedAt            *time.Time `json:"updated_at"`
}

func (s *Service) view(ctx context.Context, conn db.IntegrationConnection) (ConnectionView, error) {
	v := ConnectionView{
		Configured: true, UUID: &conn.Uuid, Key: conn.Key, BaseURL: conn.BaseUrl,
		Active: conn.Active, APIVersion: conn.ApiVersion, APIKeySet: conn.ApiKeyEnc != "",
		CreatedAt: ts(conn.CreatedAt), UpdatedAt: ts(conn.UpdatedAt),
	}
	if v.APIKeySet {
		m := MaskedAPIKey
		v.APIKeyMasked = &m
	}
	if conn.DefaultWarehouseID.Valid {
		wh, err := s.q.GetWarehouseByID(ctx, db.GetWarehouseByIDParams{
			ID: conn.DefaultWarehouseID.Int64, OrganizationID: conn.OrganizationID,
		})
		if err != nil {
			return ConnectionView{}, fmt.Errorf("glorian admin: default warehouse: %w", err)
		}
		v.DefaultWarehouseUUID = &wh.Uuid
	}
	return v, nil
}

// Get returns the connection of the brand; an unconfigured brand gets the
// defaults with Configured false.
func (s *Service) Get(ctx context.Context, brand db.Brand) (ConnectionView, error) {
	conn, err := s.connection(ctx, brand.ID)
	if errors.Is(err, ErrNotConfigured) {
		return ConnectionView{Key: glorian.ConnectionKey, APIVersion: glorian.APIVersion}, nil
	}
	if err != nil {
		return ConnectionView{}, err
	}
	return s.view(ctx, conn)
}

// PutInput replaces the connection settings. APIKey nil or blank keeps
// the stored key; DefaultWarehouseUUID nil clears it.
type PutInput struct {
	BaseURL              string     `json:"base_url"`
	Active               *bool      `json:"active"`
	APIVersion           *string    `json:"api_version"`
	DefaultWarehouseUUID *uuid.UUID `json:"default_warehouse_uuid"`
	APIKey               *string    `json:"api_key"`
}

func (in PutInput) validate() (baseURL, apiKey, version string, err error) {
	baseURL = strings.TrimSpace(in.BaseURL)
	switch u, perr := url.Parse(baseURL); {
	case baseURL == "":
		return "", "", "", invalid("base_url", "is required")
	case len(baseURL) > maxBaseURL:
		return "", "", "", invalid("base_url", fmt.Sprintf("must be at most %d characters", maxBaseURL))
	case perr != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "":
		return "", "", "", invalid("base_url", "must be an http(s) URL")
	}
	if in.APIKey != nil {
		apiKey = strings.TrimSpace(*in.APIKey)
		if len(apiKey) > maxAPIKey {
			return "", "", "", invalid("api_key", fmt.Sprintf("must be at most %d characters", maxAPIKey))
		}
	}
	version = glorian.APIVersion
	if in.APIVersion != nil {
		version = strings.TrimSpace(*in.APIVersion)
		if version != glorian.APIVersion {
			return "", "", "", invalid("api_version", "must be "+glorian.APIVersion)
		}
	}
	return baseURL, apiKey, version, nil
}

// Put creates or replaces the connection of the brand. created reports a
// new row.
func (s *Service) Put(ctx context.Context, brand db.Brand, in PutInput) (view ConnectionView, created bool, err error) {
	baseURL, apiKey, version, err := in.validate()
	if err != nil {
		return ConnectionView{}, false, err
	}
	active := in.Active != nil && *in.Active
	conn, err := s.connection(ctx, brand.ID)
	exists := err == nil
	if err != nil && !errors.Is(err, ErrNotConfigured) {
		return ConnectionView{}, false, err
	}
	if active && apiKey == "" && (!exists || conn.ApiKeyEnc == "") {
		return ConnectionView{}, false, invalid("api_key", "is required to activate the connection")
	}

	orgID := conn.OrganizationID
	if !exists {
		center, err := s.q.GetBrandCenter(ctx, brand.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ConnectionView{}, false, ErrCenterMissing
		}
		if err != nil {
			return ConnectionView{}, false, err
		}
		orgID = center.ID
	}
	var warehouseID *int64
	if in.DefaultWarehouseUUID != nil {
		wh, err := s.q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: *in.DefaultWarehouseUUID, OrganizationID: orgID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ConnectionView{}, false, invalid("default_warehouse_uuid", "is not a warehouse of the brand center")
		}
		if err != nil {
			return ConnectionView{}, false, err
		}
		warehouseID = &wh.ID
	}

	if !exists {
		conn, err = s.store.Create(ctx, glorian.ConnectionInput{
			OrganizationID: orgID, BrandID: brand.ID, Key: glorian.ConnectionKey, BaseURL: baseURL,
			APIKey: apiKey, DefaultWarehouseID: warehouseID, Active: active, APIVersion: version,
		})
	} else {
		if apiKey != "" {
			if conn, err = s.store.SetAPIKey(ctx, conn.ID, apiKey); err != nil {
				return ConnectionView{}, false, err
			}
		}
		conn, err = s.store.Update(ctx, conn.ID, glorian.ConnectionUpdate{
			BaseURL: baseURL, DefaultWarehouseID: warehouseID, Active: active, APIVersion: version,
		})
	}
	if err != nil {
		return ConnectionView{}, false, err
	}
	view, err = s.view(ctx, conn)
	return view, !exists, err
}

// TestResult is the outcome of a connection ping (GET product-categories,
// one row). A failed ping is a result, not an error.
type TestResult struct {
	OK         bool    `json:"ok"`
	DurationMS int64   `json:"duration_ms"`
	HTTPStatus *int    `json:"http_status"`
	Code       *string `json:"code"`
	Message    *string `json:"message"`
}

// Test codes of a ping that did not reach a hub answer.
const (
	TestCodeMisconfigured = "MISCONFIGURED"
	TestCodeTransport     = "TRANSPORT_ERROR"
)

// Test pings the hub with the stored settings, active or not.
func (s *Service) Test(ctx context.Context, brand db.Brand) (TestResult, error) {
	conn, err := s.connection(ctx, brand.ID)
	if err != nil {
		return TestResult{}, err
	}
	c, err := s.store.Decrypt(conn)
	if err != nil {
		return TestResult{}, err
	}
	if strings.TrimSpace(c.BaseURL) == "" || strings.TrimSpace(c.APIKey) == "" {
		return failed(0, TestCodeMisconfigured, "base_url and api_key are required"), nil
	}
	client, err := s.factory(c)
	if err != nil {
		return failed(0, TestCodeMisconfigured, err.Error()), nil
	}
	start := s.now()
	_, err = client.ListCategories(ctx, glorian.ListParams{PerPage: 1})
	took := s.now().Sub(start).Milliseconds()
	if err == nil {
		return TestResult{OK: true, DurationMS: took}, nil
	}
	var apiErr *glorian.APIError
	if errors.As(err, &apiErr) {
		res := failed(took, apiErr.Code, apiErr.Error())
		status := apiErr.Status
		res.HTTPStatus = &status
		return res, nil
	}
	return failed(took, TestCodeTransport, err.Error()), nil
}

func failed(took int64, code, msg string) TestResult {
	return TestResult{DurationMS: took, Code: &code, Message: &msg}
}

// SyncRunView is one integration_sync_runs row.
type SyncRunView struct {
	UUID       uuid.UUID       `json:"uuid"`
	Kind       string          `json:"kind"`
	Status     string          `json:"status"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at"`
	Watermark  *time.Time      `json:"watermark"`
	Counts     json.RawMessage `json:"counts"`
	Error      *string         `json:"error"`
}

func syncRunView(r db.IntegrationSyncRun) SyncRunView {
	v := SyncRunView{
		UUID: r.Uuid, Kind: r.Kind, Status: r.Status, StartedAt: r.StartedAt.Time,
		FinishedAt: ts(r.FinishedAt), Watermark: ts(r.Watermark), Counts: json.RawMessage(r.Counts),
	}
	if len(v.Counts) == 0 {
		v.Counts = json.RawMessage("{}")
	}
	if r.Error.Valid {
		e := r.Error.String
		v.Error = &e
	}
	return v
}

// SyncRunKinds are the kinds of chk_integration_sync_runs_kind.
var SyncRunKinds = []string{
	glorian.KindPullCategories, glorian.KindPullProducts, glorian.KindPullDealers, glorian.KindPullStock,
	glorian.KindPushBarcodes, glorian.KindOutbound, glorian.KindReconcile,
}

// SyncRunStatuses are the statuses of a sync run.
var SyncRunStatuses = []string{glorian.RunRunning, glorian.RunSucceeded, glorian.RunFailed}

// SyncRunFilter narrows ListSyncRuns; empty fields do not filter.
type SyncRunFilter struct {
	Kind   string
	Status string
	Limit  int
}

// ListSyncRuns lists the runs of the connection, newest first.
func (s *Service) ListSyncRuns(ctx context.Context, brand db.Brand, f SyncRunFilter) ([]SyncRunView, error) {
	if f.Kind != "" && !contains(SyncRunKinds, f.Kind) {
		return nil, invalid("kind", "must be one of "+strings.Join(SyncRunKinds, ", "))
	}
	if f.Status != "" && !contains(SyncRunStatuses, f.Status) {
		return nil, invalid("status", "must be one of "+strings.Join(SyncRunStatuses, ", "))
	}
	conn, err := s.connection(ctx, brand.ID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListGlorianSyncRuns(ctx, db.ListGlorianSyncRunsParams{
		ConnectionID: conn.ID, Kind: optText(f.Kind), Status: optText(f.Status), RowLimit: int32(f.Limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]SyncRunView, 0, len(rows))
	for _, r := range rows {
		out = append(out, syncRunView(r))
	}
	return out, nil
}

// GetSyncRun returns one run of the connection with its counts (the
// reconcile details live there).
func (s *Service) GetSyncRun(ctx context.Context, brand db.Brand, id uuid.UUID) (SyncRunView, error) {
	conn, err := s.connection(ctx, brand.ID)
	if err != nil {
		return SyncRunView{}, err
	}
	row, err := s.q.GetGlorianSyncRunByUUID(ctx, db.GetGlorianSyncRunByUUIDParams{Uuid: id, ConnectionID: conn.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return SyncRunView{}, ErrNotFound
	}
	if err != nil {
		return SyncRunView{}, err
	}
	return syncRunView(row), nil
}

// SyncKinds are the kinds POST /sync-runs accepts.
var SyncKinds = []string{SyncPull, SyncPushBarcodes, SyncOutboundReplay}

// SyncQueued answers a manual sync: the task is queued, its runs show up
// in the sync run list when the worker takes it.
type SyncQueued struct {
	Kind string `json:"kind"`
	Task string `json:"task"`
}

// TriggerSync queues a manual pull (default), barcode push or held
// outbound replay of the connection.
func (s *Service) TriggerSync(ctx context.Context, brand db.Brand, kind string) (SyncQueued, error) {
	if kind == "" {
		kind = SyncPull
	}
	if !contains(SyncKinds, kind) {
		return SyncQueued{}, invalid("kind", "must be one of "+strings.Join(SyncKinds, ", "))
	}
	conn, err := s.connection(ctx, brand.ID)
	if err != nil {
		return SyncQueued{}, err
	}
	if !conn.Active {
		return SyncQueued{}, ErrInactive
	}
	if s.queue == nil {
		return SyncQueued{}, ErrQueueUnavailable
	}
	var (
		task *asynq.Task
		opts []asynq.Option
	)
	switch kind {
	case SyncPushBarcodes:
		task, err = queue.NewGlorianPushBarcodesTask(conn.ID)
		// Same debounce window as the placement listener: a pending push
		// of the window is reused.
		opts = queue.GlorianPushBarcodesOpts(conn.ID, s.now())
	case SyncOutboundReplay:
		task, err = queue.NewGlorianOrderReplayTask(conn.ID)
		opts = queue.GlorianOrderReplayOpts()
	default:
		// The pull task covers every active glorian connection (one per
		// brand); unique for a minute against double clicks.
		task, err = queue.NewGlorianPullCatalogTask()
		opts = []asynq.Option{asynq.Queue(queue.QueueMaintenance), asynq.MaxRetry(0),
			asynq.Timeout(30 * time.Minute), asynq.Unique(time.Minute)}
	}
	if err != nil {
		return SyncQueued{}, err
	}
	if err := s.enqueue(task, opts); err != nil {
		return SyncQueued{}, err
	}
	return SyncQueued{Kind: kind, Task: task.Type()}, nil
}

// enqueue queues a task; a duplicate of a pending one counts as queued.
func (s *Service) enqueue(task *asynq.Task, opts []asynq.Option) error {
	if s.queue == nil {
		return ErrQueueUnavailable
	}
	_, err := s.queue.Enqueue(task, opts...)
	if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

// OutboundView is one order outbound with its order.
type OutboundView struct {
	UUID              uuid.UUID `json:"uuid"`
	OrderUUID         uuid.UUID `json:"order_uuid"`
	OrderNo           string    `json:"order_no"`
	OrderStatus       string    `json:"order_status"`
	ExternalReference string    `json:"external_reference"`
	State             string    `json:"state"`
	HeldReason        *string   `json:"held_reason"`
	Attempts          int32     `json:"attempts"`
	LastError         *string   `json:"last_error"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func outboundView(r db.GetGlorianOutboundByUUIDRow) OutboundView {
	return OutboundView{
		UUID: r.Uuid, OrderUUID: r.OrderUuid, OrderNo: r.OrderNo, OrderStatus: r.OrderStatus,
		ExternalReference: r.ExternalReference, State: r.State, HeldReason: text(r.HeldReason),
		Attempts: r.Attempts, LastError: text(r.LastError), CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

// OutboundStates are the states of chk_order_outbounds_state.
var OutboundStates = []string{
	glorian.OutboundPending, glorian.OutboundHeld, glorian.OutboundSent, glorian.OutboundFailed, glorian.OutboundCancelled,
}

// ListOutbounds lists the outbounds of the connection in one state
// (default held), oldest first.
func (s *Service) ListOutbounds(ctx context.Context, brand db.Brand, state string, limit int) ([]OutboundView, error) {
	if state == "" {
		state = glorian.OutboundHeld
	}
	if !contains(OutboundStates, state) {
		return nil, invalid("state", "must be one of "+strings.Join(OutboundStates, ", "))
	}
	conn, err := s.connection(ctx, brand.ID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListGlorianOutbounds(ctx, db.ListGlorianOutboundsParams{
		ConnectionID: conn.ID, State: state, RowLimit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]OutboundView, 0, len(rows))
	for _, r := range rows {
		out = append(out, outboundView(db.GetGlorianOutboundByUUIDRow(r)))
	}
	return out, nil
}

// ReplayOutbound queues the replay of one held or failed outbound. The
// worker re-reads the order and the hub order, so a replay never repeats
// a step.
func (s *Service) ReplayOutbound(ctx context.Context, brand db.Brand, id uuid.UUID) (OutboundView, error) {
	conn, err := s.connection(ctx, brand.ID)
	if err != nil {
		return OutboundView{}, err
	}
	row, err := s.q.GetGlorianOutboundByUUID(ctx, db.GetGlorianOutboundByUUIDParams{Uuid: id, ConnectionID: conn.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return OutboundView{}, ErrNotFound
	}
	if err != nil {
		return OutboundView{}, err
	}
	if row.State != glorian.OutboundHeld && row.State != glorian.OutboundFailed {
		return OutboundView{}, ErrNotReplayable
	}
	task, err := queue.NewGlorianOutboundReplayOneTask(row.ID)
	if err != nil {
		return OutboundView{}, err
	}
	if err := s.enqueue(task, queue.GlorianOutboundReplayOneOpts(row.ID)); err != nil {
		return OutboundView{}, err
	}
	return outboundView(row), nil
}

// Reconcile opens a reconcile sync run and queues the drift report; the
// run is returned running, GET /sync-runs/{uuid} shows the result.
func (s *Service) Reconcile(ctx context.Context, brand db.Brand) (SyncRunView, error) {
	conn, err := s.connection(ctx, brand.ID)
	if err != nil {
		return SyncRunView{}, err
	}
	if s.queue == nil {
		return SyncRunView{}, ErrQueueUnavailable
	}
	run, err := s.reconciler.StartRun(ctx, conn)
	if err != nil {
		return SyncRunView{}, err
	}
	task, err := queue.NewGlorianReconcileTask(run.ID)
	if err == nil {
		err = s.enqueue(task, queue.GlorianReconcileOpts(run.ID))
	}
	if err != nil {
		// Do not leave a run that nothing will finish.
		_, _ = s.q.FinishIntegrationSyncRun(context.WithoutCancel(ctx), db.FinishIntegrationSyncRunParams{
			ID: run.ID, Status: glorian.RunFailed, Counts: []byte("{}"),
			Error: pgtype.Text{String: "enqueue failed", Valid: true},
		})
		return SyncRunView{}, err
	}
	return syncRunView(run), nil
}

// ClampLimit maps a requested list size onto 1..MaxListLimit (0: default).
func ClampLimit(n int) int {
	switch {
	case n <= 0:
		return DefaultListLimit
	case n > MaxListLimit:
		return MaxListLimit
	}
	return n
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func optText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func text(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

func ts(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
