package tools

import (
	"context"
	"encoding/json"
	"math/big"
	"sort"
	"strings"
	"time"

	accountinguc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

// AccountingReader is the accounting use case surface the tools use.
type AccountingReader interface {
	GetBalanceReport(ctx context.Context, c accountinguc.Caller, orgUUID *uuid.UUID, asOf *time.Time) (accountinguc.BalanceReport, error)
}

// BalanceSummary: cari bakiye özeti (accounting.read).
type BalanceSummary struct {
	accounting AccountingReader
	tree       scopefilter.TreeReader
}

// Spec implements Tool.
func (BalanceSummary) Spec() Spec {
	return Spec{
		Name: "balance_summary",
		Description: "Current account (cari) balance summary of your organization's books: totals receivable / " +
			"payable, cash and bank balances and the largest counterparty balances. A positive cari balance is a " +
			"receivable (they owe you), a negative one a payable. Filter counterparties by name.",
		InputSchema: object(map[string]any{
			"counterparty": str("Optional counterparty name filter.", 100),
			"organization": str("Optional organization uuid within your access whose books to read; default: your own.", 36),
			"limit":        limitProp(),
		}),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModuleAccounting,
		Permissions: []string{rbac.PermAccountingRead},
	}
}

type cariRow struct {
	Counterparty string `json:"counterparty"`
	Type         string `json:"type"`
	Currency     string `json:"currency"`
	Balance      string `json:"balance"`
}

type accountRow struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Currency string `json:"currency"`
	Balance  string `json:"balance"`
}

func absRat(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	if !ok {
		return new(big.Rat)
	}
	return r.Abs(r)
}

// Run implements Tool.
func (t BalanceSummary) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Counterparty string `json:"counterparty"`
		Organization string `json:"organization"`
		Limit        int    `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	errs := errCases{tool: t.Spec().Name, what: "organization books",
		forbidden: []error{accountinguc.ErrForbidden}, notFound: []error{accountinguc.ErrBookNotFound},
		invalid: asError[*accountinguc.ValidationError]}
	p := env.Principal
	var org *uuid.UUID
	if in.Organization != "" {
		id, ok := parseID(in.Organization)
		if !ok {
			return invalidArg("organization must be a uuid"), nil
		}
		org = &id
	}
	f, err := resolveScope(ctx, t.tree, p, rbac.PermAccountingRead)
	if err != nil {
		return errs.result(err)
	}
	rep, err := t.accounting.GetBalanceReport(ctx, accountinguc.Caller{UserID: p.Auth.UserInternal, Org: *p.Org, Filter: f}, org, nil)
	if err != nil {
		return errs.result(err)
	}
	q := strings.ToLower(strings.TrimSpace(in.Counterparty))
	cari := make([]accountinguc.CariBalance, 0, len(rep.Cari))
	for _, c := range rep.Cari {
		if q != "" && !strings.Contains(strings.ToLower(c.Counterparty.Name), q) {
			continue
		}
		cari = append(cari, c)
	}
	sort.SliceStable(cari, func(i, j int) bool { return absRat(cari[i].Balance).Cmp(absRat(cari[j].Balance)) > 0 })
	total := int64(len(cari))
	if n := int(limitArg(in.Limit)); len(cari) > n {
		cari = cari[:n]
	}
	rows := make([]cariRow, 0, len(cari))
	for _, c := range cari {
		rows = append(rows, cariRow{Counterparty: dataText(c.Counterparty.Name, maxNameChars), Type: c.Counterparty.Type,
			Currency: c.Currency, Balance: c.Balance})
	}
	accounts := make([]accountRow, 0, len(rep.Accounts))
	for _, a := range rep.Accounts {
		if a.Active {
			accounts = append(accounts, accountRow{Name: dataText(a.Name, maxNameChars), Type: a.Type, Currency: a.Currency, Balance: a.Balance})
		}
	}
	type out struct {
		Organization string                     `json:"organization"`
		Currency     string                     `json:"currency"`
		Totals       accountinguc.BalanceTotals `json:"totals"`
		Accounts     []accountRow               `json:"cash_and_bank_accounts"`
		Cari         List[cariRow]              `json:"cari_balances"`
	}
	return JSONResult(out{
		Organization: dataText(rep.Organization.Name, maxNameChars), Currency: rep.Currency, Totals: rep.Totals,
		Accounts: accounts, Cari: NewList(rows, total),
	})
}
