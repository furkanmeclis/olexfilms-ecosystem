package usecase

// Billing details of a cari organization (TEC-1070): the legal title, tax
// office and number, invoice address and e-mail and IBAN that the seller's
// accounting needs to cut an invoice. They belong to the counterparty
// organization (migration 000172); the accounting of an organization above
// it fills them from the cari card.

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrBillingNotEditable: the cari is a customer's, or its organization is
// not below the book (a dealer does not edit its supplier's details).
var ErrBillingNotEditable = errors.New("accounting: billing details of this cari cannot be edited here")

// BillingProfile is GET/PUT /v1/accounting/cari/{uuid}/billing.
type BillingProfile struct {
	Title     *string    `json:"title"`
	TaxOffice *string    `json:"tax_office"`
	TaxNo     *string    `json:"tax_no"`
	Address   *string    `json:"address"`
	Email     *string    `json:"email"`
	IBAN      *string    `json:"iban"`
	Editable  bool       `json:"editable"`
	UpdatedAt *time.Time `json:"updated_at"`
	UpdatedBy *string    `json:"updated_by"`
}

// BillingInput is the body of PUT /v1/accounting/cari/{uuid}/billing; an
// empty field clears it.
type BillingInput struct {
	Title, TaxOffice, TaxNo, Address, Email, IBAN string
}

var taxNoShape = regexp.MustCompile(`^[A-Za-z0-9-]{1,32}$`)

type billingArgs struct {
	title, office, taxNo, address, email, iban pgtype.Text
}

func (in BillingInput) validate() (billingArgs, error) {
	var a billingArgs
	limit := func(field, v string, max int) (pgtype.Text, error) {
		v = strings.TrimSpace(v)
		if len([]rune(v)) > max {
			return pgtype.Text{}, invalid(field, fmt.Sprintf("must be at most %d characters", max))
		}
		return pgtype.Text{String: v, Valid: v != ""}, nil
	}
	var err error
	if a.title, err = limit("title", in.Title, 255); err != nil {
		return a, err
	}
	if a.office, err = limit("tax_office", in.TaxOffice, 120); err != nil {
		return a, err
	}
	if a.taxNo, err = limit("tax_no", in.TaxNo, 32); err != nil {
		return a, err
	}
	if a.taxNo.Valid && !taxNoShape.MatchString(a.taxNo.String) {
		return a, invalid("tax_no", "letters, digits and dashes only")
	}
	if a.address, err = limit("address", in.Address, 2000); err != nil {
		return a, err
	}
	if a.email, err = limit("email", in.Email, 255); err != nil {
		return a, err
	}
	if a.email.Valid {
		addr, perr := mail.ParseAddress(a.email.String)
		if perr != nil || addr.Address != a.email.String {
			return a, invalid("email", "must be an e-mail address")
		}
	}
	if strings.TrimSpace(in.IBAN) != "" {
		iban, err := NormalizeIBAN(in.IBAN)
		if err != nil {
			return a, err
		}
		a.iban = pgtype.Text{String: iban, Valid: true}
	}
	return a, nil
}

// cariOrganization resolves the counterparty organization of a cari in the
// book and whether it sits below the book (editable).
func (s *Service) cariOrganization(ctx context.Context, book db.Organization, id uuid.UUID) (int64, bool, error) {
	c, err := s.cari(ctx, book.ID, id)
	if err != nil {
		return 0, false, err
	}
	if c.Counterparty.Type != "organization" || c.Counterparty.UUID == nil {
		return 0, false, ErrBillingNotEditable
	}
	o, err := s.q.GetOrganizationByUUID(ctx, *c.Counterparty.UUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, ErrCariNotFound
	}
	if err != nil {
		return 0, false, fmt.Errorf("accounting: counterparty: %w", err)
	}
	below, err := s.q.Descendants(ctx, book.ID)
	if err != nil {
		return 0, false, fmt.Errorf("accounting: descendants: %w", err)
	}
	for _, d := range below {
		if d.ID == o.ID {
			return o.ID, true, nil
		}
	}
	return o.ID, false, nil
}

func (s *Service) billing(ctx context.Context, orgID int64, editable bool) (BillingProfile, error) {
	r, err := s.q.GetOrganizationBilling(ctx, orgID)
	if err != nil {
		return BillingProfile{}, fmt.Errorf("accounting: billing: %w", err)
	}
	return BillingProfile{
		Title: textPtr(r.BillingTitle), TaxOffice: textPtr(r.TaxOffice), TaxNo: textPtr(r.TaxNo),
		Address: textPtr(r.BillingAddress), Email: textPtr(r.BillingEmail), IBAN: textPtr(r.Iban),
		Editable: editable, UpdatedAt: tsPtr(r.BillingUpdatedAt), UpdatedBy: strPtr(r.BillingUpdatedByName),
	}, nil
}

// GetCariBilling reads the billing details of an organization cari of the
// book (accounting.read). Editable reports whether the active book may
// change them.
func (s *Service) GetCariBilling(ctx context.Context, c Caller, orgUUID *uuid.UUID, id uuid.UUID) (BillingProfile, error) {
	book, err := s.readBook(ctx, c, orgUUID)
	if err != nil {
		return BillingProfile{}, err
	}
	orgID, below, err := s.cariOrganization(ctx, book, id)
	if err != nil {
		return BillingProfile{}, err
	}
	active, err := s.activeOrg(ctx, c)
	if err != nil {
		return BillingProfile{}, err
	}
	return s.billing(ctx, orgID, below && book.ID == active.ID)
}

// UpdateCariBilling replaces the billing details of an organization below
// the active book (accounting.write on the active book).
func (s *Service) UpdateCariBilling(ctx context.Context, c Caller, id uuid.UUID, in BillingInput) (BillingProfile, error) {
	book, err := s.writeBook(ctx, c)
	if err != nil {
		return BillingProfile{}, err
	}
	a, err := in.validate()
	if err != nil {
		return BillingProfile{}, err
	}
	orgID, below, err := s.cariOrganization(ctx, book, id)
	if err != nil {
		return BillingProfile{}, err
	}
	if !below {
		return BillingProfile{}, ErrBillingNotEditable
	}
	var user pgtype.Int8
	if actor := c.actor(); actor != nil {
		user = pgtype.Int8{Int64: *actor, Valid: true}
	}
	if err := s.q.UpdateOrganizationBilling(ctx, db.UpdateOrganizationBillingParams{
		ID: orgID, BillingTitle: a.title, TaxOffice: a.office, TaxNo: a.taxNo,
		BillingAddress: a.address, BillingEmail: a.email, Iban: a.iban, UserID: user,
	}); err != nil {
		return BillingProfile{}, fmt.Errorf("accounting: update billing: %w", err)
	}
	return s.billing(ctx, orgID, true)
}
