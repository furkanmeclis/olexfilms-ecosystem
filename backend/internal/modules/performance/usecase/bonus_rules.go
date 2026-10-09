package usecase

// TEC-497 (F5-05h): dealer bonus rules and the bonus payout day over HTTP
// (the "Prim kuralları" tab). The tables and queries come from TEC-490 /
// TEC-493; like the accruals they need a dealer with the performance and
// dealer_accounting modules on.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// DefaultBonusPayoutDay is the payout day of a dealer without bonus
// settings (bonusPayoutDate).
const DefaultBonusPayoutDay = 5

type BonusRuleView struct {
	UUID         uuid.UUID `json:"uuid"`
	Name         string    `json:"name"`
	Metric       string    `json:"metric"`
	ThresholdPct string    `json:"threshold_pct"`
	Kind         string    `json:"kind"`
	Amount       *string   `json:"amount"`
	Percent      *string   `json:"percent"`
	Currency     *string   `json:"currency"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type BonusRuleInput struct {
	Name         string  `json:"name"`
	Metric       string  `json:"metric"`
	ThresholdPct string  `json:"threshold_pct"`
	Kind         string  `json:"kind"`
	Amount       *string `json:"amount"`
	Percent      *string `json:"percent"`
	Currency     *string `json:"currency"`
	Active       bool    `json:"active"`
}

type BonusSettingsView struct {
	PayoutDay int16 `json:"payout_day"`
}

type BonusSettingsInput struct {
	PayoutDay int16 `json:"payout_day"`
}

func bonusRuleView(r db.BonusRule) BonusRuleView {
	return BonusRuleView{
		UUID: r.Uuid, Name: r.Name, Metric: r.Metric, ThresholdPct: numText(r.ThresholdPct), Kind: r.Kind,
		Amount: numPtr(r.Amount), Percent: numPtr(r.Percent), Currency: textPtr(r.Currency), Active: r.Active,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

// bonusRuleArgs validates a rule: threshold 0 < x <= 1000; fixed needs a
// positive amount and a currency, percent_of_revenue a percent 0 < x <= 100
// (chk_bonus_rules_kind).
func bonusRuleArgs(in BonusRuleInput) (db.UpdateBonusRuleParams, error) {
	out := db.UpdateBonusRuleParams{Name: strings.TrimSpace(in.Name), Metric: in.Metric, Kind: in.Kind, Active: in.Active}
	if out.Name == "" {
		return out, invalid("name", "is required")
	}
	if !contains(model.StaffMetrics, in.Metric) {
		return out, invalid("metric", "invalid")
	}
	thr, err := numeric(in.ThresholdPct)
	if err != nil || numFloat(thr) > 1000 {
		return out, invalid("threshold_pct", "must be a decimal between 0 and 1000")
	}
	out.ThresholdPct = thr
	switch in.Kind {
	case model.BonusFixed:
		if in.Amount == nil {
			return out, invalid("amount", "is required")
		}
		amount, err := numeric(*in.Amount)
		if err != nil {
			return out, invalid("amount", "must be a positive decimal")
		}
		cur := currencyArg(in.Currency)
		if cur.Valid && len(cur.String) != 3 {
			return out, invalid("currency", "must be an ISO 4217 code")
		}
		out.Amount, out.Currency = amount, cur
	case model.BonusPercentOfRevenue:
		if in.Percent == nil {
			return out, invalid("percent", "is required")
		}
		pct, err := numeric(*in.Percent)
		if err != nil || numFloat(pct) > 100 {
			return out, invalid("percent", "must be a decimal between 0 and 100")
		}
		out.Percent = pct
	default:
		return out, invalid("kind", "invalid")
	}
	return out, nil
}

// withRuleCurrency defaults a fixed rule without currency to the dealer
// currency.
func (s *Service) withRuleCurrency(ctx context.Context, c Caller, arg *db.UpdateBonusRuleParams) error {
	if arg.Kind != model.BonusFixed || arg.Currency.Valid {
		return nil
	}
	cur, err := s.orgCurrency(ctx, c)
	arg.Currency = cur
	return err
}

func (s *Service) orgCurrency(ctx context.Context, c Caller) (pgtype.Text, error) {
	org, err := s.q.GetOrganizationByID(ctx, c.Org.InternalID)
	if err != nil {
		return pgtype.Text{}, err
	}
	return pgtype.Text{String: org.Currency, Valid: org.Currency != ""}, nil
}

func (s *Service) bonusDealer(ctx context.Context, c Caller) error {
	if c.Org.OrgType != "dealer" {
		return ErrForbidden
	}
	return s.ensureBonusFeatures(ctx, c.Org.InternalID, true)
}

func (s *Service) ListBonusRules(ctx context.Context, c Caller, active *bool) ([]BonusRuleView, error) {
	if err := s.bonusDealer(ctx, c); err != nil {
		return nil, err
	}
	rows, err := s.q.ListBonusRules(ctx, db.ListBonusRulesParams{OrganizationID: c.Org.InternalID, Active: boolArg(active)})
	if err != nil {
		return nil, err
	}
	out := make([]BonusRuleView, 0, len(rows))
	for _, r := range rows {
		out = append(out, bonusRuleView(r))
	}
	return out, nil
}

func (s *Service) CreateBonusRule(ctx context.Context, c Caller, in BonusRuleInput) (BonusRuleView, error) {
	if err := s.bonusDealer(ctx, c); err != nil {
		return BonusRuleView{}, err
	}
	arg, err := bonusRuleArgs(in)
	if err != nil {
		return BonusRuleView{}, err
	}
	if err := s.withRuleCurrency(ctx, c, &arg); err != nil {
		return BonusRuleView{}, err
	}
	row, err := s.q.CreateBonusRule(ctx, db.CreateBonusRuleParams{
		OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, Name: arg.Name, Metric: arg.Metric,
		ThresholdPct: arg.ThresholdPct, Kind: arg.Kind, Amount: arg.Amount, Percent: arg.Percent,
		Currency: arg.Currency, Active: arg.Active, CreatedByUserID: userArg(c),
	})
	if err != nil {
		return BonusRuleView{}, mapPG(err)
	}
	return bonusRuleView(row), nil
}

func (s *Service) bonusRule(ctx context.Context, c Caller, id uuid.UUID) (db.BonusRule, error) {
	if err := s.bonusDealer(ctx, c); err != nil {
		return db.BonusRule{}, err
	}
	cur, err := s.q.GetBonusRule(ctx, db.GetBonusRuleParams{Uuid: id, OrganizationID: c.Org.InternalID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.BonusRule{}, ErrNotFound
	}
	return cur, err
}

func (s *Service) UpdateBonusRule(ctx context.Context, c Caller, id uuid.UUID, in BonusRuleInput) (BonusRuleView, error) {
	cur, err := s.bonusRule(ctx, c, id)
	if err != nil {
		return BonusRuleView{}, err
	}
	arg, err := bonusRuleArgs(in)
	if err != nil {
		return BonusRuleView{}, err
	}
	if err := s.withRuleCurrency(ctx, c, &arg); err != nil {
		return BonusRuleView{}, err
	}
	arg.ID, arg.OrganizationID = cur.ID, c.Org.InternalID
	row, err := s.q.UpdateBonusRule(ctx, arg)
	if err != nil {
		return BonusRuleView{}, mapPG(err)
	}
	return bonusRuleView(row), nil
}

// DeleteBonusRule deletes a rule without accruals; a rule with accruals is
// kept for their history and deactivated instead.
func (s *Service) DeleteBonusRule(ctx context.Context, c Caller, id uuid.UUID) error {
	cur, err := s.bonusRule(ctx, c, id)
	if err != nil {
		return err
	}
	n, err := s.q.DeleteBonusRule(ctx, db.DeleteBonusRuleParams{ID: cur.ID, OrganizationID: c.Org.InternalID})
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err = s.q.UpdateBonusRule(ctx, db.UpdateBonusRuleParams{
		ID: cur.ID, OrganizationID: c.Org.InternalID, Name: cur.Name, Metric: cur.Metric, ThresholdPct: cur.ThresholdPct,
		Kind: cur.Kind, Amount: cur.Amount, Percent: cur.Percent, Currency: cur.Currency, Active: false,
	})
	return err
}

func (s *Service) GetBonusSettings(ctx context.Context, c Caller) (BonusSettingsView, error) {
	if err := s.bonusDealer(ctx, c); err != nil {
		return BonusSettingsView{}, err
	}
	row, err := s.q.GetBonusSettings(ctx, c.Org.InternalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return BonusSettingsView{PayoutDay: DefaultBonusPayoutDay}, nil
	}
	if err != nil {
		return BonusSettingsView{}, err
	}
	return BonusSettingsView{PayoutDay: row.PayoutDay}, nil
}

// UpdateBonusSettings sets the payout day (1-28, chk_bonus_settings_payout_day)
// approved bonuses are paid on in the month after their period (S20).
func (s *Service) UpdateBonusSettings(ctx context.Context, c Caller, in BonusSettingsInput) (BonusSettingsView, error) {
	if err := s.bonusDealer(ctx, c); err != nil {
		return BonusSettingsView{}, err
	}
	if in.PayoutDay < 1 || in.PayoutDay > 28 {
		return BonusSettingsView{}, invalid("payout_day", "must be between 1 and 28")
	}
	row, err := s.q.UpsertBonusSettings(ctx, db.UpsertBonusSettingsParams{OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, PayoutDay: in.PayoutDay})
	if err != nil {
		return BonusSettingsView{}, mapPG(err)
	}
	return BonusSettingsView{PayoutDay: row.PayoutDay}, nil
}
