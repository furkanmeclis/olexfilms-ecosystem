package usecase

// TEC-1062: the center opens an order for a recipient that is not a dealer
// yet (a dealer candidate, or a retail delivery point of a Glorian supply).
// The recipient is a dealer organization in status "pending" directly
// under the center: no members, no sign-in, no trial, no contract (K23
// binds distributor-opened dealers only). It is a normal buyer of the
// order flow (stock in transit, shipping, the Glorian outbound held
// missing_customer_link until a hub dealer matches its phone), and its row
// in pending_recipients is the center's waiting pool. The center later
// converts it into an active dealer (the owner attached by phone, the
// mobile OTP sign-in) or keeps it as a retail point. Orders stay on the
// same organization, so nothing has to be re-linked.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Recipient kinds and pool states (chk_pending_recipients_*).
const (
	RecipientDealerCandidate = "dealer_candidate"
	RecipientRetailPoint     = "retail_point"

	RecipientPending   = "pending"
	RecipientRetail    = "retail"
	RecipientConverted = "converted"

	// RecipientOrgStatus is organizations.status of a recipient that is not
	// an active dealer (pool or retail point).
	RecipientOrgStatus = "pending"
)

// RecipientKinds and RecipientStates are the accepted filter values.
var (
	RecipientKinds  = []string{RecipientDealerCandidate, RecipientRetailPoint}
	RecipientStates = []string{RecipientPending, RecipientRetail, RecipientConverted}
)

// PendingRecipientSort is the list contract of GET /v1/tenant/pending-recipients.
var PendingRecipientSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"name": "name", "kind": "kind", "state": "state", "city": "city", "created_at": "created_at",
	},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

var taxNoRe = regexp.MustCompile(`^[0-9]{10,11}$`)

// RecipientInput is a new recipient typed on the order form.
type RecipientInput struct {
	Name      string
	Phone     string
	City      string
	District  string
	Address   string
	Location  AddressInput
	TaxNo     string
	TaxOffice string
	Kind      string
	Note      string
}

// PendingRecipient is one row of the waiting pool.
type PendingRecipient struct {
	UUID        uuid.UUID  `json:"uuid"`
	Slug        string     `json:"slug"`
	Name        string     `json:"name"`
	Phone       string     `json:"phone"`
	City        string     `json:"city"`
	District    string     `json:"district"`
	Address     string     `json:"address"`
	TaxNo       *string    `json:"tax_no,omitempty"`
	TaxOffice   *string    `json:"tax_office,omitempty"`
	Kind        string     `json:"kind"`
	State       string     `json:"state"`
	OrgStatus   string     `json:"organization_status"`
	Note        *string    `json:"note,omitempty"`
	OrdersCount int64      `json:"orders_count"`
	CreatedAt   time.Time  `json:"created_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}

// RecipientListInput filters the pool list.
type RecipientListInput struct {
	Q       string
	States  []string
	Kinds   []string
	Created apiquery.TimeRange
	Sort    apiquery.ResolvedSort
	Limit   int32
	Offset  int32
}

// ConvertInput turns a recipient into an active dealer. Owner is optional:
// with a phone the person becomes dealer_owner (OTP sign-in on the phone).
type ConvertInput struct {
	Owner *AccountInput
}

// ConvertResult is the converted recipient and its owner (when given).
type ConvertResult struct {
	Recipient PendingRecipient `json:"recipient"`
	Owner     *TeamMember      `json:"owner,omitempty"`
}

// poolCenter returns the caller's organization when it keeps a pool (the
// brand center).
func (s *Service) poolCenter(ctx context.Context, q *db.Queries, orgID int64) (db.Organization, error) {
	center, err := q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return db.Organization{}, err
	}
	if center.Type != TypeCenter {
		return db.Organization{}, fmt.Errorf("%w: only the center opens recipients", ErrInvalidRequest)
	}
	return center, nil
}

// CreatePendingRecipient opens a recipient under the center (orgID) and
// puts it in the waiting pool. actor is the calling user.
func (s *Service) CreatePendingRecipient(ctx context.Context, orgID, actor int64, in RecipientInput) (PendingRecipient, error) {
	center, err := s.poolCenter(ctx, s.q, orgID)
	if err != nil {
		return PendingRecipient{}, err
	}
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = RecipientDealerCandidate
	}
	if kind != RecipientDealerCandidate && kind != RecipientRetailPoint {
		return PendingRecipient{}, fmt.Errorf("%w: kind must be dealer_candidate or retail_point", ErrInvalidRequest)
	}
	if strings.TrimSpace(in.Name) == "" {
		return PendingRecipient{}, fmt.Errorf("%w: name is required", ErrInvalidRequest)
	}
	if strings.TrimSpace(in.Phone) == "" {
		return PendingRecipient{}, fmt.Errorf("%w: phone is required", ErrInvalidRequest)
	}
	taxNo := strings.TrimSpace(in.TaxNo)
	if taxNo != "" && !taxNoRe.MatchString(taxNo) {
		return PendingRecipient{}, fmt.Errorf("%w: tax_no must be a 10 digit VKN or an 11 digit TCKN", ErrInvalidRequest)
	}
	if len(strings.TrimSpace(in.Note)) > 2000 {
		return PendingRecipient{}, fmt.Errorf("%w: note is too long", ErrInvalidRequest)
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return PendingRecipient{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	inTx := s.WithTx(tx)
	parentUUID := center.Uuid
	reg, err := inTx.RegisterOrganization(ctx, RegisterInput{
		OrganizationName: in.Name, Phone: in.Phone, City: in.City, District: in.District,
		Address: in.Address, Location: in.Location, Type: TypeDealer, ParentUUID: &parentUUID,
		InitialStatus: RecipientOrgStatus, NoTrial: true,
	}, 0)
	if err != nil {
		return PendingRecipient{}, err
	}
	q := inTx.q
	org, err := q.GetOrganizationByUUID(ctx, reg.Organization.UUID)
	if err != nil {
		return PendingRecipient{}, err
	}
	if taxNo != "" {
		p := db.SetRecipientInvoiceTaxParams{
			ID:               org.ID,
			InvoiceTaxOffice: textOrNull(in.TaxOffice),
			InvoiceLegalName: textOrNull(org.Name),
		}
		if len(taxNo) == 10 {
			p.InvoiceVkn = textOrNull(taxNo)
		} else {
			p.InvoiceTckn = textOrNull(taxNo)
		}
		if err := q.SetRecipientInvoiceTax(ctx, p); err != nil {
			return PendingRecipient{}, fmt.Errorf("recipient tax: %w", err)
		}
	}
	if _, err := q.CreatePendingRecipient(ctx, db.CreatePendingRecipientParams{
		OrganizationID: center.ID, BrandID: center.BrandID, RecipientOrgID: org.ID, Kind: kind,
		Note: textOrNull(in.Note), CreatedByUserID: pgtype.Int8{Int64: actor, Valid: actor != 0},
	}); err != nil {
		return PendingRecipient{}, fmt.Errorf("pending recipient: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return PendingRecipient{}, err
	}
	return s.pendingRecipient(ctx, s.q, center.ID, org.Uuid)
}

// ListPendingRecipients lists the center's pool (list contract).
func (s *Service) ListPendingRecipients(ctx context.Context, orgID int64, in RecipientListInput) ([]PendingRecipient, int64, error) {
	center, err := s.poolCenter(ctx, s.q, orgID)
	if err != nil {
		return nil, 0, err
	}
	p := db.ListPendingRecipientsParams{
		OrganizationID: center.ID, States: in.States, Kinds: in.Kinds, Q: textOrNull(in.Q),
		CreatedFrom: tsOrNull(in.Created.From), CreatedBefore: tsOrNull(in.Created.Before),
		SortKey: in.Sort.Key, SortDesc: in.Sort.Desc, LimitCount: in.Limit, OffsetCount: in.Offset,
	}
	rows, err := s.q.ListPendingRecipients(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountPendingRecipients(ctx, db.CountPendingRecipientsParams{
		OrganizationID: p.OrganizationID, States: p.States, Kinds: p.Kinds, Q: p.Q,
		CreatedFrom: p.CreatedFrom, CreatedBefore: p.CreatedBefore,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]PendingRecipient, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapPendingRecipient(r))
	}
	return out, total, nil
}

// ConvertPendingRecipient turns a pool or retail recipient into an active
// dealer of the center; with an owner the person becomes dealer_owner.
func (s *Service) ConvertPendingRecipient(ctx context.Context, orgID, actor int64, recipient uuid.UUID, in ConvertInput) (ConvertResult, error) {
	center, err := s.poolCenter(ctx, s.q, orgID)
	if err != nil {
		return ConvertResult{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ConvertResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	row, err := lockRecipient(ctx, q, center.ID, recipient)
	if err != nil {
		return ConvertResult{}, err
	}
	if row.State == RecipientConverted {
		return ConvertResult{}, fmt.Errorf("%w: the recipient is already a dealer", ErrConflict)
	}
	var owner *TeamMember
	org, err := q.GetOrganizationByID(ctx, row.RecipientOrgID)
	if err != nil {
		return ConvertResult{}, err
	}
	if in.Owner != nil && strings.TrimSpace(in.Owner.Phone) != "" {
		iso2, err := s.orgCountryISO2(ctx, center.ID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return ConvertResult{}, err
		}
		user, existing, err := accountUser(ctx, q, *in.Owner, iso2)
		if err != nil {
			return ConvertResult{}, err
		}
		if err := addMember(ctx, q, org, user.ID, "owner", nil); err != nil {
			if strings.Contains(err.Error(), "uq_organization_members") {
				return ConvertResult{}, fmt.Errorf("%w: the user is already a member of this organization", ErrConflict)
			}
			return ConvertResult{}, err
		}
		m := teamMember(user, existing, []string{rbac.DefaultMemberRole(TypeDealer, "owner")})
		owner = &m
		if s.out != nil {
			if err := s.out.Enqueue(ctx, tx, MemberAddedEvent(org.ID, user.ID, "owner")); err != nil {
				return ConvertResult{}, fmt.Errorf("organizations: outbox: %w", err)
			}
		}
	}
	org, err = q.SetRecipientOrganizationStatus(ctx, db.SetRecipientOrganizationStatusParams{ID: org.ID, Status: "active"})
	if err != nil {
		return ConvertResult{}, err
	}
	if _, err := q.ResolvePendingRecipient(ctx, db.ResolvePendingRecipientParams{
		ID: row.ID, State: RecipientConverted, ResolvedByUserID: pgtype.Int8{Int64: actor, Valid: actor != 0},
	}); err != nil {
		return ConvertResult{}, err
	}
	if err := s.emit(ctx, tx, events.OrganizationUpdated, org); err != nil {
		return ConvertResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ConvertResult{}, err
	}
	out, err := s.pendingRecipient(ctx, s.q, center.ID, recipient)
	if err != nil {
		return ConvertResult{}, err
	}
	return ConvertResult{Recipient: out, Owner: owner}, nil
}

// KeepRecipientAsRetail takes a pool recipient out of the pool as a retail
// / delivery point: it stays an organization without members (status
// pending) that orders can still be opened for.
func (s *Service) KeepRecipientAsRetail(ctx context.Context, orgID, actor int64, recipient uuid.UUID) (PendingRecipient, error) {
	center, err := s.poolCenter(ctx, s.q, orgID)
	if err != nil {
		return PendingRecipient{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return PendingRecipient{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	row, err := lockRecipient(ctx, q, center.ID, recipient)
	if err != nil {
		return PendingRecipient{}, err
	}
	if row.State != RecipientPending {
		return PendingRecipient{}, fmt.Errorf("%w: only a recipient in the pool can be kept as a retail point", ErrConflict)
	}
	if _, err := q.ResolvePendingRecipient(ctx, db.ResolvePendingRecipientParams{
		ID: row.ID, State: RecipientRetail, ResolvedByUserID: pgtype.Int8{Int64: actor, Valid: actor != 0},
	}); err != nil {
		return PendingRecipient{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PendingRecipient{}, err
	}
	return s.pendingRecipient(ctx, s.q, center.ID, recipient)
}

func lockRecipient(ctx context.Context, q *db.Queries, centerID int64, recipient uuid.UUID) (db.PendingRecipient, error) {
	row, err := q.GetPendingRecipientForUpdate(ctx, db.GetPendingRecipientForUpdateParams{
		OrganizationID: centerID, RecipientUuid: recipient,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PendingRecipient{}, ErrNotFound
	}
	return row, err
}

// pendingRecipient reads one pool row of the center by recipient uuid.
func (s *Service) pendingRecipient(ctx context.Context, q *db.Queries, centerID int64, recipient uuid.UUID) (PendingRecipient, error) {
	org, err := q.GetOrganizationByUUID(ctx, recipient)
	if errors.Is(err, pgx.ErrNoRows) {
		return PendingRecipient{}, ErrNotFound
	}
	if err != nil {
		return PendingRecipient{}, err
	}
	row, err := q.GetPendingRecipientByOrg(ctx, org.ID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.OrganizationID != centerID) {
		return PendingRecipient{}, ErrNotFound
	}
	if err != nil {
		return PendingRecipient{}, err
	}
	n, err := q.CountOrdersOfBuyer(ctx, org.ID)
	if err != nil {
		return PendingRecipient{}, err
	}
	return mapPendingRecipient(db.ListPendingRecipientsRow{
		ID: row.ID, Kind: row.Kind, State: row.State, Note: row.Note, CreatedAt: row.CreatedAt, ResolvedAt: row.ResolvedAt,
		RecipientUuid: org.Uuid, RecipientSlug: org.Slug, RecipientName: org.Name, RecipientPhone: org.Phone,
		RecipientCity: org.City, RecipientDistrict: org.District, RecipientAddress: org.Address, RecipientStatus: org.Status,
		InvoiceVkn: org.InvoiceVkn, InvoiceTckn: org.InvoiceTckn, InvoiceTaxOffice: org.InvoiceTaxOffice, OrdersCount: n,
	}), nil
}

func mapPendingRecipient(r db.ListPendingRecipientsRow) PendingRecipient {
	out := PendingRecipient{
		UUID: r.RecipientUuid, Slug: r.RecipientSlug, Name: r.RecipientName, Phone: r.RecipientPhone,
		City: r.RecipientCity, District: r.RecipientDistrict, Address: r.RecipientAddress,
		Kind: r.Kind, State: r.State, OrgStatus: r.RecipientStatus, OrdersCount: r.OrdersCount,
		CreatedAt: r.CreatedAt.Time, TaxNo: textPtr(r.InvoiceVkn), TaxOffice: textPtr(r.InvoiceTaxOffice),
		Note: textPtr(r.Note),
	}
	if out.TaxNo == nil {
		out.TaxNo = textPtr(r.InvoiceTckn)
	}
	if r.ResolvedAt.Valid {
		t := r.ResolvedAt.Time
		out.ResolvedAt = &t
	}
	return out
}

func textOrNull(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	return pgtype.Text{String: v, Valid: v != ""}
}

func tsOrNull(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
