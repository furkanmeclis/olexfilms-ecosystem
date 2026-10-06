package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Vehicle warnings (VIN and plate are not unique; ownership transfer is
// F1-06).
const (
	WarningVINDuplicate   = "vin_duplicate"
	WarningPlateDuplicate = "plate_duplicate"
)

const maxPlateLen = 20

// maxVehicleQueryLen bounds the vehicles list q (TEC-209).
const maxVehicleQueryLen = 100

// CatalogRef is a car brand or model of the vehicle catalog.
type CatalogRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// VehicleView is a vehicle of a customer in scope.
type VehicleView struct {
	UUID             uuid.UUID   `json:"uuid"`
	CustomerUUID     uuid.UUID   `json:"customer_uuid"`
	OrganizationUUID *uuid.UUID  `json:"organization_uuid"`
	Plate            *string     `json:"plate"`
	PlateNormalized  *string     `json:"plate_normalized"`
	PlateCountry     *string     `json:"plate_country"`
	VIN              *string     `json:"vin"`
	ModelYear        *int        `json:"model_year"`
	CarBrand         *CatalogRef `json:"car_brand"`
	CarModel         *CatalogRef `json:"car_model"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
	Warnings         []string    `json:"warnings"`
}

// CreateVehicleInput is POST /v1/vehicles.
type CreateVehicleInput struct {
	CustomerUUID uuid.UUID  `json:"customer_uuid"`
	Plate        string     `json:"plate"`
	PlateCountry string     `json:"plate_country"`
	CarBrandUUID *uuid.UUID `json:"car_brand_uuid"`
	CarModelUUID *uuid.UUID `json:"car_model_uuid"`
	ModelYear    *int       `json:"model_year"`
	VIN          string     `json:"vin"`
}

// UpdateVehicleInput is PATCH /v1/vehicles/{uuid}: absent keys keep the
// stored value, null clears it (the plate cannot be cleared).
type UpdateVehicleInput struct {
	Plate        Optional[string]    `json:"plate"`
	PlateCountry Optional[string]    `json:"plate_country"`
	CarBrandUUID Optional[uuid.UUID] `json:"car_brand_uuid"`
	CarModelUUID Optional[uuid.UUID] `json:"car_model_uuid"`
	ModelYear    Optional[int]       `json:"model_year"`
	VIN          Optional[string]    `json:"vin"`
}

// VehicleFilter filters GET /v1/vehicles.
type VehicleFilter struct {
	CustomerUUID *uuid.UUID
	Plate        string
	VIN          string
	// Q (TEC-209) searches plate / VIN: the vehicles index when it is up,
	// otherwise a prefix of the normalized plate or the VIN.
	Q string
	// TEC-371: car brand / model and organization (owner link) filters and
	// the sort (ParseVehicleFilter). SortExplicit keeps a q search on SQL.
	CarBrandUUIDs     []uuid.UUID
	CarModelUUIDs     []uuid.UUID
	OrganizationUUIDs []uuid.UUID
	Sort              apiquery.ResolvedSort
	SortExplicit      bool
	Limit             int32
	Offset            int32
}

// ListVehicles lists the vehicles of customers in scope.
func (s *Service) ListVehicles(ctx context.Context, c Caller, f VehicleFilter) ([]VehicleView, int64, error) {
	if c.Org.BrandID == 0 {
		return nil, 0, ErrForbidden
	}
	var userID pgtype.Int8
	if f.CustomerUUID != nil {
		u, err := s.scopedUser(ctx, s.q, c, *f.CustomerUUID)
		if err != nil {
			return nil, 0, err
		}
		userID = pgtype.Int8{Int64: u.ID, Valid: true}
	}
	plate := geo.NormalizePlate(f.Plate)
	if utf8.RuneCountInString(plate) > maxPlateLen {
		return nil, 0, invalid("plate", "must be at most 20 characters")
	}
	vin := ""
	if strings.TrimSpace(f.VIN) != "" {
		var err error
		if vin, err = NormalizeVIN(f.VIN); err != nil {
			return nil, 0, err
		}
	}
	rawQ := strings.TrimSpace(f.Q)
	if utf8.RuneCountInString(rawQ) > maxVehicleQueryLen {
		return nil, 0, invalid("q", "must be at most 100 characters")
	}
	// SQL fallback: the compact form is a prefix of the normalized plate or
	// of the VIN (both upper case, no separators); LIKE wildcards dropped.
	qNorm := strings.NewReplacer("%", "", `\`, "").Replace(geo.NormalizePlate(rawQ))
	// TEC-371: q also matches the car brand / model name ("BMW 3").
	qName := strings.NewReplacer("%", "", `\`, "").Replace(rawQ)
	sort := sortKey(f.Sort, VehiclesSortSpec)
	p := db.ListScopedVehiclesParams{
		BrandID: c.Org.BrandID, UserID: userID, OrgIds: c.orgIDs(), OrganizationUuids: f.OrganizationUUIDs,
		PlateNormalized: text(plate), Vin: text(vin), Q: text(qNorm), QName: text(strings.TrimSpace(qName)),
		CarBrandUuids: f.CarBrandUUIDs, CarModelUuids: f.CarModelUUIDs,
		SortKey: sort.Key, SortDesc: sort.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	var (
		rows    []db.ListScopedVehiclesRow
		total   int64
		indexed bool
	)
	// TEC-209: a q search goes to the vehicles index when it is up; the
	// exact plate / VIN filters stay on SQL.
	// TEC-371: an explicit sort or a brand / model / organization filter
	// stays on SQL (the index answers in relevance order).
	if p.Q.Valid && !p.PlateNormalized.Valid && !p.Vin.Valid && !f.SortExplicit && len(f.CarBrandUUIDs) == 0 &&
		len(f.CarModelUUIDs) == 0 && len(f.OrganizationUUIDs) == 0 && s.indexEnabled() {
		rows, total, indexed = s.searchVehiclesIndexed(ctx, c, p, rawQ)
	}
	if !indexed {
		var err error
		rows, err = s.q.ListScopedVehicles(ctx, p)
		if err != nil {
			return nil, 0, fmt.Errorf("customers: list vehicles: %w", err)
		}
		total, err = s.q.CountScopedVehicles(ctx, db.CountScopedVehiclesParams{
			BrandID: c.Org.BrandID, UserID: userID, OrgIds: c.orgIDs(), OrganizationUuids: p.OrganizationUuids,
			PlateNormalized: text(plate), Vin: text(vin), Q: p.Q, QName: p.QName,
			CarBrandUuids: p.CarBrandUuids, CarModelUuids: p.CarModelUuids,
		})
		if err != nil {
			return nil, 0, fmt.Errorf("customers: count vehicles: %w", err)
		}
	}
	out := make([]VehicleView, 0, len(rows))
	for _, r := range rows {
		out = append(out, vehicleView(r.Vehicle, r.CustomerUuid, r.OrganizationUuid,
			r.CarBrandUuid, r.CarBrandName, r.CarModelUuid, r.CarModelName))
	}
	return out, total, nil
}

// GetVehicle returns a vehicle whose customer is in scope (404 otherwise).
func (s *Service) GetVehicle(ctx context.Context, c Caller, id uuid.UUID) (VehicleView, error) {
	if _, err := s.scopedVehicle(ctx, s.q, c, id, false); err != nil {
		return VehicleView{}, err
	}
	return s.vehicleByUUID(ctx, s.q, id)
}

// CreateVehicle registers a vehicle for a customer in scope. The plate is
// validated against its country's format (geo.ValidatePlate); the plate
// country defaults to the organization's country (TR when unknown).
func (s *Service) CreateVehicle(ctx context.Context, c Caller, in CreateVehicleInput) (VehicleView, error) {
	if c.Org.InternalID == 0 || !c.Filter.AllowsOrg(c.Org.InternalID, c.Org.BrandID) {
		return VehicleView{}, ErrForbidden
	}
	if in.CustomerUUID == uuid.Nil {
		return VehicleView{}, invalid("customer_uuid", "is required")
	}
	plate, err := s.resolvePlate(ctx, c, in.Plate, in.PlateCountry)
	if err != nil {
		return VehicleView{}, err
	}
	vin, err := NormalizeVIN(in.VIN)
	if err != nil {
		return VehicleView{}, err
	}
	var year pgtype.Int2
	if in.ModelYear != nil {
		if err := validateModelYear(*in.ModelYear, time.Now().UTC().Year()); err != nil {
			return VehicleView{}, err
		}
		year = pgtype.Int2{Int16: int16(*in.ModelYear), Valid: true}
	}
	brandID, modelID, err := s.resolveCar(ctx, in.CarBrandUUID, in.CarModelUUID)
	if err != nil {
		return VehicleView{}, err
	}
	var row db.Vehicle
	err = s.inTxRaw(ctx, func(q *db.Queries, tx pgx.Tx) error {
		user, err := s.scopedUser(ctx, q, c, in.CustomerUUID)
		if err != nil {
			return err
		}
		if err := writableUser(user); err != nil {
			return err
		}
		row, err = q.CreateVehicle(ctx, db.CreateVehicleParams{
			UserID: user.ID, OrganizationID: pgtype.Int8{Int64: c.Org.InternalID, Valid: true}, BrandID: c.Org.BrandID,
			CarBrandID: brandID, CarModelID: modelID, ModelYear: year,
			Plate: text(plate.plate), PlateNormalized: text(plate.normalized), PlateCountry: text(plate.country),
			Vin: text(vin),
		})
		if err != nil {
			return err
		}
		return s.vehicleEvent(ctx, tx, c, events.VehicleCreated, row)
	})
	if err != nil {
		return VehicleView{}, err
	}
	return s.viewWithWarnings(ctx, row)
}

// UpdateVehicle edits a vehicle whose customer is in scope.
func (s *Service) UpdateVehicle(ctx context.Context, c Caller, id uuid.UUID, in UpdateVehicleInput) (VehicleView, error) {
	if in.Plate.Set && (in.Plate.Value == nil || strings.TrimSpace(*in.Plate.Value) == "") {
		return VehicleView{}, invalid("plate", "is required")
	}
	var row db.Vehicle
	err := s.inTxRaw(ctx, func(q *db.Queries, tx pgx.Tx) error {
		cur, err := s.scopedVehicle(ctx, q, c, id, true)
		if err != nil {
			return err
		}
		owner, err := q.GetUserByID(ctx, cur.UserID)
		if err != nil {
			return fmt.Errorf("customers: vehicle owner: %w", err)
		}
		if err := writableUser(owner); err != nil {
			return err
		}
		p := db.UpdateVehicleParams{
			ID: cur.ID, CarBrandID: cur.CarBrandID, CarModelID: cur.CarModelID, ModelYear: cur.ModelYear,
			Plate: cur.Plate, PlateNormalized: cur.PlateNormalized, PlateCountry: cur.PlateCountry, Vin: cur.Vin,
		}
		if in.Plate.Set || in.PlateCountry.Set {
			raw := cur.Plate.String
			if in.Plate.Set {
				raw = *in.Plate.Value
			}
			country := cur.PlateCountry.String
			if in.PlateCountry.Set {
				country = ""
				if in.PlateCountry.Value != nil {
					country = *in.PlateCountry.Value
				}
			}
			if strings.TrimSpace(raw) == "" {
				return invalid("plate", "is required")
			}
			pl, err := s.resolvePlate(ctx, c, raw, country)
			if err != nil {
				return err
			}
			p.Plate, p.PlateNormalized, p.PlateCountry = text(pl.plate), text(pl.normalized), text(pl.country)
		}
		if in.VIN.Set {
			raw := ""
			if in.VIN.Value != nil {
				raw = *in.VIN.Value
			}
			vin, err := NormalizeVIN(raw)
			if err != nil {
				return err
			}
			p.Vin = text(vin)
		}
		if in.ModelYear.Set {
			p.ModelYear = pgtype.Int2{}
			if in.ModelYear.Value != nil {
				if err := validateModelYear(*in.ModelYear.Value, time.Now().UTC().Year()); err != nil {
					return err
				}
				p.ModelYear = pgtype.Int2{Int16: int16(*in.ModelYear.Value), Valid: true}
			}
		}
		if p.CarBrandID, p.CarModelID, err = s.resolveCarPatch(ctx, cur.CarBrandID, cur.CarModelID,
			in.CarBrandUUID, in.CarModelUUID); err != nil {
			return err
		}
		row, err = q.UpdateVehicle(ctx, p)
		if err != nil {
			return err
		}
		return s.vehicleEvent(ctx, tx, c, events.VehicleUpdated, row)
	})
	if err != nil {
		return VehicleView{}, err
	}
	return s.viewWithWarnings(ctx, row)
}

// DeleteVehicle soft-deletes a vehicle whose customer is in scope.
func (s *Service) DeleteVehicle(ctx context.Context, c Caller, id uuid.UUID) error {
	return s.inTxRaw(ctx, func(q *db.Queries, tx pgx.Tx) error {
		cur, err := s.scopedVehicle(ctx, q, c, id, true)
		if err != nil {
			return err
		}
		owner, err := q.GetUserByID(ctx, cur.UserID)
		if err != nil {
			return fmt.Errorf("customers: vehicle owner: %w", err)
		}
		if err := writableUser(owner); err != nil {
			return err
		}
		n, err := q.SoftDeleteVehicle(ctx, cur.ID)
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrVehicleNotFound
		}
		return s.vehicleEvent(ctx, tx, c, events.VehicleDeleted, cur)
	})
}

// vehicleEvent writes a vehicle record event to the outbox (TEC-209: the
// search sync refreshes the vehicles document). No personal data in the
// payload. A service without an outbox (exports, tests) writes none.
func (s *Service) vehicleEvent(ctx context.Context, tx pgx.Tx, c Caller, name string, v db.Vehicle) error {
	if s.out == nil {
		return nil
	}
	id, u := v.ID, v.Uuid
	ev := events.New(name).WithTenant(c.Org.InternalID).WithEntity("vehicle", &id, &u).
		WithPayload(map[string]any{"vehicle_uuid": v.Uuid.String(), "brand_id": v.BrandID, "user_id": v.UserID})
	if c.UserID != 0 {
		ev = ev.WithActor(c.UserID)
	}
	if err := s.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("customers: outbox: %w", err)
	}
	return nil
}

// scopedVehicle loads a vehicle of the domain brand whose customer is in
// scope; anything else reads as not found.
func (s *Service) scopedVehicle(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID, lock bool) (db.Vehicle, error) {
	var (
		v   db.Vehicle
		err error
	)
	if lock {
		v, err = q.GetVehicleByUUIDForUpdate(ctx, id)
	} else {
		v, err = q.GetVehicleByUUID(ctx, id)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Vehicle{}, ErrVehicleNotFound
	}
	if err != nil {
		return db.Vehicle{}, fmt.Errorf("customers: vehicle: %w", err)
	}
	if c.Org.BrandID == 0 || v.BrandID != c.Org.BrandID {
		return db.Vehicle{}, ErrVehicleNotFound
	}
	if c.portalUserID != 0 { // TEC-243: portal, own vehicles only
		if v.UserID != c.portalUserID {
			return db.Vehicle{}, ErrVehicleNotFound
		}
		return v, nil
	}
	if err := s.requireInScope(ctx, q, c, v.UserID); err != nil {
		if errors.Is(err, ErrCustomerNotFound) {
			return db.Vehicle{}, ErrVehicleNotFound
		}
		return db.Vehicle{}, err
	}
	return v, nil
}

type resolvedPlate struct {
	plate, normalized, country string
}

// resolvePlate validates a plate with geo.ValidatePlate. The country
// defaults to the organization's country, then TR.
func (s *Service) resolvePlate(ctx context.Context, c Caller, raw, country string) (resolvedPlate, error) {
	plate := strings.ToUpper(strings.Join(strings.Fields(raw), " "))
	if plate == "" {
		return resolvedPlate{}, invalid("plate", "is required")
	}
	if utf8.RuneCountInString(plate) > maxPlateLen {
		return resolvedPlate{}, invalid("plate", "must be at most 20 characters")
	}
	country = strings.ToUpper(strings.TrimSpace(country))
	if country == "" {
		iso2, err := s.q.GetOrganizationCountryISO2(ctx, c.Org.InternalID)
		if err != nil {
			return resolvedPlate{}, fmt.Errorf("customers: organization country: %w", err)
		}
		country = strings.ToUpper(iso2)
		if country == "" {
			country = "TR"
		}
	}
	if s.plates == nil {
		return resolvedPlate{}, invalid("plate_country", "plate validation is not configured")
	}
	check, err := s.plates.ValidatePlate(ctx, country, plate)
	switch {
	case errors.Is(err, geo.ErrInvalidPlate):
		return resolvedPlate{}, &InvalidPlateError{Country: country}
	case errors.Is(err, geo.ErrNotFound), errors.Is(err, geo.ErrInvalid):
		return resolvedPlate{}, invalid("plate_country", "has no active plate format")
	case err != nil:
		return resolvedPlate{}, fmt.Errorf("customers: plate: %w", err)
	}
	return resolvedPlate{plate: plate, normalized: check.Normalized, country: check.Country}, nil
}

// resolveCar maps car brand/model UUIDs of the vehicle catalog to ids. A
// model alone implies its brand; a model of another brand is refused.
func (s *Service) resolveCar(ctx context.Context, brandUUID, modelUUID *uuid.UUID) (pgtype.Int8, pgtype.Int8, error) {
	var brandID, modelID pgtype.Int8
	if brandUUID != nil {
		b, err := s.q.GetCarBrandByUUID(ctx, *brandUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return brandID, modelID, invalid("car_brand_uuid", "car brand not found")
		}
		if err != nil {
			return brandID, modelID, fmt.Errorf("customers: car brand: %w", err)
		}
		if !b.Active {
			return brandID, modelID, invalid("car_brand_uuid", "car brand is not active")
		}
		brandID = pgtype.Int8{Int64: b.ID, Valid: true}
	}
	if modelUUID != nil {
		m, err := s.q.GetCarModelByUUID(ctx, *modelUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return brandID, modelID, invalid("car_model_uuid", "car model not found")
		}
		if err != nil {
			return brandID, modelID, fmt.Errorf("customers: car model: %w", err)
		}
		if !m.Active {
			return brandID, modelID, invalid("car_model_uuid", "car model is not active")
		}
		if brandID.Valid && m.CarBrandID != brandID.Int64 {
			return brandID, modelID, invalid("car_model_uuid", "car model does not belong to the car brand")
		}
		if !brandID.Valid {
			b, err := s.q.GetCarBrandByID(ctx, m.CarBrandID)
			if err != nil {
				return brandID, modelID, fmt.Errorf("customers: car brand: %w", err)
			}
			if !b.Active {
				return brandID, modelID, invalid("car_brand_uuid", "car brand is not active")
			}
			brandID = pgtype.Int8{Int64: b.ID, Valid: true}
		}
		modelID = pgtype.Int8{Int64: m.ID, Valid: true}
	}
	return brandID, modelID, nil
}

// resolveCarPatch applies a PATCH of car brand/model: a new brand drops a
// model of another brand, a model alone switches to its brand, null clears
// (clearing the brand also clears the model).
func (s *Service) resolveCarPatch(ctx context.Context, curBrand, curModel pgtype.Int8,
	brand, model Optional[uuid.UUID],
) (pgtype.Int8, pgtype.Int8, error) {
	newBrand, newModel := curBrand, curModel
	if brand.Set {
		if brand.Value == nil {
			newBrand, newModel = pgtype.Int8{}, pgtype.Int8{}
		} else {
			id, _, err := s.resolveCar(ctx, brand.Value, nil)
			if err != nil {
				return curBrand, curModel, err
			}
			if id.Int64 != curBrand.Int64 || !curBrand.Valid {
				newModel = pgtype.Int8{}
			}
			newBrand = id
		}
	}
	if model.Set {
		if model.Value == nil {
			newModel = pgtype.Int8{}
		} else {
			var brandArg *uuid.UUID
			if brand.Set {
				brandArg = brand.Value
			}
			b, m, err := s.resolveCar(ctx, brandArg, model.Value)
			if err != nil {
				return curBrand, curModel, err
			}
			newBrand, newModel = b, m
		}
	}
	return newBrand, newModel, nil
}

// viewWithWarnings loads the API view of a written vehicle and warns about a
// VIN or plate already registered on another vehicle of the brand.
func (s *Service) viewWithWarnings(ctx context.Context, v db.Vehicle) (VehicleView, error) {
	view, err := s.vehicleByUUID(ctx, s.q, v.Uuid)
	if err != nil {
		return VehicleView{}, err
	}
	brand := pgtype.Int8{Int64: v.BrandID, Valid: true}
	if v.Vin.Valid {
		others, err := s.q.FindVehiclesByVIN(ctx, db.FindVehiclesByVINParams{Vin: v.Vin, BrandID: brand})
		if err != nil {
			return VehicleView{}, fmt.Errorf("customers: vin lookup: %w", err)
		}
		for _, o := range others {
			if o.ID != v.ID {
				view.Warnings = append(view.Warnings, WarningVINDuplicate)
				break
			}
		}
	}
	if v.PlateNormalized.Valid {
		others, err := s.q.FindVehiclesByPlate(ctx, db.FindVehiclesByPlateParams{
			PlateCountry: v.PlateCountry, PlateNormalized: v.PlateNormalized, BrandID: brand,
		})
		if err != nil {
			return VehicleView{}, fmt.Errorf("customers: plate lookup: %w", err)
		}
		for _, o := range others {
			if o.ID != v.ID {
				view.Warnings = append(view.Warnings, WarningPlateDuplicate)
				break
			}
		}
	}
	return view, nil
}

func (s *Service) vehicleByUUID(ctx context.Context, q *db.Queries, id uuid.UUID) (VehicleView, error) {
	r, err := q.GetVehicleViewByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return VehicleView{}, ErrVehicleNotFound
	}
	if err != nil {
		return VehicleView{}, fmt.Errorf("customers: vehicle view: %w", err)
	}
	return vehicleView(r.Vehicle, r.CustomerUuid, r.OrganizationUuid,
		r.CarBrandUuid, r.CarBrandName, r.CarModelUuid, r.CarModelName), nil
}

func vehicleView(v db.Vehicle, customer uuid.UUID, org pgtype.UUID,
	brandUUID pgtype.UUID, brandName pgtype.Text, modelUUID pgtype.UUID, modelName pgtype.Text,
) VehicleView {
	out := VehicleView{
		UUID: v.Uuid, CustomerUUID: customer, Plate: strOrNil(v.Plate), PlateNormalized: strOrNil(v.PlateNormalized),
		PlateCountry: strOrNil(v.PlateCountry), VIN: strOrNil(v.Vin),
		CreatedAt: v.CreatedAt.Time, UpdatedAt: v.UpdatedAt.Time, Warnings: []string{},
	}
	if org.Valid {
		id := uuid.UUID(org.Bytes)
		out.OrganizationUUID = &id
	}
	if v.ModelYear.Valid {
		y := int(v.ModelYear.Int16)
		out.ModelYear = &y
	}
	if brandUUID.Valid {
		out.CarBrand = &CatalogRef{UUID: uuid.UUID(brandUUID.Bytes), Name: brandName.String}
	}
	if modelUUID.Valid {
		out.CarModel = &CatalogRef{UUID: uuid.UUID(modelUUID.Bytes), Name: modelName.String}
	}
	return out
}
