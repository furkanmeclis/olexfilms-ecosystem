package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Opening balances (TEC-177, F1-07g). One per cari, written through
// POST /v1/accounting/opening-balances (step-up) or, by the F2 migrator,
// through PostOpeningBalanceTx inside its own transaction. The ledger rules
// are in posting/opening.go.

// SourceOpeningBalance is the source_type of opening balance rows.
const SourceOpeningBalance = posting.SourceOpeningBalance

// Audit actions (activity_events.action) of opening balances.
const (
	AuditOpeningBalancePosted = "accounting.opening_balance_posted"
	AuditOpeningBalanceVoided = "accounting.opening_balance_voided"
	entryResource             = "finance_entry"
)

// ErrOpeningBalanceExists: the cari already has an open opening balance with
// other values (reverse it first).
var ErrOpeningBalanceExists = errors.New("accounting: cari already has an opening balance")

// OpeningBalanceInput is an opening balance request on the active
// organization's book. The cari is named by CariUUID or by the counterparty
// organization (the book's parent or a direct child; opened when missing).
type OpeningBalanceInput struct {
	CariUUID        *uuid.UUID
	CounterpartyOrg *uuid.UUID
	// Side: "debit" (the counterparty owes us) or "credit" (we owe it).
	Side     string
	Amount   string
	Currency string // empty: the organization's currency
	// Date is the opening date (YYYY-MM-DD).
	Date        string
	Description string
}

// PostOpeningBalance books the opening balance of a cari in the active
// organization's book. The bool is true when the same opening balance was
// already booked (nothing written).
func (s *Service) PostOpeningBalance(ctx context.Context, c Caller, in OpeningBalanceInput) (Entry, bool, error) {
	book, err := s.writeBook(ctx, c)
	if err != nil {
		return Entry{}, false, err
	}
	side := strings.TrimSpace(in.Side)
	if side != posting.SideDebit && side != posting.SideCredit {
		return Entry{}, false, invalid("side", "must be debit or credit")
	}
	if strings.TrimSpace(in.Amount) == "" {
		return Entry{}, false, invalid("amount", "is required")
	}
	day, err := time.Parse(time.DateOnly, strings.TrimSpace(in.Date))
	if err != nil {
		return Entry{}, false, invalid("opening_date", "must be a date (YYYY-MM-DD)")
	}
	description := strings.TrimSpace(in.Description)
	if len([]rune(description)) > 1000 {
		return Entry{}, false, invalid("description", "must be at most 1000 characters")
	}
	cur := book.Currency
	if strings.TrimSpace(in.Currency) != "" {
		if cur, err = fxrates.NormalizeCode(in.Currency); err != nil {
			return Entry{}, false, invalid("currency", "must be a three-letter ISO-4217 code")
		}
	}
	if in.CariUUID == nil && in.CounterpartyOrg == nil {
		return Entry{}, false, invalid("cari_uuid", "a cari is required")
	}
	counterparty, err := s.counterparty(ctx, book, in.CariUUID, in.CounterpartyOrg)
	if err != nil {
		return Entry{}, false, err
	}
	var res posting.OpeningResult
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		res, err = s.PostOpeningBalanceTx(ctx, tx, posting.OpeningBalance{
			OrganizationID: book.ID, CounterpartyOrgID: counterparty, Side: side,
			Amount: strings.TrimSpace(in.Amount), Currency: cur, Date: day,
			Description: description, ActorUserID: c.actor(),
		})
		return err
	})
	if err != nil {
		return Entry{}, false, err
	}
	out, err := s.entry(ctx, s.q, book.ID, res.Entry.Uuid)
	return out, res.Replayed, err
}

// PostOpeningBalanceTx books an opening balance in tx and writes its audit
// row (the outbox event is written by posting). It takes no Caller: the F2
// migrator calls it with internal organization ids. Errors are the
// usecase errors (ErrOpeningBalanceExists, ErrCounterparty, ValidationError,
// ErrRateNotFound).
func (s *Service) PostOpeningBalanceTx(ctx context.Context, tx pgx.Tx, o posting.OpeningBalance) (posting.OpeningResult, error) {
	res, err := s.poster.PostOpeningBalanceTx(ctx, tx, o)
	if err != nil {
		if errors.Is(err, posting.ErrIdempotencyConflict) {
			return posting.OpeningResult{}, ErrOpeningBalanceExists
		}
		if errors.Is(err, posting.ErrOpeningDate) {
			return posting.OpeningResult{}, invalid("opening_date", "must be between 2000-01-01 and today")
		}
		return posting.OpeningResult{}, postingErr(err)
	}
	if res.Replayed {
		return res, nil
	}
	side := posting.SideDebit
	if res.Entry.Direction == posting.DirectionCollection {
		side = posting.SideCredit
	}
	if err := s.auditEntry(ctx, tx, AuditOpeningBalancePosted, res.Entry, o.ActorUserID, map[string]any{
		"cari_uuid": res.Cari.Uuid.String(), "side": side,
		"opening_date": res.Entry.CreatedAt.Time.UTC().Format(time.DateOnly),
	}); err != nil {
		return posting.OpeningResult{}, err
	}
	return res, nil
}

// auditEntry writes the audit row of a ledger row in tx.
func (s *Service) auditEntry(ctx context.Context, tx pgx.Tx, action string, e db.FinanceEntry, actor *int64, extra map[string]any) error {
	payload := map[string]any{
		"entry_id":        e.ID,
		"entry_uuid":      e.Uuid.String(),
		"organization_id": e.OrganizationID,
		"brand_id":        e.BrandID,
		"direction":       e.Direction,
		"category":        e.Category,
		"orig_currency":   e.OrigCurrency,
		"orig_amount":     posting.FormatNumeric(e.OrigAmount),
		"currency":        e.Currency,
		"amount":          posting.FormatNumeric(e.Amount),
		"source_type":     e.SourceType.String,
		"source_uuid":     uuid.UUID(e.SourceUuid.Bytes).String(),
		"revision":        e.Revision,
	}
	if e.ReversalOfID.Valid {
		payload["reversal_of_id"] = e.ReversalOfID.Int64
	}
	for k, v := range extra {
		payload[k] = v
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("accounting: audit payload: %w", err)
	}
	if _, err := s.q.WithTx(tx).InsertActivityEvent(ctx, db.InsertActivityEventParams{
		ActorUserID: i8(actor), Action: action, Resource: entryResource,
		ResourceUuid: pgtype.UUID{Bytes: e.Uuid, Valid: true}, Payload: body,
	}); err != nil {
		return fmt.Errorf("accounting: audit: %w", err)
	}
	return nil
}
