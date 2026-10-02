package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// OrgRef names a party of a request.
type OrgRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Type string    `json:"type"`
}

// ProductRef names a line's product.
type ProductRef struct {
	UUID     uuid.UUID `json:"uuid"`
	SKU      string    `json:"sku"`
	Name     string    `json:"name"`
	UnitType string    `json:"unit_type"`
}

// ItemView is one unit of a request. unit_price/line_total are the giver's
// purchase price frozen at approval (null before, or without a price).
type ItemView struct {
	UUID      uuid.UUID  `json:"uuid"`
	UnitUUID  uuid.UUID  `json:"unit_uuid"`
	Barcode   string     `json:"barcode"`
	UnitKind  string     `json:"unit_kind"`
	Product   ProductRef `json:"product"`
	Quantity  *int32     `json:"quantity"`
	Meters    *string    `json:"meters"`
	UnitPrice *string    `json:"unit_price"`
	LineTotal *string    `json:"line_total"`
	Shipped   bool       `json:"shipped"`
	Received  bool       `json:"received"`
	Restored  bool       `json:"restored"`
}

// TransferView is a request as the API returns it. Role is the active
// organization's side (sender, receiver, parent).
type TransferView struct {
	UUID         uuid.UUID  `json:"uuid"`
	TransferNo   string     `json:"transfer_no"`
	Status       string     `json:"status"`
	Role         string     `json:"role"`
	Sender       OrgRef     `json:"sender"`
	Receiver     OrgRef     `json:"receiver"`
	Parent       OrgRef     `json:"parent"`
	Currency     string     `json:"currency"`
	Total        *string    `json:"total"`
	Note         *string    `json:"note"`
	DecisionNote *string    `json:"decision_note"`
	CancelReason *string    `json:"cancel_reason"`
	ItemCount    int64      `json:"item_count"`
	DecidedAt    *time.Time `json:"decided_at"`
	ShippedAt    *time.Time `json:"shipped_at"`
	ReceivedAt   *time.Time `json:"received_at"`
	CancelledAt  *time.Time `json:"cancelled_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// AvailableTransitions are the statuses the caller may move the request to.
	AvailableTransitions []string   `json:"available_transitions"`
	Items                []ItemView `json:"items,omitempty"`
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

type orgCache map[int64]OrgRef

func (m orgCache) get(ctx context.Context, q *db.Queries, id int64) (OrgRef, error) {
	if r, ok := m[id]; ok {
		return r, nil
	}
	o, err := q.GetOrganizationByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		m[id] = OrgRef{}
		return OrgRef{}, nil
	}
	if err != nil {
		return OrgRef{}, fmt.Errorf("transfers: organization: %w", err)
	}
	r := OrgRef{UUID: o.Uuid, Name: o.Name, Type: o.Type}
	m[id] = r
	return r, nil
}

func (s *Service) summary(ctx context.Context, q *db.Queries, c Caller, r db.StockTransferRequest, orgs orgCache) (TransferView, error) {
	sender, err := orgs.get(ctx, q, r.FromOrgID)
	if err != nil {
		return TransferView{}, err
	}
	receiver, err := orgs.get(ctx, q, r.ToOrgID)
	if err != nil {
		return TransferView{}, err
	}
	parent, err := orgs.get(ctx, q, r.ApproverOrgID)
	if err != nil {
		return TransferView{}, err
	}
	n, err := q.CountTransferRequestItems(ctx, r.ID)
	if err != nil {
		return TransferView{}, fmt.Errorf("transfers: item count: %w", err)
	}
	party := partyOf(c, r)
	return TransferView{
		UUID: r.Uuid, TransferNo: r.TransferNo, Status: r.Status, Role: string(party),
		Sender: sender, Receiver: receiver, Parent: parent,
		Currency: strings.TrimSpace(r.Currency), Total: numericTextPtr(r.Total, 2),
		Note: textPtr(r.Reason), DecisionNote: textPtr(r.DecisionNote), CancelReason: textPtr(r.CancelReason),
		ItemCount: n, DecidedAt: tsPtr(r.DecidedAt), ShippedAt: tsPtr(r.ShippedAt),
		ReceivedAt: tsPtr(r.ReceivedAt), CancelledAt: tsPtr(r.CancelledAt),
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		AvailableTransitions: availableTransitions(r.Status, party, c.can),
	}, nil
}

// view is the full request: summary and units.
func (s *Service) view(ctx context.Context, q *db.Queries, c Caller, r db.StockTransferRequest) (TransferView, error) {
	v, err := s.summary(ctx, q, c, r, orgCache{})
	if err != nil {
		return TransferView{}, err
	}
	rows, err := q.ListTransferRequestItems(ctx, r.ID)
	if err != nil {
		return TransferView{}, fmt.Errorf("transfers: items: %w", err)
	}
	v.Items = make([]ItemView, 0, len(rows))
	for _, it := range rows {
		iv := ItemView{
			UUID: it.Uuid, UnitUUID: it.UnitUuid, Barcode: it.Barcode, UnitKind: it.UnitKind,
			Product: ProductRef{UUID: it.ProductUuid, SKU: it.ProductSku, Name: it.ProductName, UnitType: it.ProductUnitType},
			Meters:  numericTextPtr(it.Meters, 2), UnitPrice: numericTextPtr(it.UnitPrice, 4),
			LineTotal: numericTextPtr(it.LineTotal, 2),
			Shipped:   it.OutMovementID.Valid, Received: it.InMovementID.Valid, Restored: it.RestoreMovementID.Valid,
		}
		if it.Quantity.Valid {
			qv := it.Quantity.Int32
			iv.Quantity = &qv
		}
		v.Items = append(v.Items, iv)
	}
	return v, nil
}

// Get returns one request the active organization is a party of (else 404).
func (s *Service) Get(ctx context.Context, c Caller, id uuid.UUID) (TransferView, error) {
	r, err := s.q.GetTransferRequestByUUID(ctx, db.GetTransferRequestByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return TransferView{}, ErrNotFound
	}
	if err != nil {
		return TransferView{}, fmt.Errorf("transfers: get: %w", err)
	}
	if !visible(c, r) {
		return TransferView{}, ErrNotFound
	}
	return s.view(ctx, s.q, c, r)
}

// Directions of the list.
const (
	DirectionAll      = ""
	DirectionOutgoing = "outgoing"
	DirectionIncoming = "incoming"
	DirectionApproval = "approval"
)

// ListFilter narrows the request list.
type ListFilter struct {
	Direction string
	Status    string
	Limit     int32
	Offset    int32
}

// List returns the requests of the active organization: as the giver
// (outgoing), the receiver (incoming) or the common parent (approval).
func (s *Service) List(ctx context.Context, c Caller, f ListFilter) ([]TransferView, int64, error) {
	if c.Org.InternalID == 0 {
		return nil, 0, ErrForbidden
	}
	dir := strings.TrimSpace(f.Direction)
	switch dir {
	case DirectionAll, DirectionOutgoing, DirectionIncoming, DirectionApproval:
	default:
		return nil, 0, invalid("direction", "must be outgoing, incoming or approval")
	}
	status := pgtype.Text{}
	if st := strings.TrimSpace(f.Status); st != "" {
		if !IsStatus(st) {
			return nil, 0, invalid("status", "unknown transfer status")
		}
		status = pgtype.Text{String: st, Valid: true}
	}
	rows, err := s.q.ListTransferRequestsForOrg(ctx, db.ListTransferRequestsForOrgParams{
		BrandID: c.Org.BrandID, Direction: dir, OrgID: c.Org.InternalID, Status: status,
		RowLimit: f.Limit, RowOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("transfers: list: %w", err)
	}
	total, err := s.q.CountTransferRequestsForOrg(ctx, db.CountTransferRequestsForOrgParams{
		BrandID: c.Org.BrandID, Direction: dir, OrgID: c.Org.InternalID, Status: status,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("transfers: count: %w", err)
	}
	orgs := orgCache{}
	out := make([]TransferView, 0, len(rows))
	for _, r := range rows {
		v, err := s.summary(ctx, s.q, c, r, orgs)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, nil
}
