package usecase

// TEC-209: Meilisearch vehicles index (plate, normalized plate, VIN). A
// document carries the vehicle brand, the organizations the owner is linked
// to in that brand (customer_organizations, the same rule the list applies)
// and the owner as filterable fields. GET /v1/vehicles?q= filters the index
// on the caller's vehicles.read scope and the domain brand (K20) and then
// reloads the hits from Postgres with the same scope. Deleted vehicles and
// vehicles of anonymized, merged or deleted owners never enter the index
// (K19): Document answers searchengine.ErrSkipDocument.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// VehicleSearchSpec is the Meilisearch spec (index suffix) of vehicles.
const VehicleSearchSpec = searchengine.SpecVehicles

// VehicleIndexStore is what the vehicles search adapter reads.
type VehicleIndexStore interface {
	ListVehiclesForIndex(ctx context.Context) ([]db.ListVehiclesForIndexRow, error)
	GetVehicleForIndex(ctx context.Context, id uuid.UUID) (db.GetVehicleForIndexRow, error)
}

// VehicleSearchAdapter indexes vehicles in Meilisearch.
type VehicleSearchAdapter struct{ q VehicleIndexStore }

// NewVehicleSearchAdapter creates the vehicles search adapter.
func NewVehicleSearchAdapter(q VehicleIndexStore) *VehicleSearchAdapter {
	return &VehicleSearchAdapter{q: q}
}

// Spec implements searchengine.Adapter.
func (a *VehicleSearchAdapter) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         VehicleSearchSpec,
		LabelKey:   "search.specs_vehicles",
		Permission: rbac.PermVehiclesRead,
		Icon:       "car",
		Searchable: []string{"title", "subtitle", "keywords"},
		Filterable: []string{"organization_ids", "brand_ids", "customer_user_id"},
		ListScoped: true,
	}
}

// ListAll implements searchengine.Adapter.
func (a *VehicleSearchAdapter) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	rows, err := a.q.ListVehiclesForIndex(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]searchengine.Document, 0, len(rows))
	for _, r := range rows {
		out = append(out, vehicleDocument(db.GetVehicleForIndexRow(r)))
	}
	return out, nil
}

// Document implements searchengine.Adapter.
func (a *VehicleSearchAdapter) Document(ctx context.Context, id string) (searchengine.Document, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("vehicles search: invalid uuid")
	}
	row, err := a.q.GetVehicleForIndex(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return searchengine.Document{}, searchengine.ErrSkipDocument
	}
	if err != nil {
		return searchengine.Document{}, err
	}
	return vehicleDocument(row), nil
}

func vehicleDocument(r db.GetVehicleForIndexRow) searchengine.Document {
	car := strings.TrimSpace(r.CarBrandName.String + " " + r.CarModelName.String)
	if r.ModelYear.Valid {
		car = strings.TrimSpace(car + " " + strconv.Itoa(int(r.ModelYear.Int16)))
	}
	title := r.Plate.String
	if strings.TrimSpace(title) == "" {
		title = r.Vin.String
	}
	owner := strings.TrimSpace(r.OwnerName + " " + r.OwnerSurname)
	doc := searchengine.Document{
		ID:              r.Uuid.String(),
		Spec:            VehicleSearchSpec,
		Title:           title,
		Subtitle:        strings.Join(searchengine.Keywords(car, owner), " · "),
		Href:            "/vehicles/" + r.Uuid.String(),
		Icon:            "car",
		OrganizationIDs: r.OrganizationIds,
		BrandIDs:        []int64{r.BrandID},
		CustomerUserID:  r.UserID,
		Keywords: searchengine.Keywords(
			r.Plate.String, r.PlateNormalized.String, geo.NormalizePlate(r.Plate.String), r.Vin.String,
			car, owner, r.OwnerPhone.String,
		),
	}
	if doc.OrganizationIDs == nil {
		doc.OrganizationIDs = []int64{}
	}
	if p := r.OwnerPhone.String; strings.HasPrefix(p, "+") && len(p) > 4 {
		doc.Keywords = append(doc.Keywords, strings.TrimPrefix(p, "+"))
	}
	return doc
}

// vehicleIndexFilter is the Meilisearch filter of the caller's vehicles
// scope: the domain brand (K20), the organizations of an organization
// relative scope and the owner filter. ok is false when the scope reaches
// no vehicle.
func vehicleIndexFilter(c Caller, userID pgtype.Int8) (string, bool) {
	if c.Org.BrandID == 0 {
		return "", false
	}
	f := (&searchengine.Filter{}).Eq("brand_ids", c.Org.BrandID)
	if ids := c.orgIDs(); ids != nil {
		if len(ids) == 0 {
			return "", false
		}
		f.In("organization_ids", ids)
	}
	if userID.Valid {
		f.Eq("customer_user_id", userID.Int64)
	}
	return f.String(), true
}

// searchVehiclesIndexed answers a q search from the index: the hits are
// reloaded through ListScopedVehicles with the same scope. handled is false
// when the caller must fall back to the SQL search.
func (s *Service) searchVehiclesIndexed(ctx context.Context, c Caller, p db.ListScopedVehiclesParams, q string) ([]db.ListScopedVehiclesRow, int64, bool) {
	filter, ok := vehicleIndexFilter(c, p.UserID)
	if !ok {
		return []db.ListScopedVehiclesRow{}, 0, true
	}
	ids, total, err := s.finder.SearchIDs(ctx, VehicleSearchSpec, q, filter, int(p.LimitCount), int(p.OffsetCount))
	if err != nil {
		return nil, 0, false
	}
	uuids, rank := searchengine.ParseUUIDs(ids)
	if len(uuids) == 0 {
		return []db.ListScopedVehiclesRow{}, total, true
	}
	p.Q, p.QName = pgtype.Text{}, pgtype.Text{}
	p.Uuids = uuids
	p.LimitCount, p.OffsetCount = int32(len(uuids)), 0
	rows, err := s.q.ListScopedVehicles(ctx, p)
	if err != nil {
		return nil, 0, false
	}
	return searchengine.Reorder(rows, func(r db.ListScopedVehiclesRow) uuid.UUID { return r.Vehicle.Uuid }, rank), total, true
}

// indexCustomerRecords refreshes the services, vehicles and warranties
// documents of a customer (name / phone / organization links changed, or
// the person was anonymized). Fail-soft, async.
func (s *Service) indexCustomerRecords(ctx context.Context, id uuid.UUID) {
	if s.search == nil || s.q == nil || id == uuid.Nil {
		return
	}
	row, err := s.q.ListSearchUuidsByCustomer(ctx, id)
	if err != nil {
		return
	}
	for _, u := range row.ServiceUuids {
		s.search.EnqueueUpsert(ctx, searchengine.SpecServices, u.String())
	}
	for _, u := range row.VehicleUuids {
		s.search.EnqueueUpsert(ctx, VehicleSearchSpec, u.String())
	}
	for _, u := range row.WarrantyUuids {
		s.search.EnqueueUpsert(ctx, searchengine.SpecWarranties, u.String())
	}
}

var _ searchengine.Adapter = (*VehicleSearchAdapter)(nil)
