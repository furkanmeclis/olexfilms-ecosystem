package usecase

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-346 (F3-07f): accounting reports of an organization's own book:
// income/expense/net (P&L), service and product sale margin, cari aging and
// staff cost. They are plain queries over the ledger and the F3-07c/d/e
// snapshots; nothing is stored.
//
// Scope (conservative default of the issue): every report reads the active
// organization's own book only. Unlike the statement and balance report
// (TEC-172 readBook), naming an organization below the active one reads as
// not found, so a center or distributor never sees a dealer's internal P&L,
// margin or payroll; it keeps seeing its own cari with the dealer (F1).
//
// Amounts are in the book currency (finance_entries.amount); periods are
// calendar days in UTC, both ends inclusive, like the statement.

// P&L grouping (group query parameter).
const (
	PnlGroupMonth    = "month"
	PnlGroupCategory = "category"
)

// Cari aging sides and counterparty kinds.
const (
	AgingReceivable = "receivable"
	AgingPayable    = "payable"
)

// ownBook returns the active organization: the only book these reports
// read. A different organization_uuid reads as not found.
func (s *Service) ownBook(ctx context.Context, c Caller, orgUUID *uuid.UUID) (db.Organization, error) {
	active, err := s.activeOrg(ctx, c)
	if err != nil {
		return db.Organization{}, err
	}
	if orgUUID != nil && *orgUUID != active.Uuid {
		return db.Organization{}, ErrBookNotFound
	}
	return active, nil
}

// ResolveOwnBook returns the book a TEC-346 report request targets.
func (s *Service) ResolveOwnBook(ctx context.Context, c Caller, orgUUID *uuid.UUID) (db.Organization, error) {
	return s.ownBook(ctx, c, orgUUID)
}

// PnlGroup validates the P&L grouping ("" = month).
func PnlGroup(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "", PnlGroupMonth:
		return PnlGroupMonth, nil
	case PnlGroupCategory:
		return PnlGroupCategory, nil
	default:
		return "", invalid("group", "must be month or category")
	}
}

func (p StatementPeriod) createdRange() (pgtype.Timestamptz, pgtype.Timestamptz) {
	var from, to pgtype.Timestamptz
	if p.From != nil {
		from = pgtype.Timestamptz{Time: dayStart(*p.From), Valid: true}
	}
	if p.To != nil {
		to = pgtype.Timestamptz{Time: dayStart(*p.To).AddDate(0, 0, 1), Valid: true}
	}
	return from, to
}

// --- P&L -----------------------------------------------------------------------

// PnlLine is one month or category of the P&L.
type PnlLine struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Income     string `json:"income"`
	Expense    string `json:"expense"`
	Net        string `json:"net"`
	EntryCount int64  `json:"entry_count"`
}

// PnlTotals sums the P&L.
type PnlTotals struct {
	Income  string `json:"income"`
	Expense string `json:"expense"`
	Net     string `json:"net"`
}

// PnlReport is the income, expense and net of the book over a period.
type PnlReport struct {
	Organization Ref       `json:"organization"`
	Currency     string    `json:"currency"`
	From         *string   `json:"from"`
	To           *string   `json:"to"`
	Group        string    `json:"group"`
	Lines        []PnlLine `json:"lines"`
	Totals       PnlTotals `json:"totals"`
	GeneratedAt  time.Time `json:"generated_at"`
}

// GetPnlReport returns the P&L of the caller's own book.
func (s *Service) GetPnlReport(ctx context.Context, c Caller, orgUUID *uuid.UUID, p StatementPeriod, group string, loc i18n.Locale) (PnlReport, error) {
	if err := p.validate(); err != nil {
		return PnlReport{}, err
	}
	g, err := PnlGroup(group)
	if err != nil {
		return PnlReport{}, err
	}
	book, err := s.ownBook(ctx, c, orgUUID)
	if err != nil {
		return PnlReport{}, err
	}
	return s.pnlReport(ctx, book, p, g, loc)
}

type pnlAcc struct {
	key, label      string
	income, expense *big.Rat
	count           int64
	incomeFirst     bool
}

func (s *Service) pnlReport(ctx context.Context, book db.Organization, p StatementPeriod, group string, loc i18n.Locale) (PnlReport, error) {
	from, to := p.createdRange()
	rows, err := s.q.ListPnlSums(ctx, db.ListPnlSumsParams{OrganizationID: book.ID, CreatedFrom: from, CreatedTo: to})
	if err != nil {
		return PnlReport{}, fmt.Errorf("accounting: pnl: %w", err)
	}
	rep := PnlReport{
		Organization: Ref{UUID: book.Uuid, Name: book.Name}, Currency: book.Currency,
		From: dayString(p.From), To: dayString(p.To), Group: group, GeneratedAt: time.Now().UTC(),
	}
	byKey := map[string]*pnlAcc{}
	order := []*pnlAcc{}
	totalIn, totalOut := new(big.Rat), new(big.Rat)
	for _, r := range rows {
		key, label := r.Month, r.Month
		if group == PnlGroupCategory {
			key = r.Category
			label = categoryLabel(loc, r.Category)
		}
		a, ok := byKey[key]
		if !ok {
			a = &pnlAcc{key: key, label: label, income: new(big.Rat), expense: new(big.Rat),
				incomeFirst: r.Direction == accounting.DirectionIncome}
			byKey[key] = a
			order = append(order, a)
		}
		amount := ratOf(r.Total)
		if r.Direction == accounting.DirectionIncome {
			a.income.Add(a.income, amount)
			totalIn.Add(totalIn, amount)
		} else {
			a.expense.Add(a.expense, amount)
			totalOut.Add(totalOut, amount)
		}
		a.count += r.EntryCount
	}
	if group == PnlGroupCategory {
		// Income categories first, then expense; larger amounts first.
		sort.SliceStable(order, func(i, j int) bool {
			if order[i].incomeFirst != order[j].incomeFirst {
				return order[i].incomeFirst
			}
			vi, vj := order[i].expense, order[j].expense
			if order[i].incomeFirst {
				vi, vj = order[i].income, order[j].income
			}
			if c := vi.Cmp(vj); c != 0 {
				return c > 0
			}
			return order[i].key < order[j].key
		})
	}
	rep.Lines = make([]PnlLine, 0, len(order))
	for _, a := range order {
		rep.Lines = append(rep.Lines, PnlLine{
			Key: a.key, Label: a.label, Income: money(a.income), Expense: money(a.expense),
			Net: money(new(big.Rat).Sub(a.income, a.expense)), EntryCount: a.count,
		})
	}
	rep.Totals = PnlTotals{Income: money(totalIn), Expense: money(totalOut), Net: money(new(big.Rat).Sub(totalIn, totalOut))}
	return rep, nil
}

// categoryLabel is the localized category name (the key when the catalog
// has no label, e.g. a module-specific category).
func categoryLabel(loc i18n.Locale, category string) string {
	key := "accounting.category." + category
	if v := i18n.Translate(loc, key); v != key {
		return v
	}
	return category
}

// --- Margin --------------------------------------------------------------------

// MarginFigures are revenue, cost, gross profit and margin (percent of
// revenue). Cost, profit and margin are nil without pricing.purchase.read;
// margin is nil when there is no revenue.
type MarginFigures struct {
	Revenue     string  `json:"revenue"`
	Cost        *string `json:"cost"`
	GrossProfit *string `json:"gross_profit"`
	MarginPct   *string `json:"margin_pct"`
}

// MarginServices is the service part of the margin report (F3-07c).
type MarginServices struct {
	ServiceCount int64 `json:"service_count"`
	MarginFigures
}

// MarginProduct is one product of the product sale breakdown (F3-07d).
type MarginProduct struct {
	ProductUUID uuid.UUID `json:"product_uuid"`
	SKU         string    `json:"sku"`
	Name        string    `json:"name"`
	Quantity    string    `json:"quantity"`
	SaleCount   int64     `json:"sale_count"`
	// CostIncomplete: some lines had no purchase cost snapshot (counted as
	// zero cost). Nil without pricing.purchase.read.
	CostIncomplete *bool `json:"cost_incomplete"`
	MarginFigures
}

// MarginReport is the service and product sale margin of the book.
type MarginReport struct {
	Organization Ref             `json:"organization"`
	Currency     string          `json:"currency"`
	From         *string         `json:"from"`
	To           *string         `json:"to"`
	CostVisible  bool            `json:"cost_visible"`
	Services     MarginServices  `json:"services"`
	ProductSales MarginFigures   `json:"product_sales"`
	Products     []MarginProduct `json:"products"`
	Total        MarginFigures   `json:"total"`
	GeneratedAt  time.Time       `json:"generated_at"`
}

// GetMarginReport returns the margin report of the caller's own book.
// showCost reports whether the caller holds pricing.purchase.read.
func (s *Service) GetMarginReport(ctx context.Context, c Caller, orgUUID *uuid.UUID, p StatementPeriod, showCost bool) (MarginReport, error) {
	if err := p.validate(); err != nil {
		return MarginReport{}, err
	}
	book, err := s.ownBook(ctx, c, orgUUID)
	if err != nil {
		return MarginReport{}, err
	}
	return s.marginReport(ctx, book, p, showCost)
}

func (s *Service) marginReport(ctx context.Context, book db.Organization, p StatementPeriod, showCost bool) (MarginReport, error) {
	from, to := p.createdRange()
	svc, err := s.q.GetMarginServiceSummary(ctx, db.GetMarginServiceSummaryParams{
		OrganizationID: book.ID, CreatedFrom: from, CreatedTo: to,
	})
	if err != nil {
		return MarginReport{}, fmt.Errorf("accounting: service margin: %w", err)
	}
	rows, err := s.q.ListMarginProductSales(ctx, db.ListMarginProductSalesParams{
		OrganizationID: book.ID, SoldFrom: from, SoldTo: to,
	})
	if err != nil {
		return MarginReport{}, fmt.Errorf("accounting: product margin: %w", err)
	}
	rep := MarginReport{
		Organization: Ref{UUID: book.Uuid, Name: book.Name}, Currency: book.Currency,
		From: dayString(p.From), To: dayString(p.To), CostVisible: showCost, GeneratedAt: time.Now().UTC(),
		Services: MarginServices{
			ServiceCount:  svc.ServiceCount,
			MarginFigures: marginFigures(ratOf(svc.Revenue), ratOf(svc.Cost), showCost),
		},
		Products: make([]MarginProduct, 0, len(rows)),
	}
	salesRev, salesCost := new(big.Rat), new(big.Rat)
	for _, r := range rows {
		rev, cost := ratOf(r.Revenue), ratOf(r.Cost)
		salesRev.Add(salesRev, rev)
		salesCost.Add(salesCost, cost)
		mp := MarginProduct{
			ProductUUID: r.ProductUuid, SKU: r.Sku, Name: r.Name, Quantity: money(ratOf(r.Quantity)),
			SaleCount: r.SaleCount, MarginFigures: marginFigures(rev, cost, showCost),
		}
		if showCost {
			incomplete := r.LinesWithoutCost > 0
			mp.CostIncomplete = &incomplete
		}
		rep.Products = append(rep.Products, mp)
	}
	rep.ProductSales = marginFigures(salesRev, salesCost, showCost)
	rep.Total = marginFigures(
		new(big.Rat).Add(ratOf(svc.Revenue), salesRev),
		new(big.Rat).Add(ratOf(svc.Cost), salesCost), showCost)
	return rep, nil
}

func marginFigures(revenue, cost *big.Rat, showCost bool) MarginFigures {
	f := MarginFigures{Revenue: money(revenue)}
	if !showCost {
		return f
	}
	c := money(cost)
	profit := new(big.Rat).Sub(revenue, cost)
	g := money(profit)
	f.Cost, f.GrossProfit = &c, &g
	if revenue.Sign() > 0 {
		m := new(big.Rat).Mul(profit, big.NewRat(100, 1))
		m.Quo(m, revenue)
		v := m.FloatString(2)
		f.MarginPct = &v
	}
	return f
}

// --- Cari aging ------------------------------------------------------------------

// AgingBuckets splits an open balance by the age of the rows that make it
// up (days before the as-of day).
type AgingBuckets struct {
	Days0To30  string `json:"days_0_30"`
	Days31To60 string `json:"days_31_60"`
	Days61To90 string `json:"days_61_90"`
	Days90Plus string `json:"days_90_plus"`
}

// AgingLine is one open cari of the aging report. Balance is the open
// amount as a positive number; Side says who owes whom.
type AgingLine struct {
	CariUUID     uuid.UUID    `json:"cari_uuid"`
	Counterparty Counterparty `json:"counterparty"`
	Side         string       `json:"side"`
	Balance      string       `json:"balance"`
	Buckets      AgingBuckets `json:"buckets"`
}

// AgingTotal sums one side of the aging report.
type AgingTotal struct {
	Balance string       `json:"balance"`
	Buckets AgingBuckets `json:"buckets"`
}

// AgingTotals holds both sides.
type AgingTotals struct {
	Receivable AgingTotal `json:"receivable"`
	Payable    AgingTotal `json:"payable"`
}

// AgingReport is the cari aging of the book as of a day (customer and
// organization cari accounts with an open balance; receivables first).
type AgingReport struct {
	Organization Ref         `json:"organization"`
	Currency     string      `json:"currency"`
	AsOf         string      `json:"as_of"`
	Lines        []AgingLine `json:"lines"`
	Totals       AgingTotals `json:"totals"`
	GeneratedAt  time.Time   `json:"generated_at"`
}

// GetCariAgingReport returns the cari aging of the caller's own book as of
// a day (nil = today, UTC).
func (s *Service) GetCariAgingReport(ctx context.Context, c Caller, orgUUID *uuid.UUID, asOf *time.Time) (AgingReport, error) {
	book, err := s.ownBook(ctx, c, orgUUID)
	if err != nil {
		return AgingReport{}, err
	}
	return s.agingReport(ctx, book, asOf)
}

// agingEntry is one cari row of the aging walk.
type agingEntry struct {
	at     time.Time
	signed *big.Rat
}

type agingSums [4]*big.Rat

func newAgingSums() agingSums {
	return agingSums{new(big.Rat), new(big.Rat), new(big.Rat), new(big.Rat)}
}

func (a agingSums) buckets() AgingBuckets {
	return AgingBuckets{Days0To30: money(a[0]), Days31To60: money(a[1]), Days61To90: money(a[2]), Days90Plus: money(a[3])}
}

func (a agingSums) add(b agingSums) {
	for i := range a {
		a[i].Add(a[i], b[i])
	}
}

// agingBucket maps an age in days to its bucket: 0-30, 31-60, 61-90, 90+.
func agingBucket(days int) int {
	switch {
	case days <= 30:
		return 0
	case days <= 60:
		return 1
	case days <= 90:
		return 2
	default:
		return 3
	}
}

// ageBalance spreads an open balance (positive) over the rows that raised
// it, newest first (FIFO: payments settle the oldest rows). entries are the
// cari rows newest first, signed so that a positive amount raises the
// balance. Whatever no row explains lands in the oldest bucket.
func ageBalance(open *big.Rat, entries []agingEntry, asOf time.Time) agingSums {
	out := newAgingSums()
	left := new(big.Rat).Set(open)
	ref := dayStart(asOf)
	for _, e := range entries {
		if left.Sign() <= 0 {
			break
		}
		if e.signed.Sign() <= 0 {
			continue
		}
		part := e.signed
		if part.Cmp(left) > 0 {
			part = left
		}
		days := int(ref.Sub(dayStart(e.at)).Hours() / 24)
		if days < 0 {
			days = 0
		}
		b := agingBucket(days)
		out[b].Add(out[b], part)
		left = new(big.Rat).Sub(left, part)
	}
	if left.Sign() > 0 {
		out[3].Add(out[3], left)
	}
	return out
}

func (s *Service) agingReport(ctx context.Context, book db.Organization, asOf *time.Time) (AgingReport, error) {
	day := dayStart(time.Now().UTC())
	if asOf != nil {
		day = dayStart(*asOf)
	}
	to := pgtype.Timestamptz{Time: day.AddDate(0, 0, 1), Valid: true}
	cariRows, err := s.q.ListCariBalancesAsOf(ctx, db.ListCariBalancesAsOfParams{OrganizationID: book.ID, CreatedTo: to})
	if err != nil {
		return AgingReport{}, fmt.Errorf("accounting: aging balances: %w", err)
	}
	lineRows, err := s.q.ListCariAgingLines(ctx, db.ListCariAgingLinesParams{OrganizationID: book.ID, CreatedTo: to})
	if err != nil {
		return AgingReport{}, fmt.Errorf("accounting: aging lines: %w", err)
	}
	entries := map[uuid.UUID][]agingEntry{}
	for _, r := range lineRows {
		entries[r.CariUuid] = append(entries[r.CariUuid], agingEntry{at: r.CreatedAt.Time.UTC(), signed: ratOf(r.SignedAmount)})
	}
	rep := AgingReport{
		Organization: Ref{UUID: book.Uuid, Name: book.Name}, Currency: book.Currency,
		AsOf: day.Format(time.DateOnly), GeneratedAt: time.Now().UTC(),
	}
	recvBal, payBal := new(big.Rat), new(big.Rat)
	recvSums, paySums := newAgingSums(), newAgingSums()
	var recv, pay []AgingLine
	for _, r := range cariRows {
		bal := ratOf(r.Balance)
		if bal.Sign() == 0 {
			continue
		}
		cp := Counterparty{Type: r.CounterpartyType}
		if r.CounterpartyType == "organization" {
			cp.UUID, cp.Name, cp.OrgType = uuidPtr(r.CounterpartyOrgUuid), r.CounterpartyOrgName.String, r.CounterpartyOrgType.String
		} else {
			cp.UUID, cp.Name = uuidPtr(r.CounterpartyUserUuid), r.CounterpartyUserName
		}
		side := AgingReceivable
		walk := entries[r.Uuid]
		if bal.Sign() < 0 {
			// A payable: walk the rows that lowered the balance.
			side = AgingPayable
			bal = new(big.Rat).Neg(bal)
			neg := make([]agingEntry, len(walk))
			for i, e := range walk {
				neg[i] = agingEntry{at: e.at, signed: new(big.Rat).Neg(e.signed)}
			}
			walk = neg
		}
		sums := ageBalance(bal, walk, day)
		line := AgingLine{CariUUID: r.Uuid, Counterparty: cp, Side: side, Balance: money(bal), Buckets: sums.buckets()}
		if side == AgingReceivable {
			recvBal.Add(recvBal, bal)
			recvSums.add(sums)
			recv = append(recv, line)
		} else {
			payBal.Add(payBal, bal)
			paySums.add(sums)
			pay = append(pay, line)
		}
	}
	rep.Lines = make([]AgingLine, 0, len(recv)+len(pay))
	rep.Lines = append(rep.Lines, recv...)
	rep.Lines = append(rep.Lines, pay...)
	rep.Totals = AgingTotals{
		Receivable: AgingTotal{Balance: money(recvBal), Buckets: recvSums.buckets()},
		Payable:    AgingTotal{Balance: money(payBal), Buckets: paySums.buckets()},
	}
	return rep, nil
}

// --- Staff cost --------------------------------------------------------------------

// StaffCostLine is the salary/advance/bonus total of one staff card.
type StaffCostLine struct {
	StaffUUID    uuid.UUID `json:"staff_uuid"`
	Name         string    `json:"name"`
	Title        *string   `json:"title"`
	Active       bool      `json:"active"`
	Salary       string    `json:"salary"`
	Advance      string    `json:"advance"`
	Bonus        string    `json:"bonus"`
	Total        string    `json:"total"`
	PaymentCount int64     `json:"payment_count"`
}

// StaffCostTotals sums the staff cost report.
type StaffCostTotals struct {
	Salary  string `json:"salary"`
	Advance string `json:"advance"`
	Bonus   string `json:"bonus"`
	Total   string `json:"total"`
}

// StaffCostReport is the staff cost of the book over a period (payment
// day, F3-07e; voided payments are left out).
type StaffCostReport struct {
	Organization Ref             `json:"organization"`
	Currency     string          `json:"currency"`
	From         *string         `json:"from"`
	To           *string         `json:"to"`
	Lines        []StaffCostLine `json:"lines"`
	Totals       StaffCostTotals `json:"totals"`
	GeneratedAt  time.Time       `json:"generated_at"`
}

// GetStaffCostReport returns the staff cost of the caller's own book.
func (s *Service) GetStaffCostReport(ctx context.Context, c Caller, orgUUID *uuid.UUID, p StatementPeriod) (StaffCostReport, error) {
	if err := p.validate(); err != nil {
		return StaffCostReport{}, err
	}
	book, err := s.ownBook(ctx, c, orgUUID)
	if err != nil {
		return StaffCostReport{}, err
	}
	return s.staffCostReport(ctx, book, p)
}

func (s *Service) staffCostReport(ctx context.Context, book db.Organization, p StatementPeriod) (StaffCostReport, error) {
	params := db.ListStaffCostTotalsParams{OrganizationID: book.ID}
	if p.From != nil {
		params.PaidFrom = pgtype.Date{Time: dayStart(*p.From), Valid: true}
	}
	if p.To != nil {
		params.PaidTo = pgtype.Date{Time: dayStart(*p.To), Valid: true}
	}
	rows, err := s.q.ListStaffCostTotals(ctx, params)
	if err != nil {
		return StaffCostReport{}, fmt.Errorf("accounting: staff cost: %w", err)
	}
	rep := StaffCostReport{
		Organization: Ref{UUID: book.Uuid, Name: book.Name}, Currency: book.Currency,
		From: dayString(p.From), To: dayString(p.To), GeneratedAt: time.Now().UTC(),
		Lines: make([]StaffCostLine, 0, len(rows)),
	}
	salary, advance, bonus, total := new(big.Rat), new(big.Rat), new(big.Rat), new(big.Rat)
	for _, r := range rows {
		salary.Add(salary, ratOf(r.Salary))
		advance.Add(advance, ratOf(r.Advance))
		bonus.Add(bonus, ratOf(r.Bonus))
		total.Add(total, ratOf(r.Total))
		var title *string
		if r.Title.Valid {
			v := r.Title.String
			title = &v
		}
		rep.Lines = append(rep.Lines, StaffCostLine{
			StaffUUID: r.Uuid, Name: r.Name, Title: title, Active: r.Active,
			Salary: money(ratOf(r.Salary)), Advance: money(ratOf(r.Advance)), Bonus: money(ratOf(r.Bonus)),
			Total: money(ratOf(r.Total)), PaymentCount: r.PaymentCount,
		})
	}
	rep.Totals = StaffCostTotals{Salary: money(salary), Advance: money(advance), Bonus: money(bonus), Total: money(total)}
	return rep, nil
}
