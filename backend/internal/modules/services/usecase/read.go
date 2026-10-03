package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// AnonymizedNameKey is the catalog label of an anonymized customer (K19).
const AnonymizedNameKey = "customers.anonymized_name"

// StatusLabelKey is the i18n key of a status label.
func StatusLabelKey(status string) string { return "services.status." + status }

// Ref names a related record.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// OrgRef names the service organization.
type OrgRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Type string    `json:"type"`
}

// CustomerRef names the customer (masked when anonymized).
type CustomerRef struct {
	UUID       uuid.UUID `json:"uuid"`
	Name       string    `json:"name"`
	Surname    string    `json:"surname"`
	Phone      *string   `json:"phone"`
	Anonymized bool      `json:"anonymized"`
}

// ProductRef names an item's product.
type ProductRef struct {
	UUID     uuid.UUID `json:"uuid"`
	SKU      string    `json:"sku"`
	Name     string    `json:"name"`
	UnitType string    `json:"unit_type"`
}

// ItemView is one service item.
type ItemView struct {
	UUID         uuid.UUID  `json:"uuid"`
	Product      ProductRef `json:"product"`
	Barcode      string     `json:"barcode"`
	UnitKind     string     `json:"unit_kind"`
	Kind         string     `json:"kind"`
	Quantity     *int32     `json:"quantity"`
	Meters       *string    `json:"meters"`
	AppliedParts []string   `json:"applied_parts"`
	Notes        *string    `json:"notes"`
	CreatedAt    time.Time  `json:"created_at"`
	// Correction is the consumption correction of the item (TEC-230): the
	// unit went back to stock, ReplacementBarcode (if any) was consumed
	// instead. nil when the item was not corrected.
	Correction *ItemCorrectionView `json:"correction"`
}

// ItemCorrectionView is the consumption correction of an item (TEC-230).
type ItemCorrectionView struct {
	UUID               uuid.UUID `json:"uuid"`
	Reason             string    `json:"reason"`
	ReplacementBarcode *string   `json:"replacement_barcode"`
	CreatedAt          time.Time `json:"created_at"`
}

// ImageView is one service image; URL is the authenticated download path.
type ImageView struct {
	UUID      uuid.UUID `json:"uuid"`
	Title     *string   `json:"title"`
	SortOrder int32     `json:"sort_order"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// StatusLogView is one status change. ByOtherOrganization marks a change
// made by another organization than the service's (e.g. the center).
type StatusLogView struct {
	FromStatus          *string   `json:"from_status"`
	ToStatus            string    `json:"to_status"`
	Note                *string   `json:"note"`
	ByOtherOrganization bool      `json:"by_other_organization"`
	CreatedAt           time.Time `json:"created_at"`
}

// WarrantyView summarises one warranty the completed service issued
// (TEC-186); ServiceItemUUID and ProductName tie it to the service item.
type WarrantyView struct {
	UUID            uuid.UUID  `json:"uuid"`
	PublicCode      string     `json:"public_code"`
	ServiceItemUUID uuid.UUID  `json:"service_item_uuid"`
	ProductName     string     `json:"product_name"`
	ItemKind        string     `json:"item_kind"`
	Status          string     `json:"status"`
	StartAt         time.Time  `json:"start_at"`
	EndAt           time.Time  `json:"end_at"`
	ExpiredAt       *time.Time `json:"expired_at"`
	VoidedAt        *time.Time `json:"voided_at"`
}

// ServiceView is a service as the API returns it.
type ServiceView struct {
	UUID           uuid.UUID   `json:"uuid"`
	ServiceNo      string      `json:"service_no"`
	Status         string      `json:"status"`
	StatusLabel    string      `json:"status_label"`
	Organization   OrgRef      `json:"organization"`
	Customer       CustomerRef `json:"customer"`
	VehicleUUID    uuid.UUID   `json:"vehicle_uuid"`
	CarBrand       Ref         `json:"car_brand"`
	CarModel       Ref         `json:"car_model"`
	ModelYear      *int16      `json:"model_year"`
	Plate          *string     `json:"plate"`
	PlateCountry   *string     `json:"plate_country"`
	VIN            *string     `json:"vin"`
	KM             *int32      `json:"km"`
	Package        *string     `json:"package"`
	Notes          *string     `json:"notes"`
	HasMeasurement bool        `json:"has_measurement"`
	CancelReason   *string     `json:"cancel_reason"`
	CompletedAt    *time.Time  `json:"completed_at"`
	CancelledAt    *time.Time  `json:"cancelled_at"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
	// Editable: the caller may edit the form (km, package, notes, images).
	Editable bool `json:"editable"`
	// ItemsEditable: the caller may add or remove items.
	ItemsEditable bool `json:"items_editable"`
	// AvailableTransitions are the statuses the caller may move the service to.
	AvailableTransitions []string        `json:"available_transitions"`
	Items                []ItemView      `json:"items,omitempty"`
	Images               []ImageView     `json:"images,omitempty"`
	StatusLogs           []StatusLogView `json:"status_logs,omitempty"`
	Warranties           []WarrantyView  `json:"warranties,omitempty"`
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
	v := strings.TrimSpace(t.String)
	return &v
}

func (s *Service) summary(ctx context.Context, q *db.Queries, c Caller, svc db.Service) (ServiceView, error) {
	refs, err := q.GetServiceRefs(ctx, svc.ID)
	if err != nil {
		return ServiceView{}, fmt.Errorf("services: refs: %w", err)
	}
	loc := i18n.FromContext(ctx).Locale
	cust := CustomerRef{UUID: refs.CustomerUuid, Name: refs.CustomerName, Surname: refs.CustomerSurname,
		Phone: textPtr(refs.CustomerPhone)}
	if refs.CustomerStatus == "anonymized" {
		cust = CustomerRef{UUID: refs.CustomerUuid, Name: i18n.Translate(loc, AnonymizedNameKey), Anonymized: true}
	}
	canWrite := c.allows(rbac.PermServicesWrite, svc) && formEditable(c, svc)
	v := ServiceView{
		UUID: svc.Uuid, ServiceNo: svc.ServiceNo, Status: svc.Status,
		StatusLabel:  i18n.Translate(loc, StatusLabelKey(svc.Status)),
		Organization: OrgRef{UUID: refs.OrganizationUuid, Name: refs.OrganizationName, Type: refs.OrganizationType},
		Customer:     cust, VehicleUUID: refs.VehicleUuid,
		CarBrand: Ref{UUID: refs.CarBrandUuid, Name: refs.CarBrandName},
		CarModel: Ref{UUID: refs.CarModelUuid, Name: refs.CarModelName},
		Plate:    textPtr(svc.Plate), PlateCountry: textPtr(svc.PlateCountry), VIN: textPtr(svc.Vin),
		Package: textPtr(svc.Package), Notes: textPtr(svc.Notes), HasMeasurement: svc.HasMeasurement,
		CancelReason: textPtr(svc.CancelReason), CompletedAt: tsPtr(svc.CompletedAt), CancelledAt: tsPtr(svc.CancelledAt),
		CreatedAt: svc.CreatedAt.Time, UpdatedAt: svc.UpdatedAt.Time,
		Editable: canWrite, ItemsEditable: canWrite && itemsEditable(svc.Status),
		AvailableTransitions: availableTransitions(c, svc),
	}
	if svc.ModelYear.Valid {
		y := svc.ModelYear.Int16
		v.ModelYear = &y
	}
	if svc.Km.Valid {
		k := svc.Km.Int32
		v.KM = &k
	}
	return v, nil
}

// ImageURL is the authenticated download path of a service image.
func ImageURL(serviceUUID, imageUUID uuid.UUID) string {
	return "/v1/services/" + serviceUUID.String() + "/images/" + imageUUID.String()
}

// view is the full service: summary, items, images and status logs.
func (s *Service) view(ctx context.Context, q *db.Queries, c Caller, svc db.Service) (ServiceView, error) {
	v, err := s.summary(ctx, q, c, svc)
	if err != nil {
		return ServiceView{}, err
	}
	items, err := q.ListServiceItems(ctx, svc.ID)
	if err != nil {
		return ServiceView{}, fmt.Errorf("services: items: %w", err)
	}
	corrections, err := q.ListServiceItemCorrections(ctx, svc.ID)
	if err != nil {
		return ServiceView{}, fmt.Errorf("services: corrections: %w", err)
	}
	correctionByItem := make(map[int64]*ItemCorrectionView, len(corrections))
	for _, cr := range corrections {
		correctionByItem[cr.ServiceItemID] = &ItemCorrectionView{
			UUID: cr.Uuid, Reason: cr.Reason, ReplacementBarcode: textPtr(cr.ReplacementBarcode),
			CreatedAt: cr.CreatedAt.Time,
		}
	}
	v.Items = make([]ItemView, 0, len(items))
	itemByID := make(map[int64]ItemView, len(items))
	for _, it := range items {
		p, err := q.GetProduct(ctx, db.GetProductParams{ID: it.ProductID, BrandID: svc.BrandID})
		if err != nil {
			return ServiceView{}, fmt.Errorf("services: product: %w", err)
		}
		u, err := q.GetUnit(ctx, it.UnitID)
		if err != nil {
			return ServiceView{}, fmt.Errorf("services: unit: %w", err)
		}
		parts := []string{}
		_ = json.Unmarshal(it.AppliedParts, &parts)
		iv := ItemView{
			UUID: it.Uuid, Product: ProductRef{UUID: p.Uuid, SKU: p.Sku, Name: p.Name, UnitType: p.UnitType},
			Barcode: u.Barcode, UnitKind: u.UnitKind, Kind: it.Kind, Meters: numericTextPtr(it.Meters),
			AppliedParts: parts, Notes: textPtr(it.Notes), CreatedAt: it.CreatedAt.Time,
			Correction: correctionByItem[it.ID],
		}
		if it.Quantity.Valid {
			qv := it.Quantity.Int32
			iv.Quantity = &qv
		}
		v.Items = append(v.Items, iv)
		itemByID[it.ID] = iv
	}
	images, err := q.ListServiceImages(ctx, svc.ID)
	if err != nil {
		return ServiceView{}, fmt.Errorf("services: images: %w", err)
	}
	v.Images = make([]ImageView, 0, len(images))
	for _, im := range images {
		v.Images = append(v.Images, ImageView{
			UUID: im.Uuid, Title: textPtr(im.Title), SortOrder: im.SortOrder,
			URL: ImageURL(svc.Uuid, im.Uuid), CreatedAt: im.CreatedAt.Time,
		})
	}
	logs, err := q.ListServiceStatusLogs(ctx, svc.ID)
	if err != nil {
		return ServiceView{}, fmt.Errorf("services: status logs: %w", err)
	}
	v.StatusLogs = make([]StatusLogView, 0, len(logs))
	for _, l := range logs {
		v.StatusLogs = append(v.StatusLogs, StatusLogView{
			FromStatus: textPtr(l.FromStatus), ToStatus: l.ToStatus, Note: textPtr(l.Note),
			ByOtherOrganization: l.ActorOrgID.Valid && l.ActorOrgID.Int64 != svc.OrganizationID,
			CreatedAt:           l.CreatedAt.Time,
		})
	}
	warranties, err := q.ListWarrantiesByService(ctx, db.ListWarrantiesByServiceParams{ServiceID: svc.ID, BrandID: svc.BrandID})
	if err != nil {
		return ServiceView{}, fmt.Errorf("services: warranties: %w", err)
	}
	v.Warranties = make([]WarrantyView, 0, len(warranties))
	for _, w := range warranties {
		item := itemByID[w.ServiceItemID]
		v.Warranties = append(v.Warranties, WarrantyView{
			UUID: w.Uuid, PublicCode: w.PublicCode, ServiceItemUUID: item.UUID, ProductName: item.Product.Name,
			ItemKind: w.ItemKind, Status: w.Status, StartAt: w.StartAt.Time, EndAt: w.EndAt.Time,
			ExpiredAt: tsPtr(w.ExpiredAt), VoidedAt: tsPtr(w.VoidedAt),
		})
	}
	return v, nil
}

func (s *Service) getVisible(ctx context.Context, c Caller, id uuid.UUID) (db.Service, error) {
	svc, err := s.q.GetServiceByUUID(ctx, db.GetServiceByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Service{}, ErrNotFound
	}
	if err != nil {
		return db.Service{}, fmt.Errorf("services: get: %w", err)
	}
	if !visible(c, svc) {
		return db.Service{}, ErrNotFound
	}
	return svc, nil
}

// Get returns one service inside the caller's services.read scope (else 404).
func (s *Service) Get(ctx context.Context, c Caller, id uuid.UUID) (ServiceView, error) {
	svc, err := s.getVisible(ctx, c, id)
	if err != nil {
		return ServiceView{}, err
	}
	return s.view(ctx, s.q, c, svc)
}

// ListFilter narrows the service list. Q matches the service number,
// plate, VIN and the customer's name or phone. CreatedFrom is inclusive,
// CreatedTo exclusive (TEC-183).
type ListFilter struct {
	Q            string
	Status       string
	CustomerUUID string
	VehicleUUID  string
	CreatedFrom  *time.Time
	CreatedTo    *time.Time
	Limit        int32
	Offset       int32
}

// List returns services inside the caller's services.read scope (dealer:
// its own organization, distributor: its subtree, center: the brand).
func (s *Service) List(ctx context.Context, c Caller, f ListFilter) ([]ServiceView, int64, error) {
	p := db.ListServicesInScopeParams{
		BrandID: c.Org.BrandID, OrgIds: c.Filter.OrgIDsArg(), RowLimit: f.Limit, RowOffset: f.Offset,
	}
	if c.Filter.UserOnly() {
		p.CreatedByUserID = pgtype.Int8{Int64: c.Filter.UserID, Valid: true}
	}
	if st := strings.TrimSpace(f.Status); st != "" {
		if !IsStatus(st) {
			return nil, 0, invalid("status", "unknown service status")
		}
		p.Status = pgtype.Text{String: st, Valid: true}
	}
	if f.CreatedFrom != nil {
		p.CreatedFrom = pgtype.Timestamptz{Time: *f.CreatedFrom, Valid: true}
	}
	if f.CreatedTo != nil {
		if f.CreatedFrom != nil && !f.CreatedTo.After(*f.CreatedFrom) {
			return nil, 0, invalid("created_to", "must be after created_from")
		}
		p.CreatedTo = pgtype.Timestamptz{Time: *f.CreatedTo, Valid: true}
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		if len([]rune(q)) > 100 {
			return nil, 0, invalid("q", "must be at most 100 characters")
		}
		p.Q = pgtype.Text{String: q, Valid: true}
	}
	if raw := strings.TrimSpace(f.CustomerUUID); raw != "" {
		id, err := parseUUID("customer_uuid", raw)
		if err != nil {
			return nil, 0, err
		}
		u, err := s.q.GetUserByUUID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return []ServiceView{}, 0, nil
		}
		if err != nil {
			return nil, 0, fmt.Errorf("services: customer: %w", err)
		}
		p.CustomerUserID = pgtype.Int8{Int64: u.ID, Valid: true}
	}
	if raw := strings.TrimSpace(f.VehicleUUID); raw != "" {
		id, err := parseUUID("vehicle_uuid", raw)
		if err != nil {
			return nil, 0, err
		}
		v, err := s.q.GetVehicleByUUID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return []ServiceView{}, 0, nil
		}
		if err != nil {
			return nil, 0, fmt.Errorf("services: vehicle: %w", err)
		}
		p.VehicleID = pgtype.Int8{Int64: v.ID, Valid: true}
	}
	var (
		rows    []db.Service
		total   int64
		indexed bool
	)
	// TEC-209: a text search goes to the services index when it is up; the
	// date bounds are not indexed, so they stay on SQL.
	if p.Q.Valid && !p.CreatedFrom.Valid && !p.CreatedTo.Valid && s.indexEnabled() {
		rows, total, indexed = s.searchIndexed(ctx, c, p)
	}
	if !indexed {
		var err error
		rows, err = s.q.ListServicesInScope(ctx, p)
		if err != nil {
			return nil, 0, fmt.Errorf("services: list: %w", err)
		}
		total, err = s.q.CountServicesInScope(ctx, db.CountServicesInScopeParams{
			BrandID: p.BrandID, OrgIds: p.OrgIds, CreatedByUserID: p.CreatedByUserID,
			CustomerUserID: p.CustomerUserID, VehicleID: p.VehicleID, Status: p.Status, Q: p.Q,
			CreatedFrom: p.CreatedFrom, CreatedTo: p.CreatedTo,
		})
		if err != nil {
			return nil, 0, fmt.Errorf("services: count: %w", err)
		}
	}
	out := make([]ServiceView, 0, len(rows))
	for _, r := range rows {
		v, err := s.summary(ctx, s.q, c, r)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, nil
}
