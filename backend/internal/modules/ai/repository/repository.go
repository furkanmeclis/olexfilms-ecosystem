// Package repository wraps the AI assistant persistence (TEC-383, F4-01a):
// list contracts of conversations and usage, the pending action
// compare-and-set and the usage ledger with its monthly projection.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
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
	Sort           []apiquery.SortField
	Limit, Offset  int32
}

// ListConversations returns a page of conversations and the total. An
// unknown sort field is an *apiquery.ValidationError.
func (s *Store) ListConversations(ctx context.Context, f ConversationFilter) ([]db.AiConversation, int64, error) {
	sort, err := apiquery.ResolveSort(f.Sort, ConversationSort)
	if err != nil {
		return nil, 0, err
	}
	q := textNarg(f.Q)
	rows, err := s.q.ListAIConversations(ctx, db.ListAIConversationsParams{
		OrganizationID: f.OrganizationID, UserID: f.UserID, Channel: f.Channel, Q: q,
		SortKey: sort.Key, SortDesc: sort.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountAIConversations(ctx, db.CountAIConversationsParams{
		OrganizationID: f.OrganizationID, UserID: f.UserID, Channel: f.Channel, Q: q,
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

// ClaimPendingAction moves the caller's pending, unexpired action to
// executing with a single compare-and-set UPDATE. ok is false when another
// request already claimed it, it was resolved or it expired.
func (s *Store) ClaimPendingAction(ctx context.Context, id uuid.UUID, orgID, userID int64) (db.AiPendingAction, bool, error) {
	a, err := s.q.ClaimAIPendingAction(ctx, db.ClaimAIPendingActionParams{
		Uuid: id, OrganizationID: orgID, UserID: userID,
	})
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
