package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TEC-239 (F2-03b): the portal service detail and the portal service PDF.
//
// A portal user (customer / fleet, aud=portal) sees a service of the domain
// brand (K20) when they are its customer or hold at least one of its
// warranties (a vehicle transfer moves the active warranties to the new
// owner). Anything else is ErrNotFound, so existence does not leak.
//
// The projection is deliberately narrow: no measurement fields (the
// measurement reports stay hidden), no purchase / sale prices, no stock
// unit barcode or consumed amount, no staff notes, no customer contact
// data. Warranty summaries list only the warranties the user holds.

// ResourcePortalPDF is the export resource of the service PDF requested from
// the portal (exports.PortalResourcePrefix): polled and downloaded through
// /v1/portal/exports.
const ResourcePortalPDF = "portal.service_pdf"

// PDFQueryPortalUserID is the export query key of the portal user; the
// worker re-checks the ownership with it.
const PDFQueryPortalUserID = "portal_user_id"

// PortalServiceView is a service as the portal returns it.
type PortalServiceView struct {
	UUID        uuid.UUID  `json:"uuid"`
	ServiceNo   string     `json:"service_no"`
	Status      string     `json:"status"`
	StatusLabel string     `json:"status_label"`
	VehicleUUID uuid.UUID  `json:"vehicle_uuid"`
	CarBrand    Ref        `json:"car_brand"`
	CarModel    Ref        `json:"car_model"`
	ModelYear   *int16     `json:"model_year"`
	Plate       *string    `json:"plate"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at"`
	// AppliedParts is the union of the items' applied parts (vehicle part
	// picker SVG keys), sorted.
	AppliedParts []string             `json:"applied_parts"`
	Products     []PortalProductView  `json:"products"`
	Dealer       PortalDealerView     `json:"dealer"`
	Warranties   []PortalWarrantyView `json:"warranties"`
}

// PortalProductView is one product used in the service (no price).
type PortalProductView struct {
	ServiceItemUUID uuid.UUID `json:"service_item_uuid"`
	Name            string    `json:"name"`
	Category        string    `json:"category"`
	AppliedParts    []string  `json:"applied_parts"`
}

// PortalDealerView is the card of the organization that performed the
// service. WhatsApp is the organization phone in E.164 (nil when unset).
type PortalDealerView struct {
	UUID     uuid.UUID `json:"uuid"`
	Name     string    `json:"name"`
	City     string    `json:"city"`
	District string    `json:"district"`
	Address  string    `json:"address"`
	WhatsApp *string   `json:"whatsapp"`
}

// PortalWarrantyView summarises one warranty of the service the user holds.
type PortalWarrantyView struct {
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

// portalOwns: the user is the service customer or holds one of its
// warranties.
func portalOwns(svc db.Service, userID int64, warranties []db.Warranty) bool {
	if userID <= 0 {
		return false
	}
	if svc.CustomerUserID == userID {
		return true
	}
	for _, w := range warranties {
		if w.HolderUserID == userID {
			return true
		}
	}
	return false
}

// FleetPortal lets a fleet user read the services of their fleet's
// vehicles on the portal (TEC-474, fleet usecase): holder is the fleet's
// primary user, the holder of the fleet warranties.
type FleetPortal interface {
	PortalServiceHolder(ctx context.Context, brandID, userID int64, svc db.Service) (holder int64, ok bool, err error)
}

// WithFleetPortal wires the fleet portal access of the portal service
// detail and PDF (the review stays the owner's).
func (s *Service) WithFleetPortal(f FleetPortal) *Service {
	s.fleetPortal = f
	return s
}

// portalViewable loads a service the user owns, or (TEC-474) a service on a
// vehicle of the user's fleet the fleet portal shows. holder is the user
// whose warranties the views print: the user, or the fleet's primary user.
func (s *Service) portalViewable(ctx context.Context, brandID, userID int64, id uuid.UUID) (db.Service, []db.Warranty, int64, error) {
	svc, warranties, err := s.portalService(ctx, brandID, userID, id)
	if err == nil || !errors.Is(err, ErrNotFound) || s.fleetPortal == nil || brandID <= 0 || userID <= 0 {
		return svc, warranties, userID, err
	}
	svc, err = s.q.GetServiceByUUID(ctx, db.GetServiceByUUIDParams{Uuid: id, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Service{}, nil, 0, ErrNotFound
	}
	if err != nil {
		return db.Service{}, nil, 0, fmt.Errorf("services: portal get: %w", err)
	}
	holder, ok, err := s.fleetPortal.PortalServiceHolder(ctx, brandID, userID, svc)
	if err != nil {
		return db.Service{}, nil, 0, fmt.Errorf("services: portal fleet: %w", err)
	}
	if !ok {
		return db.Service{}, nil, 0, ErrNotFound
	}
	warranties, err = s.q.ListWarrantiesByService(ctx, db.ListWarrantiesByServiceParams{ServiceID: svc.ID, BrandID: svc.BrandID})
	if err != nil {
		return db.Service{}, nil, 0, fmt.Errorf("services: portal warranties: %w", err)
	}
	return svc, warranties, holder, nil
}

// portalService loads a service of the brand the user owns, with its
// warranties (ErrNotFound otherwise).
func (s *Service) portalService(ctx context.Context, brandID, userID int64, id uuid.UUID) (db.Service, []db.Warranty, error) {
	if brandID <= 0 || userID <= 0 {
		return db.Service{}, nil, ErrNotFound
	}
	svc, err := s.q.GetServiceByUUID(ctx, db.GetServiceByUUIDParams{Uuid: id, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Service{}, nil, ErrNotFound
	}
	if err != nil {
		return db.Service{}, nil, fmt.Errorf("services: portal get: %w", err)
	}
	warranties, err := s.q.ListWarrantiesByService(ctx, db.ListWarrantiesByServiceParams{ServiceID: svc.ID, BrandID: svc.BrandID})
	if err != nil {
		return db.Service{}, nil, fmt.Errorf("services: portal warranties: %w", err)
	}
	if !portalOwns(svc, userID, warranties) {
		return db.Service{}, nil, ErrNotFound
	}
	return svc, warranties, nil
}

// PortalAuthorize checks a portal request for a service (ErrNotFound when
// the user does not own it in the brand, nor sees it through their fleet).
func (s *Service) PortalAuthorize(ctx context.Context, brandID, userID int64, id uuid.UUID) (db.Service, error) {
	svc, _, _, err := s.portalViewable(ctx, brandID, userID, id)
	return svc, err
}

// portalItem is one loaded service item for the portal view.
type portalItem struct {
	ID       int64
	UUID     uuid.UUID
	Product  string
	Category string
	Parts    []string
}

// PortalGet returns the portal detail of a service the user owns.
func (s *Service) PortalGet(ctx context.Context, brandID, userID int64, id uuid.UUID) (PortalServiceView, error) {
	svc, warranties, holder, err := s.portalViewable(ctx, brandID, userID, id)
	if err != nil {
		return PortalServiceView{}, err
	}
	refs, err := s.q.GetServiceRefs(ctx, svc.ID)
	if err != nil {
		return PortalServiceView{}, fmt.Errorf("services: portal refs: %w", err)
	}
	org, err := s.q.GetOrganizationByID(ctx, svc.OrganizationID)
	if err != nil {
		return PortalServiceView{}, fmt.Errorf("services: portal organization: %w", err)
	}
	rows, err := s.q.ListServiceItems(ctx, svc.ID)
	if err != nil {
		return PortalServiceView{}, fmt.Errorf("services: portal items: %w", err)
	}
	categories := map[int64]string{}
	items := make([]portalItem, 0, len(rows))
	for _, it := range rows {
		p, err := s.q.GetProduct(ctx, db.GetProductParams{ID: it.ProductID, BrandID: svc.BrandID})
		if err != nil {
			return PortalServiceView{}, fmt.Errorf("services: portal product: %w", err)
		}
		cat, ok := categories[p.CategoryID]
		if !ok {
			c, err := s.q.GetProductCategory(ctx, db.GetProductCategoryParams{ID: p.CategoryID, BrandID: svc.BrandID})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return PortalServiceView{}, fmt.Errorf("services: portal category: %w", err)
			}
			cat = c.Name
			categories[p.CategoryID] = cat
		}
		parts := []string{}
		_ = json.Unmarshal(it.AppliedParts, &parts)
		items = append(items, portalItem{ID: it.ID, UUID: it.Uuid, Product: p.Name, Category: cat, Parts: parts})
	}
	loc := i18n.FromContext(ctx).Locale
	return portalView(svc, refs, org, items, warranties, holder, loc), nil
}

// portalView maps the loaded data to the portal projection (pure).
func portalView(svc db.Service, refs db.GetServiceRefsRow, org db.Organization, items []portalItem,
	warranties []db.Warranty, userID int64, loc i18n.Locale,
) PortalServiceView {
	v := PortalServiceView{
		UUID: svc.Uuid, ServiceNo: svc.ServiceNo, Status: svc.Status,
		StatusLabel: i18n.Translate(loc, StatusLabelKey(svc.Status)),
		VehicleUUID: refs.VehicleUuid,
		CarBrand:    Ref{UUID: refs.CarBrandUuid, Name: refs.CarBrandName},
		CarModel:    Ref{UUID: refs.CarModelUuid, Name: refs.CarModelName},
		Plate:       textPtr(svc.Plate),
		CreatedAt:   svc.CreatedAt.Time, CompletedAt: tsPtr(svc.CompletedAt),
		AppliedParts: []string{},
		Products:     make([]PortalProductView, 0, len(items)),
		Dealer: PortalDealerView{UUID: org.Uuid, Name: org.Name, City: org.City, District: org.District,
			Address: org.Address},
		Warranties: []PortalWarrantyView{},
	}
	if svc.ModelYear.Valid {
		y := svc.ModelYear.Int16
		v.ModelYear = &y
	}
	// organizations.phone is E.164 or empty (migration 000048 moved
	// anything else to phone_raw).
	if ph := strings.TrimSpace(org.Phone); strings.HasPrefix(ph, "+") {
		v.Dealer.WhatsApp = &ph
	}
	seen := map[string]bool{}
	byID := make(map[int64]portalItem, len(items))
	for _, it := range items {
		byID[it.ID] = it
		parts := it.Parts
		if parts == nil {
			parts = []string{}
		}
		v.Products = append(v.Products, PortalProductView{ServiceItemUUID: it.UUID, Name: it.Product,
			Category: it.Category, AppliedParts: parts})
		for _, p := range parts {
			if !seen[p] {
				seen[p] = true
				v.AppliedParts = append(v.AppliedParts, p)
			}
		}
	}
	sort.Strings(v.AppliedParts)
	for _, w := range warranties {
		if w.HolderUserID != userID {
			continue
		}
		it := byID[w.ServiceItemID]
		v.Warranties = append(v.Warranties, PortalWarrantyView{
			UUID: w.Uuid, PublicCode: w.PublicCode, ServiceItemUUID: it.UUID, ProductName: it.Product,
			ItemKind: w.ItemKind, Status: w.Status, StartAt: w.StartAt.Time, EndAt: w.EndAt.Time,
			ExpiredAt: tsPtr(w.ExpiredAt), VoidedAt: tsPtr(w.VoidedAt),
		})
	}
	return v
}

// --- Portal PDF ------------------------------------------------------------------

// PortalAuthorize checks a portal PDF request (ErrNotFound when the user
// does not own the service in the brand).
func (p *PDFService) PortalAuthorize(ctx context.Context, brandID, userID int64, id uuid.UUID) (db.Service, error) {
	return p.svc.PortalAuthorize(ctx, brandID, userID, id)
}

// BuildPortal loads the PDF of a portal export job: it re-checks that the
// job's user still owns the service and prints only the warranties they
// hold. The customer phone stays masked as on the panel PDF.
func (p *PDFService) BuildPortal(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (PDFDoc, error) {
	id, err := uuid.Parse(strings.TrimSpace(q[PDFQueryServiceUUID]))
	if err != nil {
		return PDFDoc{}, ErrNotFound
	}
	brandID, err := strconv.ParseInt(strings.TrimSpace(q[PDFQueryBrandID]), 10, 64)
	if err != nil || brandID <= 0 {
		return PDFDoc{}, errors.New("services pdf: brand is required")
	}
	userID, err := strconv.ParseInt(strings.TrimSpace(q[PDFQueryPortalUserID]), 10, 64)
	if err != nil || userID <= 0 {
		return PDFDoc{}, errPDFScope
	}
	svc, warranties, holder, err := p.svc.portalViewable(ctx, brandID, userID, id)
	if err != nil {
		return PDFDoc{}, err
	}
	d, err := p.doc(ctx, svc, loc)
	if err != nil {
		return PDFDoc{}, err
	}
	held := map[string]bool{}
	for _, w := range warranties {
		if w.HolderUserID == holder {
			held[w.PublicCode] = true
		}
	}
	kept := d.Warranties[:0]
	for _, w := range d.Warranties {
		if held[w.PublicCode] {
			kept = append(kept, w)
		}
	}
	d.Warranties = kept
	return d, nil
}

// PortalPDFAdapter exports the service PDF requested from the portal.
type PortalPDFAdapter struct{ *PDFAdapter }

// NewPortalPDFAdapter builds the portal export adapter.
func NewPortalPDFAdapter(svc *PDFService) *PortalPDFAdapter {
	return &PortalPDFAdapter{PDFAdapter: &PDFAdapter{svc: svc}}
}

// Resource implements ioengine.ResourceAdapter.
func (a *PortalPDFAdapter) Resource() string { return ResourcePortalPDF }

// Export implements ioengine.ResourceAdapter. Query: service_uuid,
// brand_id and portal_user_id; portal jobs never carry an organization.
func (a *PortalPDFAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	delete(q, ioengine.QueryOrganizationID)
	d, err := a.svc.BuildPortal(ctx, q, loc)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	ds := PDFDataset(d, loc)
	ds.Resource = ResourcePortalPDF
	return ds, nil
}

var (
	_ ioengine.ResourceAdapter  = (*PortalPDFAdapter)(nil)
	_ ioengine.DocumentRenderer = (*PortalPDFAdapter)(nil)
)
