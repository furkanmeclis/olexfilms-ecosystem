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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	StaffPaymentSalary  = "salary"
	StaffPaymentAdvance = "advance"
	StaffPaymentBonus   = "bonus"

	SourceStaffPayment = "staff_payment"
)

var periodRe = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

type StaffProfile struct {
	UUID          uuid.UUID  `json:"uuid"`
	UserUUID      *uuid.UUID `json:"user_uuid"`
	Name          string     `json:"name"`
	Title         *string    `json:"title"`
	HiredOn       *string    `json:"hired_on"`
	MonthlySalary *string    `json:"monthly_salary"`
	Currency      string     `json:"currency"`
	Active        bool       `json:"active"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type StaffPayment struct {
	UUID             uuid.UUID `json:"uuid"`
	StaffUUID        uuid.UUID `json:"staff_uuid"`
	Type             string    `json:"type"`
	Period           string    `json:"period"`
	Amount           string    `json:"amount"`
	Currency         string    `json:"currency"`
	PaidOn           string    `json:"paid_on"`
	Description      *string   `json:"description"`
	TargetNote       *string   `json:"target_note"`
	FinanceEntryUUID uuid.UUID `json:"finance_entry_uuid"`
	PeriodAdvances   string    `json:"period_advances"`
	CreatedAt        time.Time `json:"created_at"`
}

type StaffProfileFilter struct {
	Active        *bool
	Limit, Offset int32
}

type CreateStaffProfileInput struct {
	UserUUID      *uuid.UUID
	Name          string
	Title         *string
	HiredOn       *time.Time
	MonthlySalary *string
	Active        bool
}

type UpdateStaffProfileInput struct {
	UserUUID      *uuid.UUID
	ClearUser     bool
	Name          *string
	Title         *string
	ClearTitle    bool
	HiredOn       *time.Time
	ClearHiredOn  bool
	MonthlySalary *string
	ClearSalary   bool
	Active        *bool
}

type StaffPaymentInput struct {
	Type        string
	Period      string
	Amount      *string
	AccountUUID *uuid.UUID
	Description *string
	TargetNote  *string
	PaidOn      *time.Time
}

type PayrollResult struct {
	Period  string         `json:"period"`
	Created int            `json:"created"`
	Skipped int            `json:"skipped"`
	Items   []StaffPayment `json:"items"`
}

func staffProfileOf(r db.StaffProfile, userUUID *uuid.UUID) StaffProfile {
	out := StaffProfile{
		UUID:      r.Uuid,
		UserUUID:  userUUID,
		Name:      r.Name,
		Currency:  r.Currency,
		Active:    r.Active,
		CreatedAt: r.CreatedAt.Time,
		UpdatedAt: r.UpdatedAt.Time,
	}
	if r.Title.Valid {
		v := r.Title.String
		out.Title = &v
	}
	if r.HiredOn.Valid {
		v := r.HiredOn.Time.Format(time.DateOnly)
		out.HiredOn = &v
	}
	if r.MonthlySalary.Valid {
		v := posting.FormatNumeric(r.MonthlySalary)
		out.MonthlySalary = &v
	}
	return out
}

func staffPaymentOf(r db.StaffPayment, staff db.StaffProfile, entry db.FinanceEntry, advances string) StaffPayment {
	out := StaffPayment{
		UUID:             r.Uuid,
		StaffUUID:        staff.Uuid,
		Type:             r.Type,
		Period:           r.Period,
		Amount:           posting.FormatNumeric(r.Amount),
		Currency:         r.Currency,
		PaidOn:           r.PaidOn.Time.Format(time.DateOnly),
		FinanceEntryUUID: entry.Uuid,
		PeriodAdvances:   advances,
		CreatedAt:        r.CreatedAt.Time,
	}
	if r.Description.Valid {
		desc, target := splitPaymentDescription(r.Description.String)
		if desc != "" {
			out.Description = &desc
		}
		if target != "" {
			out.TargetNote = &target
		}
	}
	return out
}

func (s *Service) ListStaffProfiles(ctx context.Context, c Caller, f StaffProfileFilter) ([]StaffProfile, int64, error) {
	book, err := s.writeBook(ctx, c)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.q.ListStaffProfiles(ctx, db.ListStaffProfilesParams{
		OrganizationID: book.ID,
		Active:         boolArg(f.Active),
		PageLimit:      f.Limit,
		PageOffset:     f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("accounting: list staff profiles: %w", err)
	}
	total, err := s.q.CountStaffProfiles(ctx, db.CountStaffProfilesParams{
		OrganizationID: book.ID,
		Active:         boolArg(f.Active),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("accounting: count staff profiles: %w", err)
	}
	out := make([]StaffProfile, 0, len(rows))
	for _, r := range rows {
		userUUID, err := s.staffUserUUID(ctx, r)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, staffProfileOf(r, userUUID))
	}
	return out, total, nil
}

func (s *Service) CreateStaffProfile(ctx context.Context, c Caller, in CreateStaffProfileInput) (StaffProfile, error) {
	book, err := s.writeBook(ctx, c)
	if err != nil {
		return StaffProfile{}, err
	}
	name, err := staffName(in.Name)
	if err != nil {
		return StaffProfile{}, err
	}
	salary, err := optionalMoney("monthly_salary", in.MonthlySalary)
	if err != nil {
		return StaffProfile{}, err
	}
	userID, err := s.staffUserID(ctx, book, in.UserUUID)
	if err != nil {
		return StaffProfile{}, err
	}
	row, err := s.q.CreateStaffProfile(ctx, db.CreateStaffProfileParams{
		OrganizationID: book.ID,
		BrandID:        book.BrandID,
		UserID:         userID,
		Name:           name,
		Title:          textArg(trimmedPtr(in.Title)),
		HiredOn:        dateArg(in.HiredOn),
		MonthlySalary:  salary,
		Currency:       book.Currency,
		Active:         in.Active,
	})
	if err != nil {
		return StaffProfile{}, fmt.Errorf("accounting: create staff profile: %w", err)
	}
	userUUID, err := s.staffUserUUID(ctx, row)
	if err != nil {
		return StaffProfile{}, err
	}
	return staffProfileOf(row, userUUID), nil
}

func (s *Service) UpdateStaffProfile(ctx context.Context, c Caller, id uuid.UUID, in UpdateStaffProfileInput) (StaffProfile, error) {
	book, err := s.writeBook(ctx, c)
	if err != nil {
		return StaffProfile{}, err
	}
	cur, err := s.staffRow(ctx, book.ID, id)
	if err != nil {
		return StaffProfile{}, err
	}
	arg := db.UpdateStaffProfileParams{
		ID:             cur.ID,
		OrganizationID: book.ID,
		UserID:         cur.UserID,
		Name:           cur.Name,
		Title:          cur.Title,
		HiredOn:        cur.HiredOn,
		MonthlySalary:  cur.MonthlySalary,
		Currency:       book.Currency,
		Active:         cur.Active,
	}
	if in.ClearUser {
		arg.UserID = pgtype.Int8{}
	} else if in.UserUUID != nil {
		arg.UserID, err = s.staffUserID(ctx, book, in.UserUUID)
		if err != nil {
			return StaffProfile{}, err
		}
	}
	if in.Name != nil {
		arg.Name, err = staffName(*in.Name)
		if err != nil {
			return StaffProfile{}, err
		}
	}
	switch {
	case in.ClearTitle:
		arg.Title = pgtype.Text{}
	case in.Title != nil:
		arg.Title = textArg(trimmedPtr(in.Title))
	}
	switch {
	case in.ClearHiredOn:
		arg.HiredOn = pgtype.Date{}
	case in.HiredOn != nil:
		arg.HiredOn = dateArg(in.HiredOn)
	}
	switch {
	case in.ClearSalary:
		arg.MonthlySalary = pgtype.Numeric{}
	case in.MonthlySalary != nil:
		arg.MonthlySalary, err = optionalMoney("monthly_salary", in.MonthlySalary)
		if err != nil {
			return StaffProfile{}, err
		}
	}
	if in.Active != nil {
		arg.Active = *in.Active
	}
	row, err := s.q.UpdateStaffProfile(ctx, arg)
	if err != nil {
		return StaffProfile{}, fmt.Errorf("accounting: update staff profile: %w", err)
	}
	userUUID, err := s.staffUserUUID(ctx, row)
	if err != nil {
		return StaffProfile{}, err
	}
	return staffProfileOf(row, userUUID), nil
}

func (s *Service) CreateStaffPayment(ctx context.Context, c Caller, staffUUID uuid.UUID, in StaffPaymentInput) (StaffPayment, error) {
	book, err := s.writeBook(ctx, c)
	if err != nil {
		return StaffPayment{}, err
	}
	staff, err := s.staffRow(ctx, book.ID, staffUUID)
	if err != nil {
		return StaffPayment{}, err
	}
	return s.createStaffPayment(ctx, c, book, staff, in)
}

func (s *Service) RunPayroll(ctx context.Context, c Caller, period string) (PayrollResult, error) {
	book, err := s.writeBook(ctx, c)
	if err != nil {
		return PayrollResult{}, err
	}
	period, err = validPeriod(period)
	if err != nil {
		return PayrollResult{}, err
	}
	rows, err := s.q.ListStaffProfiles(ctx, db.ListStaffProfilesParams{
		OrganizationID: book.ID,
		Active:         pgtype.Bool{Bool: true, Valid: true},
		PageLimit:      10000,
	})
	if err != nil {
		return PayrollResult{}, fmt.Errorf("accounting: payroll staff: %w", err)
	}
	out := PayrollResult{Period: period, Items: []StaffPayment{}}
	for _, staff := range rows {
		if !staff.MonthlySalary.Valid || moneyText(staff.MonthlySalary) == "0.00" {
			out.Skipped++
			continue
		}
		amount := moneyText(staff.MonthlySalary)
		p, err := s.createStaffPayment(ctx, c, book, staff, StaffPaymentInput{
			Type:   StaffPaymentSalary,
			Period: period,
			Amount: &amount,
		})
		switch {
		case err == nil:
			out.Created++
			out.Items = append(out.Items, p)
		case errors.Is(err, ErrStaffSalaryExists):
			out.Skipped++
		default:
			return PayrollResult{}, err
		}
	}
	return out, nil
}

func (s *Service) createStaffPayment(
	ctx context.Context,
	c Caller,
	book db.Organization,
	staff db.StaffProfile,
	in StaffPaymentInput,
) (StaffPayment, error) {
	paymentType, category, err := staffPaymentType(in.Type)
	if err != nil {
		return StaffPayment{}, err
	}
	period, err := validPeriod(in.Period)
	if err != nil {
		return StaffPayment{}, err
	}
	amount := ""
	if in.Amount != nil {
		amount = strings.TrimSpace(*in.Amount)
	}
	if paymentType == StaffPaymentSalary && amount == "" {
		if !staff.MonthlySalary.Valid || moneyText(staff.MonthlySalary) == "0.00" {
			return StaffPayment{}, invalid("amount", "salary amount is required when staff has no monthly salary")
		}
		amount = moneyText(staff.MonthlySalary)
	}
	if paymentType != StaffPaymentSalary && amount == "" {
		return StaffPayment{}, invalid("amount", "is required")
	}
	amountNum, err := requiredMoney("amount", amount)
	if err != nil {
		return StaffPayment{}, err
	}
	desc, err := staffPaymentDescription(in.Description, in.TargetNote)
	if err != nil {
		return StaffPayment{}, err
	}
	var accountID int64
	if in.AccountUUID != nil {
		a, err := s.q.GetFinanceAccountByUUID(ctx, *in.AccountUUID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && a.OrganizationID != book.ID) {
			return StaffPayment{}, ErrAccountNotFound
		}
		if err != nil {
			return StaffPayment{}, fmt.Errorf("accounting: account: %w", err)
		}
		if !a.Active {
			return StaffPayment{}, invalid("account_uuid", "the account is inactive")
		}
		accountID = a.ID
	}
	paidOn := time.Now().UTC()
	if in.PaidOn != nil {
		paidOn = *in.PaidOn
	}

	var payment db.StaffPayment
	var entry db.FinanceEntry
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		qtx := s.q.WithTx(tx)
		payment, err = qtx.CreateStaffPayment(ctx, db.CreateStaffPaymentParams{
			OrganizationID:  book.ID,
			BrandID:         book.BrandID,
			StaffID:         staff.ID,
			Type:            paymentType,
			Period:          period,
			Amount:          amountNum,
			Currency:        book.Currency,
			PaidOn:          pgtype.Date{Time: paidOn, Valid: true},
			Description:     textArg(&desc),
			CreatedByUserID: int8PtrArg(c.actor()),
		})
		if err != nil {
			return staffPaymentCreateErr(err)
		}
		res, err := s.poster.PostExpense(ctx, tx, posting.Entry{
			OrganizationID: book.ID,
			Source:         posting.Source{Type: SourceStaffPayment, UUID: payment.Uuid},
			Category:       category,
			Amount:         posting.FormatNumeric(payment.Amount),
			Currency:       payment.Currency,
			AccountID:      accountID,
			Description:    desc,
			ActorUserID:    c.actor(),
		})
		if err != nil {
			return postingErr(err)
		}
		entry = res.Entry
		payment, err = qtx.SetStaffPaymentFinanceEntry(ctx, db.SetStaffPaymentFinanceEntryParams{
			FinanceEntryID: pgtype.Int8{Int64: entry.ID, Valid: true},
			ID:             payment.ID,
			OrganizationID: book.ID,
		})
		if err != nil {
			return fmt.Errorf("accounting: link staff payment: %w", err)
		}
		return nil
	})
	if err != nil {
		return StaffPayment{}, err
	}
	advances, err := s.periodAdvanceTotal(ctx, book.ID, staff.ID, period)
	if err != nil {
		return StaffPayment{}, err
	}
	return staffPaymentOf(payment, staff, entry, advances), nil
}

func (s *Service) staffRow(ctx context.Context, orgID int64, id uuid.UUID) (db.StaffProfile, error) {
	row, err := s.q.GetStaffProfileByUUID(ctx, db.GetStaffProfileByUUIDParams{Uuid: id, OrganizationID: orgID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.StaffProfile{}, ErrStaffNotFound
	}
	if err != nil {
		return db.StaffProfile{}, fmt.Errorf("accounting: staff profile: %w", err)
	}
	return row, nil
}

func (s *Service) staffUserID(ctx context.Context, book db.Organization, userUUID *uuid.UUID) (pgtype.Int8, error) {
	if userUUID == nil {
		return pgtype.Int8{}, nil
	}
	u, err := s.q.GetUserByUUID(ctx, *userUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.Int8{}, invalid("user_uuid", "user not found")
	}
	if err != nil {
		return pgtype.Int8{}, fmt.Errorf("accounting: staff user: %w", err)
	}
	if _, err := s.q.GetOrganizationMember(ctx, db.GetOrganizationMemberParams{
		OrganizationID: book.ID,
		UserID:         u.ID,
	}); errors.Is(err, pgx.ErrNoRows) {
		return pgtype.Int8{}, invalid("user_uuid", "user is not a member of the organization")
	} else if err != nil {
		return pgtype.Int8{}, fmt.Errorf("accounting: staff membership: %w", err)
	}
	return pgtype.Int8{Int64: u.ID, Valid: true}, nil
}

func (s *Service) staffUserUUID(ctx context.Context, r db.StaffProfile) (*uuid.UUID, error) {
	if !r.UserID.Valid {
		return nil, nil
	}
	u, err := s.q.GetUserByID(ctx, r.UserID.Int64)
	if err != nil {
		return nil, fmt.Errorf("accounting: staff user uuid: %w", err)
	}
	return &u.Uuid, nil
}

func (s *Service) periodAdvanceTotal(ctx context.Context, orgID, staffID int64, period string) (string, error) {
	rows, err := s.q.SumStaffPaymentsByPeriod(ctx, db.SumStaffPaymentsByPeriodParams{
		OrganizationID: orgID,
		Period:         period,
	})
	if err != nil {
		return "", fmt.Errorf("accounting: staff period totals: %w", err)
	}
	total := new(big.Rat)
	for _, r := range rows {
		if r.StaffID == staffID && r.Type == StaffPaymentAdvance {
			total.Add(total, numericRat(r.Total))
		}
	}
	return total.FloatString(2), nil
}

func staffName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", invalid("name", "is required")
	}
	if len([]rune(name)) > 200 {
		return "", invalid("name", "must be at most 200 characters")
	}
	return name, nil
}

func staffPaymentType(raw string) (string, string, error) {
	switch strings.TrimSpace(raw) {
	case StaffPaymentSalary:
		return StaffPaymentSalary, "salary", nil
	case StaffPaymentAdvance:
		return StaffPaymentAdvance, accounting.CategoryStaffAdvance, nil
	case StaffPaymentBonus:
		return StaffPaymentBonus, accounting.CategoryStaffBonus, nil
	default:
		return "", "", invalid("type", "must be salary, advance or bonus")
	}
}

func validPeriod(raw string) (string, error) {
	period := strings.TrimSpace(raw)
	if !periodRe.MatchString(period) {
		return "", invalid("period", "must be YYYY-MM")
	}
	return period, nil
}

func staffPaymentDescription(description, targetNote *string) (string, error) {
	desc := ""
	if description != nil {
		desc = strings.TrimSpace(*description)
	}
	target := ""
	if targetNote != nil {
		target = strings.TrimSpace(*targetNote)
	}
	if len([]rune(desc)) > 1500 {
		return "", invalid("description", "must be at most 1500 characters")
	}
	if len([]rune(target)) > 400 {
		return "", invalid("target_note", "must be at most 400 characters")
	}
	if target == "" {
		return desc, nil
	}
	if desc == "" {
		return "target_note: " + target, nil
	}
	return desc + "\ntarget_note: " + target, nil
}

func splitPaymentDescription(raw string) (string, string) {
	const prefix = "\ntarget_note: "
	if i := strings.LastIndex(raw, prefix); i >= 0 {
		return raw[:i], raw[i+len(prefix):]
	}
	if strings.HasPrefix(raw, "target_note: ") {
		return "", strings.TrimPrefix(raw, "target_note: ")
	}
	return raw, ""
}

func optionalMoney(field string, raw *string) (pgtype.Numeric, error) {
	if raw == nil {
		return pgtype.Numeric{}, nil
	}
	v := strings.TrimSpace(*raw)
	if v == "" {
		return pgtype.Numeric{}, nil
	}
	return requiredMoney(field, v)
}

func requiredMoney(field, raw string) (pgtype.Numeric, error) {
	v := strings.TrimSpace(raw)
	if v == "" || strings.ContainsAny(v, "eE/") {
		return pgtype.Numeric{}, invalid(field, "must be a positive decimal with at most 2 decimals")
	}
	if i := strings.IndexByte(v, '.'); i >= 0 && len(v)-i-1 > 2 {
		return pgtype.Numeric{}, invalid(field, "must be a positive decimal with at most 2 decimals")
	}
	r, ok := new(big.Rat).SetString(v)
	if !ok || r.Sign() <= 0 {
		return pgtype.Numeric{}, invalid(field, "must be a positive decimal with at most 2 decimals")
	}
	var n pgtype.Numeric
	if err := n.Scan(r.FloatString(2)); err != nil {
		return pgtype.Numeric{}, invalid(field, "must be a valid decimal")
	}
	return n, nil
}

func moneyText(n pgtype.Numeric) string {
	if !n.Valid {
		return "0.00"
	}
	return posting.FormatNumeric(n)
}

func numericRat(n pgtype.Numeric) *big.Rat {
	r, ok := new(big.Rat).SetString(moneyText(n))
	if !ok {
		return new(big.Rat)
	}
	return r
}

func boolArg(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

func textArg(v *string) pgtype.Text {
	if v == nil || *v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

func dateArg(v *time.Time) pgtype.Date {
	if v == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *v, Valid: true}
}

func int8PtrArg(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func staffPaymentCreateErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == "uq_staff_payments_salary_period" {
		return ErrStaffSalaryExists
	}
	return fmt.Errorf("accounting: create staff payment: %w", err)
}
