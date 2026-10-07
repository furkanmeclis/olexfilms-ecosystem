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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
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

	// Payment states (TEC-381): a payment dated in the future waits as
	// planned (no ledger row) until its paid_on; cancelled is a planned
	// payment called off.
	StaffPaymentPlanned   = "planned"
	StaffPaymentPosted    = "posted"
	StaffPaymentCancelled = "cancelled"
)

// StaffPaymentStatuses lists the status filter values.
var StaffPaymentStatuses = []string{StaffPaymentPlanned, StaffPaymentPosted, StaffPaymentCancelled}

// StaffPaymentTypes lists the type filter values.
var StaffPaymentTypes = []string{StaffPaymentSalary, StaffPaymentAdvance, StaffPaymentBonus}

// StaffPaymentSortSpec is the sort whitelist of the payment lists
// (docs/list-contract.md); newest payment day first by default.
var StaffPaymentSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"paid_on": "paid_on", "amount": "amount", "created_at": "created_at",
		"type": "type", "status": "status", "period": "period", "staff_name": "staff_name",
	},
	Default: apiquery.SortField{Field: "paid_on", Desc: true},
}

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
	UUID      uuid.UUID `json:"uuid"`
	StaffUUID uuid.UUID `json:"staff_uuid"`
	StaffName string    `json:"staff_name"`
	Type      string    `json:"type"`
	Period    string    `json:"period"`
	Amount    string    `json:"amount"`
	Currency  string    `json:"currency"`
	PaidOn    string    `json:"paid_on"`
	// Status: planned (paid_on still ahead, no ledger row), posted or
	// cancelled (TEC-381).
	Status      string     `json:"status"`
	AccountUUID *uuid.UUID `json:"account_uuid"`
	Description *string    `json:"description"`
	TargetNote  *string    `json:"target_note"`
	// FinanceEntryUUID is the ledger row; null until the payment is posted.
	FinanceEntryUUID *uuid.UUID `json:"finance_entry_uuid"`
	PeriodAdvances   string     `json:"period_advances"`
	CreatedAt        time.Time  `json:"created_at"`
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

// UpdateStaffPaymentInput edits a planned payment; nil keeps a field.
type UpdateStaffPaymentInput struct {
	Type             *string
	Period           *string
	Amount           *string
	AccountUUID      *uuid.UUID
	Description      *string
	ClearDescription bool
	TargetNote       *string
	ClearTargetNote  bool
	PaidOn           *time.Time
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

func staffPaymentOf(r db.ListStaffPaymentsRow, advances string) StaffPayment {
	out := StaffPayment{
		UUID:           r.Uuid,
		StaffUUID:      r.StaffUuid,
		StaffName:      r.StaffName,
		Type:           r.Type,
		Period:         r.Period,
		Amount:         posting.FormatNumeric(r.Amount),
		Currency:       r.Currency,
		PaidOn:         r.PaidOn.Time.Format(time.DateOnly),
		Status:         r.Status,
		PeriodAdvances: advances,
		CreatedAt:      r.CreatedAt.Time,
	}
	if r.AccountUuid.Valid {
		id := uuid.UUID(r.AccountUuid.Bytes)
		out.AccountUUID = &id
	}
	if r.FinanceEntryUuid.Valid {
		id := uuid.UUID(r.FinanceEntryUuid.Bytes)
		out.FinanceEntryUUID = &id
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
	return s.createStaffPayment(ctx, c, book, staff, in, false)
}

// StaffPaymentFilter filters the payment lists (docs/list-contract.md).
type StaffPaymentFilter struct {
	Types    []string
	Statuses []string
	Period   *string
	// PaidFrom / PaidTo are payment days, both inclusive.
	PaidFrom, PaidTo *time.Time
	Sort             apiquery.ResolvedSort
	Limit, Offset    int32
}

// StaffPaymentPage is one page of a payment list with the amount total of
// every matching payment (the "upcoming payments" sum of the planned view).
type StaffPaymentPage struct {
	Items       []StaffPayment
	Total       int64
	TotalAmount string
	Currency    string
}

// ListStaffPayments is the payment history of one staff card (TEC-349)
// with the status / payment day filters of TEC-381.
func (s *Service) ListStaffPayments(ctx context.Context, c Caller, staffUUID uuid.UUID, f StaffPaymentFilter) (StaffPaymentPage, error) {
	book, err := s.writeBook(ctx, c)
	if err != nil {
		return StaffPaymentPage{}, err
	}
	staff, err := s.staffRow(ctx, book.ID, staffUUID)
	if err != nil {
		return StaffPaymentPage{}, err
	}
	return s.listStaffPayments(ctx, book, &staff.ID, f)
}

// ListBookStaffPayments lists the payments of every staff card of the book,
// e.g. the planned payments still to go out (TEC-381).
func (s *Service) ListBookStaffPayments(ctx context.Context, c Caller, f StaffPaymentFilter) (StaffPaymentPage, error) {
	book, err := s.writeBook(ctx, c)
	if err != nil {
		return StaffPaymentPage{}, err
	}
	return s.listStaffPayments(ctx, book, nil, f)
}

func (s *Service) listStaffPayments(ctx context.Context, book db.Organization, staffID *int64, f StaffPaymentFilter) (StaffPaymentPage, error) {
	var periodArg pgtype.Text
	if f.Period != nil {
		p, err := validPeriod(*f.Period)
		if err != nil {
			return StaffPaymentPage{}, err
		}
		periodArg = pgtype.Text{String: p, Valid: true}
	}
	if f.PaidFrom != nil && f.PaidTo != nil && f.PaidFrom.After(*f.PaidTo) {
		return StaffPaymentPage{}, invalid("paid_on_from", "must not be after paid_on_to")
	}
	sort := f.Sort
	if sort.Key == "" {
		sort = apiquery.ResolvedSort{Key: StaffPaymentSortSpec.Default.Field, Desc: StaffPaymentSortSpec.Default.Desc}
	}
	staffArg := int8PtrArg(staffID)
	paidFrom, paidTo := dateArg(f.PaidFrom), dateArg(f.PaidTo)
	rows, err := s.q.ListStaffPayments(ctx, db.ListStaffPaymentsParams{
		OrganizationID: book.ID,
		StaffID:        staffArg,
		Period:         periodArg,
		Types:          f.Types,
		Statuses:       f.Statuses,
		PaidFrom:       paidFrom,
		PaidTo:         paidTo,
		SortKey:        sort.Key,
		SortDesc:       sort.Desc,
		PageLimit:      f.Limit,
		PageOffset:     f.Offset,
	})
	if err != nil {
		return StaffPaymentPage{}, fmt.Errorf("accounting: list staff payments: %w", err)
	}
	count, err := s.q.CountStaffPayments(ctx, db.CountStaffPaymentsParams{
		OrganizationID: book.ID,
		StaffID:        staffArg,
		Period:         periodArg,
		Types:          f.Types,
		Statuses:       f.Statuses,
		PaidFrom:       paidFrom,
		PaidTo:         paidTo,
	})
	if err != nil {
		return StaffPaymentPage{}, fmt.Errorf("accounting: count staff payments: %w", err)
	}
	type staffPeriod struct {
		staff  int64
		period string
	}
	advances := map[staffPeriod]string{}
	out := StaffPaymentPage{
		Items: make([]StaffPayment, 0, len(rows)), Total: count.Total,
		TotalAmount: moneyText(count.Amount), Currency: book.Currency,
	}
	for _, r := range rows {
		key := staffPeriod{r.StaffID, r.Period}
		adv, ok := advances[key]
		if !ok {
			adv, err = s.periodAdvanceTotal(ctx, book.ID, r.StaffID, r.Period)
			if err != nil {
				return StaffPaymentPage{}, err
			}
			advances[key] = adv
		}
		out.Items = append(out.Items, staffPaymentOf(r, adv))
	}
	return out, nil
}

// RunPayroll writes the salary of every active staff card for a period.
// paidOn (nil: today in the organization's time zone) is the payment day:
// a future day writes planned salaries that are booked on that day
// (TEC-381).
func (s *Service) RunPayroll(ctx context.Context, c Caller, period string, paidOn *time.Time) (PayrollResult, error) {
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
			PaidOn: paidOn,
		}, true)
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
	allowTargetless bool,
) (StaffPayment, error) {
	paymentType, _, err := staffPaymentType(in.Type)
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
	var accountID pgtype.Int8
	if in.AccountUUID != nil {
		accountID, err = s.paymentAccount(ctx, book, *in.AccountUUID)
		if err != nil {
			return StaffPayment{}, err
		}
	} else if !allowTargetless {
		return StaffPayment{}, invalid("account_uuid", "is required")
	}
	now := s.clock()
	today := bookToday(book, now)
	paidOn := today
	if in.PaidOn != nil {
		paidOn = dayStart(*in.PaidOn)
	}
	status := StaffPaymentPosted
	if paidOn.After(today) {
		status = StaffPaymentPlanned
	}

	var payment db.StaffPayment
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
			Status:          status,
			AccountID:       accountID,
		})
		if err != nil {
			return staffPaymentCreateErr(err)
		}
		if status == StaffPaymentPosted {
			return s.postStaffPayment(ctx, tx, payment, c.actor(), now)
		}
		return nil
	})
	if err != nil {
		return StaffPayment{}, err
	}
	return s.staffPaymentView(ctx, book.ID, payment.ID)
}

// UpdateStaffPayment edits a planned payment (TEC-381). When the new paid_on
// is today or past it is booked at once on that day; a posted payment is
// only undone by a reversal (ErrStaffPaymentNotPlanned).
func (s *Service) UpdateStaffPayment(ctx context.Context, c Caller, id uuid.UUID, in UpdateStaffPaymentInput) (StaffPayment, error) {
	book, err := s.writeBook(ctx, c)
	if err != nil {
		return StaffPayment{}, err
	}
	cur, err := s.staffPaymentRow(ctx, book.ID, id)
	if err != nil {
		return StaffPayment{}, err
	}
	if cur.Status != StaffPaymentPlanned {
		return StaffPayment{}, ErrStaffPaymentNotPlanned
	}
	arg := db.UpdatePlannedStaffPaymentParams{ID: cur.ID, OrganizationID: book.ID}
	typ := cur.Type
	if in.Type != nil {
		typ = *in.Type
	}
	if arg.Type, _, err = staffPaymentType(typ); err != nil {
		return StaffPayment{}, err
	}
	period := cur.Period
	if in.Period != nil {
		period = *in.Period
	}
	if arg.Period, err = validPeriod(period); err != nil {
		return StaffPayment{}, err
	}
	arg.Amount = cur.Amount
	if in.Amount != nil {
		if arg.Amount, err = requiredMoney("amount", *in.Amount); err != nil {
			return StaffPayment{}, err
		}
	}
	arg.AccountID = cur.AccountID
	if in.AccountUUID != nil {
		if arg.AccountID, err = s.paymentAccount(ctx, book, *in.AccountUUID); err != nil {
			return StaffPayment{}, err
		}
	}
	desc, target := "", ""
	if cur.Description.Valid {
		desc, target = splitPaymentDescription(cur.Description.String)
	}
	switch {
	case in.ClearDescription:
		desc = ""
	case in.Description != nil:
		desc = *in.Description
	}
	switch {
	case in.ClearTargetNote:
		target = ""
	case in.TargetNote != nil:
		target = *in.TargetNote
	}
	combined, err := staffPaymentDescription(&desc, &target)
	if err != nil {
		return StaffPayment{}, err
	}
	arg.Description = textArg(&combined)
	arg.PaidOn = cur.PaidOn
	if in.PaidOn != nil {
		arg.PaidOn = pgtype.Date{Time: dayStart(*in.PaidOn), Valid: true}
	}
	now := s.clock()
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		qtx := s.q.WithTx(tx)
		locked, err := qtx.LockStaffPayment(ctx, db.LockStaffPaymentParams{ID: cur.ID, OrganizationID: book.ID})
		if err != nil {
			return fmt.Errorf("accounting: lock staff payment: %w", err)
		}
		if locked.Status != StaffPaymentPlanned || locked.VoidedAt.Valid {
			return ErrStaffPaymentNotPlanned
		}
		updated, err := qtx.UpdatePlannedStaffPayment(ctx, arg)
		if err != nil {
			return staffPaymentCreateErr(err)
		}
		if !updated.PaidOn.Time.After(bookToday(book, now)) {
			return s.postStaffPayment(ctx, tx, updated, c.actor(), now)
		}
		return nil
	})
	if err != nil {
		return StaffPayment{}, err
	}
	return s.staffPaymentView(ctx, book.ID, cur.ID)
}

// CancelStaffPayment calls off a planned payment; it never reached the
// ledger, so nothing is reversed (TEC-381).
func (s *Service) CancelStaffPayment(ctx context.Context, c Caller, id uuid.UUID) (StaffPayment, error) {
	book, err := s.writeBook(ctx, c)
	if err != nil {
		return StaffPayment{}, err
	}
	cur, err := s.staffPaymentRow(ctx, book.ID, id)
	if err != nil {
		return StaffPayment{}, err
	}
	_, err = s.q.CancelPlannedStaffPayment(ctx, db.CancelPlannedStaffPaymentParams{ID: cur.ID, OrganizationID: book.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return StaffPayment{}, ErrStaffPaymentNotPlanned
	}
	if err != nil {
		return StaffPayment{}, fmt.Errorf("accounting: cancel staff payment: %w", err)
	}
	return s.staffPaymentView(ctx, book.ID, cur.ID)
}

// dueStaffPaymentBatch bounds one pass of the posting job; the next hourly
// run picks up the rest.
const dueStaffPaymentBatch = 500

// PostDueStaffPayments books the planned payments whose paid_on has arrived
// in their organization's time zone (worker-core, TEC-381). Each payment is
// locked and re-checked in its own transaction and the ledger write is
// source-keyed, so a second run writes nothing. It returns the number of
// payments booked.
func (s *Service) PostDueStaffPayments(ctx context.Context) (int, error) {
	now := s.clock()
	due, err := s.q.ListDueStaffPayments(ctx, db.ListDueStaffPaymentsParams{
		Now:       pgtype.Timestamptz{Time: now, Valid: true},
		PageLimit: dueStaffPaymentBatch,
	})
	if err != nil {
		return 0, fmt.Errorf("accounting: due staff payments: %w", err)
	}
	posted := 0
	var errs []error
	for _, d := range due {
		done := false
		err := s.inTx(ctx, func(tx pgx.Tx) error {
			p, err := s.q.WithTx(tx).LockStaffPayment(ctx, db.LockStaffPaymentParams(d))
			if err != nil {
				return fmt.Errorf("accounting: lock staff payment: %w", err)
			}
			if p.Status != StaffPaymentPlanned || p.VoidedAt.Valid {
				return nil
			}
			done = true
			return s.postStaffPayment(ctx, tx, p, nil, now)
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("staff payment %d: %w", d.ID, err))
			continue
		}
		if done {
			posted++
		}
	}
	return posted, errors.Join(errs...)
}

// PostDueStaffPaymentsTask is the worker-core task body.
func (s *Service) PostDueStaffPaymentsTask(ctx context.Context) error {
	_, err := s.PostDueStaffPayments(ctx)
	return err
}

// postStaffPayment writes the expense row of a payment on its paid_on and
// marks it posted. Payroll rows without an account are the targetless
// staff_payment rows of 000097.
func (s *Service) postStaffPayment(ctx context.Context, tx pgx.Tx, p db.StaffPayment, actor *int64, now time.Time) error {
	_, category, err := staffPaymentType(p.Type)
	if err != nil {
		return err
	}
	desc := ""
	if p.Description.Valid {
		desc = p.Description.String
	}
	res, err := s.poster.PostExpense(ctx, tx, posting.Entry{
		OrganizationID: p.OrganizationID,
		Source:         posting.Source{Type: SourceStaffPayment, UUID: p.Uuid},
		Category:       category,
		Amount:         posting.FormatNumeric(p.Amount),
		Currency:       p.Currency,
		AccountID:      p.AccountID.Int64,
		AllowNoTarget:  !p.AccountID.Valid,
		Description:    desc,
		ActorUserID:    actor,
		PostedAt:       ledgerTime(p.PaidOn.Time, now),
	})
	if err != nil {
		return postingErr(err)
	}
	if _, err := s.q.WithTx(tx).SetStaffPaymentFinanceEntry(ctx, db.SetStaffPaymentFinanceEntryParams{
		FinanceEntryID: pgtype.Int8{Int64: res.Entry.ID, Valid: true},
		ID:             p.ID,
		OrganizationID: p.OrganizationID,
	}); err != nil {
		return fmt.Errorf("accounting: link staff payment: %w", err)
	}
	return nil
}

// ledgerTime places a payment's ledger row on its paid_on (the reports
// read UTC calendar days): now when paid_on is today (UTC), else the start
// of that day.
func ledgerTime(paidOn, now time.Time) time.Time {
	day := dayStart(paidOn)
	if dayStart(now.UTC()).Equal(day) {
		return now
	}
	return day
}

// bookToday is today's date in the organization's time zone, as a UTC
// midnight like the parsed paid_on dates.
func bookToday(book db.Organization, now time.Time) time.Time {
	loc, err := time.LoadLocation(book.Timezone)
	if err != nil || book.Timezone == "" {
		loc, _ = time.LoadLocation(i18n.DefaultTimezone)
	}
	if loc == nil {
		loc = time.UTC
	}
	return dayStart(now.In(loc))
}

func (s *Service) paymentAccount(ctx context.Context, book db.Organization, id uuid.UUID) (pgtype.Int8, error) {
	a, err := s.q.GetFinanceAccountByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && a.OrganizationID != book.ID) {
		return pgtype.Int8{}, ErrAccountNotFound
	}
	if err != nil {
		return pgtype.Int8{}, fmt.Errorf("accounting: account: %w", err)
	}
	if !a.Active {
		return pgtype.Int8{}, invalid("account_uuid", "the account is inactive")
	}
	return pgtype.Int8{Int64: a.ID, Valid: true}, nil
}

func (s *Service) staffPaymentRow(ctx context.Context, orgID int64, id uuid.UUID) (db.StaffPayment, error) {
	row, err := s.q.GetStaffPaymentByUUID(ctx, db.GetStaffPaymentByUUIDParams{Uuid: id, OrganizationID: orgID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.VoidedAt.Valid) {
		return db.StaffPayment{}, ErrStaffPaymentNotFound
	}
	if err != nil {
		return db.StaffPayment{}, fmt.Errorf("accounting: staff payment: %w", err)
	}
	return row, nil
}

// staffPaymentView reads one payment back in its API shape.
func (s *Service) staffPaymentView(ctx context.Context, orgID, id int64) (StaffPayment, error) {
	rows, err := s.q.ListStaffPayments(ctx, db.ListStaffPaymentsParams{
		OrganizationID: orgID,
		PaymentID:      pgtype.Int8{Int64: id, Valid: true},
		SortKey:        "paid_on",
		PageLimit:      1,
	})
	if err != nil {
		return StaffPayment{}, fmt.Errorf("accounting: staff payment view: %w", err)
	}
	if len(rows) == 0 {
		return StaffPayment{}, ErrStaffPaymentNotFound
	}
	adv, err := s.periodAdvanceTotal(ctx, orgID, rows[0].StaffID, rows[0].Period)
	if err != nil {
		return StaffPayment{}, err
	}
	return staffPaymentOf(rows[0], adv), nil
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
