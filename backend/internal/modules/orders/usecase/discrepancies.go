package usecase

// Receipt discrepancies (TEC-1227).
//
// The buyer receives an order unit by unit (scanning) or all at once. Units
// it did not get or got damaged are reported with the received step
// (TransitionInput.Discrepancies, one per shipped unit, with a reason):
//
//   - missing: the unit gets no received movement and stays in transit
//     until the record is resolved: delivered (the buyer found it; received
//     is posted now) or returned (the seller took it back;
//     order_cancel_restore puts it back to the order_out's source).
//   - damaged: the unit is received like the others; the record carries the
//     reason and photos and is closed by either side (a return goes
//     through the return flow).
//
// The order is received with open records ("received + open discrepancy");
// the seller's staff (orders.approve or orders.ship) are notified
// (orders.receipt_discrepancy). The books still run on receipt with the
// order total (a credit for missing goods is not part of this step). No
// record carries a price.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Discrepancy kinds, statuses and resolutions
// (order_receipt_discrepancies, migration 000185).
const (
	DiscrepancyMissing = "missing"
	DiscrepancyDamaged = "damaged"

	DiscrepancyOpen     = "open"
	DiscrepancyResolved = "resolved"

	ResolutionDelivered = "delivered"
	ResolutionReturned  = "returned"
	ResolutionClosed    = "closed"
)

// Discrepancy limits.
const (
	MaxDiscrepancyReason   = 500
	MaxDiscrepancyPhotos   = 5
	MaxDiscrepancyPhotoLen = 10 << 20
)

// Discrepancy activity actions (timeline).
const (
	ActionDiscrepancyReported = "order.discrepancy_reported"
	ActionDiscrepancyResolved = "order.discrepancy_resolved"
)

// Discrepancy errors.
var (
	ErrDiscrepancyNotFound = errors.New("orders: discrepancy not found")
	// ErrDiscrepancyClosed: the record is already resolved.
	ErrDiscrepancyClosed = errors.New("orders: discrepancy is already resolved")
	// ErrDiscrepancyPhotoLimit: at most MaxDiscrepancyPhotos photos.
	ErrDiscrepancyPhotoLimit = errors.New("orders: a discrepancy has at most 5 photos")
)

// DiscrepancyInput reports one shipped unit at receipt.
type DiscrepancyInput struct {
	UnitUUID string `json:"unit_uuid"`
	Kind     string `json:"kind"`
	Reason   string `json:"reason"`
}

// ResolveDiscrepancyInput closes a record.
type ResolveDiscrepancyInput struct {
	Resolution string
	Note       *string
}

// DiscrepancyView is one receipt discrepancy. It carries no price: the
// warehouse roles read it too.
type DiscrepancyView struct {
	UUID           uuid.UUID  `json:"uuid"`
	Kind           string     `json:"kind"`
	Status         string     `json:"status"`
	Reason         string     `json:"reason"`
	Resolution     *string    `json:"resolution"`
	ResolutionNote *string    `json:"resolution_note"`
	UnitUUID       uuid.UUID  `json:"unit_uuid"`
	Barcode        string     `json:"barcode"`
	ProductName    string     `json:"product_name"`
	ProductSKU     string     `json:"product_sku"`
	Quantity       *int32     `json:"quantity"`
	Meters         *string    `json:"meters"`
	PhotoCount     int        `json:"photo_count"`
	CreatedBy      *UserRef   `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	ResolvedBy     *UserRef   `json:"resolved_by"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	// CanResolveAs: the resolutions the caller may choose now.
	CanResolveAs []string `json:"can_resolve_as"`
	// CanAddPhoto: the caller (the buyer) may attach a photo now.
	CanAddPhoto bool `json:"can_add_photo"`
}

type reportedUnit struct {
	a      db.ListOrderItemUnitsByOrderRow
	kind   string
	reason string
}

// resolveDiscrepancies matches the reported units to the shipped
// assignments of the order. At least one unit must be received.
func resolveDiscrepancies(units []db.ListOrderItemUnitsByOrderRow, in []DiscrepancyInput) (map[int64]reportedUnit, error) {
	out := map[int64]reportedUnit{}
	if len(in) == 0 {
		return out, nil
	}
	byUnit := make(map[uuid.UUID]db.ListOrderItemUnitsByOrderRow, len(units))
	for _, a := range units {
		byUnit[a.UnitUuid] = a
	}
	missing := 0
	for i, d := range in {
		field := fmt.Sprintf("discrepancies[%d]", i)
		id, err := uuid.Parse(strings.TrimSpace(d.UnitUUID))
		if err != nil {
			return nil, invalid(field+".unit_uuid", "must be a UUID")
		}
		a, ok := byUnit[id]
		if !ok {
			return nil, invalid(field+".unit_uuid", "the unit is not on this order")
		}
		if _, dup := out[a.ID]; dup {
			return nil, invalid(field+".unit_uuid", "the unit is reported twice")
		}
		kind := strings.TrimSpace(d.Kind)
		if kind != DiscrepancyMissing && kind != DiscrepancyDamaged {
			return nil, invalid(field+".kind", "must be missing or damaged")
		}
		reason := strings.TrimSpace(d.Reason)
		if reason == "" {
			return nil, invalid(field+".reason", "is required")
		}
		if utf8.RuneCountInString(reason) > MaxDiscrepancyReason {
			return nil, invalid(field+".reason", fmt.Sprintf("at most %d characters", MaxDiscrepancyReason))
		}
		if kind == DiscrepancyMissing {
			missing++
		}
		out[a.ID] = reportedUnit{a: a, kind: kind, reason: reason}
	}
	if missing >= len(units) {
		return nil, invalid("discrepancies", "at least one unit must be received; cancel the order if nothing arrived")
	}
	return out, nil
}

// recordDiscrepancies writes the records of a receipt, the timeline steps
// and the seller notification.
func (s *Service) recordDiscrepancies(ctx context.Context, q *db.Queries, tx pgx.Tx, c Caller, o db.Order,
	reported map[int64]reportedUnit, units []db.ListOrderItemUnitsByOrderRow) (map[string]any, error) {
	if len(reported) == 0 {
		return nil, nil
	}
	missing, damaged := 0, 0
	for _, a := range units { // assignment order: stable records
		r, ok := reported[a.ID]
		if !ok {
			continue
		}
		if _, err := q.CreateOrderReceiptDiscrepancy(ctx, db.CreateOrderReceiptDiscrepancyParams{
			OrderID: o.ID, OrderItemUnitID: a.ID, UnitID: a.UnitID, Kind: r.kind, Reason: r.reason,
			CreatedByUserID: c.actor(),
		}); err != nil {
			return nil, fmt.Errorf("orders: discrepancy: %w", err)
		}
		if r.kind == DiscrepancyMissing {
			missing++
		} else {
			damaged++
		}
		p := activityPayload{UnitUUID: a.UnitUuid.String(), Barcode: a.Barcode, Kind: r.kind, Reason: r.reason}
		if err := s.recordActivity(ctx, q, c, o, ActionDiscrepancyReported, p); err != nil {
			return nil, err
		}
	}
	notify, err := s.sellerStaff(ctx, q, o, c)
	if err != nil {
		return nil, err
	}
	buyer, err := q.GetOrganizationByID(ctx, o.BuyerOrgID)
	if err != nil {
		return nil, fmt.Errorf("orders: buyer: %w", err)
	}
	if err := s.emitWith(ctx, tx, events.OrdersReceiptDiscrepancy, o, "", c, map[string]any{
		"notify_user_ids": notify, "buyer_name": buyer.Name,
		"missing_count": missing, "damaged_count": damaged,
	}); err != nil {
		return nil, err
	}
	return map[string]any{"missing": missing, "damaged": damaged}, nil
}

// sellerStaff are the seller's members holding orders.approve or
// orders.ship, the actor left out.
func (s *Service) sellerStaff(ctx context.Context, q *db.Queries, o db.Order, c Caller) ([]int64, error) {
	seen := map[int64]bool{}
	out := []int64{}
	for _, perm := range []string{rbac.PermOrdersApprove, rbac.PermOrdersShip} {
		ids, err := q.ListTransferNotifyUserIDs(ctx, db.ListTransferNotifyUserIDsParams{
			OrganizationID: o.SellerOrgID, PermissionSlug: perm,
		})
		if err != nil {
			return nil, fmt.Errorf("orders: recipients: %w", err)
		}
		for _, id := range ids {
			if id != c.Principal.UserInternal && !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out, nil
}

// receivedMovement is the received movement of one shipped assignment.
func receivedMovement(c Caller, o db.Order, a db.ListOrderItemUnitsByOrderRow) ledger.Movement {
	buyer := ledger.Owner{Type: ledger.OwnerOrganization, ID: o.BuyerOrgID, OrgID: o.BuyerOrgID}
	m := ledger.Movement{
		Type: ledger.TypeReceived, UnitID: a.UnitID, To: &buyer,
		Source: LedgerSource, RefType: LedgerRefType, RefID: a.ID, ActorUserID: actorOf(c),
		Metadata: map[string]any{"order_uuid": o.Uuid.String(), "order_no": o.OrderNo},
	}
	if a.UnitKind == ledger.KindFixed {
		m.Quantity = a.Quantity.Int32
	}
	return m
}

// resolutionsFor lists what the caller may close an open record with.
func resolutionsFor(c Caller, o db.Order, kind string) []string {
	out := []string{}
	if o.Status != StatusReceived {
		return out
	}
	party := partyOf(c, o)
	buyer := party == PartyBuyer && c.can(rbac.PermOrdersReceive)
	seller := party == PartySeller && c.can(rbac.PermOrdersShip)
	switch kind {
	case DiscrepancyMissing:
		if buyer {
			out = append(out, ResolutionDelivered)
		}
		if seller {
			out = append(out, ResolutionReturned)
		}
	case DiscrepancyDamaged:
		if buyer || seller {
			out = append(out, ResolutionClosed)
		}
	}
	return out
}

// ResolveDiscrepancy closes an open record: a missing unit the buyer found
// (delivered: received is posted), a missing unit the seller took back
// (returned: order_cancel_restore) or a damaged unit's record (closed).
func (s *Service) ResolveDiscrepancy(ctx context.Context, c Caller, orderUUID, discUUID uuid.UUID, in ResolveDiscrepancyInput) (OrderView, error) {
	res := strings.TrimSpace(in.Resolution)
	if in.Note != nil && utf8.RuneCountInString(strings.TrimSpace(*in.Note)) > MaxDiscrepancyReason {
		return OrderView{}, invalid("note", fmt.Sprintf("at most %d characters", MaxDiscrepancyReason))
	}
	var result db.Order
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		o, err := s.lockVisible(ctx, q, c, orderUUID)
		if err != nil {
			return err
		}
		d, err := q.LockOrderReceiptDiscrepancy(ctx, db.LockOrderReceiptDiscrepancyParams{Uuid: discUUID, OrderID: o.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDiscrepancyNotFound
		}
		if err != nil {
			return fmt.Errorf("orders: discrepancy: %w", err)
		}
		if d.Status != DiscrepancyOpen {
			return ErrDiscrepancyClosed
		}
		allowed := resolutionsFor(c, o, d.Kind)
		ok := false
		for _, r := range allowed {
			ok = ok || r == res
		}
		if !ok {
			if len(allowed) == 0 {
				return ErrForbidden
			}
			return invalid("resolution", "must be one of "+strings.Join(allowed, ", "))
		}
		if d.Kind == DiscrepancyMissing {
			units, err := q.ListOrderItemUnitsByOrder(ctx, o.ID)
			if err != nil {
				return fmt.Errorf("orders: shipped units: %w", err)
			}
			var a *db.ListOrderItemUnitsByOrderRow
			for i := range units {
				if units[i].ID == d.OrderItemUnitID {
					a = &units[i]
				}
			}
			if a == nil || !a.MovementID.Valid {
				return fmt.Errorf("%w: the unit was not shipped", ErrStockUnavailable)
			}
			if res == ResolutionDelivered {
				if _, err := s.ledger.Post(ctx, tx, receivedMovement(c, o, *a)); err != nil {
					return stockErr("received", a.Barcode, err)
				}
			} else if err := s.restoreOne(ctx, q, tx, c, o, *a, "missing unit returned to the seller"); err != nil {
				return err
			}
		}
		if _, err := q.ResolveOrderReceiptDiscrepancy(ctx, db.ResolveOrderReceiptDiscrepancyParams{
			ID: d.ID, Resolution: pgtype.Text{String: res, Valid: true}, ResolutionNote: textOrNull(in.Note), ResolvedByUserID: c.actor(),
		}); err != nil {
			return fmt.Errorf("orders: resolve discrepancy: %w", err)
		}
		u, err := q.GetUnit(ctx, d.UnitID)
		if err != nil {
			return fmt.Errorf("orders: unit: %w", err)
		}
		p := activityPayload{UnitUUID: u.Uuid.String(), Barcode: u.Barcode, Kind: d.Kind, Resolution: res}
		if in.Note != nil {
			p.Reason = strings.TrimSpace(*in.Note)
		}
		if err := s.recordActivity(ctx, q, c, o, ActionDiscrepancyResolved, p); err != nil {
			return err
		}
		result = o
		return s.emit(ctx, tx, events.OrdersUpdated, o, o.Status, c)
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.view(ctx, s.q, c, result)
}

// restoreOne posts the order_cancel_restore of one shipped assignment.
func (s *Service) restoreOne(ctx context.Context, q *db.Queries, tx pgx.Tx, c Caller, o db.Order,
	a db.ListOrderItemUnitsByOrderRow, reason string) error {
	m := ledger.Movement{
		Type: ledger.TypeOrderCancelRestore, UnitID: a.UnitID,
		Source: LedgerSource, RefType: LedgerRefType, RefID: a.ID, ActorUserID: actorOf(c),
		Reason:   reason,
		Metadata: map[string]any{"order_uuid": o.Uuid.String(), "order_no": o.OrderNo},
	}
	if a.UnitKind == ledger.KindFixed {
		out, err := q.GetStockMovement(ctx, a.MovementID.Int64)
		if err != nil {
			return fmt.Errorf("orders: order_out of %s: %w", a.Barcode, err)
		}
		if !out.FromOwnerType.Valid || !out.FromOwnerID.Valid {
			return fmt.Errorf("%w: order_out of %s has no source", ErrStockUnavailable, a.Barcode)
		}
		src := ledger.Owner{Type: ledger.OwnerType(out.FromOwnerType.String), ID: out.FromOwnerID.Int64, OrgID: out.OrganizationID}
		m.To, m.Quantity = &src, a.Quantity.Int32
	}
	if _, err := s.ledger.Post(ctx, tx, m); err != nil {
		return stockErr("order_cancel_restore", a.Barcode, err)
	}
	return nil
}

// DiscrepancyPhotoObjectKey is the storage key of a discrepancy photo.
func DiscrepancyPhotoObjectKey(orgUUID, orderUUID, discUUID, photoUUID uuid.UUID, ext string) string {
	return fmt.Sprintf("orgs/%s/orders/%s/discrepancies/%s/%s.%s", orgUUID, orderUUID, discUUID, photoUUID, ext)
}

// photoTypes are the accepted photo content types.
var photoTypes = map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}

// AddDiscrepancyPhoto stores a photo of an open record (the buyer,
// orders.receive; at most MaxDiscrepancyPhotos).
func (s *Service) AddDiscrepancyPhoto(ctx context.Context, c Caller, orderUUID, discUUID uuid.UUID, in ReceiptInput) (OrderView, error) {
	if s.store == nil {
		return OrderView{}, ErrDocumentUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(in.Body, MaxDiscrepancyPhotoLen+1))
	if err != nil {
		return OrderView{}, invalid("file", "the file could not be read")
	}
	if len(raw) == 0 || len(raw) > MaxDiscrepancyPhotoLen {
		return OrderView{}, &ValidationError{Field: "file", Code: CodeReceiptFileInvalid, Message: "the file is empty or too large"}
	}
	ct := http.DetectContentType(raw)
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}
	ext, ok := photoTypes[ct]
	if !ok {
		return OrderView{}, &ValidationError{Field: "file", Code: CodeReceiptFileInvalid, Message: "JPEG, PNG or WebP only"}
	}
	var result db.Order
	var key string
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		o, err := s.lockVisible(ctx, q, c, orderUUID)
		if err != nil {
			return err
		}
		if partyOf(c, o) != PartyBuyer || !c.can(rbac.PermOrdersReceive) {
			return ErrForbidden
		}
		d, err := q.LockOrderReceiptDiscrepancy(ctx, db.LockOrderReceiptDiscrepancyParams{Uuid: discUUID, OrderID: o.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDiscrepancyNotFound
		}
		if err != nil {
			return fmt.Errorf("orders: discrepancy: %w", err)
		}
		if d.Status != DiscrepancyOpen {
			return ErrDiscrepancyClosed
		}
		if len(d.PhotoKeys) >= MaxDiscrepancyPhotos {
			return ErrDiscrepancyPhotoLimit
		}
		seller, err := q.GetOrganizationByID(ctx, o.SellerOrgID)
		if err != nil {
			return fmt.Errorf("orders: seller: %w", err)
		}
		key = DiscrepancyPhotoObjectKey(seller.Uuid, o.Uuid, d.Uuid, uuid.New(), ext)
		if err := s.store.Upload(ctx, storage.File{
			Body: bytes.NewReader(raw), Size: int64(len(raw)), ContentType: ct,
			Filename: "discrepancy-" + o.OrderNo + "." + ext,
			Metadata: map[string]string{"order_uuid": o.Uuid.String(), "discrepancy_uuid": d.Uuid.String()},
		}, key); err != nil {
			return fmt.Errorf("orders: upload photo: %w", err)
		}
		if _, err := q.AddOrderReceiptDiscrepancyPhoto(ctx, db.AddOrderReceiptDiscrepancyPhotoParams{ID: d.ID, PhotoKey: key}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrDiscrepancyPhotoLimit
			}
			return fmt.Errorf("orders: photo key: %w", err)
		}
		result = o
		return nil
	})
	if err != nil {
		if key != "" {
			_ = s.store.Delete(context.WithoutCancel(ctx), key)
		}
		return OrderView{}, err
	}
	return s.view(ctx, s.q, c, result)
}

// OpenDiscrepancyPhoto streams photo index (0-based) of a record of an
// order the caller sees (both sides).
func (s *Service) OpenDiscrepancyPhoto(ctx context.Context, c Caller, orderUUID, discUUID uuid.UUID, index int) (DocumentFile, error) {
	o, err := s.q.GetOrderByUUID(ctx, db.GetOrderByUUIDParams{Uuid: orderUUID, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return DocumentFile{}, ErrNotFound
	}
	if err != nil {
		return DocumentFile{}, fmt.Errorf("orders: get: %w", err)
	}
	if !visible(c, o) {
		return DocumentFile{}, ErrNotFound
	}
	d, err := s.q.LockOrderReceiptDiscrepancy(ctx, db.LockOrderReceiptDiscrepancyParams{Uuid: discUUID, OrderID: o.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return DocumentFile{}, ErrDiscrepancyNotFound
	}
	if err != nil {
		return DocumentFile{}, fmt.Errorf("orders: discrepancy: %w", err)
	}
	if index < 0 || index >= len(d.PhotoKeys) {
		return DocumentFile{}, ErrDocumentNotFound
	}
	if s.store == nil {
		return DocumentFile{}, ErrDocumentUnavailable
	}
	key := d.PhotoKeys[index]
	rc, size, err := s.store.Download(ctx, key)
	if err != nil {
		return DocumentFile{}, fmt.Errorf("orders: download photo: %w", err)
	}
	ext := key[strings.LastIndex(key, ".")+1:]
	ct := "application/octet-stream"
	for t, e := range photoTypes {
		if e == ext {
			ct = t
		}
	}
	return DocumentFile{Body: rc, Size: size, ContentType: ct, Filename: fmt.Sprintf("discrepancy-%s-%d.%s", o.OrderNo, index+1, ext)}, nil
}

// withDiscrepancies fills the receipt records of an order (detail only).
func (s *Service) withDiscrepancies(ctx context.Context, q *db.Queries, c Caller, o db.Order, v *OrderView) error {
	v.Discrepancies = []DiscrepancyView{}
	rows, err := q.ListOrderReceiptDiscrepancies(ctx, o.ID)
	if err != nil {
		return fmt.Errorf("orders: discrepancies: %w", err)
	}
	buyerCanPhoto := partyOf(c, o) == PartyBuyer && c.can(rbac.PermOrdersReceive)
	for _, r := range rows {
		dv := DiscrepancyView{
			UUID: r.Uuid, Kind: r.Kind, Status: r.Status, Reason: r.Reason,
			Resolution:     textPtr(pgtype.Text{String: r.Resolution.String, Valid: r.Resolution.Valid}),
			ResolutionNote: textPtr(r.ResolutionNote), UnitUUID: r.UnitUuid, Barcode: r.Barcode,
			ProductName: r.ProductName, ProductSKU: r.ProductSku, Meters: numericTextPtr(r.Meters, 2),
			PhotoCount: len(r.PhotoKeys), CreatedAt: r.CreatedAt.Time, ResolvedAt: tsPtr(r.ResolvedAt),
			CreatedBy:    userRef(r.CreatedByUuid, r.CreatedByName, r.CreatedBySurname),
			ResolvedBy:   userRef(r.ResolvedByUuid, r.ResolvedByName, r.ResolvedBySurname),
			CanResolveAs: []string{},
		}
		if r.Quantity.Valid {
			n := r.Quantity.Int32
			dv.Quantity = &n
		}
		if r.Status == DiscrepancyOpen {
			dv.CanResolveAs = resolutionsFor(c, o, r.Kind)
			dv.CanAddPhoto = buyerCanPhoto && len(r.PhotoKeys) < MaxDiscrepancyPhotos
			v.OpenDiscrepancies++
		}
		v.Discrepancies = append(v.Discrepancies, dv)
	}
	return nil
}
