package usecase

// TEC-398 (F4-02f): conversation bulk actions (bulk engine resource
// "conversations", POST /v1/conversations/bulk, conversations.manage). A
// platform resource (no organization): only platform admins hold the
// permission (S2). Every action is undoable from the bulk operation log.
//
//   - close: status = closed. Undo restores the previous status.
//   - assign: params.assignee_user_uuid (a platform admin). Undo restores
//     the previous assignee (the owner organization stays).
//   - set_ai_mode: params.ai_mode (auto | paused | off); paused without an
//     end time. Undo restores the mode and the pause end.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ResourceBulk is the bulk engine resource of conversations.
const ResourceBulk = "conversations"

// Bulk action ids and params.
const (
	BulkClose             = "close"
	BulkAssign            = "assign"
	BulkSetAIMode         = "set_ai_mode"
	ParamAssigneeUserUUID = "assignee_user_uuid"
	ParamAIMode           = "ai_mode"
	maxBulkQueryTargets   = 10000
)

// BulkAdapter implements bulkengine.BulkAdapter for conversations.
type BulkAdapter struct{ q *db.Queries }

// NewBulkAdapter creates the conversation bulk adapter.
func NewBulkAdapter(q *db.Queries) *BulkAdapter { return &BulkAdapter{q: q} }

// Resource implements bulkengine.BulkAdapter.
func (a *BulkAdapter) Resource() string { return ResourceBulk }

// WithQueries binds the adapter to the run transaction.
func (a *BulkAdapter) WithQueries(q *db.Queries) bulkengine.BulkAdapter { return &BulkAdapter{q: q} }

// BulkActions implements bulkengine.BulkAdapter.
func (a *BulkAdapter) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: BulkClose, LabelKey: "bulk.actions.conversations.close", Permission: rbac.PermConversationsManage,
			Reversible: true, ConfirmKey: "bulk.confirm.conversations.close",
		},
		{
			ID: BulkAssign, LabelKey: "bulk.actions.conversations.assign", Permission: rbac.PermConversationsManage, Reversible: true,
			Params: []bulkengine.BulkActionParam{
				{Key: ParamAssigneeUserUUID, Kind: "uuid", Required: true, LabelKey: "bulk.params.assignee"},
			},
		},
		{
			ID: BulkSetAIMode, LabelKey: "bulk.actions.conversations.set_ai_mode", Permission: rbac.PermConversationsManage, Reversible: true,
			Params: []bulkengine.BulkActionParam{
				{Key: ParamAIMode, Kind: "enum", Required: true, LabelKey: "bulk.params.ai_mode", Options: model.AIModes},
			},
		},
	}
}

func bulkParamsError(action string, params map[string]string) error {
	switch action {
	case BulkAssign:
		if _, err := uuid.Parse(strings.TrimSpace(params[ParamAssigneeUserUUID])); err != nil {
			return fmt.Errorf("%s must be a user uuid", ParamAssigneeUserUUID)
		}
	case BulkSetAIMode:
		if !slices.Contains(model.AIModes, strings.TrimSpace(params[ParamAIMode])) {
			return fmt.Errorf("%s must be one of %s", ParamAIMode, strings.Join(model.AIModes, ", "))
		}
	}
	return nil
}

// ResolveTargets implements bulkengine.BulkAdapter: selected uuids, or every
// conversation matching the GET /v1/conversations filters of target.query.
func (a *BulkAdapter) ResolveTargets(ctx context.Context, action string, target bulkengine.BulkTarget) ([]string, error) {
	if err := bulkParamsError(action, target.Params); err != nil {
		return nil, err
	}
	switch target.Scope {
	case "ids":
		out := make([]string, 0, len(target.IDs))
		for _, id := range target.IDs {
			if id = strings.TrimSpace(id); id != "" && !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
		if len(out) == 0 {
			return nil, errors.New("no target ids")
		}
		return out, nil
	case "query":
	default:
		return nil, errors.New("invalid target scope")
	}
	values := url.Values{}
	for k, v := range target.Query {
		values.Set(k, v)
	}
	f, err := ParseConversationFilter(ctx, a.q, values)
	if err != nil {
		return nil, err
	}
	f.Limit, f.Offset = maxBulkQueryTargets, 0
	rows, _, err := repository.FromQueries(a.q).ListConversations(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Uuid.String())
	}
	return out, nil
}

func (a *BulkAdapter) conversation(ctx context.Context, entityUUID string) (db.Conversation, error) {
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return db.Conversation{}, bulkengine.ErrEntityGone
	}
	c, err := a.q.GetConversationByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Conversation{}, bulkengine.ErrEntityGone
	}
	return c, err
}

func conversationState(action string, c db.Conversation) map[string]any {
	switch action {
	case BulkAssign:
		var v any
		if c.AssignedUserID.Valid {
			v = c.AssignedUserID.Int64
		}
		return map[string]any{"assigned_user_id": v}
	case BulkSetAIMode:
		return map[string]any{"ai_mode": c.AiMode}
	default:
		return map[string]any{"status": c.Status}
	}
}

// ApplyItem implements bulkengine.BulkAdapter.
func (a *BulkAdapter) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	run, ok := bulkengine.RunFrom(ctx)
	if !ok {
		return bulkengine.BulkItemResult{}, errors.New("bulk run context is required")
	}
	if err := bulkParamsError(action, run.Params); err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	fail := func(msg string) (bulkengine.BulkItemResult, error) {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: msg}, nil
	}
	cur, err := a.conversation(ctx, entityUUID)
	if errors.Is(err, bulkengine.ErrEntityGone) {
		return fail("not found")
	}
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	res := bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "conversation", OK: true, Op: "update",
		Previous: conversationState(action, cur),
	}
	switch action {
	case BulkClose:
		res.Applied = map[string]any{"status": model.StatusClosed}
		if cur.Status == model.StatusClosed {
			return res, nil
		}
		_, err = a.q.SetConversationStatus(ctx, db.SetConversationStatusParams{ID: cur.ID, Status: model.StatusClosed})
	case BulkAssign:
		id, _ := uuid.Parse(strings.TrimSpace(run.Params[ParamAssigneeUserUUID]))
		user, uerr := assignableUser(ctx, a.q, &id)
		if errors.Is(uerr, ErrAssigneeNotAllowed) || errors.Is(uerr, ErrInvalidRequest) {
			return fail(uerr.Error())
		}
		if uerr != nil {
			return bulkengine.BulkItemResult{}, uerr
		}
		res.Applied = map[string]any{"assigned_user_id": user.Int64}
		if cur.AssignedUserID == user {
			return res, nil
		}
		_, err = a.q.AssignConversation(ctx, db.AssignConversationParams{ID: cur.ID, AssignedUserID: user, AssignedOrgID: cur.AssignedOrgID})
	case BulkSetAIMode:
		mode := strings.TrimSpace(run.Params[ParamAIMode])
		res.Previous["ai_paused_until"] = tsPtr(cur.AiPausedUntil)
		res.Applied = map[string]any{"ai_mode": mode}
		if cur.AiMode == mode && !cur.AiPausedUntil.Valid {
			return res, nil
		}
		_, err = a.q.SetConversationAIMode(ctx, db.SetConversationAIModeParams{ID: cur.ID, AiMode: mode})
	default:
		return fail("unknown action")
	}
	if err != nil {
		return bulkengine.BulkItemResult{}, fmt.Errorf("conversations: bulk %s: %w", action, err)
	}
	return res, nil
}

// CurrentState implements bulkengine.StateReader (undo conflict check).
func (a *BulkAdapter) CurrentState(ctx context.Context, action, entityUUID string) (map[string]any, error) {
	cur, err := a.conversation(ctx, entityUUID)
	if err != nil {
		return nil, err
	}
	return conversationState(action, cur), nil
}

// RevertItem implements bulkengine.BulkAdapter.
func (a *BulkAdapter) RevertItem(ctx context.Context, action, entityUUID string, previous map[string]any) error {
	cur, err := a.conversation(ctx, entityUUID)
	if err != nil {
		return err
	}
	switch action {
	case BulkClose:
		status, _ := previous["status"].(string)
		if !slices.Contains(model.Statuses, status) {
			return errors.New("missing conversation status snapshot")
		}
		_, err = a.q.SetConversationStatus(ctx, db.SetConversationStatusParams{ID: cur.ID, Status: status})
	case BulkAssign:
		user := pgtype.Int8{}
		if n, ok := previous["assigned_user_id"].(float64); ok {
			user = pgtype.Int8{Int64: int64(n), Valid: true}
		} else if n, ok := previous["assigned_user_id"].(int64); ok {
			user = pgtype.Int8{Int64: n, Valid: true}
		}
		_, err = a.q.AssignConversation(ctx, db.AssignConversationParams{ID: cur.ID, AssignedUserID: user, AssignedOrgID: cur.AssignedOrgID})
	case BulkSetAIMode:
		mode, _ := previous["ai_mode"].(string)
		if !slices.Contains(model.AIModes, mode) {
			return errors.New("missing conversation ai mode snapshot")
		}
		p := db.SetConversationAIModeParams{ID: cur.ID, AiMode: mode}
		if s, ok := previous["ai_paused_until"].(string); ok && mode == model.AIModePaused {
			if t, perr := time.Parse(time.RFC3339Nano, s); perr == nil {
				p.AiPausedUntil = ts(t)
			}
		}
		_, err = a.q.SetConversationAIMode(ctx, p)
	default:
		return errors.New("unsupported action")
	}
	return err
}

var (
	_ bulkengine.BulkAdapter = (*BulkAdapter)(nil)
	_ bulkengine.TxBinder    = (*BulkAdapter)(nil)
	_ bulkengine.StateReader = (*BulkAdapter)(nil)
)
