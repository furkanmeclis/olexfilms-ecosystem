// Package repository wraps the WhatsApp conversation persistence (TEC-393,
// F4-02a): the inbox list contract, the cursor timeline, the AI run log
// retention and the contact opt-out ledger with its state projection.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Store is the conversation repository.
type Store struct {
	q *db.Queries
}

// New creates a repository on a pool or a transaction.
func New(conn db.DBTX) *Store {
	return &Store{q: db.New(conn)}
}

// Queries returns the generated query set.
func (s *Store) Queries() *db.Queries { return s.q }

// ConversationSort is the sort contract of the conversation list. Empty
// last_message_at values sort last in both directions.
var ConversationSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"last_message_at": "last_message_at", "created_at": "created_at", "unread_count": "unread_count",
	},
	Default: apiquery.SortField{Field: "last_message_at", Desc: true},
}

// ConversationFilter selects conversations. Nil slices and pointers mean no
// filter; LastMessage.Before is exclusive (apiquery.DateRange). The enum
// slices must already be validated (model.Statuses, model.AIModes,
// model.IdentityKinds).
type ConversationFilter struct {
	Channel         string
	Statuses        []string
	IdentityKinds   []string
	AIModes         []string
	AssignedUserIDs []int64
	LastMessage     apiquery.TimeRange
	Unread          *bool
	Q               string
	Sort            []apiquery.SortField
	Limit, Offset   int32
}

// ListConversations returns a page of conversations and the total. An
// unknown sort field is an *apiquery.ValidationError; equal sort values are
// ordered by id in the sort direction.
func (s *Store) ListConversations(ctx context.Context, f ConversationFilter) ([]db.Conversation, int64, error) {
	sort, err := apiquery.ResolveSort(f.Sort, ConversationSort)
	if err != nil {
		return nil, 0, err
	}
	p := db.ListConversationsParams{
		Channel: textNarg(f.Channel), Statuses: f.Statuses, IdentityKinds: f.IdentityKinds,
		AiModes: f.AIModes, AssignedUserIds: f.AssignedUserIDs,
		LastMessageFrom: tsNarg(f.LastMessage.From), LastMessageBefore: tsNarg(f.LastMessage.Before),
		Unread: boolNarg(f.Unread), Q: textNarg(f.Q),
		SortKey: sort.Key, SortDesc: sort.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	rows, err := s.q.ListConversations(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountConversations(ctx, db.CountConversationsParams{
		Channel: p.Channel, Statuses: p.Statuses, IdentityKinds: p.IdentityKinds,
		AiModes: p.AiModes, AssignedUserIds: p.AssignedUserIds,
		LastMessageFrom: p.LastMessageFrom, LastMessageBefore: p.LastMessageBefore,
		Unread: p.Unread, Q: p.Q,
	})
	return rows, total, err
}

// MessageCursor is a timeline position: the (created_at, id) of a message.
type MessageCursor struct {
	At time.Time
	ID int64
}

// CursorOf returns the cursor of a message.
func CursorOf(m db.Message) MessageCursor {
	return MessageCursor{At: m.CreatedAt.Time, ID: m.ID}
}

// ListMessagesBefore returns up to limit messages older than before (nil =
// the newest page), newest first. next is the cursor of the following
// (older) page, nil when this page is the last one.
func (s *Store) ListMessagesBefore(ctx context.Context, conversationID int64, before *MessageCursor, limit int32) ([]db.Message, *MessageCursor, error) {
	p := db.ListConversationMessagesBeforeParams{ConversationID: conversationID, LimitCount: limit + 1}
	if before != nil {
		p.CursorAt = pgtype.Timestamptz{Time: before.At, Valid: true}
		p.CursorID = pgtype.Int8{Int64: before.ID, Valid: true}
	}
	rows, err := s.q.ListConversationMessagesBefore(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	if int32(len(rows)) <= limit {
		return rows, nil, nil
	}
	rows = rows[:limit]
	next := CursorOf(rows[len(rows)-1])
	return rows, &next, nil
}

// ListMessagesAfter returns up to limit messages newer than after, oldest
// first.
func (s *Store) ListMessagesAfter(ctx context.Context, conversationID int64, after MessageCursor, limit int32) ([]db.Message, error) {
	return s.q.ListConversationMessagesAfter(ctx, db.ListConversationMessagesAfterParams{
		ConversationID: conversationID,
		CursorAt:       pgtype.Timestamptz{Time: after.At, Valid: true},
		CursorID:       after.ID,
		LimitCount:     limit,
	})
}

// StartAIRun opens the run of a triggering message. ok is false when the
// message already has a run (at most one run per inbound message).
func (s *Store) StartAIRun(ctx context.Context, p db.CreateConversationAIRunParams) (db.ConversationAiRun, bool, error) {
	run, err := s.q.CreateConversationAIRun(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ConversationAiRun{}, false, nil
	}
	if err != nil {
		return db.ConversationAiRun{}, false, err
	}
	return run, true, nil
}

const purgeBatch = 1000

// PurgeExpiredAIRuns deletes AI runs older than the retention window
// (model.AIRunRetentionDays) in batches and returns the deleted count.
func (s *Store) PurgeExpiredAIRuns(ctx context.Context) (int64, error) {
	return s.PurgeAIRunsBefore(ctx, time.Now().AddDate(0, 0, -model.AIRunRetentionDays))
}

// PurgeAIRunsBefore deletes AI runs created before cutoff in batches.
// Messages of a purged run keep their content; ai_run_id becomes NULL.
func (s *Store) PurgeAIRunsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	var total int64
	for {
		n, err := s.q.PurgeConversationAIRunsBefore(ctx, db.PurgeConversationAIRunsBeforeParams{
			Cutoff: pgtype.Timestamptz{Time: cutoff, Valid: true}, BatchSize: purgeBatch,
		})
		if err != nil {
			return total, err
		}
		total += n
		if n < purgeBatch || ctx.Err() != nil {
			return total, nil
		}
	}
}

// OptOut is one opt-out ledger entry.
type OptOut struct {
	ContactE164     string
	Scope           string
	Action          string
	Source          string
	ConversationID  *int64
	CreatedByUserID *int64
	Note            string
}

// RecordOptOut appends a ledger row and returns it with the resulting
// state. The projection is maintained by the ledger's AFTER INSERT trigger
// in the same statement.
func (s *Store) RecordOptOut(ctx context.Context, o OptOut) (db.ContactOptOut, db.ContactOptOutState, error) {
	row, err := s.q.InsertContactOptOut(ctx, db.InsertContactOptOutParams{
		ContactE164: o.ContactE164, Scope: o.Scope, Action: o.Action, Source: o.Source,
		ConversationID: int8Narg(o.ConversationID), CreatedByUserID: int8Narg(o.CreatedByUserID),
		Note: textNarg(o.Note),
	})
	if err != nil {
		return db.ContactOptOut{}, db.ContactOptOutState{}, err
	}
	state, err := s.q.GetContactOptOutState(ctx, db.GetContactOptOutStateParams{
		ContactE164: row.ContactE164, Scope: row.Scope,
	})
	if err != nil {
		return db.ContactOptOut{}, db.ContactOptOutState{}, err
	}
	return row, state, nil
}

// IsOptedOut reports whether the contact is currently opted out of scope
// (no ledger entry = not opted out).
func (s *Store) IsOptedOut(ctx context.Context, contactE164, scope string) (bool, error) {
	state, err := s.q.GetContactOptOutState(ctx, db.GetContactOptOutStateParams{
		ContactE164: contactE164, Scope: scope,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return state.OptedOut, nil
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

func boolNarg(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

func tsNarg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
