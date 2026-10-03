// Package repository is the read-only access to the old hub's message
// archive, legacy_messages (TEC-263). The archive is written once by the
// migrator, has no UI and never enters the notification center; the only
// change it accepts is the K19 anonymization (customers usecase).
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// MaxPageSize bounds one page of ListForPerson.
const MaxPageSize = 200

// Store is the subset of the generated queries the repository uses.
type Store interface {
	ListLegacyMessagesByUser(ctx context.Context, arg db.ListLegacyMessagesByUserParams) ([]db.ListLegacyMessagesByUserRow, error)
	CountLegacyMessagesByChannel(ctx context.Context, brandID int64) ([]db.CountLegacyMessagesByChannelRow, error)
}

// Repository reads legacy_messages.
type Repository struct{ q Store }

// New binds the repository to the generated queries.
func New(q Store) *Repository { return &Repository{q: q} }

// Cursor is the keyset position of the last row of a page.
type Cursor struct {
	At time.Time
	ID int64
}

// ListForPerson returns a person's archived messages of a brand (by account
// id and/or E.164 phone), newest first. after nil reads the first page.
func (r *Repository) ListForPerson(ctx context.Context, brandID int64, userID *int64, phone string, after *Cursor, size int) ([]db.ListLegacyMessagesByUserRow, error) {
	if size <= 0 || size > MaxPageSize {
		size = MaxPageSize
	}
	arg := db.ListLegacyMessagesByUserParams{
		BrandID:   brandID,
		Recipient: pgtype.Text{String: phone, Valid: phone != ""},
		PageSize:  int32(size), //nolint:gosec // bounded by MaxPageSize
	}
	if userID != nil {
		arg.UserID = pgtype.Int8{Int64: *userID, Valid: true}
	}
	if !arg.UserID.Valid && !arg.Recipient.Valid {
		return []db.ListLegacyMessagesByUserRow{}, nil
	}
	if after != nil {
		arg.BeforeAt = pgtype.Timestamptz{Time: after.At, Valid: true}
		arg.BeforeID = pgtype.Int8{Int64: after.ID, Valid: true}
	}
	rows, err := r.q.ListLegacyMessagesByUser(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("legacymessages: list: %w", err)
	}
	return rows, nil
}

// NextCursor is the cursor after row (the list orders by sent_at, else
// created_at, then id).
func NextCursor(row db.ListLegacyMessagesByUserRow) Cursor {
	at := row.CreatedAt.Time
	if row.SentAt.Valid {
		at = row.SentAt.Time
	}
	return Cursor{At: at, ID: row.ID}
}

// CountByChannel returns the number of archived messages per channel.
func (r *Repository) CountByChannel(ctx context.Context, brandID int64) (map[string]int64, error) {
	rows, err := r.q.CountLegacyMessagesByChannel(ctx, brandID)
	if err != nil {
		return nil, fmt.Errorf("legacymessages: count: %w", err)
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		out[row.Channel] = row.Total
	}
	return out, nil
}
