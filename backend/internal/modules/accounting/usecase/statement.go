package usecase

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-175 (F1-07e): cari statement (ekstre) and balance report.
//
// A statement is read from the book that owns the cari (the active
// organization, or an organization below it: TEC-172 readBook), so a dealer
// reads its own cari with its distributor and never the distributor's book.
// Columns follow the cari sign of 000047 seen from the book owner: debit
// (borç) raises what the counterparty owes, credit (alacak) lowers it. A
// reversal row is shown in the opposite column of the row it reverses, with
// a positive amount, so the running balance stays a plain sum.

// StatementLine is one ledger row of a statement period.
type StatementLine struct {
	UUID           uuid.UUID  `json:"uuid"`
	Date           time.Time  `json:"date"`
	Direction      string     `json:"direction"`
	Category       string     `json:"category"`
	CategoryLabel  string     `json:"category_label"`
	Description    string     `json:"description"`
	SourceType     *string    `json:"source_type"`
	SourceUUID     *uuid.UUID `json:"source_uuid"`
	SourceLabel    string     `json:"source_label"`
	ReversalOfUUID *uuid.UUID `json:"reversal_of_uuid"`
	// Reversed: a later row reverses this one (TEC-195: such a row is no
	// longer disputable).
	Reversed     bool   `json:"reversed"`
	OrigCurrency string `json:"orig_currency"`
	OrigAmount   string `json:"orig_amount"`
	Debit        string `json:"debit"`
	Credit       string `json:"credit"`
	Balance      string `json:"balance"`
}

// Statement is the cari statement of one period. From/To are calendar days
// (UTC, inclusive); rows before From make up the opening balance.
type Statement struct {
	Organization   Ref             `json:"organization"`
	Cari           Cari            `json:"cari"`
	Currency       string          `json:"currency"`
	From           *string         `json:"from"`
	To             *string         `json:"to"`
	OpeningBalance string          `json:"opening_balance"`
	TotalDebit     string          `json:"total_debit"`
	TotalCredit    string          `json:"total_credit"`
	ClosingBalance string          `json:"closing_balance"`
	Lines          []StatementLine `json:"lines"`
	GeneratedAt    time.Time       `json:"generated_at"`
}

// StatementPeriod is the requested period (calendar days, both optional).
type StatementPeriod struct {
	From, To *time.Time
}

func (p StatementPeriod) validate() error {
	if p.From != nil && p.To != nil && p.To.Before(*p.From) {
		return invalid("to", "must not be before from")
	}
	return nil
}

// ResolveCari returns the book and the cari a statement request targets,
// with the TEC-172 read scope (anything outside it reads as not found).
func (s *Service) ResolveCari(ctx context.Context, c Caller, orgUUID *uuid.UUID, id uuid.UUID) (db.Organization, error) {
	book, err := s.readBook(ctx, c, orgUUID)
	if err != nil {
		return db.Organization{}, err
	}
	if _, err := s.cariRow(ctx, book.ID, id); err != nil {
		return db.Organization{}, err
	}
	return book, nil
}

// GetStatement returns the statement of a cari of the book. Labels are in
// locale.
func (s *Service) GetStatement(ctx context.Context, c Caller, orgUUID *uuid.UUID, id uuid.UUID, p StatementPeriod, loc i18n.Locale) (Statement, error) {
	if err := p.validate(); err != nil {
		return Statement{}, err
	}
	book, err := s.readBook(ctx, c, orgUUID)
	if err != nil {
		return Statement{}, err
	}
	return s.statement(ctx, book, id, p, loc)
}

func (s *Service) cariRow(ctx context.Context, orgID int64, id uuid.UUID) (db.GetCariAccountWithBalanceRow, error) {
	r, err := s.q.GetCariAccountWithBalance(ctx, db.GetCariAccountWithBalanceParams{Uuid: id, OrganizationID: orgID})
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrCariNotFound
	}
	if err != nil {
		return r, fmt.Errorf("accounting: cari: %w", err)
	}
	return r, nil
}

// statement builds the statement of a cari in book; the caller has checked
// that the book is readable.
func (s *Service) statement(ctx context.Context, book db.Organization, id uuid.UUID, p StatementPeriod, loc i18n.Locale) (Statement, error) {
	if err := p.validate(); err != nil {
		return Statement{}, err
	}
	row, err := s.cariRow(ctx, book.ID, id)
	if err != nil {
		return Statement{}, err
	}
	cariID := pgtype.Int8{Int64: row.ID, Valid: true}
	opening := new(big.Rat)
	var from, to pgtype.Timestamptz
	if p.From != nil {
		from = pgtype.Timestamptz{Time: dayStart(*p.From), Valid: true}
		n, err := s.q.GetCariStatementOpening(ctx, db.GetCariStatementOpeningParams{
			CariID: cariID, OrganizationID: book.ID, CreatedBefore: from,
		})
		if err != nil {
			return Statement{}, fmt.Errorf("accounting: statement opening: %w", err)
		}
		opening = ratOf(n)
	}
	if p.To != nil {
		to = pgtype.Timestamptz{Time: dayStart(*p.To).AddDate(0, 0, 1), Valid: true}
	}
	rows, err := s.q.ListCariStatementLines(ctx, db.ListCariStatementLinesParams{
		CariID: cariID, OrganizationID: book.ID, CreatedFrom: from, CreatedTo: to,
	})
	if err != nil {
		return Statement{}, fmt.Errorf("accounting: statement lines: %w", err)
	}
	st := Statement{
		Organization: Ref{UUID: book.Uuid, Name: book.Name},
		Cari:         cariOf(row),
		Currency:     row.Currency,
		From:         dayString(p.From),
		To:           dayString(p.To),
		GeneratedAt:  time.Now().UTC(),
	}
	inputs := make([]lineInput, 0, len(rows))
	for _, r := range rows {
		inputs = append(inputs, lineInput{signed: ratOf(r.SignedAmount)})
	}
	sums := runStatement(opening, inputs)
	st.OpeningBalance = sums.opening
	st.TotalDebit, st.TotalCredit, st.ClosingBalance = sums.debit, sums.credit, sums.closing
	st.Lines = make([]StatementLine, 0, len(rows))
	for i, r := range rows {
		st.Lines = append(st.Lines, statementLineOf(r, sums.lines[i], loc))
	}
	return st, nil
}

func statementLineOf(r db.ListCariStatementLinesRow, m lineMoney, loc i18n.Locale) StatementLine {
	l := StatementLine{
		UUID: r.Uuid, Date: r.CreatedAt.Time.UTC(), Direction: r.Direction, Category: r.Category,
		CategoryLabel:  i18n.Translate(loc, "accounting.category."+r.Category),
		SourceUUID:     uuidPtr(r.SourceUuid),
		ReversalOfUUID: uuidPtr(r.ReversalOfUuid),
		Reversed:       r.Reversed,
		OrigCurrency:   r.OrigCurrency, OrigAmount: posting.FormatNumeric(r.OrigAmount),
		Debit: m.debit, Credit: m.credit, Balance: m.balance,
	}
	l.Description = l.CategoryLabel
	if r.Description.Valid && r.Description.String != "" {
		l.Description = r.Description.String
	}
	if r.SourceType.Valid {
		v := r.SourceType.String
		l.SourceType = &v
		l.SourceLabel = SourceLabel(loc, v)
	}
	return l
}

// SourceLabel is the localized name of a ledger source type (the raw type
// when the catalog has no label for it).
func SourceLabel(loc i18n.Locale, sourceType string) string {
	key := "accounting.source." + sourceType
	if v := i18n.Translate(loc, key); v != key {
		return v
	}
	return sourceType
}

// lineInput / lineMoney / statementSums keep the arithmetic apart from the
// database so the running balance is unit tested.
type lineInput struct {
	signed *big.Rat
}

type lineMoney struct {
	debit, credit, balance string
}

type statementSums struct {
	opening, debit, credit, closing string
	lines                           []lineMoney
}

// runStatement splits every signed cari effect into debit/credit and
// carries the running balance from the opening balance.
func runStatement(opening *big.Rat, lines []lineInput) statementSums {
	bal := new(big.Rat).Set(opening)
	debit, credit := new(big.Rat), new(big.Rat)
	out := statementSums{opening: money(opening), lines: make([]lineMoney, 0, len(lines))}
	zero := money(new(big.Rat))
	for _, l := range lines {
		m := lineMoney{debit: zero, credit: zero}
		if l.signed.Sign() >= 0 {
			debit.Add(debit, l.signed)
			m.debit = money(l.signed)
		} else {
			abs := new(big.Rat).Neg(l.signed)
			credit.Add(credit, abs)
			m.credit = money(abs)
		}
		bal.Add(bal, l.signed)
		m.balance = money(bal)
		out.lines = append(out.lines, m)
	}
	out.debit, out.credit, out.closing = money(debit), money(credit), money(bal)
	return out
}

func money(r *big.Rat) string { return r.FloatString(2) }

// ratOf converts a stored NUMERIC(18,2) to a rational (exact: the string
// form has two decimals).
func ratOf(n pgtype.Numeric) *big.Rat {
	r, ok := new(big.Rat).SetString(posting.FormatNumeric(n))
	if !ok {
		return new(big.Rat)
	}
	return r
}

func dayStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func dayString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	v := t.Format(time.DateOnly)
	return &v
}

// --- Balance report ----------------------------------------------------------

// CariBalance is one cari of the balance report.
type CariBalance struct {
	UUID         uuid.UUID    `json:"uuid"`
	Counterparty Counterparty `json:"counterparty"`
	Currency     string       `json:"currency"`
	Active       bool         `json:"active"`
	Balance      string       `json:"balance"`
	EntryCount   int64        `json:"entry_count"`
}

// AccountBalance is one cash/bank account of the balance report.
type AccountBalance struct {
	UUID       uuid.UUID `json:"uuid"`
	Type       string    `json:"type"`
	Name       string    `json:"name"`
	Currency   string    `json:"currency"`
	Active     bool      `json:"active"`
	Balance    string    `json:"balance"`
	EntryCount int64     `json:"entry_count"`
}

// BalanceTotals sums the report. Receivable is the sum of the positive cari
// balances, Payable the sum of the negative ones (as a positive amount).
type BalanceTotals struct {
	Receivable string `json:"receivable"`
	Payable    string `json:"payable"`
	CariNet    string `json:"cari_net"`
	Cash       string `json:"cash"`
	Bank       string `json:"bank"`
	Accounts   string `json:"accounts"`
}

// BalanceReport is every cari and cash/bank balance of a book as of a day
// (end of day UTC, inclusive; nil = now).
type BalanceReport struct {
	Organization Ref              `json:"organization"`
	Currency     string           `json:"currency"`
	AsOf         *string          `json:"as_of"`
	Cari         []CariBalance    `json:"cari"`
	Accounts     []AccountBalance `json:"accounts"`
	Totals       BalanceTotals    `json:"totals"`
	GeneratedAt  time.Time        `json:"generated_at"`
}

// GetBalanceReport returns the balance report of the book.
func (s *Service) GetBalanceReport(ctx context.Context, c Caller, orgUUID *uuid.UUID, asOf *time.Time) (BalanceReport, error) {
	book, err := s.readBook(ctx, c, orgUUID)
	if err != nil {
		return BalanceReport{}, err
	}
	return s.balanceReport(ctx, book, asOf)
}

// ResolveBook returns the book a report request targets (TEC-172 scope).
func (s *Service) ResolveBook(ctx context.Context, c Caller, orgUUID *uuid.UUID) (db.Organization, error) {
	return s.readBook(ctx, c, orgUUID)
}

func (s *Service) balanceReport(ctx context.Context, book db.Organization, asOf *time.Time) (BalanceReport, error) {
	var to pgtype.Timestamptz
	if asOf != nil {
		to = pgtype.Timestamptz{Time: dayStart(*asOf).AddDate(0, 0, 1), Valid: true}
	}
	cariRows, err := s.q.ListCariBalancesAsOf(ctx, db.ListCariBalancesAsOfParams{OrganizationID: book.ID, CreatedTo: to})
	if err != nil {
		return BalanceReport{}, fmt.Errorf("accounting: cari balances: %w", err)
	}
	accRows, err := s.q.ListFinanceAccountBalancesAsOf(ctx, db.ListFinanceAccountBalancesAsOfParams{OrganizationID: book.ID, CreatedTo: to})
	if err != nil {
		return BalanceReport{}, fmt.Errorf("accounting: account balances: %w", err)
	}
	rep := BalanceReport{
		Organization: Ref{UUID: book.Uuid, Name: book.Name}, Currency: book.Currency,
		AsOf: dayString(asOf), GeneratedAt: time.Now().UTC(),
		Cari: make([]CariBalance, 0, len(cariRows)), Accounts: make([]AccountBalance, 0, len(accRows)),
	}
	recv, pay, cash, bank := new(big.Rat), new(big.Rat), new(big.Rat), new(big.Rat)
	for _, r := range cariRows {
		b := ratOf(r.Balance)
		if b.Sign() > 0 {
			recv.Add(recv, b)
		} else {
			pay.Sub(pay, b)
		}
		cp := Counterparty{Type: r.CounterpartyType}
		if r.CounterpartyType == "organization" {
			cp.UUID, cp.Name, cp.OrgType = uuidPtr(r.CounterpartyOrgUuid), r.CounterpartyOrgName.String, r.CounterpartyOrgType.String
		} else {
			cp.UUID, cp.Name = uuidPtr(r.CounterpartyUserUuid), r.CounterpartyUserName
		}
		rep.Cari = append(rep.Cari, CariBalance{
			UUID: r.Uuid, Counterparty: cp, Currency: r.Currency, Active: r.Active,
			Balance: money(b), EntryCount: r.EntryCount,
		})
	}
	for _, r := range accRows {
		b := ratOf(r.Balance)
		if r.Type == "bank" {
			bank.Add(bank, b)
		} else {
			cash.Add(cash, b)
		}
		rep.Accounts = append(rep.Accounts, AccountBalance{
			UUID: r.Uuid, Type: r.Type, Name: r.Name, Currency: r.Currency, Active: r.Active,
			Balance: money(b), EntryCount: r.EntryCount,
		})
	}
	rep.Totals = BalanceTotals{
		Receivable: money(recv), Payable: money(pay), CariNet: money(new(big.Rat).Sub(recv, pay)),
		Cash: money(cash), Bank: money(bank), Accounts: money(new(big.Rat).Add(cash, bank)),
	}
	return rep, nil
}

// --- Locale ------------------------------------------------------------------

// ResolveLocale returns the report language of the caller (K10: user ->
// active organization -> brand center -> Accept-Language -> tr). A valid
// override (request body/query) wins.
func (s *Service) ResolveLocale(ctx context.Context, c Caller, override string) i18n.Locale {
	if l, ok := i18n.Parse(override); ok {
		return l
	}
	src := i18n.Sources{AcceptLanguage: i18n.AcceptLanguage(ctx)}
	if c.UserID > 0 {
		params := db.GetLocaleSourcesParams{UserID: c.UserID}
		if c.Org.UUID != uuid.Nil {
			params.OrganizationUuid = pgtype.UUID{Bytes: c.Org.UUID, Valid: true}
		}
		if c.Org.BrandID > 0 {
			params.BrandID = pgtype.Int8{Int64: c.Org.BrandID, Valid: true}
		}
		if row, err := s.q.GetLocaleSources(ctx, params); err == nil {
			src.UserLocale, src.OrgLocale, src.CenterLocale = row.UserLocale, row.OrgLocale, row.CenterLocale
		}
	}
	return i18n.Resolve(src).Locale
}
