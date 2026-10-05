package usecase

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	AccountingSourceServiceIncome = "service_income"

	PaymentCash = "cash"
	PaymentCard = "card"
	PaymentCari = "cari"

	warrantyClaimIncomeWarning = "warranty_claim_service"
)

// IncomeInput is POST /v1/services/{uuid}/income.
type IncomeInput struct {
	Amount        string
	PaymentMethod string
	AccountUUID   *uuid.UUID
	Description   string
}

// IncomeResult is returned when service income is posted or reversed.
type IncomeResult struct {
	Service        ServiceView `json:"service"`
	EntryUUID      *uuid.UUID  `json:"entry_uuid,omitempty"`
	ReversalUUIDs  []uuid.UUID `json:"reversal_uuids,omitempty"`
	Warning        *string     `json:"warning,omitempty"`
	WarningMessage *string     `json:"warning_message,omitempty"`
}

// ProfitView is GET /v1/services/{uuid}/profit and the optional service-list
// profit column. Cost fields are nil without pricing.purchase.read.
type ProfitView struct {
	Revenue     *string `json:"revenue"`
	Cost        *string `json:"cost"`
	GrossProfit *string `json:"gross_profit"`
	MarginPct   *string `json:"margin_pct"`
}

// RecordIncome posts one service-income accounting row and stores its link on
// the service. A second open income is a conflict; reversals are explicit.
func (s *Service) RecordIncome(ctx context.Context, c Caller, id uuid.UUID, in IncomeInput) (IncomeResult, error) {
	if s.income == nil {
		return IncomeResult{}, errors.New("services: income accounting is not wired")
	}
	amount, err := normalizeAmount(in.Amount)
	if err != nil {
		return IncomeResult{}, err
	}
	method := strings.TrimSpace(in.PaymentMethod)
	switch method {
	case PaymentCash, PaymentCard, PaymentCari:
	default:
		return IncomeResult{}, invalid("payment_method", "must be cash, card or cari")
	}
	desc := strings.TrimSpace(in.Description)
	if len([]rune(desc)) > 1000 {
		return IncomeResult{}, invalid("description", "must be at most 1000 characters")
	}
	if method != PaymentCari && in.AccountUUID == nil {
		return IncomeResult{}, invalid("account_uuid", "is required for cash and card payments")
	}
	if method == PaymentCari && in.AccountUUID != nil {
		return IncomeResult{}, invalid("account_uuid", "is not used for cari payments")
	}

	var svc db.Service
	var entry db.FinanceEntry
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		locked, err := s.lockVisible(ctx, q, c, id)
		if err != nil {
			return err
		}
		if !c.allows(rbac.PermAccountingWrite, locked) {
			return ErrForbidden
		}
		if locked.Status != StatusCompleted {
			return ErrNotEditable
		}
		if locked.IncomeEntryID.Valid {
			return ErrIncomeAlreadyRecorded
		}
		org, err := q.GetOrganizationByID(ctx, locked.OrganizationID)
		if err != nil {
			return fmt.Errorf("services: income organization: %w", err)
		}
		e := posting.Entry{
			OrganizationID: locked.OrganizationID,
			Source:         posting.Source{Type: AccountingSourceServiceIncome, UUID: locked.Uuid},
			Category:       accounting.CategoryServiceIncome,
			Amount:         amount,
			Currency:       org.Currency,
			Description:    desc,
			ActorUserID:    actorPtr(c),
		}
		switch method {
		case PaymentCash, PaymentCard:
			account, err := q.GetFinanceAccountByUUID(ctx, *in.AccountUUID)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && account.OrganizationID != locked.OrganizationID) {
				return ErrForbidden
			}
			if err != nil {
				return fmt.Errorf("services: income account: %w", err)
			}
			if !account.Active {
				return invalid("account_uuid", "the account is inactive")
			}
			if method == PaymentCash && account.Type != "cash" {
				return invalid("account_uuid", "cash payments need a cash account")
			}
			if method == PaymentCard && account.Type != "bank" {
				return invalid("account_uuid", "card payments need a bank account")
			}
			e.AccountID = account.ID
		case PaymentCari:
			cari, err := q.CreateCariForUserIfMissing(ctx, db.CreateCariForUserIfMissingParams{
				OrganizationID: locked.OrganizationID,
				BrandID:        locked.BrandID, CounterpartyUserID: locked.CustomerUserID, Currency: org.Currency,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				cari, err = q.GetCariAccountByCounterpartyUser(ctx, db.GetCariAccountByCounterpartyUserParams{
					OrganizationID: locked.OrganizationID, CounterpartyUserID: locked.CustomerUserID,
				})
			}
			if err != nil {
				return fmt.Errorf("services: income customer cari: %w", err)
			}
			if !cari.Active {
				return invalid("payment_method", "the customer cari is inactive")
			}
			e.CariID = cari.ID
		}
		res, err := s.income.PostIncome(ctx, tx, e)
		if err != nil {
			return fmt.Errorf("services: post income: %w", err)
		}
		entry = res.Entry
		num := pgtype.Numeric{}
		if err := num.Scan(amount); err != nil {
			return fmt.Errorf("services: income amount: %w", err)
		}
		svc, err = q.SetServiceIncome(ctx, db.SetServiceIncomeParams{
			ID: locked.ID, BrandID: locked.BrandID,
			IncomeEntryID: pgtype.Int8{Int64: entry.ID, Valid: true}, IncomeAmount: num,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrIncomeAlreadyRecorded
		}
		if err != nil {
			return fmt.Errorf("services: set income: %w", err)
		}
		return nil
	})
	if err != nil {
		return IncomeResult{}, err
	}
	v, err := s.view(ctx, s.q, c, svc)
	if err != nil {
		return IncomeResult{}, err
	}
	return IncomeResult{Service: v, EntryUUID: &entry.Uuid, Warning: incomeWarning(svc), WarningMessage: incomeWarningMessage(svc)}, nil
}

// DeleteIncome reverses service income and clears the service link.
func (s *Service) DeleteIncome(ctx context.Context, c Caller, id uuid.UUID, reason string) (IncomeResult, error) {
	if s.income == nil {
		return IncomeResult{}, errors.New("services: income accounting is not wired")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "service income reversed"
	}
	if len([]rune(reason)) > 1000 {
		return IncomeResult{}, invalid("reason", "must be at most 1000 characters")
	}
	var svc db.Service
	var reversals []uuid.UUID
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		locked, err := s.lockVisible(ctx, q, c, id)
		if err != nil {
			return err
		}
		if !c.allows(rbac.PermAccountingWrite, locked) {
			return ErrForbidden
		}
		if !locked.IncomeEntryID.Valid {
			return ErrIncomeNotRecorded
		}
		vr, err := s.income.VoidBySourceTx(ctx, tx, posting.Source{Type: AccountingSourceServiceIncome, UUID: locked.Uuid}, reason, actorPtr(c))
		if err != nil {
			return fmt.Errorf("services: reverse income: %w", err)
		}
		for _, r := range vr.Reversals {
			reversals = append(reversals, r.Uuid)
		}
		svc, err = q.ClearServiceIncome(ctx, db.ClearServiceIncomeParams{ID: locked.ID, BrandID: locked.BrandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrIncomeNotRecorded
		}
		if err != nil {
			return fmt.Errorf("services: clear income: %w", err)
		}
		return nil
	})
	if err != nil {
		return IncomeResult{}, err
	}
	v, err := s.view(ctx, s.q, c, svc)
	if err != nil {
		return IncomeResult{}, err
	}
	return IncomeResult{Service: v, ReversalUUIDs: reversals}, nil
}

// Profit returns revenue minus consumed purchase cost. Cost is hidden without
// pricing.purchase.read.
func (s *Service) Profit(ctx context.Context, c Caller, id uuid.UUID) (ProfitView, error) {
	svc, err := s.getVisible(ctx, c, id)
	if err != nil {
		return ProfitView{}, err
	}
	return s.profit(ctx, s.q, c, svc)
}

func (s *Service) profit(ctx context.Context, q *db.Queries, c Caller, svc db.Service) (ProfitView, error) {
	if !canReadIncome(c, svc) {
		return ProfitView{}, nil
	}
	out := ProfitView{Revenue: numericTextPtr(svc.IncomeAmount)}
	if !canReadPurchase(c, svc) {
		return out, nil
	}
	cost, err := q.GetServiceConsumedPurchaseCost(ctx, db.GetServiceConsumedPurchaseCostParams{
		ServiceID: svc.ID, BrandID: svc.BrandID,
	})
	if err != nil {
		return ProfitView{}, fmt.Errorf("services: profit cost: %w", err)
	}
	costText := posting.FormatNumeric(cost)
	out.Cost = &costText
	revenue := numericRat(svc.IncomeAmount)
	if revenue == nil {
		zero := "0.00"
		out.GrossProfit = &zero
		return out, nil
	}
	costRat := numericRat(cost)
	gross := revenue
	if costRat != nil {
		gross = new(big.Rat).Set(revenue)
		gross.Sub(gross, costRat)
	}
	grossText := gross.FloatString(2)
	out.GrossProfit = &grossText
	if revenue.Sign() > 0 {
		margin := new(big.Rat).Set(gross)
		margin.Mul(margin, big.NewRat(100, 1))
		margin.Quo(margin, revenue)
		marginText := margin.FloatString(2)
		out.MarginPct = &marginText
	}
	return out, nil
}

func canReadIncome(c Caller, svc db.Service) bool {
	return c.allows(rbac.PermAccountingRead, svc) || c.allows(rbac.PermAccountingWrite, svc)
}

func canReadPurchase(c Caller, svc db.Service) bool {
	return c.allows(rbac.PermPricingPurchaseRead, svc)
}

func incomeWarning(svc db.Service) *string {
	if !svc.WarrantyClaimID.Valid {
		return nil
	}
	w := warrantyClaimIncomeWarning
	return &w
}

func incomeWarningMessage(svc db.Service) *string {
	if !svc.WarrantyClaimID.Valid {
		return nil
	}
	msg := "This service is linked to a warranty claim; income was recorded anyway."
	return &msg
}

func normalizeAmount(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", invalid("amount", "is required")
	}
	var n pgtype.Numeric
	if err := n.Scan(raw); err != nil {
		return "", invalid("amount", "must be a decimal amount")
	}
	r := numericRat(n)
	if r == nil || r.Sign() <= 0 {
		return "", invalid("amount", "must be greater than zero")
	}
	return r.FloatString(2), nil
}
