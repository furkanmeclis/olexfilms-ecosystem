package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	_ ActionStore = (*repository.Store)(nil)
	_ ToolGate    = (*tools.Registry)(nil)
)

// --- fakes -------------------------------------------------------------------

// fakeStore keeps ai_pending_actions in memory with the same conditional
// UPDATE semantics as the SQL queries (one mutex = row lock).
type fakeStore struct {
	mu     sync.Mutex
	now    func() time.Time
	rows   []*db.AiPendingAction
	nextID int64
}

func (s *fakeStore) find(pred func(*db.AiPendingAction) bool) *db.AiPendingAction {
	for _, r := range s.rows {
		if pred(r) {
			return r
		}
	}
	return nil
}

func (s *fakeStore) CreatePendingAction(_ context.Context, p db.CreateAIPendingActionParams) (db.AiPendingAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.find(func(r *db.AiPendingAction) bool { return r.IdempotencyKey == p.IdempotencyKey }) != nil {
		return db.AiPendingAction{}, errors.New("duplicate idempotency key")
	}
	s.nextID++
	now := pgtype.Timestamptz{Time: s.now(), Valid: true}
	r := &db.AiPendingAction{
		ID: s.nextID, Uuid: uuid.New(), OrganizationID: p.OrganizationID, BrandID: p.BrandID, UserID: p.UserID,
		Source: p.Source, SourceRef: p.SourceRef, ToolUseID: p.ToolUseID, ToolName: p.ToolName, Input: p.Input,
		Preview: p.Preview, Status: model.ActionPending, IdempotencyKey: p.IdempotencyKey, ExpiresAt: p.ExpiresAt,
		CreatedAt: now, UpdatedAt: now,
	}
	s.rows = append(s.rows, r)
	return *r, nil
}

func (s *fakeStore) one(r *db.AiPendingAction) (db.AiPendingAction, bool, error) {
	if r == nil {
		return db.AiPendingAction{}, false, nil
	}
	return *r, true, nil
}

func (s *fakeStore) PendingActionByKey(_ context.Context, key string) (db.AiPendingAction, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.one(s.find(func(r *db.AiPendingAction) bool { return r.IdempotencyKey == key }))
}

func (s *fakeStore) mine(id uuid.UUID, orgID, userID int64) func(*db.AiPendingAction) bool {
	return func(r *db.AiPendingAction) bool {
		return r.Uuid == id && r.OrganizationID == orgID && r.UserID == userID
	}
}

func (s *fakeStore) PendingActionForUser(_ context.Context, id uuid.UUID, orgID, userID int64) (db.AiPendingAction, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.one(s.find(s.mine(id, orgID, userID)))
}

func (s *fakeStore) ListPendingActions(_ context.Context, orgID, userID int64) ([]db.AiPendingAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []db.AiPendingAction
	for _, r := range s.rows {
		if r.OrganizationID == orgID && r.UserID == userID && r.Status == model.ActionPending && r.ExpiresAt.Time.After(s.now()) {
			out = append(out, *r)
		}
	}
	return out, nil
}

func (s *fakeStore) set(r *db.AiPendingAction, status string) {
	r.Status = status
	r.UpdatedAt = pgtype.Timestamptz{Time: s.now(), Valid: true}
	if status != model.ActionPending && status != model.ActionExecuting {
		r.ResolvedAt = r.UpdatedAt
	}
}

func (s *fakeStore) ClaimPendingAction(_ context.Context, id uuid.UUID, orgID, userID int64, input, preview []byte) (db.AiPendingAction, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.find(s.mine(id, orgID, userID))
	if r == nil || r.Status != model.ActionPending || !r.ExpiresAt.Time.After(s.now()) {
		return db.AiPendingAction{}, false, nil
	}
	s.set(r, model.ActionExecuting)
	if input != nil {
		r.Input = input
	}
	if preview != nil {
		r.Preview = preview
	}
	return *r, true, nil
}

func (s *fakeStore) ResolvePendingAction(_ context.Context, p db.ResolveAIPendingActionParams) (db.AiPendingAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.find(func(r *db.AiPendingAction) bool { return r.ID == p.ID && r.Status == model.ActionExecuting })
	if r == nil {
		return db.AiPendingAction{}, errors.New("no rows")
	}
	s.set(r, p.Status)
	r.Result, r.Error = p.Result, p.Error
	return *r, nil
}

func (s *fakeStore) CancelPendingAction(_ context.Context, id uuid.UUID, orgID, userID int64) (db.AiPendingAction, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.find(s.mine(id, orgID, userID))
	if r == nil || r.Status != model.ActionPending {
		return db.AiPendingAction{}, false, nil
	}
	s.set(r, model.ActionCancelled)
	return *r, true, nil
}

func (s *fakeStore) CancelPendingActionsForSource(_ context.Context, p db.CancelAIPendingActionsForSourceParams) ([]db.AiPendingAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []db.AiPendingAction
	for _, r := range s.rows {
		if r.Source == p.Source && r.SourceRef == p.SourceRef && r.OrganizationID == p.OrganizationID &&
			r.UserID == p.UserID && r.Status == model.ActionPending {
			s.set(r, model.ActionCancelled)
			out = append(out, *r)
		}
	}
	return out, nil
}

func (s *fakeStore) ExpirePendingAction(_ context.Context, id int64, now time.Time) (db.AiPendingAction, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.find(func(r *db.AiPendingAction) bool {
		return r.ID == id && r.Status == model.ActionPending && !r.ExpiresAt.Time.After(now)
	})
	if r != nil {
		s.set(r, model.ActionExpired)
	}
	return s.one(r)
}

func (s *fakeStore) ExpirePendingActions(_ context.Context, now time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for _, r := range s.rows {
		if r.Status == model.ActionPending && !r.ExpiresAt.Time.After(now) {
			s.set(r, model.ActionExpired)
			n++
		}
	}
	return n, nil
}

func (s *fakeStore) FailStalePendingActions(_ context.Context, before time.Time, errText string) ([]db.AiPendingAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []db.AiPendingAction
	for _, r := range s.rows {
		if r.Status == model.ActionExecuting && !r.UpdatedAt.Time.After(before) {
			s.set(r, model.ActionFailed)
			r.Error = pgtype.Text{String: errText, Valid: true}
			out = append(out, *r)
		}
	}
	return out, nil
}

func (s *fakeStore) row(t *testing.T, id uuid.UUID) db.AiPendingAction {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.find(func(r *db.AiPendingAction) bool { return r.Uuid == id })
	if r == nil {
		t.Fatalf("no action %s", id)
	}
	return *r
}

// noteTool is a write tool whose Run stands for the module repository
// write: writes counts every call.
type noteTool struct {
	writes *atomic.Int32
	delay  time.Duration
	runErr error
}

func (noteTool) Spec() tools.Spec {
	return tools.Spec{
		Name: "create_note", Description: "test write tool", Kind: tools.KindWrite, Realm: tools.RealmPanel,
		Permissions: []string{rbac.PermTasksWrite},
		InputSchema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"title"},
			"properties": map[string]any{
				"title":    map[string]any{"type": "string", "minLength": 1, "maxLength": 20},
				"priority": map[string]any{"type": "string", "enum": []string{"low", "high"}},
				"due_date": map[string]any{"type": "string", "format": "date"},
				"minutes":  map[string]any{"type": "integer", "minimum": 15, "maximum": 120},
			}},
	}
}

type noteInput struct {
	Title    string `json:"title"`
	Priority string `json:"priority,omitempty"`
	DueDate  string `json:"due_date,omitempty"`
	Minutes  int    `json:"minutes,omitempty"`
}

func (t noteTool) Propose(_ context.Context, _ tools.Env, raw json.RawMessage) (tools.Proposal, *tools.Result, error) {
	var in noteInput
	if err := tools.Decode(t.Spec().InputSchema, raw, &in); err != nil {
		r := tools.ErrorResult(tools.CodeInvalidInput, err.Error())
		return tools.Proposal{}, &r, nil
	}
	if strings.HasPrefix(in.Title, "!") {
		r := tools.ErrorResult(tools.CodeInvalidInput, "invalid input: title must not start with !")
		return tools.Proposal{}, &r, nil
	}
	out, _ := json.Marshal(in)
	return tools.Proposal{Input: out, Preview: tools.Preview{
		Summary: "Create note " + in.Title,
		Fields:  []tools.Field{{Key: "title", Value: in.Title}},
		Edit: []tools.EditField{
			{Key: "title", Type: tools.EditText, Value: in.Title, Required: true},
			{Key: "priority", Type: tools.EditSelect, Value: in.Priority, Options: []string{"low", "high"}},
			{Key: "due_date", Type: tools.EditDate, Value: in.DueDate},
			{Key: "minutes", Type: tools.EditNumber},
		},
	}}, nil, nil
}

func (t noteTool) Run(_ context.Context, _ tools.Env, raw json.RawMessage) (tools.Result, error) {
	time.Sleep(t.delay)
	if t.runErr != nil {
		return tools.Result{}, t.runErr
	}
	t.writes.Add(1)
	id := uuid.New()
	return tools.Result{Content: `{"ok":true,"input":` + string(raw) + `}`, Link: &tools.Link{Kind: "note", UUID: id.String()}}, nil
}

type recorded struct {
	actor   int64
	action  string
	payload map[string]any
	origin  activity.Origin
}

type fakeActivity struct {
	mu     sync.Mutex
	events []recorded
}

func (f *fakeActivity) Record(ctx context.Context, actorID *int64, action, _ string, _ *uuid.UUID, payload map[string]any, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, _ := activity.OriginFrom(ctx)
	f.events = append(f.events, recorded{actor: *actorID, action: action, payload: payload, origin: o})
}

// --- harness -----------------------------------------------------------------

type harness struct {
	t        *testing.T
	ctx      context.Context
	now      time.Time
	store    *fakeStore
	writes   *atomic.Int32
	activity *fakeActivity
	actions  *Actions
	reg      *tools.Registry
}

func newHarness(t *testing.T, tool func(*atomic.Int32) tools.Tool) *harness {
	h := &harness{t: t, ctx: context.Background(), now: time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC),
		writes: &atomic.Int32{}, activity: &fakeActivity{}}
	clock := func() time.Time { return h.now }
	h.store = &fakeStore{now: clock}
	if tool == nil {
		tool = func(w *atomic.Int32) tools.Tool { return noteTool{writes: w} }
	}
	h.reg = tools.NewRegistry(nil)
	h.reg.Register(tool(h.writes))
	h.actions = NewActions(h.store, h.reg, h.activity, nil)
	h.actions.SetClock(clock)
	return h
}

func user(internal int64, perms ...string) tools.Principal {
	scopes := map[string]rbac.Scope{}
	for _, p := range perms {
		scopes[p] = rbac.ScopeManaged
	}
	return tools.Principal{
		Auth:  authctx.Principal{UserID: uuid.New(), UserInternal: internal, PermissionScopes: scopes},
		Org:   &orgctx.Scope{InternalID: 10, OrgType: "center", BrandID: 1, BrandSlug: "olex"},
		Realm: tools.RealmPanel,
	}
}

func (h *harness) propose(p tools.Principal, toolUseID, input string) Card {
	h.t.Helper()
	out, err := h.actions.Propose(h.ctx, ProposeCall{Principal: p, Source: model.SourcePanel, SourceRef: "conv-1",
		ToolUseID: toolUseID, ToolName: "create_note", Input: json.RawMessage(input)})
	if err != nil || out.Card == nil {
		h.t.Fatalf("propose: %+v %v", out, err)
	}
	return *out.Card
}

func wantStatus(t *testing.T, err error, status int, code string) {
	t.Helper()
	got, gotCode, ok := ErrorStatus(err)
	if !ok || got != status || gotCode != code {
		t.Fatalf("err %v: want %d %s, got %d %s", err, status, code, got, gotCode)
	}
}

// --- acceptance --------------------------------------------------------------

// TEC-387 acceptance: no write happens without a confirmation. Propose
// only stores the card; Call refuses a write tool; cancel and a new
// message resolve cards without running; only Confirm writes, once.
func TestNoWriteWithoutConfirmation(t *testing.T) {
	h := newHarness(t, nil)
	owner := user(7, rbac.PermTasksWrite)

	res, err := h.reg.Call(h.ctx, owner, "create_note", json.RawMessage(`{"title":"x"}`))
	if !errors.Is(err, tools.ErrConfirmationRequired) || res.Code != tools.CodeConfirmationRequired {
		t.Fatalf("Call on a write tool: %+v %v", res, err)
	}

	card := h.propose(owner, "toolu_1", `{"title":"Call dealer"}`)
	if card.Status != model.ActionPending || card.Preview.Action != "create_note" || card.Preview.Summary == "" ||
		!card.ExpiresAt.Equal(h.now.Add(30*time.Minute)) {
		t.Fatalf("card: %+v", card)
	}
	if again := h.propose(owner, "toolu_1", `{"title":"Call dealer"}`); again.ActionUUID != card.ActionUUID {
		t.Fatalf("same tool_use proposed twice made a second card")
	}
	if list, _ := h.actions.ListPending(h.ctx, owner); len(list) != 1 {
		t.Fatalf("pending list: %d", len(list))
	}
	// Rejected proposals come back as model results, not cards.
	bad, err := h.actions.Propose(h.ctx, ProposeCall{Principal: owner, Source: model.SourcePanel, SourceRef: "conv-1",
		ToolUseID: "toolu_bad", ToolName: "create_note", Input: json.RawMessage(`{"title":""}`)})
	if err != nil || bad.Card != nil || bad.Result == nil || bad.Result.Code != tools.CodeInvalidInput {
		t.Fatalf("invalid proposal: %+v %v", bad, err)
	}
	denied, err := h.actions.Propose(h.ctx, ProposeCall{Principal: user(7), Source: model.SourcePanel,
		ToolUseID: "toolu_denied", ToolName: "create_note", Input: json.RawMessage(`{"title":"x"}`)})
	if err != nil || denied.Result == nil || denied.Result.Code != tools.CodeToolNotAllowed {
		t.Fatalf("proposal without permission: %+v %v", denied, err)
	}

	cancelled, err := h.actions.Cancel(h.ctx, owner, card.ActionUUID)
	if err != nil || cancelled.Status != model.ActionCancelled || !cancelled.ToolResult.IsError {
		t.Fatalf("cancel: %+v %v", cancelled, err)
	}
	if _, err := h.actions.Confirm(h.ctx, owner, card.ActionUUID, nil); !errors.Is(err, ErrActionResolved) {
		t.Fatalf("confirm after cancel: %v", err)
	}
	wantStatus(t, ErrActionResolved, http.StatusConflict, CodeActionResolved)

	moved := h.propose(owner, "toolu_2", `{"title":"Second"}`)
	outs, err := h.actions.CancelForSource(h.ctx, owner, model.SourcePanel, "conv-1")
	if err != nil || len(outs) != 1 || outs[0].ActionUUID != moved.ActionUUID || outs[0].Status != model.ActionCancelled {
		t.Fatalf("new message: %+v %v", outs, err)
	}
	if n := h.writes.Load(); n != 0 {
		t.Fatalf("writes without confirmation = %d, want 0", n)
	}

	third := h.propose(owner, "toolu_3", `{"title":"Third"}`)
	out, err := h.actions.Confirm(h.ctx, owner, third.ActionUUID, nil)
	if err != nil || out.Status != model.ActionConfirmed || out.Link == nil || out.ToolResult.IsError {
		t.Fatalf("confirm: %+v %v", out, err)
	}
	if n := h.writes.Load(); n != 1 {
		t.Fatalf("writes after one confirmation = %d, want 1", n)
	}
	row := h.store.row(t, third.ActionUUID)
	if row.Status != model.ActionConfirmed || !row.ResolvedAt.Valid || !strings.Contains(string(row.Result), `"ok":true`) {
		t.Fatalf("row: %+v", row)
	}
	// Activity log: actor + via = ai.
	if len(h.activity.events) != 1 {
		t.Fatalf("activity events: %+v", h.activity.events)
	}
	ev := h.activity.events[0]
	if ev.actor != 7 || ev.action != "ai.action.confirmed" || ev.origin.Via != "ai" ||
		ev.origin.ActionUUID != third.ActionUUID.String() || ev.payload["tool"] != "create_note" {
		t.Fatalf("activity: %+v", ev)
	}
}

// TEC-387 acceptance: two parallel confirmations of one action run it
// once; the losers get AI_ACTION_RESOLVED.
func TestParallelConfirmRunsOnce(t *testing.T) {
	h := newHarness(t, func(w *atomic.Int32) tools.Tool { return noteTool{writes: w, delay: 20 * time.Millisecond} })
	owner := user(7, rbac.PermTasksWrite)
	card := h.propose(owner, "toolu_p", `{"title":"Once"}`)

	const n = 8
	var (
		wg       sync.WaitGroup
		ok, lost atomic.Int32
		start    = make(chan struct{})
	)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := h.actions.Confirm(h.ctx, owner, card.ActionUUID, nil)
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrActionResolved):
				lost.Add(1)
			default:
				t.Errorf("confirm: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if ok.Load() != 1 || lost.Load() != n-1 || h.writes.Load() != 1 {
		t.Fatalf("ok=%d lost=%d writes=%d, want 1/%d/1", ok.Load(), lost.Load(), h.writes.Load(), n-1)
	}
}

// TEC-387 acceptance: only the user who started the action can confirm or
// cancel it; anyone else gets 404.
func TestOtherUserCannotConfirm(t *testing.T) {
	h := newHarness(t, nil)
	owner := user(7, rbac.PermTasksWrite)
	card := h.propose(owner, "toolu_o", `{"title":"Mine"}`)

	other := user(8, rbac.PermTasksWrite)
	otherOrg := user(7, rbac.PermTasksWrite)
	otherOrg.Org = &orgctx.Scope{InternalID: 11, OrgType: "dealer", BrandID: 1}
	for name, p := range map[string]tools.Principal{"other user": other, "same user other org": otherOrg} {
		_, err := h.actions.Confirm(h.ctx, p, card.ActionUUID, nil)
		if !errors.Is(err, ErrActionNotFound) {
			t.Fatalf("%s confirm: %v", name, err)
		}
		wantStatus(t, err, http.StatusNotFound, "NOT_FOUND")
		if _, err := h.actions.Cancel(h.ctx, p, card.ActionUUID); !errors.Is(err, ErrActionNotFound) {
			t.Fatalf("%s cancel: %v", name, err)
		}
	}
	if _, err := h.actions.Confirm(h.ctx, owner, uuid.New(), nil); !errors.Is(err, ErrActionNotFound) {
		t.Fatalf("unknown action: %v", err)
	}
	if h.writes.Load() != 0 || h.store.row(t, card.ActionUUID).Status != model.ActionPending {
		t.Fatalf("foreign confirmation changed something")
	}
}

// TEC-387 acceptance: a confirmation in the 31st minute is refused with
// 422 AI_ACTION_EXPIRED and the card becomes expired; nothing runs.
func TestConfirmAfterExpiry(t *testing.T) {
	h := newHarness(t, nil)
	owner := user(7, rbac.PermTasksWrite)
	card := h.propose(owner, "toolu_e", `{"title":"Late"}`)

	h.now = h.now.Add(29 * time.Minute)
	if list, _ := h.actions.ListPending(h.ctx, owner); len(list) != 1 {
		t.Fatalf("card gone before its expiry")
	}
	h.now = h.now.Add(2 * time.Minute) // minute 31
	_, err := h.actions.Confirm(h.ctx, owner, card.ActionUUID, nil)
	if !errors.Is(err, ErrActionExpired) {
		t.Fatalf("confirm at minute 31: %v", err)
	}
	wantStatus(t, err, http.StatusUnprocessableEntity, CodeActionExpired)
	if row := h.store.row(t, card.ActionUUID); row.Status != model.ActionExpired || !row.ResolvedAt.Valid {
		t.Fatalf("row after late confirmation: %+v", row)
	}
	if _, err := h.actions.Confirm(h.ctx, owner, card.ActionUUID, nil); !errors.Is(err, ErrActionExpired) {
		t.Fatalf("second late confirmation: %v", err)
	}
	if _, err := h.actions.Cancel(h.ctx, owner, card.ActionUUID); !errors.Is(err, ErrActionExpired) {
		t.Fatalf("cancel after expiry: %v", err)
	}
	if h.writes.Load() != 0 {
		t.Fatalf("expired action ran")
	}
}

// TEC-387 acceptance: when the permission was revoked between the proposal
// and the confirmation, the action is failed and the caller gets 403;
// nothing runs.
func TestConfirmAfterPermissionRevoked(t *testing.T) {
	h := newHarness(t, nil)
	owner := user(7, rbac.PermTasksWrite)
	card := h.propose(owner, "toolu_r", `{"title":"Revoked"}`)

	revoked := owner
	revoked.Auth.PermissionScopes = map[string]rbac.Scope{}
	out, err := h.actions.Confirm(h.ctx, revoked, card.ActionUUID, nil)
	if !errors.Is(err, ErrActionForbidden) {
		t.Fatalf("confirm without permission: %v", err)
	}
	wantStatus(t, err, http.StatusForbidden, "FORBIDDEN")
	row := h.store.row(t, card.ActionUUID)
	if out.Status != model.ActionFailed || row.Status != model.ActionFailed || !strings.Contains(row.Error.String, "permission revoked") {
		t.Fatalf("outcome %+v row %+v", out, row)
	}
	if h.writes.Load() != 0 {
		t.Fatalf("revoked action ran")
	}
	if len(h.activity.events) != 1 || h.activity.events[0].action != "ai.action.failed" || h.activity.events[0].origin.Via != "ai" {
		t.Fatalf("activity: %+v", h.activity.events)
	}
	// Also with edits: the permission check comes first.
	card2 := h.propose(owner, "toolu_r2", `{"title":"Revoked2"}`)
	if _, err := h.actions.Confirm(h.ctx, revoked, card2.ActionUUID, map[string]any{"title": "x"}); !errors.Is(err, ErrActionForbidden) {
		t.Fatalf("edited confirm without permission: %v", err)
	}
}

// TEC-387 acceptance: the sweep moves expired pending actions to expired
// and actions executing for over 10 minutes to failed; a second run
// changes nothing.
func TestSweepRunsOnce(t *testing.T) {
	h := newHarness(t, nil)
	owner := user(7, rbac.PermTasksWrite)
	stale := h.propose(owner, "toolu_s1", `{"title":"Stale"}`)
	if _, ok, _ := h.store.ClaimPendingAction(h.ctx, stale.ActionUUID, 10, 7, nil, nil); !ok {
		t.Fatal("claim")
	}
	h.now = h.now.Add(31 * time.Minute)
	fresh := h.propose(owner, "toolu_s2", `{"title":"Fresh"}`)
	if _, ok, _ := h.store.ClaimPendingAction(h.ctx, fresh.ActionUUID, 10, 7, nil, nil); !ok {
		t.Fatal("claim fresh")
	}
	open := h.propose(owner, "toolu_s3", `{"title":"Open"}`)
	h.now = h.now.Add(-31 * time.Minute)
	expired := h.propose(owner, "toolu_s4", `{"title":"Expired"}`)
	h.now = h.now.Add(31 * time.Minute)

	res, err := h.actions.Sweep(h.ctx)
	if err != nil || res.Expired != 1 || res.Failed != 1 {
		t.Fatalf("first sweep: %+v %v", res, err)
	}
	res, err = h.actions.Sweep(h.ctx)
	if err != nil || res.Expired != 0 || res.Failed != 0 {
		t.Fatalf("second sweep must change nothing: %+v %v", res, err)
	}
	if r := h.store.row(t, stale.ActionUUID); r.Status != model.ActionFailed || !strings.Contains(r.Error.String, "outcome unknown") {
		t.Fatalf("stale: %+v", r)
	}
	if r := h.store.row(t, expired.ActionUUID); r.Status != model.ActionExpired {
		t.Fatalf("expired: %+v", r)
	}
	if h.store.row(t, fresh.ActionUUID).Status != model.ActionExecuting ||
		h.store.row(t, open.ActionUUID).Status != model.ActionPending {
		t.Fatalf("sweep touched live actions")
	}
	if err := h.actions.SweepTask(h.ctx); err != nil {
		t.Fatal(err)
	}
}

// TEC-387 acceptance: an edited field must fit the tool schema (400
// VALIDATION_ERROR, the card stays pending); a valid edit runs with the
// edited input and preview.
func TestEditedFieldMustFitSchema(t *testing.T) {
	h := newHarness(t, nil)
	owner := user(7, rbac.PermTasksWrite)
	card := h.propose(owner, "toolu_ed", `{"title":"Edit me"}`)

	for name, edits := range map[string]map[string]any{
		"required cleared":    {"title": "  "},
		"too long":            {"title": strings.Repeat("x", 21)},
		"not an option":       {"priority": "urgent"},
		"bad date":            {"due_date": "07.10.2026"},
		"not editable":        {"organization_id": "1"},
		"number out of range": {"minutes": "5"},
		"number as text":      {"minutes": "abc"},
		"wrong type":          {"title": map[string]any{"a": 1}},
		"tool rule":           {"title": "!nope"},
	} {
		_, err := h.actions.Confirm(h.ctx, owner, card.ActionUUID, edits)
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("%s: want a validation error, got %v", name, err)
		}
		wantStatus(t, err, http.StatusBadRequest, "VALIDATION_ERROR")
		if h.store.row(t, card.ActionUUID).Status != model.ActionPending {
			t.Fatalf("%s: card left pending state", name)
		}
	}
	if h.writes.Load() != 0 {
		t.Fatalf("invalid edits ran the tool")
	}

	out, err := h.actions.Confirm(h.ctx, owner, card.ActionUUID, map[string]any{
		"title": "Edited", "priority": "high", "minutes": "30", "due_date": "",
	})
	if err != nil || out.Status != model.ActionConfirmed {
		t.Fatalf("valid edit: %+v %v", out, err)
	}
	row := h.store.row(t, card.ActionUUID)
	var stored noteInput
	_ = json.Unmarshal(row.Input, &stored)
	if stored != (noteInput{Title: "Edited", Priority: "high", Minutes: 30}) || !strings.Contains(string(row.Preview), "Create note Edited") {
		t.Fatalf("stored input %+v preview %s", stored, row.Preview)
	}
	if !strings.Contains(out.ToolResult.Content, `"title":"Edited"`) {
		t.Fatalf("tool ran with %s", out.ToolResult.Content)
	}
}

// A run that fails is a failed outcome (not an error) with a model result.
func TestConfirmRunFailureIsFailedOutcome(t *testing.T) {
	h := newHarness(t, func(w *atomic.Int32) tools.Tool { return noteTool{writes: w, runErr: errors.New("db down")} })
	owner := user(7, rbac.PermTasksWrite)
	card := h.propose(owner, "toolu_f", `{"title":"Boom"}`)
	out, err := h.actions.Confirm(h.ctx, owner, card.ActionUUID, nil)
	if err != nil || out.Status != model.ActionFailed || !out.ToolResult.IsError {
		t.Fatalf("failed run: %+v %v", out, err)
	}
	if row := h.store.row(t, card.ActionUUID); row.Status != model.ActionFailed || !strings.Contains(row.Error.String, "db down") {
		t.Fatalf("row: %+v", row)
	}
	if len(h.activity.events) != 1 || h.activity.events[0].action != "ai.action.failed" {
		t.Fatalf("activity: %+v", h.activity.events)
	}
}
