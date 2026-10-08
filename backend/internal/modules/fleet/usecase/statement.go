package usecase

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// maxStatementDays bounds one statement period.
const maxStatementDays = 366

// Statement line kinds.
const (
	LineServiceIncome = "service_income"
	LineCollection    = "collection"
	LineOther         = "other"
)

// StatementPeriod is the inclusive day range of a statement.
type StatementPeriod struct {
	From, To time.Time // dates (UTC midnight)
}

// ParsePeriod validates period_from / period_to (YYYY-MM-DD, both required,
// from <= to, at most 366 days).
func ParsePeriod(from, to string) (StatementPeriod, error) {
	var p StatementPeriod
	var err error
	if p.From, err = time.Parse(time.DateOnly, strings.TrimSpace(from)); err != nil {
		return p, &ValidationError{Field: "period_from", Message: "must be a date (YYYY-MM-DD)"}
	}
	if p.To, err = time.Parse(time.DateOnly, strings.TrimSpace(to)); err != nil {
		return p, &ValidationError{Field: "period_to", Message: "must be a date (YYYY-MM-DD)"}
	}
	if p.To.Before(p.From) {
		return p, &ValidationError{Field: "period_to", Message: "must not be before period_from"}
	}
	if p.To.Sub(p.From) > maxStatementDays*24*time.Hour {
		return p, &ValidationError{Field: "period_to", Message: "the period is at most 366 days"}
	}
	return p, nil
}

// StatementService is the service of a service income line.
type StatementService struct {
	UUID        uuid.UUID  `json:"uuid"`
	ServiceNo   string     `json:"service_no"`
	Plate       *string    `json:"plate"`
	VehicleUUID *uuid.UUID `json:"vehicle_uuid"`
	CompletedAt *time.Time `json:"completed_at"`
}

// StatementLine is one ledger row of the fleet cari.
type StatementLine struct {
	UUID        uuid.UUID         `json:"uuid"`
	Date        time.Time         `json:"date"`
	Kind        string            `json:"kind"`
	Direction   string            `json:"direction"`
	Category    string            `json:"category"`
	Debit       string            `json:"debit"`
	Credit      string            `json:"credit"`
	Balance     string            `json:"balance"`
	Description *string           `json:"description"`
	IsReversal  bool              `json:"is_reversal"`
	Service     *StatementService `json:"service"`
}

// PartyRef names the fleet or the dealer of a statement.
type PartyRef struct {
	UUID      uuid.UUID `json:"uuid"`
	Name      string    `json:"name"`
	LegalName string    `json:"legal_name,omitempty"`
	TaxNumber string    `json:"tax_number,omitempty"`
}

// StatementView is GET /v1/fleets/{uuid}/statement: the fleet cari in the
// caller's ledger over the period. Debit raises what the fleet owes
// (service income, charges), credit lowers it (collections).
type StatementView struct {
	Fleet              PartyRef        `json:"fleet"`
	Dealer             PartyRef        `json:"dealer"`
	CariUUID           *uuid.UUID      `json:"cari_uuid"`
	Currency           string          `json:"currency"`
	PeriodFrom         string          `json:"period_from"`
	PeriodTo           string          `json:"period_to"`
	OpeningBalance     string          `json:"opening_balance"`
	ServiceIncomeTotal string          `json:"service_income_total"`
	CollectionTotal    string          `json:"collection_total"`
	DebitTotal         string          `json:"debit_total"`
	CreditTotal        string          `json:"credit_total"`
	ClosingBalance     string          `json:"closing_balance"`
	ServiceCount       int             `json:"service_count"`
	Lines              []StatementLine `json:"lines"`
}

// Statement is GET /v1/fleets/{uuid}/statement: only the caller
// organization's own link (its own cari); ErrNoDealerLink without one.
func (s *Service) Statement(ctx context.Context, c Caller, fleetUUID uuid.UUID, p StatementPeriod) (StatementView, error) {
	q := db.New(s.conn)
	a, err := s.resolve(ctx, q, c, fleetUUID)
	if err != nil {
		return StatementView{}, err
	}
	if a.own == nil {
		return StatementView{}, ErrNoDealerLink
	}
	dealer, err := q.GetOrganizationByID(ctx, c.OrgID)
	if err != nil {
		return StatementView{}, fmt.Errorf("fleet: dealer: %w", err)
	}
	return buildStatement(ctx, q, dealer, a.fleet, *a.own, p)
}

// StatementForJob builds the statement of an export job: the job
// organization must still hold an active link to the fleet (the worker
// re-checks what the handler authorized).
func (s *Service) StatementForJob(ctx context.Context, jobOrgID int64, fleetUUID uuid.UUID, p StatementPeriod) (StatementView, error) {
	q := db.New(s.conn)
	dealer, err := q.GetOrganizationByID(ctx, jobOrgID)
	if err != nil {
		return StatementView{}, fmt.Errorf("fleet: job organization: %w", err)
	}
	f, err := q.GetFleetByUUID(ctx, fleetUUID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && f.Organization.BrandID != dealer.BrandID) {
		return StatementView{}, ErrNotFound
	}
	if err != nil {
		return StatementView{}, fmt.Errorf("fleet: get: %w", err)
	}
	link, err := q.GetOpenFleetDealerLink(ctx, db.GetOpenFleetDealerLinkParams{FleetOrgID: f.Organization.ID, DealerOrgID: dealer.ID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && link.Status != model.LinkActive) {
		return StatementView{}, ErrNoDealerLink
	}
	if err != nil {
		return StatementView{}, fmt.Errorf("fleet: link: %w", err)
	}
	return buildStatement(ctx, q, dealer, f, link, p)
}

func buildStatement(ctx context.Context, q *db.Queries, dealer db.Organization, f db.GetFleetByUUIDRow, link db.FleetDealerLink, p StatementPeriod) (StatementView, error) {
	out := StatementView{
		Fleet: PartyRef{
			UUID: f.Organization.Uuid, Name: f.Organization.Name,
			LegalName: f.FleetProfile.LegalName, TaxNumber: f.FleetProfile.TaxNumber,
		},
		Dealer:     PartyRef{UUID: dealer.Uuid, Name: dealer.Name},
		Currency:   dealer.Currency,
		PeriodFrom: p.From.Format(time.DateOnly), PeriodTo: p.To.Format(time.DateOnly),
		Lines: []StatementLine{},
	}
	loc, err := time.LoadLocation(dealer.Timezone)
	if err != nil {
		loc = time.UTC
	}
	start := time.Date(p.From.Year(), p.From.Month(), p.From.Day(), 0, 0, 0, 0, loc)
	end := time.Date(p.To.Year(), p.To.Month(), p.To.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)

	opening, income, collection, debit, credit := new(big.Rat), new(big.Rat), new(big.Rat), new(big.Rat), new(big.Rat)
	if link.CariAccountID.Valid {
		cariID := link.CariAccountID.Int64
		cari, err := q.GetCariAccount(ctx, db.GetCariAccountParams{ID: cariID, OrganizationID: dealer.ID})
		if err != nil {
			return StatementView{}, fmt.Errorf("fleet: cari: %w", err)
		}
		out.CariUUID, out.Currency = &cari.Uuid, cari.Currency
		ob, err := q.FleetCariBalanceBefore(ctx, db.FleetCariBalanceBeforeParams{
			CariID: cariID, OrganizationID: dealer.ID, Before: pgtype.Timestamptz{Time: start, Valid: true},
		})
		if err != nil {
			return StatementView{}, fmt.Errorf("fleet: opening balance: %w", err)
		}
		opening = rat(ob)
		rows, err := q.ListFleetStatementEntries(ctx, db.ListFleetStatementEntriesParams{
			CariID: cariID, OrganizationID: dealer.ID,
			PeriodFrom: pgtype.Timestamptz{Time: start, Valid: true}, PeriodTo: pgtype.Timestamptz{Time: end, Valid: true},
		})
		if err != nil {
			return StatementView{}, fmt.Errorf("fleet: statement: %w", err)
		}
		running := new(big.Rat).Set(opening)
		services := map[uuid.UUID]bool{}
		for _, r := range rows {
			amt := rat(r.Amount)
			l := StatementLine{
				UUID: r.Uuid, Date: r.CreatedAt.Time, Direction: r.Direction, Category: r.Category,
				Debit: "", Credit: "", Description: textPtr(r.Description), IsReversal: r.IsReversal, Kind: LineOther,
			}
			switch r.Direction {
			case "income", "charge", "payment":
				debit.Add(debit, amt)
				running.Add(running, amt)
				l.Debit = fmtRat(amt)
			default:
				credit.Add(credit, amt)
				running.Sub(running, amt)
				l.Credit = fmtRat(amt)
			}
			switch {
			case r.SourceType.String == model.SourceServiceIncome:
				l.Kind = LineServiceIncome
				income.Add(income, amt)
				if r.ServiceUuid.Valid {
					sv := &StatementService{
						UUID: r.ServiceUuid.Bytes, ServiceNo: r.ServiceNo.String, Plate: textPtr(r.Plate),
						CompletedAt: timePtr(r.ServiceCompletedAt),
					}
					if r.VehicleUuid.Valid {
						v := uuid.UUID(r.VehicleUuid.Bytes)
						sv.VehicleUUID = &v
					}
					l.Service = sv
					if !r.IsReversal {
						services[sv.UUID] = true
					}
				}
			case r.Direction == "collection":
				l.Kind = LineCollection
				collection.Add(collection, amt)
			}
			l.Balance = fmtRat(running)
			out.Lines = append(out.Lines, l)
		}
		out.ServiceCount = len(services)
	}
	closing := new(big.Rat).Add(opening, debit)
	closing.Sub(closing, credit)
	out.OpeningBalance, out.ServiceIncomeTotal, out.CollectionTotal = fmtRat(opening), fmtRat(income), fmtRat(collection)
	out.DebitTotal, out.CreditTotal, out.ClosingBalance = fmtRat(debit), fmtRat(credit), fmtRat(closing)
	return out, nil
}

func rat(n pgtype.Numeric) *big.Rat {
	r, ok := new(big.Rat).SetString(money(n))
	if !ok {
		return new(big.Rat)
	}
	return r
}

func fmtRat(r *big.Rat) string { return r.FloatString(2) }

// --- Export (I/O engine) -------------------------------------------------------

// ResourceStatement is the export resource of the fleet statement.
const ResourceStatement = "tenant.fleet.statement"

// Export query keys written by the handler after it authorized the request.
const (
	QueryFleetUUID  = "fleet_uuid"
	QueryPeriodFrom = "period_from"
	QueryPeriodTo   = "period_to"
)

// StatementAdapter exports the fleet statement (PDF / XLSX / CSV).
type StatementAdapter struct{ svc *Service }

// NewStatementAdapter creates the fleet statement export adapter.
func NewStatementAdapter(svc *Service) *StatementAdapter { return &StatementAdapter{svc: svc} }

var _ ioengine.ResourceAdapter = (*StatementAdapter)(nil)

// Resource implements ioengine.ResourceAdapter.
func (a *StatementAdapter) Resource() string { return ResourceStatement }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *StatementAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "date", LabelKey: "fleet.statement.date", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "kind", LabelKey: "fleet.statement.kind", Type: ioengine.ColumnTypeString, Weight: 0.9},
		{Key: "service_no", LabelKey: "fleet.statement.service_no", Type: ioengine.ColumnTypeString, Weight: 0.9},
		{Key: "plate", LabelKey: "fleet.statement.plate", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "debit", LabelKey: "fleet.statement.debit", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "credit", LabelKey: "fleet.statement.credit", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "balance", LabelKey: "fleet.statement.balance", Type: ioengine.ColumnTypeString, AlignRight: true},
	}
}

// Export implements ioengine.ResourceAdapter. Query: fleet_uuid,
// period_from, period_to; the job organization is the dealer.
func (a *StatementAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	jobOrg, err := strconv.ParseInt(q[ioengine.QueryOrganizationID], 10, 64)
	if err != nil || jobOrg <= 0 {
		return ioengine.Dataset{}, errors.New("fleet export: organization is required")
	}
	id, err := uuid.Parse(strings.TrimSpace(q[QueryFleetUUID]))
	if err != nil {
		return ioengine.Dataset{}, ErrNotFound
	}
	p, err := ParsePeriod(q[QueryPeriodFrom], q[QueryPeriodTo])
	if err != nil {
		return ioengine.Dataset{}, err
	}
	st, err := a.svc.StatementForJob(ctx, jobOrg, id, p)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	return StatementDataset(st, loc), nil
}

// StatementDataset maps a statement to an export dataset: the opening row,
// one row per ledger line and the totals.
func StatementDataset(st StatementView, loc i18n.Locale) ioengine.Dataset {
	t := func(key string) string { return i18n.Translate(loc, key) }
	rows := make([]map[string]any, 0, len(st.Lines)+1)
	rows = append(rows, map[string]any{
		"date": st.PeriodFrom, "kind": t("fleet.statement.opening_balance"), "service_no": "", "plate": "",
		"debit": "", "credit": "", "balance": st.OpeningBalance,
	})
	for _, l := range st.Lines {
		serviceNo, plate := "", ""
		if l.Service != nil {
			serviceNo = l.Service.ServiceNo
			if l.Service.Plate != nil {
				plate = *l.Service.Plate
			}
		}
		rows = append(rows, map[string]any{
			"date": l.Date.Format(time.DateOnly), "kind": t("fleet.statement.kind." + l.Kind),
			"service_no": serviceNo, "plate": plate, "debit": l.Debit, "credit": l.Credit, "balance": l.Balance,
		})
	}
	return ioengine.Dataset{
		Resource: ResourceStatement,
		Columns:  (&StatementAdapter{}).ExportColumns(),
		Rows:     rows,
		Info: []ioengine.InfoLine{
			{LabelKey: "fleet.statement.fleet", Value: st.Fleet.LegalName + " (" + st.Fleet.TaxNumber + ")"},
			{LabelKey: "fleet.statement.dealer", Value: st.Dealer.Name},
			{LabelKey: "fleet.statement.period", Value: st.PeriodFrom + " – " + st.PeriodTo},
			{LabelKey: "fleet.statement.currency", Value: st.Currency},
			{LabelKey: "fleet.statement.service_income_total", Value: st.ServiceIncomeTotal},
			{LabelKey: "fleet.statement.collection_total", Value: st.CollectionTotal},
		},
		Totals: map[string]any{
			"date": st.PeriodTo, "kind": t("fleet.statement.closing_balance"),
			"debit": st.DebitTotal, "credit": st.CreditTotal, "balance": st.ClosingBalance,
		},
	}
}

// ImportSchema implements ioengine.ResourceAdapter (export only).
func (a *StatementAdapter) ImportSchema() []ioengine.ImportField { return nil }

// ApplyRow implements ioengine.ResourceAdapter (export only).
func (a *StatementAdapter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "export only"}, nil
}

// RevertRow implements ioengine.ResourceAdapter (export only).
func (a *StatementAdapter) RevertRow(context.Context, string, string, map[string]any) error {
	return nil
}
