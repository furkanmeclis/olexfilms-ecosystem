package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const maxPlateLen = 20

var vinRe = regexp.MustCompile(`^[A-HJ-NPR-Z0-9]{17}$`)

// Optional is a PATCH field: absent keeps the value, null clears it.
type Optional[T any] struct {
	Set   bool
	Value *T
}

// UnmarshalJSON records presence; null leaves Value nil.
func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// AddVehicleInput is POST /v1/fleets/{uuid}/vehicles: a new vehicle
// (plate required) or, with vehicle_uuid, an existing vehicle of the
// fleet's users linked to the fleet (the answer to FLEET_VEHICLE_EXISTS).
type AddVehicleInput struct {
	VehicleUUID  *uuid.UUID `json:"vehicle_uuid"`
	Plate        string     `json:"plate"`
	PlateCountry string     `json:"plate_country"`
	VIN          string     `json:"vin"`
	CarBrandUUID *uuid.UUID `json:"car_brand_uuid"`
	CarModelUUID *uuid.UUID `json:"car_model_uuid"`
	ModelYear    *int       `json:"model_year"`
}

// UpdateVehicleInput is PATCH /v1/fleets/{uuid}/vehicles/{vehicle_uuid}.
type UpdateVehicleInput struct {
	Plate        Optional[string]    `json:"plate"`
	PlateCountry Optional[string]    `json:"plate_country"`
	VIN          Optional[string]    `json:"vin"`
	CarBrandUUID Optional[uuid.UUID] `json:"car_brand_uuid"`
	CarModelUUID Optional[uuid.UUID] `json:"car_model_uuid"`
	ModelYear    Optional[int]       `json:"model_year"`
}

// VehicleFilter narrows GET /v1/fleets/{uuid}/vehicles.
type VehicleFilter struct {
	Q             string
	Sort          []apiquery.SortField
	Limit, Offset int32
}

// ListVehicles is GET /v1/fleets/{uuid}/vehicles. last_service_at and
// active_warranty_count count the caller's own work only.
func (s *Service) ListVehicles(ctx context.Context, c Caller, fleetUUID uuid.UUID, f VehicleFilter) ([]VehicleView, int64, error) {
	if _, err := apiquery.ResolveSort(f.Sort, repository.VehicleSort); err != nil {
		return nil, 0, err
	}
	repo := repository.New(s.conn)
	a, err := s.resolve(ctx, repo.Queries(), c, fleetUUID)
	if err != nil {
		return nil, 0, err
	}
	rows, total, err := repo.ListFleetVehicles(ctx, repository.VehicleFilter{
		FleetOrgID: a.orgID(), ServiceOrgIDs: c.orgIDs(), Q: f.Q, Sort: f.Sort, Limit: f.Limit, Offset: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]VehicleView, 0, len(rows))
	for _, r := range rows {
		out = append(out, vehicleRowView(r))
	}
	return out, total, nil
}

// vehicleData is a validated vehicle.
type vehicleData struct {
	plate, normalized, country, vin string
	brandID, modelID                pgtype.Int8
	year                            pgtype.Int2
}

// preparePlate validates a plate against its country's format; the country
// defaults to the organization's country, then TR.
func (s *Service) preparePlate(ctx context.Context, q *db.Queries, orgID int64, raw, country string) (string, string, string, error) {
	plate := strings.ToUpper(strings.Join(strings.Fields(raw), " "))
	if plate == "" {
		return "", "", "", &ValidationError{Field: "plate", Message: "is required"}
	}
	if utf8.RuneCountInString(plate) > maxPlateLen {
		return "", "", "", &ValidationError{Field: "plate", Message: "must be at most 20 characters"}
	}
	country = strings.ToUpper(strings.TrimSpace(country))
	if country == "" {
		iso2, err := q.GetOrganizationCountryISO2(ctx, orgID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", "", "", fmt.Errorf("fleet: organization country: %w", err)
		}
		if country = strings.ToUpper(iso2); country == "" {
			country = "TR"
		}
	}
	if s.plates == nil {
		return "", "", "", &ValidationError{Field: "plate_country", Message: "plate validation is not configured"}
	}
	check, err := s.plates.ValidatePlate(ctx, country, plate)
	switch {
	case errors.Is(err, geo.ErrInvalidPlate):
		return "", "", "", &ValidationError{Field: "plate", Message: "does not match the plate format of " + country}
	case errors.Is(err, geo.ErrNotFound), errors.Is(err, geo.ErrInvalid):
		return "", "", "", &ValidationError{Field: "plate_country", Message: "has no active plate format"}
	case err != nil:
		return "", "", "", fmt.Errorf("fleet: plate: %w", err)
	}
	return plate, check.Normalized, check.Country, nil
}

func normalizeVIN(raw string) (string, error) {
	v := strings.NewReplacer(" ", "", "-", "", "\t", "").Replace(strings.ToUpper(raw))
	if v == "" {
		return "", nil
	}
	if !vinRe.MatchString(v) {
		return "", &ValidationError{Field: "vin", Message: "must be 17 letters or digits (no I, O or Q)"}
	}
	return v, nil
}

func modelYear(y *int, current int) (pgtype.Int2, error) {
	if y == nil {
		return pgtype.Int2{}, nil
	}
	if *y < 1900 || *y > current+1 {
		return pgtype.Int2{}, &ValidationError{Field: "model_year", Message: "must be between 1900 and next year"}
	}
	return pgtype.Int2{Int16: int16(*y), Valid: true}, nil
}

// resolveCar maps catalog UUIDs to ids; a model alone implies its brand.
func resolveCar(ctx context.Context, q *db.Queries, brandUUID, modelUUID *uuid.UUID) (pgtype.Int8, pgtype.Int8, error) {
	var brandID, modelID pgtype.Int8
	if brandUUID != nil {
		b, err := q.GetCarBrandByUUID(ctx, *brandUUID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !b.Active) {
			return brandID, modelID, &ValidationError{Field: "car_brand_uuid", Message: "car brand not found"}
		}
		if err != nil {
			return brandID, modelID, fmt.Errorf("fleet: car brand: %w", err)
		}
		brandID = pgtype.Int8{Int64: b.ID, Valid: true}
	}
	if modelUUID != nil {
		m, err := q.GetCarModelByUUID(ctx, *modelUUID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !m.Active) {
			return brandID, modelID, &ValidationError{Field: "car_model_uuid", Message: "car model not found"}
		}
		if err != nil {
			return brandID, modelID, fmt.Errorf("fleet: car model: %w", err)
		}
		if brandID.Valid && m.CarBrandID != brandID.Int64 {
			return brandID, modelID, &ValidationError{Field: "car_model_uuid", Message: "car model does not belong to the car brand"}
		}
		brandID = pgtype.Int8{Int64: m.CarBrandID, Valid: true}
		modelID = pgtype.Int8{Int64: m.ID, Valid: true}
	}
	return brandID, modelID, nil
}

// match is an existing vehicle of the brand with the plate or the VIN.
type match struct {
	vehicle db.Vehicle
	// inFleet: already a vehicle of this fleet; attachable: owned by an
	// active user of this fleet and in no other fleet. Anything else is
	// another customer's vehicle.
	inFleet, attachable bool
}

// findMatch looks for a vehicle of the brand with the normalized plate (in
// its country) or the VIN.
func findMatch(ctx context.Context, q *db.Queries, fleet db.Organization, d vehicleData) (*match, error) {
	brand := pgtype.Int8{Int64: fleet.BrandID, Valid: true}
	var found []db.Vehicle
	if d.normalized != "" {
		rows, err := q.FindVehiclesByPlate(ctx, db.FindVehiclesByPlateParams{
			PlateCountry:    pgtype.Text{String: d.country, Valid: true},
			PlateNormalized: pgtype.Text{String: d.normalized, Valid: true}, BrandID: brand,
		})
		if err != nil {
			return nil, fmt.Errorf("fleet: plate lookup: %w", err)
		}
		found = append(found, rows...)
	}
	if d.vin != "" {
		rows, err := q.FindVehiclesByVIN(ctx, db.FindVehiclesByVINParams{
			Vin: pgtype.Text{String: d.vin, Valid: true}, BrandID: brand,
		})
		if err != nil {
			return nil, fmt.Errorf("fleet: vin lookup: %w", err)
		}
		found = append(found, rows...)
	}
	if len(found) == 0 {
		return nil, nil
	}
	return classify(ctx, q, fleet, found[0])
}

// classify decides whether an existing vehicle may join the fleet.
func classify(ctx context.Context, q *db.Queries, fleet db.Organization, v db.Vehicle) (*match, error) {
	m := &match{vehicle: v}
	if v.FleetOrgID.Valid {
		m.inFleet = v.FleetOrgID.Int64 == fleet.ID
		return m, nil
	}
	fu, err := q.GetFleetUserByUserID(ctx, v.UserID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return nil, fmt.Errorf("fleet: owner: %w", err)
	default:
		m.attachable = fu.FleetOrgID == fleet.ID
	}
	return m, nil
}

// AddVehicle is POST /v1/fleets/{uuid}/vehicles. A new vehicle is owned by
// the fleet's primary user (ErrPrimaryUserRequired without one) and
// registered by the caller's organization. A plate / VIN that already
// exists answers *VehicleExistsError when the fleet's users own it (link
// suggestion) and ErrVehicleOtherOwner for another customer's vehicle.
// With vehicle_uuid the existing vehicle joins the fleet under the same
// rule. created is false when an existing vehicle was linked.
func (s *Service) AddVehicle(ctx context.Context, c Caller, fleetUUID uuid.UUID, in AddVehicleInput) (VehicleView, bool, error) {
	var (
		out     VehicleView
		created bool
	)
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		a, err := s.resolve(ctx, q, c, fleetUUID)
		if err != nil {
			return err
		}
		fleet := a.fleet.Organization
		var v db.Vehicle
		if in.VehicleUUID != nil {
			cur, err := q.GetVehicleByUUIDForUpdate(ctx, *in.VehicleUUID)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && cur.BrandID != fleet.BrandID) {
				return ErrNotFound
			}
			if err != nil {
				return fmt.Errorf("fleet: vehicle: %w", err)
			}
			m, err := classify(ctx, q, fleet, cur)
			if err != nil {
				return err
			}
			switch {
			case m.inFleet:
				v = cur
			case m.attachable:
				if v, err = s.attach(ctx, q, tx, c, fleet, cur); err != nil {
					return err
				}
			default:
				return ErrVehicleOtherOwner
			}
		} else {
			if !a.fleet.FleetProfile.PrimaryUserID.Valid {
				return ErrPrimaryUserRequired
			}
			d, err := s.prepareNew(ctx, q, c.OrgID, in)
			if err != nil {
				return err
			}
			if v, err = s.createOrMatch(ctx, q, tx, c, a, d); err != nil {
				return err
			}
			created = true
		}
		out, err = vehicleView(ctx, q, v)
		return err
	})
	return out, created, err
}

func (s *Service) prepareNew(ctx context.Context, q *db.Queries, orgID int64, in AddVehicleInput) (vehicleData, error) {
	var d vehicleData
	var err error
	if d.plate, d.normalized, d.country, err = s.preparePlate(ctx, q, orgID, in.Plate, in.PlateCountry); err != nil {
		return d, err
	}
	if d.vin, err = normalizeVIN(in.VIN); err != nil {
		return d, err
	}
	if d.year, err = modelYear(in.ModelYear, s.now().UTC().Year()); err != nil {
		return d, err
	}
	if d.brandID, d.modelID, err = resolveCar(ctx, q, in.CarBrandUUID, in.CarModelUUID); err != nil {
		return d, err
	}
	return d, nil
}

// createOrMatch registers a validated vehicle for the fleet unless the
// plate / VIN already exists (link suggestion or refusal).
func (s *Service) createOrMatch(ctx context.Context, q *db.Queries, tx pgx.Tx, c Caller, a access, d vehicleData) (db.Vehicle, error) {
	fleet := a.fleet.Organization
	m, err := findMatch(ctx, q, fleet, d)
	if err != nil {
		return db.Vehicle{}, err
	}
	if m != nil {
		if m.inFleet || m.attachable {
			return db.Vehicle{}, &VehicleExistsError{
				VehicleUUID: m.vehicle.Uuid, Plate: m.vehicle.Plate.String, InFleet: m.inFleet,
			}
		}
		return db.Vehicle{}, ErrVehicleOtherOwner
	}
	v, err := q.CreateVehicle(ctx, db.CreateVehicleParams{
		UserID: a.fleet.FleetProfile.PrimaryUserID.Int64, OrganizationID: pgtype.Int8{Int64: c.OrgID, Valid: true},
		BrandID: fleet.BrandID, CarBrandID: d.brandID, CarModelID: d.modelID, ModelYear: d.year,
		Plate: optText(d.plate), PlateNormalized: optText(d.normalized), PlateCountry: optText(d.country),
		Vin: optText(d.vin),
	})
	if err != nil {
		return db.Vehicle{}, fmt.Errorf("fleet: create vehicle: %w", err)
	}
	if err := s.vehicleEvent(ctx, tx, c, events.VehicleCreated, v); err != nil {
		return db.Vehicle{}, err
	}
	return s.attach(ctx, q, tx, c, fleet, v)
}

// attach puts a vehicle into the fleet and writes fleet.vehicle_added.
func (s *Service) attach(ctx context.Context, q *db.Queries, tx pgx.Tx, c Caller, fleet db.Organization, v db.Vehicle) (db.Vehicle, error) {
	v, err := q.SetVehicleFleet(ctx, db.SetVehicleFleetParams{
		ID: v.ID, FleetOrgID: pgtype.Int8{Int64: fleet.ID, Valid: true},
	})
	if err != nil {
		return db.Vehicle{}, fmt.Errorf("fleet: set fleet: %w", err)
	}
	if err := s.vehicleEvent(ctx, tx, c, events.VehicleUpdated, v); err != nil {
		return db.Vehicle{}, err
	}
	if err := s.emit(ctx, tx, events.FleetVehicleAdded, c.OrgID, c.UserID, fleet, map[string]any{
		"vehicle_uuid": v.Uuid.String(),
	}); err != nil {
		return db.Vehicle{}, err
	}
	return v, nil
}

// UpdateVehicle is PATCH /v1/fleets/{uuid}/vehicles/{vehicle_uuid}.
func (s *Service) UpdateVehicle(ctx context.Context, c Caller, fleetUUID, id uuid.UUID, in UpdateVehicleInput) (VehicleView, error) {
	if in.Plate.Set && (in.Plate.Value == nil || strings.TrimSpace(*in.Plate.Value) == "") {
		return VehicleView{}, &ValidationError{Field: "plate", Message: "is required"}
	}
	var out VehicleView
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		a, err := s.resolve(ctx, q, c, fleetUUID)
		if err != nil {
			return err
		}
		cur, err := q.GetFleetVehicleForUpdate(ctx, db.GetFleetVehicleForUpdateParams{Uuid: id, FleetOrgID: a.orgID()})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("fleet: vehicle: %w", err)
		}
		p := db.UpdateVehicleParams{
			ID: cur.ID, CarBrandID: cur.CarBrandID, CarModelID: cur.CarModelID, ModelYear: cur.ModelYear,
			Plate: cur.Plate, PlateNormalized: cur.PlateNormalized, PlateCountry: cur.PlateCountry, Vin: cur.Vin,
		}
		if in.Plate.Set || in.PlateCountry.Set {
			raw, country := cur.Plate.String, cur.PlateCountry.String
			if in.Plate.Set {
				raw = *in.Plate.Value
			}
			if in.PlateCountry.Set {
				country = ""
				if in.PlateCountry.Value != nil {
					country = *in.PlateCountry.Value
				}
			}
			plate, norm, iso, err := s.preparePlate(ctx, q, c.OrgID, raw, country)
			if err != nil {
				return err
			}
			p.Plate, p.PlateNormalized, p.PlateCountry = optText(plate), optText(norm), optText(iso)
		}
		if in.VIN.Set {
			raw := ""
			if in.VIN.Value != nil {
				raw = *in.VIN.Value
			}
			vin, err := normalizeVIN(raw)
			if err != nil {
				return err
			}
			p.Vin = optText(vin)
		}
		if in.ModelYear.Set {
			if p.ModelYear, err = modelYear(in.ModelYear.Value, s.now().UTC().Year()); err != nil {
				return err
			}
		}
		if in.CarBrandUUID.Set || in.CarModelUUID.Set {
			b, m, err := resolveCar(ctx, q, in.CarBrandUUID.Value, in.CarModelUUID.Value)
			if err != nil {
				return err
			}
			switch {
			case in.CarModelUUID.Set && m.Valid: // the model implies its brand
				p.CarBrandID, p.CarModelID = b, m
			case in.CarModelUUID.Set:
				p.CarModelID = pgtype.Int8{}
				if in.CarBrandUUID.Set {
					p.CarBrandID = b
				}
			default: // a new brand drops the old model
				if b != cur.CarBrandID {
					p.CarModelID = pgtype.Int8{}
				}
				p.CarBrandID = b
			}
		}
		row, err := q.UpdateVehicle(ctx, p)
		if err != nil {
			return fmt.Errorf("fleet: update vehicle: %w", err)
		}
		if err := s.vehicleEvent(ctx, tx, c, events.VehicleUpdated, row); err != nil {
			return err
		}
		out, err = vehicleView(ctx, q, row)
		return err
	})
	return out, err
}

// RemoveVehicle is DELETE /v1/fleets/{uuid}/vehicles/{vehicle_uuid}: the
// vehicle leaves the fleet (fleet_org_id NULL); it is not deleted.
func (s *Service) RemoveVehicle(ctx context.Context, c Caller, fleetUUID, id uuid.UUID) error {
	return s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		a, err := s.resolve(ctx, q, c, fleetUUID)
		if err != nil {
			return err
		}
		cur, err := q.GetFleetVehicleForUpdate(ctx, db.GetFleetVehicleForUpdateParams{Uuid: id, FleetOrgID: a.orgID()})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("fleet: vehicle: %w", err)
		}
		row, err := q.SetVehicleFleet(ctx, db.SetVehicleFleetParams{ID: cur.ID})
		if err != nil {
			return fmt.Errorf("fleet: remove vehicle: %w", err)
		}
		return s.vehicleEvent(ctx, tx, c, events.VehicleUpdated, row)
	})
}

// vehicleEvent writes a vehicle record event (the search sync refreshes
// the vehicle document); same payload as the customers module.
func (s *Service) vehicleEvent(ctx context.Context, tx pgx.Tx, c Caller, name string, v db.Vehicle) error {
	if s.out == nil {
		return nil
	}
	id, u := v.ID, v.Uuid
	ev := events.New(name).WithTenant(c.OrgID).WithEntity("vehicle", &id, &u).
		WithPayload(map[string]any{"vehicle_uuid": v.Uuid.String(), "brand_id": v.BrandID, "user_id": v.UserID})
	if c.UserID != 0 {
		ev = ev.WithActor(c.UserID)
	}
	if err := s.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("fleet: outbox: %w", err)
	}
	return nil
}

// vehicleView maps a written vehicle (no service columns).
func vehicleView(ctx context.Context, q *db.Queries, v db.Vehicle) (VehicleView, error) {
	out := VehicleView{
		UUID: v.Uuid, Plate: textPtr(v.Plate), PlateCountry: textPtr(v.PlateCountry), VIN: textPtr(v.Vin),
		CreatedAt: v.CreatedAt.Time,
	}
	if v.ModelYear.Valid {
		y := int(v.ModelYear.Int16)
		out.ModelYear = &y
	}
	if v.CarBrandID.Valid {
		b, err := q.GetCarBrandByID(ctx, v.CarBrandID.Int64)
		if err != nil {
			return VehicleView{}, fmt.Errorf("fleet: car brand: %w", err)
		}
		out.CarBrand = &CatalogRef{UUID: b.Uuid, Name: b.Name}
	}
	if v.CarModelID.Valid {
		m, err := q.GetFleetCarModelByID(ctx, v.CarModelID.Int64)
		if err != nil {
			return VehicleView{}, fmt.Errorf("fleet: car model: %w", err)
		}
		out.CarModel = &CatalogRef{UUID: m.Uuid, Name: m.Name}
	}
	return out, nil
}
