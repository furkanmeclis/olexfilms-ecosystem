// Package repository wraps the AI assistant persistence (TEC-383, F4-01a):
// list contracts of conversations and usage, the pending action
// compare-and-set and the usage ledger with its monthly projection.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Conn is a pool or a transaction: queries run on it and RecordUsage opens
// a (nested) transaction on it.
type Conn interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Store is the AI repository.
type Store struct {
	conn Conn
	q    *db.Queries
}

// New creates a repository on a pool or a transaction.
func New(conn Conn) *Store {
	return &Store{conn: conn, q: db.New(conn)}
}

// Queries returns the generated query set.
func (s *Store) Queries() *db.Queries { return s.q }

// ConversationSort is the sort contract of the conversation list.
var ConversationSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"updated_at": "updated_at", "created_at": "created_at", "title": "title",
	},
	Default: apiquery.SortField{Field: "updated_at", Desc: true},
}

// UsageSort is the sort contract of the usage list. tokens is the quota
// count (input + output + cache write).
var UsageSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"created_at": "created_at", "tokens": "tokens", "input_tokens": "input_tokens",
		"output_tokens": "output_tokens", "model": "model", "channel": "channel", "purpose": "purpose",
	},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// ConversationFilter selects the caller's own conversations of a channel.
type ConversationFilter struct {
	OrganizationID int64
	UserID         int64
	Channel        string
	Q              string
	// Created filters created_at (created_from / created_to, TEC-388).
	Created       apiquery.TimeRange
	Sort          []apiquery.SortField
	Limit, Offset int32
}

// ListConversations returns a page of conversations and the total. An
// unknown sort field is an *apiquery.ValidationError.
func (s *Store) ListConversations(ctx context.Context, f ConversationFilter) ([]db.AiConversation, int64, error) {
	sort, err := apiquery.ResolveSort(f.Sort, ConversationSort)
	if err != nil {
		return nil, 0, err
	}
	q := textNarg(f.Q)
	from, before := tsNarg(f.Created.From), tsNarg(f.Created.Before)
	rows, err := s.q.ListAIConversations(ctx, db.ListAIConversationsParams{
		OrganizationID: f.OrganizationID, UserID: f.UserID, Channel: f.Channel, Q: q,
		CreatedFrom: from, CreatedBefore: before,
		SortKey: sort.Key, SortDesc: sort.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountAIConversations(ctx, db.CountAIConversationsParams{
		OrganizationID: f.OrganizationID, UserID: f.UserID, Channel: f.Channel, Q: q,
		CreatedFrom: from, CreatedBefore: before,
	})
	return rows, total, err
}

// UsageFilter selects ledger rows. Nil slices and pointers mean no filter;
// Created.Before is exclusive (apiquery.DateRange).
type UsageFilter struct {
	BrandID         *int64
	OrganizationIDs []int64
	UserIDs         []int64
	Pools           []string
	Channels        []string
	Purposes        []string
	Models          []string
	Created         apiquery.TimeRange
	TokensMin       *int64
	TokensMax       *int64
	Sort            []apiquery.SortField
	Limit, Offset   int32
}

// ListUsage returns a page of the usage ledger and the total. An unknown
// sort field is an *apiquery.ValidationError; equal sort values are ordered
// by id in the sort direction.
func (s *Store) ListUsage(ctx context.Context, f UsageFilter) ([]db.AiUsage, int64, error) {
	sort, err := apiquery.ResolveSort(f.Sort, UsageSort)
	if err != nil {
		return nil, 0, err
	}
	p := db.ListAIUsageParams{
		BrandID: int8Narg(f.BrandID), OrganizationIds: f.OrganizationIDs, UserIds: f.UserIDs,
		Pools: f.Pools, Channels: f.Channels, Purposes: f.Purposes, Models: f.Models,
		CreatedFrom: tsNarg(f.Created.From), CreatedBefore: tsNarg(f.Created.Before),
		TokensMin: int8Narg(f.TokensMin), TokensMax: int8Narg(f.TokensMax),
		SortKey: sort.Key, SortDesc: sort.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	rows, err := s.q.ListAIUsage(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountAIUsage(ctx, db.CountAIUsageParams{
		BrandID: p.BrandID, OrganizationIds: p.OrganizationIds, UserIds: p.UserIds,
		Pools: p.Pools, Channels: p.Channels, Purposes: p.Purposes, Models: p.Models,
		CreatedFrom: p.CreatedFrom, CreatedBefore: p.CreatedBefore,
		TokensMin: p.TokensMin, TokensMax: p.TokensMax,
	})
	return rows, total, err
}

// Usage is one model call to book.
type Usage struct {
	OrganizationID   int64
	BrandID          int64
	Pool             string
	UserID           *int64
	Channel          string
	Purpose          string
	Model            string
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
}

// Period is the projection key of a moment: YYYY-MM in UTC.
func Period(t time.Time) string {
	return t.UTC().Format("2006-01")
}

// RecordUsage appends the ledger row and adds it to ai_usage_monthly in the
// same transaction (ledger + projection): either both are written or none.
func (s *Store) RecordUsage(ctx context.Context, u Usage) (db.AiUsage, db.AiUsageMonthly, error) {
	var (
		row   db.AiUsage
		month db.AiUsageMonthly
	)
	err := pgx.BeginFunc(ctx, s.conn, func(tx pgx.Tx) error {
		var err error
		row, month, err = RecordUsageTx(ctx, db.New(tx), u)
		return err
	})
	return row, month, err
}

// RecordUsageTx is RecordUsage on a caller-owned transaction.
func RecordUsageTx(ctx context.Context, q *db.Queries, u Usage) (db.AiUsage, db.AiUsageMonthly, error) {
	row, err := q.InsertAIUsage(ctx, db.InsertAIUsageParams{
		OrganizationID: u.OrganizationID, BrandID: u.BrandID, Pool: u.Pool, UserID: int8Narg(u.UserID),
		Channel: u.Channel, Purpose: u.Purpose, Model: u.Model,
		InputTokens: u.InputTokens, OutputTokens: u.OutputTokens,
		CacheReadTokens: u.CacheReadTokens, CacheWriteTokens: u.CacheWriteTokens,
	})
	if err != nil {
		return db.AiUsage{}, db.AiUsageMonthly{}, err
	}
	month, err := q.AddAIUsageMonthly(ctx, db.AddAIUsageMonthlyParams{
		OrganizationID: row.OrganizationID, BrandID: row.BrandID, Pool: row.Pool,
		Period: Period(row.CreatedAt.Time), QuotaTokens: row.QuotaTokens,
		InputTokens: row.InputTokens, OutputTokens: row.OutputTokens,
		CacheReadTokens: row.CacheReadTokens, CacheWriteTokens: row.CacheWriteTokens,
	})
	if err != nil {
		return db.AiUsage{}, db.AiUsageMonthly{}, err
	}
	return row, month, nil
}

// MonthlyTokens returns the quota tokens a pool used in the month of at.
func (s *Store) MonthlyTokens(ctx context.Context, orgID int64, pool string, at time.Time) (int64, error) {
	return s.q.GetAIUsageMonthly(ctx, db.GetAIUsageMonthlyParams{
		OrganizationID: orgID, Pool: pool, Period: Period(at),
	})
}

// CreatePendingAction stores a proposed write-tool call (TEC-387).
func (s *Store) CreatePendingAction(ctx context.Context, p db.CreateAIPendingActionParams) (db.AiPendingAction, error) {
	return s.q.CreateAIPendingAction(ctx, p)
}

// PendingActionByKey returns the action of an idempotency key; ok is false
// when there is none.
func (s *Store) PendingActionByKey(ctx context.Context, key string) (db.AiPendingAction, bool, error) {
	return oneRow(s.q.GetAIPendingActionByIdempotencyKey(ctx, key))
}

// PendingActionForUser returns the caller's own action; ok is false when it
// does not exist or belongs to another user or organization.
func (s *Store) PendingActionForUser(ctx context.Context, id uuid.UUID, orgID, userID int64) (db.AiPendingAction, bool, error) {
	return oneRow(s.q.GetAIPendingActionForUser(ctx, db.GetAIPendingActionForUserParams{
		Uuid: id, OrganizationID: orgID, UserID: userID,
	}))
}

// ListPendingActions returns the user's open, unexpired actions.
func (s *Store) ListPendingActions(ctx context.Context, orgID, userID int64) ([]db.AiPendingAction, error) {
	return s.q.ListAIPendingActionsForUser(ctx, db.ListAIPendingActionsForUserParams{OrganizationID: orgID, UserID: userID})
}

// PendingActionsPage returns one page of the caller's open, unexpired
// actions of the given sources and their total (TEC-403).
func (s *Store) PendingActionsPage(ctx context.Context, p db.ListAIPendingActionsPageParams) ([]db.AiPendingAction, int64, error) {
	rows, err := s.q.ListAIPendingActionsPage(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountAIPendingActionsPage(ctx, db.CountAIPendingActionsPageParams{
		OrganizationID: p.OrganizationID, UserID: p.UserID, Sources: p.Sources, Q: p.Q,
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ClaimPendingAction moves the caller's pending, unexpired action to
// executing with a single compare-and-set UPDATE. ok is false when another
// request already claimed it, it was resolved or it expired. Non-nil input
// and preview replace the stored ones (edited confirmation card).
func (s *Store) ClaimPendingAction(ctx context.Context, id uuid.UUID, orgID, userID int64, input, preview []byte) (db.AiPendingAction, bool, error) {
	return oneRow(s.q.ClaimAIPendingAction(ctx, db.ClaimAIPendingActionParams{
		Uuid: id, OrganizationID: orgID, UserID: userID, Input: input, Preview: preview,
	}))
}

// ResolvePendingAction finishes a claimed action: executing → confirmed |
// failed.
func (s *Store) ResolvePendingAction(ctx context.Context, p db.ResolveAIPendingActionParams) (db.AiPendingAction, error) {
	return s.q.ResolveAIPendingAction(ctx, p)
}

// CancelPendingAction cancels the caller's pending action; ok is false when
// it is no longer pending.
func (s *Store) CancelPendingAction(ctx context.Context, id uuid.UUID, orgID, userID int64) (db.AiPendingAction, bool, error) {
	return oneRow(s.q.CancelAIPendingAction(ctx, db.CancelAIPendingActionParams{
		Uuid: id, OrganizationID: orgID, UserID: userID,
	}))
}

// CancelPendingActionsForSource cancels the user's pending actions of one
// conversation.
func (s *Store) CancelPendingActionsForSource(ctx context.Context, p db.CancelAIPendingActionsForSourceParams) ([]db.AiPendingAction, error) {
	return s.q.CancelAIPendingActionsForSource(ctx, p)
}

// ExpirePendingAction expires one pending action past its expiry; ok is
// false when it is no longer pending or not yet expired.
func (s *Store) ExpirePendingAction(ctx context.Context, id int64, now time.Time) (db.AiPendingAction, bool, error) {
	return oneRow(s.q.ExpireAIPendingAction(ctx, db.ExpireAIPendingActionParams{
		ID: id, Now: pgtype.Timestamptz{Time: now, Valid: true},
	}))
}

// ExpirePendingActions expires every pending action past its expiry and
// returns how many changed.
func (s *Store) ExpirePendingActions(ctx context.Context, now time.Time) (int64, error) {
	return s.q.ExpireAIPendingActions(ctx, pgtype.Timestamptz{Time: now, Valid: true})
}

// FailStalePendingActions fails actions executing since before staleBefore.
func (s *Store) FailStalePendingActions(ctx context.Context, staleBefore time.Time, errText string) ([]db.AiPendingAction, error) {
	return s.q.FailStaleAIPendingActions(ctx, db.FailStaleAIPendingActionsParams{
		Error: pgtype.Text{String: errText, Valid: true}, StaleBefore: pgtype.Timestamptz{Time: staleBefore, Valid: true},
	})
}

// oneRow turns pgx.ErrNoRows into ok = false.
func oneRow(a db.AiPendingAction, err error) (db.AiPendingAction, bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return db.AiPendingAction{}, false, nil
	}
	if err != nil {
		return db.AiPendingAction{}, false, err
	}
	return a, true, nil
}

func textNarg(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func int8Narg(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func tsNarg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// --- chat (TEC-388) ----------------------------------------------------------

// Settings returns the platform AI settings singleton.
func (s *Store) Settings(ctx context.Context) (db.AiSetting, error) {
	return s.q.GetAISettings(ctx)
}

// OrgSettings returns the organization override; ok is false when there is
// none (enabled with the platform default quota).
func (s *Store) OrgSettings(ctx context.Context, orgID int64) (db.AiOrgSetting, bool, error) {
	row, err := s.q.GetAIOrgSettings(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.AiOrgSetting{}, false, nil
	}
	if err != nil {
		return db.AiOrgSetting{}, false, err
	}
	return row, true, nil
}

// CreateConversation starts an empty conversation.
func (s *Store) CreateConversation(ctx context.Context, p db.CreateAIConversationParams) (db.AiConversation, error) {
	return s.q.CreateAIConversation(ctx, p)
}

// ConversationForUser returns the caller's own, not deleted conversation of
// a channel; ok is false otherwise (another user's conversation is never
// distinguished from a missing one).
func (s *Store) ConversationForUser(ctx context.Context, id uuid.UUID, orgID, userID int64, channel string) (db.AiConversation, bool, error) {
	row, err := s.q.GetAIConversationForUser(ctx, db.GetAIConversationForUserParams{
		Uuid: id, OrganizationID: orgID, UserID: userID,
	})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.Channel != channel) {
		return db.AiConversation{}, false, nil
	}
	if err != nil {
		return db.AiConversation{}, false, err
	}
	return row, true, nil
}

// RenameConversation sets the title.
func (s *Store) RenameConversation(ctx context.Context, id int64, title string) (db.AiConversation, error) {
	return s.q.UpdateAIConversationTitle(ctx, db.UpdateAIConversationTitleParams{ID: id, Title: title})
}

// DeleteConversation soft-deletes the caller's conversation; ok is false
// when there was nothing to delete.
func (s *Store) DeleteConversation(ctx context.Context, id uuid.UUID, orgID, userID int64) (bool, error) {
	n, err := s.q.SoftDeleteAIConversation(ctx, db.SoftDeleteAIConversationParams{
		Uuid: id, OrganizationID: orgID, UserID: userID,
	})
	return n > 0, err
}

// TouchConversation counts a stored message and moves the conversation up.
func (s *Store) TouchConversation(ctx context.Context, id int64) error {
	_, err := s.q.TouchAIConversation(ctx, id)
	return err
}

// CreateMessage stores a message row.
func (s *Store) CreateMessage(ctx context.Context, p db.CreateAIMessageParams) (db.AiMessage, error) {
	return s.q.CreateAIMessage(ctx, p)
}

// FinishMessage completes a pending assistant row.
func (s *Store) FinishMessage(ctx context.Context, p db.FinishAIMessageParams) (db.AiMessage, error) {
	return s.q.FinishAIMessage(ctx, p)
}

// Messages returns the messages of a conversation in order.
func (s *Store) Messages(ctx context.Context, conversationID int64) ([]db.AiMessage, error) {
	return s.q.ListAIMessages(ctx, conversationID)
}

// ChatContext returns the prompt context of a user in an organization.
func (s *Store) ChatContext(ctx context.Context, userID, orgID int64) (db.GetAIChatContextRow, error) {
	return s.q.GetAIChatContext(ctx, db.GetAIChatContextParams{UserID: userID, OrganizationID: orgID})
}

// BrandCenter returns the center organization of a brand (portal chats
// belong to it).
func (s *Store) BrandCenter(ctx context.Context, brandID int64) (db.Organization, error) {
	return s.q.GetBrandCenter(ctx, brandID)
}

// EventEnqueuer writes outbox events in a transaction (outbox.Store).
type EventEnqueuer interface {
	Enqueue(ctx context.Context, tx pgx.Tx, ev events.Event) error
}

// RecordUsageEvents is RecordUsage that also writes the events build
// returns for the booked row and the new monthly total (quota thresholds)
// to the outbox in the same transaction. A nil enqueuer skips the events.
func (s *Store) RecordUsageEvents(ctx context.Context, u Usage, enq EventEnqueuer, build func(db.AiUsage, db.AiUsageMonthly) []events.Event) (db.AiUsage, db.AiUsageMonthly, error) {
	var (
		row   db.AiUsage
		month db.AiUsageMonthly
	)
	err := pgx.BeginFunc(ctx, s.conn, func(tx pgx.Tx) error {
		var err error
		row, month, err = RecordUsageTx(ctx, db.New(tx), u)
		if err != nil || enq == nil || build == nil {
			return err
		}
		for _, ev := range build(row, month) {
			if err := enq.Enqueue(ctx, tx, ev); err != nil {
				return err
			}
		}
		return nil
	})
	return row, month, err
}
