package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Counterparty is the other side of a cari: an organization, or (TEC-118)
// a user.
type Counterparty struct {
	Type string     `json:"type"`
	UUID *uuid.UUID `json:"uuid"`
	Name string     `json:"name"`
	// OrgType is the organization type of an organization counterparty.
	OrgType string `json:"org_type,omitempty"`
}

// Cari is a current account with its ledger balance. A positive balance is
// a receivable (the counterparty owes the organization), negative a debt.
type Cari struct {
	UUID         uuid.UUID    `json:"uuid"`
	Counterparty Counterparty `json:"counterparty"`
	Currency     string       `json:"currency"`
	Active       bool         `json:"active"`
	Balance      string       `json:"balance"`
	EntryCount   int64        `json:"entry_count"`
	LastEntryAt  *time.Time   `json:"last_entry_at"`
	CreatedAt    time.Time    `json:"created_at"`
}

func uuidPtr(u pgtype.UUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	id := uuid.UUID(u.Bytes)
	return &id
}

func cariOf(r db.GetCariAccountWithBalanceRow) Cari {
	out := Cari{
		UUID: r.Uuid, Currency: r.Currency, Active: r.Active,
		Balance: posting.FormatNumeric(r.Balance), EntryCount: r.EntryCount, CreatedAt: r.CreatedAt.Time,
	}
	if r.LastEntryAt.Valid {
		t := r.LastEntryAt.Time
		out.LastEntryAt = &t
	}
	out.Counterparty = Counterparty{Type: r.CounterpartyType}
	if r.CounterpartyType == "organization" {
		out.Counterparty.UUID = uuidPtr(r.CounterpartyOrgUuid)
		out.Counterparty.Name = r.CounterpartyOrgName.String
		out.Counterparty.OrgType = r.CounterpartyOrgType.String
	} else {
		out.Counterparty.UUID = uuidPtr(r.CounterpartyUserUuid)
		out.Counterparty.Name = r.CounterpartyUserName
	}
	return out
}

// CariFilter narrows ListCari.
type CariFilter struct {
	OrganizationUUID *uuid.UUID
	Active           *bool
	Q                string
	Limit, Offset    int32
}

// ListCari lists the cari accounts of the book with balances.
func (s *Service) ListCari(ctx context.Context, c Caller, f CariFilter) ([]Cari, int64, error) {
	book, err := s.readBook(ctx, c, f.OrganizationUUID)
	if err != nil {
		return nil, 0, err
	}
	var active pgtype.Bool
	if f.Active != nil {
		active = pgtype.Bool{Bool: *f.Active, Valid: true}
	}
	q := pgtype.Text{String: f.Q, Valid: f.Q != ""}
	rows, err := s.q.ListCariAccountsWithBalance(ctx, db.ListCariAccountsWithBalanceParams{
		OrganizationID: book.ID, Active: active, Q: q, PageLimit: f.Limit, PageOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("accounting: list cari: %w", err)
	}
	total, err := s.q.CountCariAccountsWithBalance(ctx, db.CountCariAccountsWithBalanceParams{
		OrganizationID: book.ID, Active: active, Q: q,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("accounting: count cari: %w", err)
	}
	out := make([]Cari, 0, len(rows))
	for _, r := range rows {
		out = append(out, cariOf(db.GetCariAccountWithBalanceRow(r)))
	}
	return out, total, nil
}

// GetCari returns one cari account of the book.
func (s *Service) GetCari(ctx context.Context, c Caller, orgUUID *uuid.UUID, id uuid.UUID) (Cari, error) {
	book, err := s.readBook(ctx, c, orgUUID)
	if err != nil {
		return Cari{}, err
	}
	return s.cari(ctx, book.ID, id)
}

func (s *Service) cari(ctx context.Context, orgID int64, id uuid.UUID) (Cari, error) {
	r, err := s.q.GetCariAccountWithBalance(ctx, db.GetCariAccountWithBalanceParams{Uuid: id, OrganizationID: orgID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Cari{}, ErrCariNotFound
	}
	if err != nil {
		return Cari{}, fmt.Errorf("accounting: cari: %w", err)
	}
	return cariOf(r), nil
}
