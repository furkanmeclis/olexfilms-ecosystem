package usecase

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Account types (finance_accounts.type).
const (
	AccountCash = "cash"
	AccountBank = "bank"
)

// Account is a cash/bank account with its ledger balance.
type Account struct {
	UUID        uuid.UUID  `json:"uuid"`
	Type        string     `json:"type"`
	Name        string     `json:"name"`
	Currency    string     `json:"currency"`
	IBAN        *string    `json:"iban"`
	Active      bool       `json:"active"`
	Balance     string     `json:"balance"`
	EntryCount  int64      `json:"entry_count"`
	LastEntryAt *time.Time `json:"last_entry_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func accountOf(a db.FinanceAccount, balance pgtype.Numeric, count int64, last pgtype.Timestamptz) Account {
	out := Account{
		UUID: a.Uuid, Type: a.Type, Name: a.Name, Currency: a.Currency, Active: a.Active,
		Balance: posting.FormatNumeric(balance), EntryCount: count,
		CreatedAt: a.CreatedAt.Time, UpdatedAt: a.UpdatedAt.Time,
	}
	if a.Iban.Valid {
		v := a.Iban.String
		out.IBAN = &v
	}
	if last.Valid {
		t := last.Time
		out.LastEntryAt = &t
	}
	return out
}

func rowAccount(r db.ListFinanceAccountsWithBalanceRow) Account {
	return accountOf(db.FinanceAccount{
		ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, BrandID: r.BrandID, Type: r.Type,
		Name: r.Name, Currency: r.Currency, Iban: r.Iban, Active: r.Active, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}, r.Balance, r.EntryCount, r.LastEntryAt)
}

// AccountFilter narrows ListAccounts (ParseAccountFilter builds it).
type AccountFilter struct {
	OrganizationUUID *uuid.UUID
	Active           *bool
	Types            []string // cash, bank
	Q                string
	Sort             apiquery.ResolvedSort
}

// ListAccounts lists the cash/bank accounts of the book with balances
// (a full array; the grid sorts and pages it client-side).
func (s *Service) ListAccounts(ctx context.Context, c Caller, f AccountFilter) ([]Account, error) {
	book, err := s.readBook(ctx, c, f.OrganizationUUID)
	if err != nil {
		return nil, err
	}
	sort := sortOrDefault(f.Sort, AccountSort)
	arg := db.ListFinanceAccountsWithBalanceParams{
		OrganizationID: book.ID, Types: f.Types, Q: likeArg(f.Q), SortKey: sort.Key, SortDesc: sort.Desc,
	}
	if f.Active != nil {
		arg.Active = pgtype.Bool{Bool: *f.Active, Valid: true}
	}
	for _, t := range f.Types {
		if t != AccountCash && t != AccountBank {
			return nil, invalid("type", "must be cash or bank")
		}
	}
	rows, err := s.q.ListFinanceAccountsWithBalance(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("accounting: list accounts: %w", err)
	}
	out := make([]Account, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowAccount(r))
	}
	return out, nil
}

// GetAccount returns one account of the book.
func (s *Service) GetAccount(ctx context.Context, c Caller, orgUUID *uuid.UUID, id uuid.UUID) (Account, error) {
	book, err := s.readBook(ctx, c, orgUUID)
	if err != nil {
		return Account{}, err
	}
	return s.account(ctx, book.ID, id)
}

func (s *Service) account(ctx context.Context, orgID int64, id uuid.UUID) (Account, error) {
	r, err := s.q.GetFinanceAccountWithBalance(ctx, db.GetFinanceAccountWithBalanceParams{Uuid: id, OrganizationID: orgID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrAccountNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("accounting: account: %w", err)
	}
	return accountOf(db.FinanceAccount{
		ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, BrandID: r.BrandID, Type: r.Type,
		Name: r.Name, Currency: r.Currency, Iban: r.Iban, Active: r.Active, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}, r.Balance, r.EntryCount, r.LastEntryAt), nil
}

// CreateAccountInput opens a cash/bank account in the organization's
// currency (K7).
type CreateAccountInput struct {
	Type string
	Name string
	IBAN *string
}

// CreateAccount opens an account in the active organization's book.
func (s *Service) CreateAccount(ctx context.Context, c Caller, in CreateAccountInput) (Account, error) {
	book, err := s.writeBook(ctx, c)
	if err != nil {
		return Account{}, err
	}
	if in.Type != AccountCash && in.Type != AccountBank {
		return Account{}, invalid("type", "must be cash or bank")
	}
	name, err := accountName(in.Name)
	if err != nil {
		return Account{}, err
	}
	iban, err := ibanArg(in.Type, trimmedPtr(in.IBAN))
	if err != nil {
		return Account{}, err
	}
	a, err := s.q.CreateFinanceAccount(ctx, db.CreateFinanceAccountParams{
		OrganizationID: book.ID, BrandID: book.BrandID, Type: in.Type, Name: name,
		Currency: book.Currency, Iban: iban, Active: true,
	})
	if err != nil {
		return Account{}, fmt.Errorf("accounting: create account: %w", err)
	}
	return s.account(ctx, book.ID, a.Uuid)
}

// UpdateAccountInput changes an account. Nil fields keep the stored value;
// ClearIBAN removes the IBAN. Accounts are never deleted, only deactivated.
type UpdateAccountInput struct {
	Name      *string
	IBAN      *string
	ClearIBAN bool
	Active    *bool
}

// UpdateAccount renames, re-IBANs or (de)activates an account of the active
// organization.
func (s *Service) UpdateAccount(ctx context.Context, c Caller, id uuid.UUID, in UpdateAccountInput) (Account, error) {
	book, err := s.writeBook(ctx, c)
	if err != nil {
		return Account{}, err
	}
	cur, err := s.q.GetFinanceAccountByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && cur.OrganizationID != book.ID) {
		return Account{}, ErrAccountNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("accounting: account: %w", err)
	}
	arg := db.UpdateFinanceAccountParams{ID: cur.ID, OrganizationID: book.ID, Name: cur.Name, Iban: cur.Iban, Active: cur.Active}
	if in.Name != nil {
		if arg.Name, err = accountName(*in.Name); err != nil {
			return Account{}, err
		}
	}
	switch {
	case in.ClearIBAN:
		arg.Iban = pgtype.Text{}
	case in.IBAN != nil:
		if arg.Iban, err = ibanArg(cur.Type, trimmedPtr(in.IBAN)); err != nil {
			return Account{}, err
		}
	}
	if in.Active != nil {
		arg.Active = *in.Active
	}
	if _, err := s.q.UpdateFinanceAccount(ctx, arg); err != nil {
		return Account{}, fmt.Errorf("accounting: update account: %w", err)
	}
	return s.account(ctx, book.ID, id)
}

func accountName(raw string) (string, error) {
	n := strings.TrimSpace(raw)
	if n == "" {
		return "", invalid("name", "is required")
	}
	if len([]rune(n)) > 200 {
		return "", invalid("name", "must be at most 200 characters")
	}
	return n, nil
}

func ibanArg(accountType string, raw *string) (pgtype.Text, error) {
	if raw == nil || *raw == "" {
		return pgtype.Text{}, nil
	}
	if accountType != AccountBank {
		return pgtype.Text{}, invalid("iban", "only a bank account has an IBAN")
	}
	iban, err := NormalizeIBAN(*raw)
	if err != nil {
		return pgtype.Text{}, err
	}
	return pgtype.Text{String: iban, Valid: true}, nil
}

// Same shape as chk_finance_accounts_iban (000047).
var ibanRe = regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}$`)

// ibanLengths are the registry lengths of the countries the brands work in;
// other countries are checked by shape and checksum only.
var ibanLengths = map[string]int{
	"TR": 26, "DE": 22, "AT": 20, "NL": 18, "BE": 16, "FR": 27, "IT": 27, "ES": 24,
	"GB": 22, "BG": 22, "GR": 27, "UA": 29, "AZ": 28, "PL": 28, "RO": 24, "CH": 21,
	"SA": 24, "AE": 23, "GE": 22,
}

// NormalizeIBAN removes spaces, upper-cases and validates an IBAN (ISO
// 13616: country length and the mod-97 check digits).
func NormalizeIBAN(raw string) (string, error) {
	iban := strings.ToUpper(strings.Join(strings.Fields(raw), ""))
	iban = strings.ReplaceAll(iban, "-", "")
	if !ibanRe.MatchString(iban) {
		return "", invalid("iban", "is not a valid IBAN")
	}
	if n, ok := ibanLengths[iban[:2]]; ok && len(iban) != n {
		return "", invalid("iban", fmt.Sprintf("a %s IBAN has %d characters", iban[:2], n))
	}
	var digits strings.Builder
	for _, r := range iban[4:] + iban[:4] {
		if r >= 'A' && r <= 'Z' {
			fmt.Fprintf(&digits, "%d", r-'A'+10)
		} else {
			digits.WriteRune(r)
		}
	}
	n, ok := new(big.Int).SetString(digits.String(), 10)
	if !ok || new(big.Int).Mod(n, big.NewInt(97)).Int64() != 1 {
		return "", invalid("iban", "IBAN check digits do not match")
	}
	return iban, nil
}
