package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Ref is a named reference to another record.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Entry is one ledger row as the API shows it. Amounts are signed (a
// reversal row is negative); Amount is in the organization's currency,
// OrigAmount in the currency the entry was made in.
type Entry struct {
	UUID                     uuid.UUID  `json:"uuid"`
	Direction                string     `json:"direction"`
	Category                 string     `json:"category"`
	CategoryLabelKey         string     `json:"category_label_key"`
	Account                  *Ref       `json:"account"`
	CariUUID                 *uuid.UUID `json:"cari_uuid"`
	CounterpartyOrganization *Ref       `json:"counterparty_organization"`
	OrigCurrency             string     `json:"orig_currency"`
	OrigAmount               string     `json:"orig_amount"`
	Currency                 string     `json:"currency"`
	Amount                   string     `json:"amount"`
	Rate                     string     `json:"rate"`
	RateDate                 string     `json:"rate_date"`
	SourceType               *string    `json:"source_type"`
	SourceUUID               *uuid.UUID `json:"source_uuid"`
	Revision                 int32      `json:"revision"`
	ReversalOfUUID           *uuid.UUID `json:"reversal_of_uuid"`
	ReversedByUUID           *uuid.UUID `json:"reversed_by_uuid"`
	Voided                   bool       `json:"voided"`
	Description              *string    `json:"description"`
	CreatedAt                time.Time  `json:"created_at"`
}

func entryOf(r db.SearchFinanceEntriesRow) Entry {
	e := Entry{
		UUID: r.Uuid, Direction: r.Direction, Category: r.Category,
		CategoryLabelKey: "accounting.category." + r.Category,
		OrigCurrency:     r.OrigCurrency, OrigAmount: posting.FormatNumeric(r.OrigAmount),
		Currency: r.Currency, Amount: posting.FormatNumeric(r.Amount), Rate: posting.FormatRate(r.Rate),
		RateDate: r.RateDate.Time.Format(time.DateOnly), Revision: r.Revision,
		ReversalOfUUID: uuidPtr(r.ReversalOfUuid), ReversedByUUID: uuidPtr(r.ReversedByUuid),
		CariUUID: uuidPtr(r.CariUuid), SourceUUID: uuidPtr(r.SourceUuid), CreatedAt: r.CreatedAt.Time,
	}
	e.Voided = e.ReversedByUUID != nil
	if id := uuidPtr(r.AccountUuid); id != nil {
		e.Account = &Ref{UUID: *id, Name: r.AccountName.String}
	}
	if id := uuidPtr(r.CounterpartyOrgUuid); id != nil {
		e.CounterpartyOrganization = &Ref{UUID: *id, Name: r.CounterpartyOrgName.String}
	}
	if r.SourceType.Valid {
		v := r.SourceType.String
		e.SourceType = &v
	}
	if r.Description.Valid {
		v := r.Description.String
		e.Description = &v
	}
	return e
}

// EntryFilter narrows ListEntries. From/To are calendar days (UTC), both
// inclusive.
type EntryFilter struct {
	OrganizationUUID *uuid.UUID
	AccountUUID      *uuid.UUID
	CariUUID         *uuid.UUID
	Direction        string
	Category         string
	SourceType       string
	From, To         *time.Time
	Limit, Offset    int32
}

// ListEntries lists the ledger rows of the book, newest first.
func (s *Service) ListEntries(ctx context.Context, c Caller, f EntryFilter) ([]Entry, int64, error) {
	book, err := s.readBook(ctx, c, f.OrganizationUUID)
	if err != nil {
		return nil, 0, err
	}
	arg := db.SearchFinanceEntriesParams{OrganizationID: book.ID, PageLimit: f.Limit, PageOffset: f.Offset}
	if f.AccountUUID != nil {
		a, err := s.q.GetFinanceAccountByUUID(ctx, *f.AccountUUID)
		if err != nil || a.OrganizationID != book.ID {
			return []Entry{}, 0, nil
		}
		arg.AccountID = pgtype.Int8{Int64: a.ID, Valid: true}
	}
	if f.CariUUID != nil {
		ca, err := s.q.GetCariAccountByUUID(ctx, *f.CariUUID)
		if err != nil || ca.OrganizationID != book.ID {
			return []Entry{}, 0, nil
		}
		arg.CariID = pgtype.Int8{Int64: ca.ID, Valid: true}
	}
	if f.Direction != "" {
		if !validDirection(f.Direction) {
			return nil, 0, invalid("direction", "is not a ledger direction")
		}
		arg.Direction = pgtype.Text{String: f.Direction, Valid: true}
	}
	if f.Category != "" {
		if !accounting.ValidCategoryKey(f.Category) {
			return nil, 0, invalid("category", "is not a category key")
		}
		arg.Category = pgtype.Text{String: f.Category, Valid: true}
	}
	if f.SourceType != "" {
		arg.SourceType = pgtype.Text{String: f.SourceType, Valid: true}
	}
	if f.From != nil {
		arg.CreatedFrom = pgtype.Timestamptz{Time: *f.From, Valid: true}
	}
	if f.To != nil {
		arg.CreatedTo = pgtype.Timestamptz{Time: f.To.AddDate(0, 0, 1), Valid: true}
	}
	if f.From != nil && f.To != nil && f.To.Before(*f.From) {
		return nil, 0, invalid("date_to", "must not be before date_from")
	}
	rows, err := s.q.SearchFinanceEntries(ctx, arg)
	if err != nil {
		return nil, 0, fmt.Errorf("accounting: list entries: %w", err)
	}
	total, err := s.q.CountSearchFinanceEntries(ctx, db.CountSearchFinanceEntriesParams{
		OrganizationID: arg.OrganizationID, AccountID: arg.AccountID, CariID: arg.CariID,
		Direction: arg.Direction, Category: arg.Category, SourceType: arg.SourceType,
		CreatedFrom: arg.CreatedFrom, CreatedTo: arg.CreatedTo,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("accounting: count entries: %w", err)
	}
	out := make([]Entry, 0, len(rows))
	for _, r := range rows {
		out = append(out, entryOf(r))
	}
	return out, total, nil
}

// GetEntry returns one ledger row of the book.
func (s *Service) GetEntry(ctx context.Context, c Caller, orgUUID *uuid.UUID, id uuid.UUID) (Entry, error) {
	book, err := s.readBook(ctx, c, orgUUID)
	if err != nil {
		return Entry{}, err
	}
	return s.entry(ctx, s.q, book.ID, id)
}

func (s *Service) entry(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (Entry, error) {
	rows, err := q.SearchFinanceEntries(ctx, db.SearchFinanceEntriesParams{
		OrganizationID: orgID, EntryUuid: pgtype.UUID{Bytes: id, Valid: true}, PageLimit: 1,
	})
	if err != nil {
		return Entry{}, fmt.Errorf("accounting: entry: %w", err)
	}
	if len(rows) == 0 {
		return Entry{}, ErrEntryNotFound
	}
	return entryOf(rows[0]), nil
}

func validDirection(d string) bool {
	switch d {
	case accounting.DirectionIncome, accounting.DirectionExpense, accounting.DirectionCharge,
		accounting.DirectionCollection, accounting.DirectionPayment:
		return true
	}
	return false
}

// --- Writes ------------------------------------------------------------------

// EntryInput is a manual income, expense or cari charge. Income/expense hit
// a cash/bank account and/or a cari (a sale or purchase on credit); a charge
// hits a cari only. The cari is named by CariUUID or by the counterparty
// organization (opened when missing).
type EntryInput struct {
	Direction       string
	Category        string
	Amount          string
	Currency        string // empty: the organization's currency
	AccountUUID     *uuid.UUID
	CariUUID        *uuid.UUID
	CounterpartyOrg *uuid.UUID
	Description     string
	// IdempotencyKey makes a retried request a no-op (the source UUID of
	// the row); empty means a fresh key.
	IdempotencyKey *uuid.UUID
}

// SettlementInput is a collection (the counterparty paid us) or a payment
// (we paid the counterparty): a cash/bank movement that closes the cari and
// never writes income or expense (TEC-99 decision 2).
type SettlementInput struct {
	AccountUUID     *uuid.UUID
	CariUUID        *uuid.UUID
	CounterpartyOrg *uuid.UUID
	Amount          string
	Currency        string
	Description     string
	IdempotencyKey  *uuid.UUID
}

// CreateEntry books a manual entry in the active organization's book.
func (s *Service) CreateEntry(ctx context.Context, c Caller, in EntryInput) (Entry, bool, error) {
	switch in.Direction {
	case accounting.DirectionIncome, accounting.DirectionExpense, accounting.DirectionCharge:
	case accounting.DirectionCollection, accounting.DirectionPayment:
		return Entry{}, false, invalid("direction", "use the collections or payments endpoint")
	default:
		return Entry{}, false, invalid("direction", "must be income, expense or charge")
	}
	cat, ok := accounting.LookupCategory(in.Category)
	if !ok || !cat.Manual || cat.Direction != in.Direction {
		return Entry{}, false, invalid("category", "is not a manual "+in.Direction+" category")
	}
	if in.Direction == accounting.DirectionCharge && in.AccountUUID != nil {
		return Entry{}, false, invalid("account_uuid", "a charge books a cari only")
	}
	if in.Direction == accounting.DirectionCharge && in.CariUUID == nil && in.CounterpartyOrg == nil {
		return Entry{}, false, invalid("counterparty_organization_uuid", "a charge needs a cari")
	}
	if in.AccountUUID == nil && in.CariUUID == nil && in.CounterpartyOrg == nil {
		return Entry{}, false, invalid("account_uuid", "an entry needs an account or a cari")
	}
	return s.write(ctx, c, in.Direction, in.Category, in.Amount, in.Currency, in.AccountUUID, in.CariUUID,
		in.CounterpartyOrg, in.Description, in.IdempotencyKey)
}

// Settle books a collection or a payment.
func (s *Service) Settle(ctx context.Context, c Caller, direction string, in SettlementInput) (Entry, bool, error) {
	var category string
	switch direction {
	case accounting.DirectionCollection:
		category = accounting.CategoryCollection
	case accounting.DirectionPayment:
		category = accounting.CategoryPayment
	default:
		return Entry{}, false, invalid("direction", "must be collection or payment")
	}
	if in.AccountUUID == nil {
		return Entry{}, false, invalid("account_uuid", "a cash or bank account is required")
	}
	if in.CariUUID == nil && in.CounterpartyOrg == nil {
		return Entry{}, false, invalid("cari_uuid", "a cari is required")
	}
	return s.write(ctx, c, direction, category, in.Amount, in.Currency, in.AccountUUID, in.CariUUID,
		in.CounterpartyOrg, in.Description, in.IdempotencyKey)
}

func (s *Service) write(ctx context.Context, c Caller, direction, category, amount, currency string,
	accountUUID, cariUUID, counterpartyUUID *uuid.UUID, description string, key *uuid.UUID) (Entry, bool, error) {
	book, err := s.writeBook(ctx, c)
	if err != nil {
		return Entry{}, false, err
	}
	description = strings.TrimSpace(description)
	if len([]rune(description)) > 1000 {
		return Entry{}, false, invalid("description", "must be at most 1000 characters")
	}
	if strings.TrimSpace(amount) == "" {
		return Entry{}, false, invalid("amount", "is required")
	}
	cur := book.Currency
	if strings.TrimSpace(currency) != "" {
		if cur, err = fxrates.NormalizeCode(currency); err != nil {
			return Entry{}, false, invalid("currency", "must be a three-letter ISO-4217 code")
		}
	}
	e := posting.Entry{
		OrganizationID: book.ID, Category: category, Amount: strings.TrimSpace(amount), Currency: cur,
		Description: description, ActorUserID: c.actor(),
		Source: posting.Source{Type: SourceManual, UUID: uuid.New()},
	}
	if key != nil && *key != uuid.Nil {
		e.Source.UUID = *key
	}
	if accountUUID != nil {
		a, err := s.q.GetFinanceAccountByUUID(ctx, *accountUUID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && a.OrganizationID != book.ID) {
			return Entry{}, false, ErrAccountNotFound
		}
		if err != nil {
			return Entry{}, false, fmt.Errorf("accounting: account: %w", err)
		}
		if !a.Active {
			return Entry{}, false, invalid("account_uuid", "the account is inactive")
		}
		e.AccountID = a.ID
	}
	if e.CounterpartyOrgID, err = s.counterparty(ctx, book, cariUUID, counterpartyUUID); err != nil {
		return Entry{}, false, err
	}

	var res posting.Result
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		switch direction {
		case accounting.DirectionIncome:
			res, err = s.poster.PostIncome(ctx, tx, e)
		case accounting.DirectionExpense:
			res, err = s.poster.PostExpense(ctx, tx, e)
		case accounting.DirectionCharge:
			res, err = s.poster.Charge(ctx, tx, e)
		case accounting.DirectionCollection:
			res, err = s.poster.Collect(ctx, tx, e)
		case accounting.DirectionPayment:
			res, err = s.poster.Pay(ctx, tx, e)
		}
		return err
	})
	if err != nil {
		return Entry{}, false, postingErr(err)
	}
	out, err := s.entry(ctx, s.q, book.ID, res.Entry.Uuid)
	return out, res.Replayed, err
}

// counterparty resolves the counterparty organization of a write: the cari
// (an organization cari of the book) or the organization itself, which must
// be the book's parent or one of its direct children (K9: the supplier is
// one level up).
func (s *Service) counterparty(ctx context.Context, book db.Organization, cariUUID, orgUUID *uuid.UUID) (int64, error) {
	switch {
	case cariUUID != nil:
		ca, err := s.q.GetCariAccountByUUID(ctx, *cariUUID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && ca.OrganizationID != book.ID) {
			return 0, ErrCariNotFound
		}
		if err != nil {
			return 0, fmt.Errorf("accounting: cari: %w", err)
		}
		if !ca.CounterpartyOrgID.Valid {
			return 0, invalid("cari_uuid", "customer cari accounts are not writable in F1")
		}
		if orgUUID != nil {
			return 0, invalid("counterparty_organization_uuid", "give either cari_uuid or counterparty_organization_uuid")
		}
		return ca.CounterpartyOrgID.Int64, nil
	case orgUUID != nil:
		o, err := s.q.GetOrganizationByUUID(ctx, *orgUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrCounterparty
		}
		if err != nil {
			return 0, fmt.Errorf("accounting: counterparty: %w", err)
		}
		parent := book.ParentID.Valid && book.ParentID.Int64 == o.ID
		child := o.ParentID.Valid && o.ParentID.Int64 == book.ID
		if o.BrandID != book.BrandID || o.ID == book.ID || (!parent && !child) {
			return 0, ErrCounterparty
		}
		return o.ID, nil
	default:
		return 0, nil
	}
}

// Void reverses a manual entry (and nothing else) of the active
// organization's book. Entries sourced by other modules are voided by their
// source (VoidBySourceTx); disputes are TEC-174.
func (s *Service) Void(ctx context.Context, c Caller, id uuid.UUID, reason string) (Entry, error) {
	book, err := s.writeBook(ctx, c)
	if err != nil {
		return Entry{}, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Entry{}, invalid("reason", "is required")
	}
	if len([]rune(reason)) > 1000 {
		return Entry{}, invalid("reason", "must be at most 1000 characters")
	}
	row, err := s.q.GetFinanceEntryInOrgByUUID(ctx, db.GetFinanceEntryInOrgByUUIDParams{Uuid: id, OrganizationID: book.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Entry{}, ErrEntryNotFound
	}
	if err != nil {
		return Entry{}, fmt.Errorf("accounting: entry: %w", err)
	}
	if row.SourceType.String != SourceManual || !row.SourceUuid.Valid || row.ReversalOfID.Valid {
		return Entry{}, ErrNotVoidable
	}
	var reversal db.FinanceEntry
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		res, err := s.poster.VoidBySourceTx(ctx, tx,
			posting.Source{Type: SourceManual, UUID: uuid.UUID(row.SourceUuid.Bytes)}, reason, c.actor())
		if err != nil {
			return err
		}
		for _, r := range res.Reversals {
			if r.ReversalOfID.Int64 == row.ID {
				reversal = r
			}
		}
		if reversal.ID == 0 {
			return ErrNotVoidable // already reversed
		}
		return nil
	})
	if err != nil {
		return Entry{}, postingErr(err)
	}
	return s.entry(ctx, s.q, book.ID, reversal.Uuid)
}

func postingErr(err error) error {
	var ve *ValidationError
	switch {
	case errors.As(err, &ve), errors.Is(err, ErrNotVoidable):
		return err
	case errors.Is(err, posting.ErrIdempotencyConflict):
		return ErrIdempotencyConflict
	case errors.Is(err, posting.ErrCrossBrand), errors.Is(err, posting.ErrOrganizationNotFound):
		return ErrCounterparty
	case errors.Is(err, fxrates.ErrRateNotFound):
		return ErrRateNotFound
	case errors.Is(err, posting.ErrInvalid):
		return invalid("amount", strings.TrimPrefix(err.Error(), "posting: invalid request: "))
	default:
		return err
	}
}
