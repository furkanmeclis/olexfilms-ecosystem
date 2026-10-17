package usecase

// Delivery methods (TEC-768, Depo F4.3): the selling organization's carrier
// catalog. Every organization has the system method "warehouse_pickup"
// (Ambar teslim), created on first use; it cannot be edited or deleted. A
// method an order shipped with cannot be deleted either (deactivate it).

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Delivery method errors; the handler maps them to 404/409.
var (
	ErrDeliveryMethodNotFound = errors.New("orders: delivery method not found")
	// ErrDeliveryMethodSystem: the system method is read-only.
	ErrDeliveryMethodSystem = errors.New("orders: the system delivery method cannot be changed")
	// ErrDeliveryMethodInUse: an order shipped with the method.
	ErrDeliveryMethodInUse = errors.New("orders: the delivery method is used by orders")
)

// SystemDeliveryMethodCode is the code of the system method (Ambar teslim).
const SystemDeliveryMethodCode = "warehouse_pickup"

// TrackingPlaceholder is replaced by the tracking number in a tracking URL
// template.
const TrackingPlaceholder = "{tracking_no}"

// Validation codes of a delivery method.
const (
	CodeDeliveryMethodCodeTaken = "DELIVERY_METHOD_CODE_TAKEN"
	CodeTrackingURLInvalid      = "TRACKING_URL_TEMPLATE_INVALID"
)

// Field bounds.
const (
	MaxDeliveryMethodName = 120
	MaxTrackingTemplate   = 500
	MaxTrackingNo         = 128
)

var deliveryCodeRe = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

// DeliveryMethodSortSpec is the list sort whitelist (docs/list-contract.md).
var DeliveryMethodSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"name": "name", "code": "code", "sort_order": "sort_order", "created_at": "created_at"},
	Default: apiquery.SortField{Field: "sort_order"},
}

// DeliveryMethodView is a delivery method as the API returns it. The system
// method's name is the localized label.
type DeliveryMethodView struct {
	UUID                uuid.UUID `json:"uuid"`
	Code                string    `json:"code"`
	Name                string    `json:"name"`
	TrackingURLTemplate *string   `json:"tracking_url_template"`
	Active              bool      `json:"active"`
	SortOrder           int32     `json:"sort_order"`
	IsSystem            bool      `json:"is_system"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// DeliveryMethodRef names the method of an order.
type DeliveryMethodRef struct {
	UUID     uuid.UUID `json:"uuid"`
	Code     string    `json:"code"`
	Name     string    `json:"name"`
	IsSystem bool      `json:"is_system"`
}

// SystemDeliveryMethodLabelKey is the i18n key of the system method's name.
const SystemDeliveryMethodLabelKey = "orders.delivery_method.warehouse_pickup"

func deliveryMethodName(ctx context.Context, m db.DeliveryMethod) string {
	if m.IsSystem {
		if l := i18n.Translate(i18n.FromContext(ctx).Locale, SystemDeliveryMethodLabelKey); l != SystemDeliveryMethodLabelKey {
			return l
		}
	}
	return m.Name
}

func deliveryMethodView(ctx context.Context, m db.DeliveryMethod) DeliveryMethodView {
	return DeliveryMethodView{
		UUID: m.Uuid, Code: m.Code, Name: deliveryMethodName(ctx, m), TrackingURLTemplate: textPtr(m.TrackingUrlTemplate),
		Active: m.Active, SortOrder: m.SortOrder, IsSystem: m.IsSystem,
		CreatedAt: m.CreatedAt.Time, UpdatedAt: m.UpdatedAt.Time,
	}
}

// TrackingURL fills a tracking URL template; empty when either is missing.
func TrackingURL(template, trackingNo string) string {
	template, trackingNo = strings.TrimSpace(template), strings.TrimSpace(trackingNo)
	if template == "" || trackingNo == "" {
		return ""
	}
	if !strings.Contains(template, TrackingPlaceholder) {
		return template
	}
	return strings.ReplaceAll(template, TrackingPlaceholder, url.PathEscape(trackingNo))
}

var placeholderRe = regexp.MustCompile(`\{[^{}]*\}`)

// validTrackingTemplate: an http(s) URL once every {placeholder} is filled.
func validTrackingTemplate(t string) bool {
	u, err := url.Parse(placeholderRe.ReplaceAllString(t, "placeholder"))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" &&
		!strings.ContainsAny(t, " \t\r\n")
}

// ensureSystemMethod creates the organization's system method if missing.
func ensureSystemMethod(ctx context.Context, q *db.Queries, orgID, brandID int64) error {
	if err := q.EnsureSystemDeliveryMethod(ctx, db.EnsureSystemDeliveryMethodParams{
		OrganizationID: orgID, BrandID: brandID, Name: "Ambar teslim",
	}); err != nil {
		return fmt.Errorf("orders: system delivery method: %w", err)
	}
	return nil
}

// DeliveryMethodFilter narrows the delivery method list.
type DeliveryMethodFilter struct {
	Q        string
	Active   *bool
	IsSystem *bool
	Sort     apiquery.ResolvedSort
	Limit    int32
	Offset   int32
}

// ListDeliveryMethods lists the active organization's methods (orders.read).
func (s *Service) ListDeliveryMethods(ctx context.Context, c Caller, f DeliveryMethodFilter) ([]DeliveryMethodView, int64, error) {
	if c.Org.InternalID == 0 {
		return nil, 0, ErrForbidden
	}
	if err := ensureSystemMethod(ctx, s.q, c.Org.InternalID, c.Org.BrandID); err != nil {
		return nil, 0, err
	}
	q := pgtype.Text{}
	if v := strings.TrimSpace(f.Q); v != "" {
		q = pgtype.Text{String: v, Valid: true}
	}
	active, system := boolArg(f.Active), boolArg(f.IsSystem)
	if f.Sort.Key == "" {
		f.Sort = apiquery.ResolvedSort{Key: "sort_order"}
	}
	rows, err := s.q.ListDeliveryMethodsFiltered(ctx, db.ListDeliveryMethodsFilteredParams{
		OrganizationID: c.Org.InternalID, Active: active, IsSystem: system, Q: q,
		SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, RowLimit: f.Limit, RowOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("orders: delivery methods: %w", err)
	}
	total, err := s.q.CountDeliveryMethodsFiltered(ctx, db.CountDeliveryMethodsFilteredParams{
		OrganizationID: c.Org.InternalID, Active: active, IsSystem: system, Q: q,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("orders: delivery methods: %w", err)
	}
	out := make([]DeliveryMethodView, 0, len(rows))
	for _, m := range rows {
		out = append(out, deliveryMethodView(ctx, m))
	}
	return out, total, nil
}

func boolArg(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

// DeliveryMethodInput is a new or edited method.
type DeliveryMethodInput struct {
	Code                string
	Name                string
	TrackingURLTemplate *string
	Active              *bool
	SortOrder           *int32
}

type deliveryMethodFields struct {
	code, name string
	template   pgtype.Text
	active     bool
	sortOrder  int32
}

func (in DeliveryMethodInput) validate() (deliveryMethodFields, error) {
	f := deliveryMethodFields{
		code: strings.ToLower(strings.TrimSpace(in.Code)), name: strings.TrimSpace(in.Name), active: true,
	}
	if !deliveryCodeRe.MatchString(f.code) {
		return f, invalid("code", "1-32 characters: lowercase letters, digits and _")
	}
	if f.code == SystemDeliveryMethodCode {
		return f, &ValidationError{Field: "code", Code: CodeDeliveryMethodCodeTaken, Message: "the code is reserved"}
	}
	if f.name == "" || len([]rune(f.name)) > MaxDeliveryMethodName {
		return f, invalid("name", fmt.Sprintf("1-%d characters", MaxDeliveryMethodName))
	}
	if in.TrackingURLTemplate != nil {
		t := strings.TrimSpace(*in.TrackingURLTemplate)
		if len(t) > MaxTrackingTemplate {
			return f, invalid("tracking_url_template", fmt.Sprintf("at most %d characters", MaxTrackingTemplate))
		}
		if t != "" {
			if !validTrackingTemplate(t) {
				return f, &ValidationError{Field: "tracking_url_template", Code: CodeTrackingURLInvalid,
					Message: "must be an http(s) URL; use " + TrackingPlaceholder + " for the tracking number"}
			}
			f.template = pgtype.Text{String: t, Valid: true}
		}
	}
	if in.Active != nil {
		f.active = *in.Active
	}
	if in.SortOrder != nil {
		if *in.SortOrder < 0 || *in.SortOrder > 100000 {
			return f, invalid("sort_order", "must be between 0 and 100000")
		}
		f.sortOrder = *in.SortOrder
	}
	return f, nil
}

func (s *Service) canManageDeliveryMethods(c Caller) bool {
	return c.Org.InternalID != 0 && c.can(rbac.PermOrdersShip)
}

func codeTaken(err error) error {
	if isUniqueViolation(err) {
		return &ValidationError{Field: "code", Code: CodeDeliveryMethodCodeTaken, Message: "the code is already used"}
	}
	return fmt.Errorf("orders: delivery method: %w", err)
}

// CreateDeliveryMethod adds a method to the active organization (orders.ship).
func (s *Service) CreateDeliveryMethod(ctx context.Context, c Caller, in DeliveryMethodInput) (DeliveryMethodView, error) {
	if !s.canManageDeliveryMethods(c) {
		return DeliveryMethodView{}, ErrForbidden
	}
	f, err := in.validate()
	if err != nil {
		return DeliveryMethodView{}, err
	}
	m, err := s.q.CreateDeliveryMethod(ctx, db.CreateDeliveryMethodParams{
		OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, Code: f.code, Name: f.name,
		TrackingUrlTemplate: f.template, Active: f.active, SortOrder: f.sortOrder,
	})
	if err != nil {
		return DeliveryMethodView{}, codeTaken(err)
	}
	return deliveryMethodView(ctx, m), nil
}

func (s *Service) lockMethod(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID) (db.DeliveryMethod, error) {
	m, err := q.LockDeliveryMethodByUUID(ctx, db.LockDeliveryMethodByUUIDParams{Uuid: id, OrganizationID: c.Org.InternalID})
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrDeliveryMethodNotFound
	}
	if err != nil {
		return m, fmt.Errorf("orders: delivery method: %w", err)
	}
	if m.IsSystem {
		return m, ErrDeliveryMethodSystem
	}
	return m, nil
}

// UpdateDeliveryMethod edits a method of the active organization; the
// system method answers ErrDeliveryMethodSystem.
func (s *Service) UpdateDeliveryMethod(ctx context.Context, c Caller, id uuid.UUID, in DeliveryMethodInput) (DeliveryMethodView, error) {
	if !s.canManageDeliveryMethods(c) {
		return DeliveryMethodView{}, ErrForbidden
	}
	var out db.DeliveryMethod
	err := s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		m, err := s.lockMethod(ctx, q, c, id)
		if err != nil {
			return err
		}
		if in.Active == nil {
			in.Active = &m.Active
		}
		if in.SortOrder == nil {
			in.SortOrder = &m.SortOrder
		}
		f, err := in.validate()
		if err != nil {
			return err
		}
		out, err = q.UpdateDeliveryMethod(ctx, db.UpdateDeliveryMethodParams{
			ID: m.ID, Code: f.code, Name: f.name, TrackingUrlTemplate: f.template, Active: f.active, SortOrder: f.sortOrder,
		})
		if err != nil {
			return codeTaken(err)
		}
		return nil
	})
	if err != nil {
		return DeliveryMethodView{}, err
	}
	return deliveryMethodView(ctx, out), nil
}

// DeleteDeliveryMethod removes an unused method of the active organization.
func (s *Service) DeleteDeliveryMethod(ctx context.Context, c Caller, id uuid.UUID) error {
	if !s.canManageDeliveryMethods(c) {
		return ErrForbidden
	}
	return s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		m, err := s.lockMethod(ctx, q, c, id)
		if err != nil {
			return err
		}
		used, err := q.CountOrdersByDeliveryMethod(ctx, pgtype.Int8{Int64: m.ID, Valid: true})
		if err != nil {
			return fmt.Errorf("orders: delivery method use: %w", err)
		}
		if used > 0 {
			return ErrDeliveryMethodInUse
		}
		if _, err := q.DeleteDeliveryMethod(ctx, m.ID); err != nil {
			return fmt.Errorf("orders: delete delivery method: %w", err)
		}
		return nil
	})
}

// resolveShippingMethod finds an active method of the seller for shipping.
func resolveShippingMethod(ctx context.Context, q *db.Queries, o db.Order, raw string) (db.DeliveryMethod, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return db.DeliveryMethod{}, invalid("delivery_method_uuid", "must be a UUID")
	}
	seller, err := q.GetOrganizationByID(ctx, o.SellerOrgID)
	if err != nil {
		return db.DeliveryMethod{}, fmt.Errorf("orders: seller: %w", err)
	}
	if err := ensureSystemMethod(ctx, q, seller.ID, seller.BrandID); err != nil {
		return db.DeliveryMethod{}, err
	}
	m, err := q.GetDeliveryMethodByUUID(ctx, db.GetDeliveryMethodByUUIDParams{Uuid: id, OrganizationID: o.SellerOrgID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !m.Active) {
		return db.DeliveryMethod{}, invalid("delivery_method_uuid", "delivery method not found")
	}
	if err != nil {
		return db.DeliveryMethod{}, fmt.Errorf("orders: delivery method: %w", err)
	}
	return m, nil
}
