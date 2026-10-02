package usecase

// TEC-191 (F1-06g): panel and portal warranty list and detail, and the
// center void.
//
// Scope (warranties.read, the same scopes as services.read): center =
// brand, distributor = its subtree, dealer = its organization (managed),
// own / assigned = services the caller created, customer / fleet (portal) =
// warranties the user holds. Everything is bound to the domain brand (K20);
// a warranty outside the scope answers 404.
//
// Void (warranties.void, center roles only, step-up at the route): active
// or expired warranties become void with a mandatory reason, in one
// transaction with a warranty.voided outbox event and an activity log row.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Warranty statuses.
const (
	StatusActive  = "active"
	StatusExpired = "expired"
	StatusVoid    = "void"
)

// Limits of the list filters and the void reason.
const (
	MaxQueryLength     = 100
	MaxDaysLeft        = 3650
	MinVoidReasonRunes = 3
	MaxVoidReasonRunes = 500
)

// ActionWarrantyVoided is the activity log action of a void.
const ActionWarrantyVoided = "warranty.voided"

// anonymizedNameKey is the catalog label of an anonymized holder (K19).
const anonymizedNameKey = "customers.anonymized_name"

// Errors of the list / detail / void use cases.
var (
	ErrWarrantyNotFound = errors.New("warranty: not found")
	ErrAlreadyVoid      = errors.New("warranty: already void")
)

// ValidationError is a bad filter or body field.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalidField(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// IsStatus reports whether s is a warranty status.
func IsStatus(s string) bool { return s == StatusActive || s == StatusExpired || s == StatusVoid }

// Caller is a panel caller: principal, active organization and the
// resolved warranties.read (or warranties.void) scope.
type Caller struct {
	Principal authctx.Principal
	Org       orgctx.Scope
	Filter    scopefilter.Filter
}

// ListFilter are the GET /v1/warranties filters. DaysLeftMin / DaysLeftMax
// bound end_at to (now + min days, now + max days].
type ListFilter struct {
	Status      string
	Q           string
	ProductUUID string
	VehicleUUID string
	DaysLeftMin *int
	DaysLeftMax *int
	Limit       int32
	Offset      int32
}

// NamedRef names a related record.
type NamedRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// ProductRefView is the covered product.
type ProductRefView struct {
	UUID uuid.UUID `json:"uuid"`
	SKU  string    `json:"sku"`
	Name string    `json:"name"`
}

// ServiceRefView is the service that issued the warranty.
type ServiceRefView struct {
	UUID      uuid.UUID `json:"uuid"`
	ServiceNo string    `json:"service_no"`
}

// OrganizationRefView is the organization that performed the service.
type OrganizationRefView struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Type string    `json:"type"`
}

// VehicleRefView is the covered vehicle (plate: the service snapshot, then
// the vehicle).
type VehicleRefView struct {
	UUID      uuid.UUID `json:"uuid"`
	BrandName string    `json:"brand_name"`
	ModelName string    `json:"model_name"`
	ModelYear *int16    `json:"model_year"`
	Plate     *string   `json:"plate"`
}

// HolderRefView is the warranty holder (panel only; masked when anonymized).
type HolderRefView struct {
	UUID       uuid.UUID `json:"uuid"`
	Name       string    `json:"name"`
	Surname    string    `json:"surname"`
	Anonymized bool      `json:"anonymized"`
}

// WarrantyListView is a warranty as the list and detail endpoints return it.
type WarrantyListView struct {
	UUID         uuid.UUID           `json:"uuid"`
	PublicCode   string              `json:"public_code"`
	Status       string              `json:"status"`
	ItemKind     string              `json:"item_kind"`
	StartAt      time.Time           `json:"start_at"`
	EndAt        time.Time           `json:"end_at"`
	ExpiredAt    *time.Time          `json:"expired_at"`
	VoidedAt     *time.Time          `json:"voided_at"`
	VoidReason   *string             `json:"void_reason"`
	CreatedAt    time.Time           `json:"created_at"`
	Product      ProductRefView      `json:"product"`
	Service      ServiceRefView      `json:"service"`
	Organization OrganizationRefView `json:"organization"`
	Vehicle      VehicleRefView      `json:"vehicle"`
	Holder       *HolderRefView      `json:"holder,omitempty"`
	// CanVoid: the caller holds warranties.void for this warranty and the
	// status allows it (panel only).
	CanVoid bool `json:"can_void"`
}

// Reader serves the list, detail and void use cases.
type Reader struct {
	pool          TxBeginner
	q             *db.Queries
	out           outbox.Enqueuer
	verifyBaseURL string
	now           func() time.Time
}

// NewReader builds the reader; verifyBaseURL is the public origin of the
// /garanti/{public_code} link written into the void event.
func NewReader(pool TxBeginner, q *db.Queries, out outbox.Enqueuer, verifyBaseURL string) *Reader {
	return &Reader{pool: pool, q: q, out: out, verifyBaseURL: verifyBaseURL, now: time.Now}
}

// WithClock replaces the clock (tests).
func (r *Reader) WithClock(now func() time.Time) *Reader {
	r.now = now
	return r
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := strings.TrimSpace(t.String)
	if v == "" {
		return nil
	}
	return &v
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// scopeParams are the scope arguments shared by the list and count queries.
type scopeParams struct {
	brandID          int64
	orgIDs           []int64
	holderUserID     pgtype.Int8
	serviceCreatedBy pgtype.Int8
}

// panelScope turns the resolved warranties.read filter into query arguments.
func panelScope(c Caller) scopeParams {
	sp := scopeParams{brandID: c.Org.BrandID, orgIDs: c.Filter.OrgIDsArg()}
	switch c.Filter.Scope {
	case rbac.ScopeOwn, rbac.ScopeAssigned:
		sp.serviceCreatedBy = pgtype.Int8{Int64: c.Filter.UserID, Valid: true}
	case rbac.ScopeCustomer:
		// A customer scope in a panel session sees its own warranties only.
		sp.orgIDs = nil
		sp.holderUserID = pgtype.Int8{Int64: c.Filter.UserID, Valid: true}
	}
	return sp
}

// escapeLike escapes the LIKE wildcards of a user search term.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// listArgs validates the filter and builds the query arguments. ok=false
// means a filter names a record that does not exist (an empty page).
func (r *Reader) listArgs(ctx context.Context, sp scopeParams, f ListFilter) (db.ListWarrantyRowsParams, bool, error) {
	p := db.ListWarrantyRowsParams{
		BrandID: sp.brandID, OrgIds: sp.orgIDs, HolderUserID: sp.holderUserID,
		ServiceCreatedBy: sp.serviceCreatedBy, RowLimit: f.Limit, RowOffset: f.Offset,
	}
	if st := strings.TrimSpace(f.Status); st != "" {
		if !IsStatus(st) {
			return p, false, invalidField("status", "unknown warranty status")
		}
		p.Status = pgtype.Text{String: st, Valid: true}
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		if utf8.RuneCountInString(q) > MaxQueryLength {
			return p, false, invalidField("q", "must be at most 100 characters")
		}
		p.Q = pgtype.Text{String: escapeLike(q), Valid: true}
		if plate := geo.NormalizePlate(q); plate != "" {
			p.QPlate = pgtype.Text{String: escapeLike(plate), Valid: true}
		}
	}
	now := r.now()
	if f.DaysLeftMin != nil {
		if *f.DaysLeftMin < 0 || *f.DaysLeftMin > MaxDaysLeft {
			return p, false, invalidField("days_left_min", "must be between 0 and 3650")
		}
		p.EndsAfter = pgtype.Timestamptz{Time: now.AddDate(0, 0, *f.DaysLeftMin), Valid: true}
	}
	if f.DaysLeftMax != nil {
		if *f.DaysLeftMax < 0 || *f.DaysLeftMax > MaxDaysLeft {
			return p, false, invalidField("days_left_max", "must be between 0 and 3650")
		}
		if f.DaysLeftMin != nil && *f.DaysLeftMax < *f.DaysLeftMin {
			return p, false, invalidField("days_left_max", "must not be less than days_left_min")
		}
		p.EndsBefore = pgtype.Timestamptz{Time: now.AddDate(0, 0, *f.DaysLeftMax), Valid: true}
		if f.DaysLeftMin == nil {
			// "Ends within N days" counts from now: already ended rows are out.
			p.EndsAfter = pgtype.Timestamptz{Time: now, Valid: true}
		}
	}
	if raw := strings.TrimSpace(f.ProductUUID); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return p, false, invalidField("product_uuid", "invalid uuid")
		}
		prod, err := r.q.GetProductByUUID(ctx, db.GetProductByUUIDParams{Uuid: id, BrandID: sp.brandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return p, false, nil
		}
		if err != nil {
			return p, false, fmt.Errorf("warranty: product: %w", err)
		}
		p.ProductID = pgtype.Int8{Int64: prod.ID, Valid: true}
	}
	if raw := strings.TrimSpace(f.VehicleUUID); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return p, false, invalidField("vehicle_uuid", "invalid uuid")
		}
		v, err := r.q.GetVehicleByUUID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return p, false, nil
		}
		if err != nil {
			return p, false, fmt.Errorf("warranty: vehicle: %w", err)
		}
		p.VehicleID = pgtype.Int8{Int64: v.ID, Valid: true}
	}
	return p, true, nil
}

func countArgs(p db.ListWarrantyRowsParams) db.CountWarrantyRowsParams {
	return db.CountWarrantyRowsParams{
		BrandID: p.BrandID, OrgIds: p.OrgIds, HolderUserID: p.HolderUserID,
		ServiceCreatedBy: p.ServiceCreatedBy, WarrantyUuid: p.WarrantyUuid, Status: p.Status,
		ProductID: p.ProductID, VehicleID: p.VehicleID, EndsAfter: p.EndsAfter, EndsBefore: p.EndsBefore,
		Q: p.Q, QPlate: p.QPlate,
	}
}

func (r *Reader) list(ctx context.Context, sp scopeParams, f ListFilter, view func(db.ListWarrantyRowsRow) WarrantyListView) ([]WarrantyListView, int64, error) {
	p, ok, err := r.listArgs(ctx, sp, f)
	if err != nil {
		return nil, 0, err
	}
	if !ok {
		return []WarrantyListView{}, 0, nil
	}
	rows, err := r.q.ListWarrantyRows(ctx, p)
	if err != nil {
		return nil, 0, fmt.Errorf("warranty: list: %w", err)
	}
	total, err := r.q.CountWarrantyRows(ctx, countArgs(p))
	if err != nil {
		return nil, 0, fmt.Errorf("warranty: count: %w", err)
	}
	out := make([]WarrantyListView, 0, len(rows))
	for _, row := range rows {
		out = append(out, view(row))
	}
	return out, total, nil
}

func (r *Reader) one(ctx context.Context, sp scopeParams, id uuid.UUID) (db.ListWarrantyRowsRow, error) {
	rows, err := r.q.ListWarrantyRows(ctx, db.ListWarrantyRowsParams{
		BrandID: sp.brandID, OrgIds: sp.orgIDs, HolderUserID: sp.holderUserID,
		ServiceCreatedBy: sp.serviceCreatedBy, WarrantyUuid: pgtype.UUID{Bytes: id, Valid: true},
		RowLimit: 1,
	})
	if err != nil {
		return db.ListWarrantyRowsRow{}, fmt.Errorf("warranty: get: %w", err)
	}
	if len(rows) == 0 {
		return db.ListWarrantyRowsRow{}, ErrWarrantyNotFound
	}
	return rows[0], nil
}

// baseView maps a row without the holder.
func baseView(row db.ListWarrantyRowsRow) WarrantyListView {
	v := WarrantyListView{
		UUID: row.Uuid, PublicCode: row.PublicCode, Status: row.Status, ItemKind: row.ItemKind,
		StartAt: row.StartAt.Time, EndAt: row.EndAt.Time, ExpiredAt: timePtr(row.ExpiredAt),
		VoidedAt: timePtr(row.VoidedAt), VoidReason: textPtr(row.VoidReason), CreatedAt: row.CreatedAt.Time,
		Product:      ProductRefView{UUID: row.ProductUuid, SKU: row.ProductSku, Name: row.ProductName},
		Service:      ServiceRefView{UUID: row.ServiceUuid, ServiceNo: row.ServiceNo},
		Organization: OrganizationRefView{UUID: row.OrganizationUuid, Name: row.OrganizationName, Type: row.OrganizationType},
		Vehicle: VehicleRefView{
			UUID: row.VehicleUuid, BrandName: row.CarBrandName, ModelName: row.CarModelName,
			Plate: textPtr(row.ServicePlate),
		},
	}
	if v.Vehicle.Plate == nil {
		v.Vehicle.Plate = textPtr(row.VehiclePlate)
	}
	if row.ServiceModelYear.Valid {
		y := row.ServiceModelYear.Int16
		v.Vehicle.ModelYear = &y
	}
	return v
}

// voidable reports whether the status can still become void.
func voidable(status string) bool { return status == StatusActive || status == StatusExpired }

// canVoid reports whether the principal holds warranties.void for a
// warranty of brandID (center: brand scope, super_admin: all).
func canVoid(p authctx.Principal, org orgctx.Scope, brandID int64) bool {
	scope, ok := p.ScopeFor(rbac.PermWarrantiesVoid)
	if !ok {
		return false
	}
	switch scope {
	case rbac.ScopeAll:
		return true
	case rbac.ScopeBrand:
		return brandID == org.BrandID
	}
	return false
}

func (r *Reader) panelView(ctx context.Context, c Caller) func(db.ListWarrantyRowsRow) WarrantyListView {
	loc := i18n.FromContext(ctx).Locale
	return func(row db.ListWarrantyRowsRow) WarrantyListView {
		v := baseView(row)
		h := &HolderRefView{UUID: row.HolderUuid, Name: row.HolderName, Surname: row.HolderSurname}
		if row.HolderStatus == "anonymized" {
			h = &HolderRefView{UUID: row.HolderUuid, Name: i18n.Translate(loc, anonymizedNameKey), Anonymized: true}
		}
		v.Holder = h
		v.CanVoid = voidable(row.Status) && canVoid(c.Principal, c.Org, row.BrandID)
		return v
	}
}

// List returns one page of the warranties in the caller's scope.
func (r *Reader) List(ctx context.Context, c Caller, f ListFilter) ([]WarrantyListView, int64, error) {
	return r.list(ctx, panelScope(c), f, r.panelView(ctx, c))
}

// Get returns one warranty in the caller's scope (404 outside it).
func (r *Reader) Get(ctx context.Context, c Caller, id uuid.UUID) (WarrantyListView, error) {
	row, err := r.one(ctx, panelScope(c), id)
	if err != nil {
		return WarrantyListView{}, err
	}
	return r.panelView(ctx, c)(row), nil
}

// portalScope: the holder's warranties of the domain brand.
func portalScope(brandID, userID int64) scopeParams {
	return scopeParams{brandID: brandID, holderUserID: pgtype.Int8{Int64: userID, Valid: true}}
}

// PortalList returns the warranties the portal user holds in the brand.
func (r *Reader) PortalList(ctx context.Context, brandID, userID int64, f ListFilter) ([]WarrantyListView, int64, error) {
	if brandID == 0 || userID == 0 {
		return []WarrantyListView{}, 0, nil
	}
	return r.list(ctx, portalScope(brandID, userID), f, baseView)
}

// PortalGet returns one warranty the portal user holds (404 otherwise).
func (r *Reader) PortalGet(ctx context.Context, brandID, userID int64, id uuid.UUID) (WarrantyListView, error) {
	if brandID == 0 || userID == 0 {
		return WarrantyListView{}, ErrWarrantyNotFound
	}
	row, err := r.one(ctx, portalScope(brandID, userID), id)
	if err != nil {
		return WarrantyListView{}, err
	}
	return baseView(row), nil
}

// NormalizeVoidReason trims and checks the void reason.
func NormalizeVoidReason(raw string) (string, error) {
	reason := strings.TrimSpace(raw)
	n := utf8.RuneCountInString(reason)
	if n < MinVoidReasonRunes {
		return "", invalidField("reason", "is required (at least 3 characters)")
	}
	if n > MaxVoidReasonRunes {
		return "", invalidField("reason", "must be at most 500 characters")
	}
	return reason, nil
}

// Void voids a warranty of the caller's brand. c.Filter is the resolved
// warranties.void scope (brand or all); out of scope = 404.
func (r *Reader) Void(ctx context.Context, c Caller, id uuid.UUID, rawReason string, meta activity.Meta) (WarrantyListView, error) {
	reason, err := NormalizeVoidReason(rawReason)
	if err != nil {
		return WarrantyListView{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return WarrantyListView{}, fmt.Errorf("warranty: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	w, err := q.GetWarrantyByUUID(ctx, db.GetWarrantyByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return WarrantyListView{}, ErrWarrantyNotFound
	}
	if err != nil {
		return WarrantyListView{}, fmt.Errorf("warranty: get: %w", err)
	}
	if !canVoid(c.Principal, c.Org, w.BrandID) || !c.Filter.AllowsOrg(w.OrganizationID, w.BrandID) {
		return WarrantyListView{}, ErrWarrantyNotFound
	}
	w, err = q.LockWarranty(ctx, db.LockWarrantyParams{ID: w.ID, BrandID: w.BrandID})
	if err != nil {
		return WarrantyListView{}, fmt.Errorf("warranty: lock: %w", err)
	}
	if !voidable(w.Status) {
		return WarrantyListView{}, ErrAlreadyVoid
	}
	var actor pgtype.Int8
	var actorPtr *int64
	if c.Principal.UserInternal != 0 {
		uid := c.Principal.UserInternal
		actor = pgtype.Int8{Int64: uid, Valid: true}
		actorPtr = &uid
	}
	prev := w.Status
	w, err = q.VoidWarrantyWithReason(ctx, db.VoidWarrantyWithReasonParams{
		ID: w.ID, BrandID: w.BrandID, ActorUserID: actor, VoidReason: reason,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return WarrantyListView{}, ErrAlreadyVoid
	}
	if err != nil {
		return WarrantyListView{}, fmt.Errorf("warranty: void: %w", err)
	}
	ctxs, err := q.ListWarrantyNoticeContexts(ctx, []int64{w.ID})
	if err != nil {
		return WarrantyListView{}, fmt.Errorf("warranty: notice context: %w", err)
	}
	var nc db.ListWarrantyNoticeContextsRow
	if len(ctxs) > 0 {
		nc = ctxs[0]
	}
	extra := map[string]any{"void_reason": reason, "previous_status": prev}
	if actorPtr != nil {
		extra["voided_by_user_id"] = *actorPtr
	}
	ev := warrantyEvent(events.WarrantyVoided, w, nc, verifyURL(r.verifyBaseURL, w.PublicCode), 0, extra)
	if r.out != nil {
		if err := r.out.Enqueue(ctx, tx, ev); err != nil {
			return WarrantyListView{}, fmt.Errorf("warranty: voided event: %w", err)
		}
	}
	payload := map[string]any{
		"public_code": w.PublicCode, "reason": reason, "previous_status": prev,
		"organization_id": w.OrganizationID, "brand_id": w.BrandID,
	}
	if c.Org.UUID != uuid.Nil {
		payload["actor_organization_uuid"] = c.Org.UUID.String()
	}
	wid := w.Uuid
	if err := activity.Write(ctx, q, actorPtr, ActionWarrantyVoided, "warranties", &wid, payload, meta); err != nil {
		return WarrantyListView{}, fmt.Errorf("warranty: audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return WarrantyListView{}, fmt.Errorf("warranty: commit: %w", err)
	}
	row, err := r.one(ctx, scopeParams{brandID: w.BrandID}, w.Uuid)
	if err != nil {
		return WarrantyListView{}, err
	}
	return r.panelView(ctx, c)(row), nil
}
