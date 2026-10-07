package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	acc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// ListStaffProfiles (GET /v1/staff-profiles?active&limit&offset).
func (h *Handler) ListStaffProfiles(w http.ResponseWriter, r *http.Request) {
	active, ok := queryBool(w, r, "active")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.ListStaffProfiles(r.Context(), caller(r), acc.StaffProfileFilter{
		Active: active,
		Limit:  q.Limit,
		Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

type createStaffProfileBody struct {
	UserUUID      *uuid.UUID `json:"user_uuid"`
	Name          string     `json:"name"`
	Title         *string    `json:"title"`
	HiredOn       string     `json:"hired_on"`
	MonthlySalary *string    `json:"monthly_salary"`
	Active        *bool      `json:"active"`
}

// CreateStaffProfile (POST /v1/staff-profiles).
func (h *Handler) CreateStaffProfile(w http.ResponseWriter, r *http.Request) {
	var b createStaffProfileBody
	if !decode(w, r, &b) {
		return
	}
	hiredOn, err := bodyDate("hired_on", b.HiredOn)
	if err != nil {
		writeError(w, r, err)
		return
	}
	active := true
	if b.Active != nil {
		active = *b.Active
	}
	p, err := h.svc.CreateStaffProfile(r.Context(), caller(r), acc.CreateStaffProfileInput{
		UserUUID:      b.UserUUID,
		Name:          b.Name,
		Title:         b.Title,
		HiredOn:       hiredOn,
		MonthlySalary: b.MonthlySalary,
		Active:        active,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, p)
}

type updateStaffProfileBody struct {
	UserUUID      json.RawMessage `json:"user_uuid"`
	Name          *string         `json:"name"`
	Title         json.RawMessage `json:"title"`
	HiredOn       json.RawMessage `json:"hired_on"`
	MonthlySalary json.RawMessage `json:"monthly_salary"`
	Active        *bool           `json:"active"`
}

// UpdateStaffProfile (PATCH /v1/staff-profiles/{uuid}).
func (h *Handler) UpdateStaffProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b updateStaffProfileBody
	if !decode(w, r, &b) {
		return
	}
	in := acc.UpdateStaffProfileInput{Name: b.Name, Active: b.Active}
	if !nullableUUID(w, r, "user_uuid", b.UserUUID, &in.UserUUID, &in.ClearUser) {
		return
	}
	if !nullableString(w, r, "title", b.Title, &in.Title, &in.ClearTitle) {
		return
	}
	var hiredOn *string
	if !nullableString(w, r, "hired_on", b.HiredOn, &hiredOn, &in.ClearHiredOn) {
		return
	}
	if hiredOn != nil {
		d, err := parseBodyDate("hired_on", *hiredOn)
		if err != nil {
			writeError(w, r, err)
			return
		}
		in.HiredOn = d
	}
	if !nullableString(w, r, "monthly_salary", b.MonthlySalary, &in.MonthlySalary, &in.ClearSalary) {
		return
	}
	p, err := h.svc.UpdateStaffProfile(r.Context(), caller(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

type staffPaymentBody struct {
	Type        string     `json:"type"`
	Period      string     `json:"period"`
	Amount      *string    `json:"amount"`
	AccountUUID *uuid.UUID `json:"account_uuid"`
	Description *string    `json:"description"`
	TargetNote  *string    `json:"target_note"`
	PaidOn      string     `json:"paid_on"`
}

// CreateStaffPayment (POST /v1/staff-profiles/{uuid}/payments).
func (h *Handler) CreateStaffPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b staffPaymentBody
	if !decode(w, r, &b) {
		return
	}
	paidOn, err := bodyDate("paid_on", b.PaidOn)
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := h.svc.CreateStaffPayment(r.Context(), caller(r), id, acc.StaffPaymentInput{
		Type:        strings.TrimSpace(b.Type),
		Period:      strings.TrimSpace(b.Period),
		Amount:      b.Amount,
		AccountUUID: b.AccountUUID,
		Description: b.Description,
		TargetNote:  b.TargetNote,
		PaidOn:      paidOn,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, p)
}

// staffPaymentPage is a payment list page with the amount total of every
// matching payment (TEC-381).
type staffPaymentPage struct {
	apiquery.Page[acc.StaffPayment]
	TotalAmount string `json:"total_amount"`
	Currency    string `json:"currency"`
}

// staffPaymentFilter reads the list contract parameters of the payment
// lists: sort, type / status (CSV), period, paid_on_from / paid_on_to.
func staffPaymentFilter(r *http.Request) (acc.StaffPaymentFilter, apiquery.Query, error) {
	values := r.URL.Query()
	q := apiquery.Parse(values)
	f := acc.StaffPaymentFilter{Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.Sort, err = apiquery.ResolveSort(q.Sort, acc.StaffPaymentSortSpec); err != nil {
		return f, q, err
	}
	if f.Types, err = apiquery.EnumList(values, "type", acc.StaffPaymentTypes...); err != nil {
		return f, q, err
	}
	if f.Statuses, err = apiquery.EnumList(values, "status", acc.StaffPaymentStatuses...); err != nil {
		return f, q, err
	}
	if v := strings.TrimSpace(values.Get("period")); v != "" {
		f.Period = &v
	}
	paid, err := apiquery.DateRange(values, "paid_on")
	if err != nil {
		return f, q, err
	}
	if paid.From != nil {
		d := paid.From.UTC()
		f.PaidFrom = &d
	}
	if paid.Before != nil {
		// paid_on is a day column: the last day before the exclusive bound.
		d := paid.Before.UTC().Add(-time.Nanosecond)
		f.PaidTo = &d
	}
	return f, q, nil
}

// ListStaffPayments (GET /v1/staff-profiles/{uuid}/payments).
func (h *Handler) ListStaffPayments(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	f, q, err := staffPaymentFilter(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	page, err := h.svc.ListStaffPayments(r.Context(), caller(r), id, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeStaffPaymentPage(w, r, page, q)
}

// ListBookStaffPayments (GET /v1/staff-payments): every staff card's
// payments, e.g. ?status=planned for the upcoming ones (TEC-381).
func (h *Handler) ListBookStaffPayments(w http.ResponseWriter, r *http.Request) {
	f, q, err := staffPaymentFilter(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	page, err := h.svc.ListBookStaffPayments(r.Context(), caller(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeStaffPaymentPage(w, r, page, q)
}

func writeStaffPaymentPage(w http.ResponseWriter, r *http.Request, page acc.StaffPaymentPage, q apiquery.Query) {
	response.JSON(w, r, http.StatusOK, staffPaymentPage{
		Page:        apiquery.NewPage(page.Items, page.Total, q.Limit, q.Offset),
		TotalAmount: page.TotalAmount,
		Currency:    page.Currency,
	})
}

type updateStaffPaymentBody struct {
	Type        *string         `json:"type"`
	Period      *string         `json:"period"`
	Amount      *string         `json:"amount"`
	AccountUUID *uuid.UUID      `json:"account_uuid"`
	Description json.RawMessage `json:"description"`
	TargetNote  json.RawMessage `json:"target_note"`
	PaidOn      *string         `json:"paid_on"`
}

// UpdateStaffPayment (PATCH /v1/staff-payments/{uuid}): edits a planned
// payment (TEC-381).
func (h *Handler) UpdateStaffPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var b updateStaffPaymentBody
	if !decode(w, r, &b) {
		return
	}
	in := acc.UpdateStaffPaymentInput{Type: b.Type, Period: b.Period, Amount: b.Amount, AccountUUID: b.AccountUUID}
	if !nullableString(w, r, "description", b.Description, &in.Description, &in.ClearDescription) {
		return
	}
	if !nullableString(w, r, "target_note", b.TargetNote, &in.TargetNote, &in.ClearTargetNote) {
		return
	}
	if b.PaidOn != nil {
		d, err := parseBodyDate("paid_on", *b.PaidOn)
		if err != nil {
			writeError(w, r, err)
			return
		}
		if d == nil {
			writeError(w, r, &acc.ValidationError{Field: "paid_on", Message: "must be a date (YYYY-MM-DD)"})
			return
		}
		in.PaidOn = d
	}
	p, err := h.svc.UpdateStaffPayment(r.Context(), caller(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

// CancelStaffPayment (POST /v1/staff-payments/{uuid}/cancel): calls off a
// planned payment (TEC-381).
func (h *Handler) CancelStaffPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	p, err := h.svc.CancelStaffPayment(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

// RunPayroll (POST /v1/staff-payments/payroll?period=YYYY-MM&paid_on=YYYY-MM-DD).
func (h *Handler) RunPayroll(w http.ResponseWriter, r *http.Request) {
	paidOn, err := parseBodyDate("paid_on", r.URL.Query().Get("paid_on"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	p, err := h.svc.RunPayroll(r.Context(), caller(r), r.URL.Query().Get("period"), paidOn)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, p)
}

func nullableUUID(
	w http.ResponseWriter,
	r *http.Request,
	field string,
	raw json.RawMessage,
	dst **uuid.UUID,
	clear *bool,
) bool {
	if len(raw) == 0 {
		return true
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		*clear = true
		return true
	}
	var id uuid.UUID
	if err := json.Unmarshal(raw, &id); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: field, Message: "must be a UUID or null"}})
		return false
	}
	*dst = &id
	return true
}

func nullableString(
	w http.ResponseWriter,
	r *http.Request,
	field string,
	raw json.RawMessage,
	dst **string,
	clear *bool,
) bool {
	if len(raw) == 0 {
		return true
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		*clear = true
		return true
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: field, Message: "must be a string or null"}})
		return false
	}
	*dst = &v
	return true
}

func parseBodyDate(field, raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	d, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return nil, &acc.ValidationError{Field: field, Message: "must be a date (YYYY-MM-DD)"}
	}
	return &d, nil
}
