package migrator

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// ServicesStep imports the hub services with their items, images and status
// logs (TEC-259, design §7):
//
//   - the service organization is the dealer's organization (TEC-254), the
//     customer the user the legacy customer maps to (TEC-255; merged
//     customers share one user), the car brand / model the catalog rows
//     (TEC-256) and the units of the items the hub stock items (TEC-257);
//   - the vehicle is the customer's vehicle with the same VIN, else the same
//     plate; a missing one is created;
//   - the legacy service number is kept; one another service already holds,
//     or one that does not fit the column, becomes LegacyServiceNoPrefix +
//     the number (or + the legacy id);
//   - the legacy status goes through serviceStatusMap; an unknown status is
//     reported and the service skipped;
//   - images are copied from the legacy hub storage to the object store;
//   - legacy status logs are notes of a dealer on a service (from / to
//     dealer, no status). They are kept as notes: from_status NULL,
//     to_status the service status, the writer's organization as
//     actor_org_id, the legacy dealers in metadata.
//
// A final service (completed / cancelled) is written with its items first
// and its final status last, because the items of a final service are
// locked. Nothing is written to the outbox and no warranty is created
// (warranties are F2-01i), and the stock ledger is not touched: the units
// of the items keep the ownership TEC-258 gave them.
type ServicesStep struct {
	// System is the migration_map source system; empty means SourceHub.
	System string
}

// Name implements Step.
func (ServicesStep) Name() string { return "services" }

func (s ServicesStep) system() string {
	if s.System == "" {
		return SourceHub
	}
	return s.System
}

// LegacyServiceNoPrefix marks a legacy service number that could not be
// kept as it is. The new numbering (DS + 8 characters) never starts with it.
const LegacyServiceNoPrefix = "OLX-"

// Column limits of services / service_images (000050).
const (
	serviceNoMax      = 32
	servicePlateMax   = 20
	servicePackageMax = 255
	serviceTitleMax   = 255
	// maxLegacyServiceImageBytes is larger than the upload limit
	// (storage.MaxProductImageBytes): legacy photos were not resized.
	maxLegacyServiceImageBytes = 20 << 20
)

// Service statuses (chk_services_status).
const (
	serviceDraft     = "draft"
	serviceCompleted = "completed"
	serviceCancelled = "cancelled"
)

// serviceStatusMap translates the legacy ServiceStatusEnum
// (app/Enums/ServiceStatusEnum.php) to the service status of this
// application. The two lists are the same.
var serviceStatusMap = map[string]string{
	"draft":      "draft",
	"pending":    "pending",
	"processing": "processing",
	"ready":      "ready",
	"completed":  "completed",
	"cancelled":  "cancelled",
}

func finalServiceStatus(status string) bool {
	return status == serviceCompleted || status == serviceCancelled
}

var serviceVINRe = regexp.MustCompile(`^[A-HJ-NPR-Z0-9]{17}$`)
var serviceCountryRe = regexp.MustCompile(`^[A-Z]{2}$`)

const servicesQuery = `SELECT id, service_no, dealer_id, customer_id, user_id, car_brand_id, car_model_id, year,
	COALESCE(vin, ''), COALESCE(plate, ''), COALESCE(plate_country, ''), km, COALESCE(package, ''), applied_parts,
	COALESCE(notes, ''), status, completed_at, review_request_sms_sent_at, created_at, updated_at
FROM services`

const serviceItemsQuery = `SELECT id, service_id, stock_item_id, usage_type, COALESCE(notes, ''), created_at, updated_at
FROM service_items`

// "order" is reserved in both engines; qualified, it needs no quoting.
const serviceImagesQuery = `SELECT i.id, i.service_id, i.image_path, COALESCE(i.title, ''), i.order, i.created_at, i.updated_at
FROM service_images i`

const serviceStatusLogsQuery = `SELECT id, service_id, from_dealer_id, to_dealer_id, user_id, COALESCE(notes, ''),
	created_at, updated_at
FROM service_status_logs`

type legacyService struct {
	ID                           int64
	ServiceNo                    string
	DealerID, CustomerID, UserID int64
	CarBrandID, CarModelID       int64
	Year                         sql.NullInt64
	VIN, Plate, PlateCountry     string
	Km                           sql.NullInt64
	Package                      string
	Parts                        []byte
	Notes, Status                string
	CompletedAt, ReviewAt        sql.NullTime
	CreatedAt, UpdatedAt         sql.NullTime
}

type legacyServiceItem struct {
	ID, ServiceID, StockItemID int64
	UsageType, Notes           string
	CreatedAt, UpdatedAt       sql.NullTime
}

type legacyServiceImage struct {
	ID, ServiceID        int64
	Path, Title          string
	Order                int64
	CreatedAt, UpdatedAt sql.NullTime
}

type legacyStatusLog struct {
	ID, ServiceID            int64
	FromDealerID, ToDealerID sql.NullInt64
	UserID                   sql.NullInt64
	Notes                    string
	CreatedAt, UpdatedAt     sql.NullTime
}

// serviceRef is a service row of this database the children are written to.
type serviceRef struct {
	ID, OrgID, BrandID int64
	UUID, OrgUUID      uuid.UUID
	Status             string // in the database now
	Target             string // the mapped legacy status (empty: unknown)
	CompletedAt        pgtype.Timestamptz
	CancelledAt        pgtype.Timestamptz
	Parts              []string // legacy applied parts of the service
}

type serviceRun struct {
	step  ServicesStep
	dst   *Target
	q     *db.Queries
	m     *Mapper
	tree  olexTree
	c     counts
	orgs  map[int64]*db.MigratorOrganizationByUUIDRow // hub dealer id -> organization (nil: unmapped)
	users map[int64]int64                             // hub user id -> users.id (0: unmapped)
}

// Run implements Step.
func (s ServicesStep) Run(ctx context.Context, src Sources, dst *Target, m *Mapper) (StepResult, error) {
	c := counts{}
	hub, err := src.Get(SourceHub)
	if err != nil {
		return StepResult{}, err
	}
	tree, err := resolveOlexTree(ctx, dst.Q, false, c)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	services, err := readLegacyServices(ctx, hub, dst)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	items, err := readLegacyServiceItems(ctx, hub, dst)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	images, err := readLegacyServiceImages(ctx, hub, dst)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	logs, err := readLegacyStatusLogs(ctx, hub, dst)
	if err != nil {
		return StepResult{Counts: c}, err
	}

	var watermark time.Time
	mark := func(ts ...sql.NullTime) {
		if t := latest(ts...); t.After(watermark) {
			watermark = t
		}
	}
	byID := map[int64]legacyService{}
	ids := map[int64]bool{}
	for _, ls := range services {
		byID[ls.ID] = ls
		ids[ls.ID] = true
		mark(ls.CreatedAt, ls.UpdatedAt)
	}
	itemsOf := map[int64][]legacyServiceItem{}
	for _, it := range items {
		itemsOf[it.ServiceID] = append(itemsOf[it.ServiceID], it)
		ids[it.ServiceID] = true
		mark(it.CreatedAt, it.UpdatedAt)
	}
	imagesOf := map[int64][]legacyServiceImage{}
	for _, im := range images {
		imagesOf[im.ServiceID] = append(imagesOf[im.ServiceID], im)
		ids[im.ServiceID] = true
		mark(im.CreatedAt, im.UpdatedAt)
	}
	logsOf := map[int64][]legacyStatusLog{}
	for _, l := range logs {
		logsOf[l.ServiceID] = append(logsOf[l.ServiceID], l)
		ids[l.ServiceID] = true
		mark(l.CreatedAt, l.UpdatedAt)
	}
	order := make([]int64, 0, len(ids))
	for id := range ids {
		order = append(order, id)
	}
	slices.Sort(order)

	r := &serviceRun{step: s, dst: dst, q: dst.Q, m: m, tree: tree, c: c,
		orgs: map[int64]*db.MigratorOrganizationByUUIDRow{}, users: map[int64]int64{}}
	for _, id := range order {
		var ref *serviceRef
		if ls, ok := byID[id]; ok {
			c.inc("services_read")
			if ref, err = r.importService(ctx, ls); err != nil {
				return StepResult{Counts: c}, fmt.Errorf("service %d: %w", id, err)
			}
		} else if ref, err = r.mappedService(ctx, id); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("service %d: %w", id, err)
		}
		if err := r.children(ctx, id, ref, itemsOf[id], imagesOf[id], logsOf[id]); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("service %d: %w", id, err)
		}
	}
	return StepResult{Counts: c, Watermark: watermark}, nil
}

func readLegacyServices(ctx context.Context, hub source.LegacySource, dst *Target) ([]legacyService, error) {
	where, args := deltaFilter(dst)
	rows, err := hub.Query(ctx, servicesQuery+where+" ORDER BY id", args...)
	if err != nil {
		return nil, err
	}
	var out []legacyService
	for rows.Next() {
		var ls legacyService
		if err := rows.Scan(&ls.ID, &ls.ServiceNo, &ls.DealerID, &ls.CustomerID, &ls.UserID, &ls.CarBrandID,
			&ls.CarModelID, &ls.Year, &ls.VIN, &ls.Plate, &ls.PlateCountry, &ls.Km, &ls.Package, &ls.Parts,
			&ls.Notes, &ls.Status, &ls.CompletedAt, &ls.ReviewAt, &ls.CreatedAt, &ls.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan service: %w", err)
		}
		out = append(out, ls)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read services: %w", err)
	}
	return out, nil
}

func readLegacyServiceItems(ctx context.Context, hub source.LegacySource, dst *Target) ([]legacyServiceItem, error) {
	where, args := deltaFilter(dst)
	rows, err := hub.Query(ctx, serviceItemsQuery+where+" ORDER BY id", args...)
	if err != nil {
		return nil, err
	}
	var out []legacyServiceItem
	for rows.Next() {
		var it legacyServiceItem
		if err := rows.Scan(&it.ID, &it.ServiceID, &it.StockItemID, &it.UsageType, &it.Notes,
			&it.CreatedAt, &it.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan service item: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read service items: %w", err)
	}
	return out, nil
}

func readLegacyServiceImages(ctx context.Context, hub source.LegacySource, dst *Target) ([]legacyServiceImage, error) {
	where, args := deltaFilterOn(dst, "i.")
	rows, err := hub.Query(ctx, serviceImagesQuery+where+" ORDER BY i.id", args...)
	if err != nil {
		return nil, err
	}
	var out []legacyServiceImage
	for rows.Next() {
		var im legacyServiceImage
		if err := rows.Scan(&im.ID, &im.ServiceID, &im.Path, &im.Title, &im.Order, &im.CreatedAt, &im.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan service image: %w", err)
		}
		out = append(out, im)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read service images: %w", err)
	}
	return out, nil
}

func readLegacyStatusLogs(ctx context.Context, hub source.LegacySource, dst *Target) ([]legacyStatusLog, error) {
	where, args := deltaFilter(dst)
	rows, err := hub.Query(ctx, serviceStatusLogsQuery+where+" ORDER BY id", args...)
	if err != nil {
		return nil, err
	}
	var out []legacyStatusLog
	for rows.Next() {
		var l legacyStatusLog
		if err := rows.Scan(&l.ID, &l.ServiceID, &l.FromDealerID, &l.ToDealerID, &l.UserID, &l.Notes,
			&l.CreatedAt, &l.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan service status log: %w", err)
		}
		out = append(out, l)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read service status logs: %w", err)
	}
	return out, nil
}

// serviceVehicle is the normalized vehicle snapshot of a legacy service.
type serviceVehicle struct {
	VIN, Plate, PlateNorm, Country pgtype.Text
	Year                           pgtype.Int2
}

// normalizeServiceVehicle cleans the legacy VIN / plate / year; notes name
// the values that were dropped or replaced.
func normalizeServiceVehicle(ls legacyService) (serviceVehicle, []string) {
	var v serviceVehicle
	var notes []string
	if vin := strings.ToUpper(strings.TrimSpace(ls.VIN)); vin != "" {
		if serviceVINRe.MatchString(vin) {
			v.VIN = pgtype.Text{String: vin, Valid: true}
		} else {
			notes = append(notes, "vin_invalid")
		}
	}
	plate := truncate(strings.TrimSpace(ls.Plate), servicePlateMax)
	if norm := geo.NormalizePlate(plate); plate != "" && norm != "" {
		country := strings.ToUpper(strings.TrimSpace(ls.PlateCountry))
		if !serviceCountryRe.MatchString(country) {
			if country != "" {
				notes = append(notes, "plate_country_invalid")
			}
			country = TRCountry
		}
		v.Plate = pgtype.Text{String: plate, Valid: true}
		v.PlateNorm = pgtype.Text{String: truncate(norm, servicePlateMax), Valid: true}
		v.Country = pgtype.Text{String: country, Valid: true}
	} else {
		notes = append(notes, "plate_missing")
	}
	if y := legacyYear(ls.Year); y.Valid {
		v.Year = y
	} else if ls.Year.Valid {
		notes = append(notes, "year_invalid")
	}
	return v, notes
}

// serviceStatusTimes are the completed_at / cancelled_at the status checks
// require: set exactly for completed / cancelled. A legacy service without a
// completion time takes its last update (reported).
func serviceStatusTimes(ls legacyService, status string) (completed, cancelled pgtype.Timestamptz, fallback bool) {
	at := func() pgtype.Timestamptz {
		for _, t := range []sql.NullTime{ls.CompletedAt, ls.UpdatedAt, ls.CreatedAt} {
			if t.Valid {
				return pgTime(t)
			}
		}
		return pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	switch status {
	case serviceCompleted:
		return at(), cancelled, !ls.CompletedAt.Valid
	case serviceCancelled:
		// The hub keeps no cancellation time.
		ls.CompletedAt = sql.NullTime{}
		return completed, at(), false
	}
	return completed, cancelled, false
}

// legacyServiceParts reads the service's applied parts (a JSON array of
// part keys; anything else is no parts).
func legacyServiceParts(raw []byte) []string {
	var parts []string
	if json.Unmarshal(raw, &parts) != nil {
		return nil
	}
	return parts
}

func (r *serviceRun) report(key string, id int64) {
	r.c.inc(key)
	r.c.inc(key + ":" + strconv.FormatInt(id, 10))
}

// importService creates or updates the service row; nil means the service
// was skipped (reported).
func (r *serviceRun) importService(ctx context.Context, ls legacyService) (*serviceRef, error) {
	target, ok := serviceStatusMap[strings.ToLower(strings.TrimSpace(ls.Status))]
	if !ok {
		r.c.inc("status_unknown:" + truncate(ls.Status, 32))
		r.report("service_skipped_status", ls.ID)
		return nil, nil
	}
	org, err := r.dealerOrg(ctx, ls.DealerID)
	if err != nil {
		return nil, err
	}
	if org == nil {
		r.report("service_skipped_dealer_unmapped", ls.ID)
		return nil, nil
	}
	customer, err := r.customerUser(ctx, ls.CustomerID)
	if err != nil {
		return nil, err
	}
	if customer == 0 {
		r.report("service_skipped_customer_unmapped", ls.ID)
		return nil, nil
	}
	carBrand, carModel, err := r.carModel(ctx, ls)
	if err != nil || carModel == 0 {
		return nil, err
	}
	creator, err := r.user(ctx, ls.UserID)
	if err != nil {
		return nil, err
	}
	if creator == 0 {
		r.report("creator_unmapped", ls.ID)
	}

	v, notes := normalizeServiceVehicle(ls)
	for _, n := range notes {
		if n == "plate_missing" {
			r.c.inc(n)
			continue
		}
		r.report(n, ls.ID)
	}
	km := pgtype.Int4{}
	if ls.Km.Valid && ls.Km.Int64 >= 0 && ls.Km.Int64 <= 1<<31-1 {
		km = pgtype.Int4{Int32: int32(ls.Km.Int64), Valid: true}
	} else if ls.Km.Valid {
		r.report("km_invalid", ls.ID)
	}
	completedAt, cancelledAt, fallback := serviceStatusTimes(ls, target)
	if fallback {
		r.report("completed_at_fallback", ls.ID)
	}

	key := Key{System: r.step.system(), Table: "services", ID: strconv.FormatInt(ls.ID, 10), TargetTable: "services"}
	sum := Checksum(ls.ServiceNo, ls.DealerID, ls.CustomerID, ls.UserID, ls.CarBrandID, ls.CarModelID, ls.Year.Int64,
		ls.Year.Valid, ls.VIN, ls.Plate, ls.PlateCountry, ls.Km.Int64, ls.Km.Valid, ls.Package, string(ls.Parts), ls.Notes,
		ls.Status, ls.CompletedAt.Time, ls.CompletedAt.Valid, ls.ReviewAt.Time, ls.ReviewAt.Valid)
	res, err := r.m.Upsert(ctx, key, sum)
	if err != nil {
		return nil, err
	}
	if _, err := r.q.MigratorLinkCustomerOrganization(ctx, db.MigratorLinkCustomerOrganizationParams{
		UserID: customer, OrganizationID: org.ID, BrandID: org.BrandID, CreatedAt: pgTime(ls.CreatedAt),
	}); err != nil {
		return nil, fmt.Errorf("link customer organization: %w", err)
	}

	ref := &serviceRef{OrgID: org.ID, OrgUUID: org.Uuid, BrandID: org.BrandID, UUID: res.UUID, Target: target,
		CompletedAt: completedAt, CancelledAt: cancelledAt, Parts: legacyServiceParts(ls.Parts)}
	pkg, notesText := pgText(truncate(strings.TrimSpace(ls.Package), servicePackageMax)), pgText(strings.TrimSpace(ls.Notes))
	createdBy := pgtype.Int8{Int64: creator, Valid: creator != 0}

	current, err := r.q.MigratorServiceByUUID(ctx, res.UUID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("read service: %w", err)
	}
	if err == nil {
		ref.ID, ref.Status = current.ID, current.Status
		if !res.Changed {
			r.c.inc("services_unchanged")
			return ref, nil
		}
		vehicleID, err := r.vehicle(ctx, ls, customer, org, carBrand, carModel, v)
		if err != nil {
			return nil, err
		}
		if err := r.q.MigratorUpdateService(ctx, db.MigratorUpdateServiceParams{
			ID: current.ID, OrganizationID: org.ID, BrandID: org.BrandID, CustomerUserID: customer,
			VehicleID: vehicleID, CarBrandID: carBrand, CarModelID: carModel, ModelYear: v.Year, Plate: v.Plate,
			PlateCountry: v.Country, Vin: v.VIN, Km: km, Package: pkg, Notes: notesText,
			CreatedByUserID: createdBy, ReviewRequestSentAt: pgTime(ls.ReviewAt),
		}); err != nil {
			return nil, fmt.Errorf("update service: %w", err)
		}
		r.c.inc("services_updated")
		return ref, nil
	}

	no, err := r.serviceNo(ctx, ls, res.UUID)
	if err != nil || no == "" {
		return nil, err
	}
	vehicleID, err := r.vehicle(ctx, ls, customer, org, carBrand, carModel, v)
	if err != nil {
		return nil, err
	}
	// A final status is written after the items (children).
	status := target
	insCompleted, insCancelled := completedAt, cancelledAt
	if finalServiceStatus(target) {
		status, insCompleted, insCancelled = serviceDraft, pgtype.Timestamptz{}, pgtype.Timestamptz{}
	}
	id, err := r.q.MigratorInsertService(ctx, db.MigratorInsertServiceParams{
		Uuid: res.UUID, ServiceNo: no, OrganizationID: org.ID, BrandID: org.BrandID, CustomerUserID: customer,
		VehicleID: vehicleID, CarBrandID: carBrand, CarModelID: carModel, ModelYear: v.Year, Plate: v.Plate,
		PlateCountry: v.Country, Vin: v.VIN, Km: km, Package: pkg, Notes: notesText, Status: status,
		CreatedByUserID: createdBy, CompletedAt: insCompleted, CancelledAt: insCancelled,
		ReviewRequestSentAt: pgTime(ls.ReviewAt), CreatedAt: pgTime(ls.CreatedAt), UpdatedAt: pgTime(ls.UpdatedAt),
	})
	if err != nil {
		return nil, fmt.Errorf("insert service: %w", err)
	}
	r.c.inc("services_created")
	ref.ID, ref.Status = id, status
	return ref, nil
}

// serviceNo keeps the legacy number; one that is empty, too long or held by
// another service is prefixed (reported). Empty means skipped.
func (r *serviceRun) serviceNo(ctx context.Context, ls legacyService, target uuid.UUID) (string, error) {
	idNo := LegacyServiceNoPrefix + strconv.FormatInt(ls.ID, 10)
	no := strings.TrimSpace(ls.ServiceNo)
	switch {
	case no == "":
		r.report("service_no_missing", ls.ID)
		no = idNo
	case len([]rune(no)) > serviceNoMax:
		r.report("service_no_too_long", ls.ID)
		no = idNo
	}
	candidates := []string{no}
	if no != idNo {
		prefixed := LegacyServiceNoPrefix + no
		if len([]rune(prefixed)) > serviceNoMax {
			prefixed = idNo
		}
		candidates = append(candidates, prefixed, idNo)
	}
	for i, cand := range candidates {
		taken, err := r.q.MigratorServiceNoTaken(ctx, db.MigratorServiceNoTakenParams{ServiceNo: cand, Uuid: target})
		if err != nil {
			return "", fmt.Errorf("service number: %w", err)
		}
		if !taken {
			if i > 0 {
				r.report("service_no_prefixed", ls.ID)
			}
			return cand, nil
		}
	}
	r.report("service_skipped_service_no_taken", ls.ID)
	return "", nil
}

// vehicle returns the customer's vehicle of the service: the one mapped for
// the service earlier, else a matching live vehicle, else a new one.
func (r *serviceRun) vehicle(ctx context.Context, ls legacyService, customer int64,
	org *db.MigratorOrganizationByUUIDRow, carBrand, carModel int64, v serviceVehicle) (int64, error) {
	key := Key{System: r.step.system(), Table: "services.vehicle", ID: strconv.FormatInt(ls.ID, 10), TargetTable: "vehicles"}
	target, mapped, err := r.m.Lookup(ctx, key.System, key.Table, key.ID)
	if err != nil {
		return 0, err
	}
	if mapped {
		row, err := r.q.MigratorVehicleByUUID(ctx, target)
		switch {
		case err == nil && row.UserID == customer:
			return row.ID, nil
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return 0, fmt.Errorf("read vehicle: %w", err)
		}
		r.report("vehicle_remapped", ls.ID)
	}
	found, err := r.q.MigratorFindVehicle(ctx, db.MigratorFindVehicleParams{
		UserID: customer, BrandID: org.BrandID, Vin: v.VIN, PlateNormalized: v.PlateNorm, PlateCountry: v.Country,
	})
	switch {
	case err == nil:
		r.c.inc("vehicles_matched")
		if !mapped {
			if _, err := r.m.Link(ctx, key, found.Uuid, ""); err != nil {
				return 0, err
			}
		}
		return found.ID, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return 0, fmt.Errorf("find vehicle: %w", err)
	}
	vehicleUUID := uuid.New()
	id, err := r.q.MigratorInsertVehicle(ctx, db.MigratorInsertVehicleParams{
		Uuid: vehicleUUID, UserID: customer, OrganizationID: pgInt8(org.ID), BrandID: org.BrandID,
		CarBrandID: pgInt8(carBrand), CarModelID: pgInt8(carModel), ModelYear: v.Year, Plate: v.Plate,
		PlateNormalized: v.PlateNorm, PlateCountry: v.Country, Vin: v.VIN, CreatedAt: pgTime(ls.CreatedAt),
	})
	if err != nil {
		return 0, fmt.Errorf("insert vehicle: %w", err)
	}
	r.c.inc("vehicles_created")
	if !mapped {
		if _, err := r.m.Link(ctx, key, vehicleUUID, ""); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// mappedService is a service an earlier run imported (delta mode: only its
// children changed). nil means it is not mapped.
func (r *serviceRun) mappedService(ctx context.Context, legacyID int64) (*serviceRef, error) {
	target, ok, err := r.m.Lookup(ctx, r.step.system(), "services", strconv.FormatInt(legacyID, 10))
	if err != nil || !ok {
		return nil, err
	}
	row, err := r.q.MigratorServiceByUUID(ctx, target)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read service: %w", err)
	}
	org, err := r.q.MigratorOrganizationUUIDByID(ctx, row.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("read organization: %w", err)
	}
	return &serviceRef{ID: row.ID, OrgID: row.OrganizationID, OrgUUID: org, BrandID: row.BrandID, UUID: row.Uuid,
		Status: row.Status, Target: row.Status}, nil
}

// children writes the items, images and logs of a service, then its final
// status.
func (r *serviceRun) children(ctx context.Context, legacyID int64, ref *serviceRef,
	items []legacyServiceItem, images []legacyServiceImage, logs []legacyStatusLog) error {
	if ref == nil {
		// Every child is reported with its own id: the validation report
		// (report.go) matches skipped rows by id.
		for _, it := range items {
			r.report("item_skipped_service_unmapped", it.ID)
		}
		for _, im := range images {
			r.report("image_skipped_service_unmapped", im.ID)
		}
		for _, l := range logs {
			r.report("log_skipped_service_unmapped", l.ID)
		}
		if len(items)+len(images)+len(logs) > 0 {
			r.c.inc("children_skipped_service_unmapped:" + strconv.FormatInt(legacyID, 10))
		}
		return nil
	}
	for _, it := range items {
		if err := r.importItem(ctx, ref, it); err != nil {
			return fmt.Errorf("item %d: %w", it.ID, err)
		}
	}
	for _, im := range images {
		if err := r.importImage(ctx, ref, im); err != nil {
			return fmt.Errorf("image %d: %w", im.ID, err)
		}
	}
	for _, l := range logs {
		if err := r.importLog(ctx, ref, l); err != nil {
			return fmt.Errorf("status log %d: %w", l.ID, err)
		}
	}
	if ref.Target == ref.Status {
		return nil
	}
	if finalServiceStatus(ref.Status) {
		// completed / cancelled are final here; the legacy hub reopened it.
		r.report("service_status_locked", legacyID)
		return nil
	}
	if err := r.q.MigratorSetServiceStatus(ctx, db.MigratorSetServiceStatusParams{
		ID: ref.ID, Status: ref.Target, CompletedAt: ref.CompletedAt, CancelledAt: ref.CancelledAt,
	}); err != nil {
		return fmt.Errorf("set status: %w", err)
	}
	ref.Status = ref.Target
	return nil
}

func (r *serviceRun) importItem(ctx context.Context, ref *serviceRef, it legacyServiceItem) error {
	r.c.inc("items_read")
	unitUUID, ok, err := r.m.Lookup(ctx, r.step.system(), "stock_items", strconv.FormatInt(it.StockItemID, 10))
	if err != nil {
		return err
	}
	if !ok {
		r.report("item_skipped_unit_unmapped", it.ID)
		return nil
	}
	unit, err := r.q.MigratorServiceUnit(ctx, db.MigratorServiceUnitParams{Uuid: unitUUID, BrandID: ref.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		r.report("item_skipped_unit_unmapped", it.ID)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read unit: %w", err)
	}
	// The hub consumed the whole stock item on completion whatever the
	// usage type said, and keeps no meters: every item is a full one.
	switch strings.ToLower(strings.TrimSpace(it.UsageType)) {
	case "full":
	case "partial":
		r.c.inc("item_partial_as_full")
	default:
		r.report("item_usage_type_unknown", it.ID)
	}
	quantity := pgtype.Int4{}
	if unit.UnitKind == "fixed" {
		quantity = pgtype.Int4{Int32: 1, Valid: true}
	}
	parts, dropped := itemParts(ref.Parts, unit.AvailableParts)
	if dropped > 0 {
		r.report("item_parts_dropped", it.ID)
	}
	notes := pgText(strings.TrimSpace(it.Notes))

	key := Key{System: r.step.system(), Table: "service_items", ID: strconv.FormatInt(it.ID, 10), TargetTable: "service_items"}
	sum := Checksum(it.ServiceID, it.StockItemID, it.UsageType, it.Notes, string(parts))
	_, mapped, err := r.m.Lookup(ctx, key.System, key.Table, key.ID)
	if err != nil {
		return err
	}
	if finalServiceStatus(ref.Status) {
		// Items of a final service are locked (000050, decision 1).
		if mapped {
			r.c.inc("items_unchanged")
		} else {
			r.report("item_skipped_service_locked", it.ID)
		}
		return nil
	}
	res, err := r.m.Upsert(ctx, key, sum)
	if err != nil {
		return err
	}
	id, err := r.q.MigratorServiceItemByUUID(ctx, res.UUID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read item: %w", err)
	}
	if err == nil {
		if !res.Changed {
			r.c.inc("items_unchanged")
			return nil
		}
		if err := r.q.MigratorUpdateServiceItem(ctx, db.MigratorUpdateServiceItemParams{
			ID: id, AppliedParts: parts, Notes: notes,
		}); err != nil {
			return fmt.Errorf("update item: %w", err)
		}
		r.c.inc("items_updated")
		return nil
	}
	if _, err := r.q.MigratorInsertServiceItem(ctx, db.MigratorInsertServiceItemParams{
		Uuid: res.UUID, ServiceID: ref.ID, OrganizationID: ref.OrgID, BrandID: ref.BrandID, ProductID: unit.ProductID,
		UnitID: unit.ID, Kind: "full", Quantity: quantity, AppliedParts: parts, Notes: notes,
		CreatedAt: pgTime(it.CreatedAt), UpdatedAt: pgTime(it.UpdatedAt),
	}); err != nil {
		return fmt.Errorf("insert item: %w", err)
	}
	r.c.inc("items_created")
	return nil
}

// itemParts keeps the service's legacy parts the item's category offers
// (service_items_check) and counts the rest.
func itemParts(parts []string, available []byte) ([]byte, int) {
	var allowed []string
	_ = json.Unmarshal(available, &allowed)
	out := []string{}
	dropped := 0
	for _, p := range parts {
		switch {
		case slices.Contains(out, p):
		case slices.Contains(allowed, p):
			out = append(out, p)
		default:
			dropped++
		}
	}
	raw, _ := json.Marshal(out)
	return raw, dropped
}

func (r *serviceRun) importImage(ctx context.Context, ref *serviceRef, im legacyServiceImage) error {
	r.c.inc("images_read")
	key := Key{System: r.step.system(), Table: "service_images", ID: strconv.FormatInt(im.ID, 10), TargetTable: "service_images"}
	title := pgText(truncate(strings.TrimSpace(im.Title), serviceTitleMax))
	sortOrder := int32(0)
	if im.Order > 0 && im.Order <= 1<<31-1 {
		sortOrder = int32(im.Order)
	}
	sum := Checksum(im.ServiceID, im.Path, im.Title, im.Order)

	target, mapped, err := r.m.Lookup(ctx, key.System, key.Table, key.ID)
	if err != nil {
		return err
	}
	if mapped {
		id, err := r.q.MigratorServiceImageByUUID(ctx, target)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read image: %w", err)
		}
		if err == nil {
			res, err := r.m.Upsert(ctx, key, sum)
			if err != nil {
				return err
			}
			if !res.Changed {
				r.c.inc("images_unchanged")
				return nil
			}
			if err := r.q.MigratorUpdateServiceImage(ctx, db.MigratorUpdateServiceImageParams{
				ID: id, Title: title, SortOrder: sortOrder,
			}); err != nil {
				return fmt.Errorf("update image: %w", err)
			}
			r.c.inc("images_updated")
			return nil
		}
	}

	// The file first: a missing one leaves the image unmapped, so a later
	// run with the file imports it.
	if r.dst.LegacyFiles == nil || r.dst.Storage == nil {
		r.report("image_skipped_no_storage", im.ID)
		return nil
	}
	body, err := readLegacyFile(r.dst.LegacyFiles, im.Path, maxLegacyServiceImageBytes)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		r.report("image_skipped_missing", im.ID)
		return nil
	case errors.Is(err, errFileTooLarge):
		r.report("image_skipped_too_large", im.ID)
		return nil
	case err != nil:
		return fmt.Errorf("read image %q: %w", im.Path, err)
	}
	mime, err := storage.DetectLogoMIME("", body)
	if err != nil {
		r.report("image_skipped_unsupported", im.ID)
		return nil
	}
	ext, err := storage.LogoExtForMIME(mime)
	if err != nil {
		r.report("image_skipped_unsupported", im.ID)
		return nil
	}

	res, err := r.m.Upsert(ctx, key, sum)
	if err != nil {
		return err
	}
	objectKey := storage.ServiceImageObjectKey(ref.OrgUUID, ref.UUID, res.UUID, ext)
	exists, err := r.dst.Storage.Exists(ctx, objectKey)
	if err != nil {
		return fmt.Errorf("image exists: %w", err)
	}
	if !exists && !r.dst.DryRun {
		if err := r.dst.Storage.Upload(ctx, storage.File{
			Body: bytes.NewReader(body), Size: int64(len(body)), ContentType: mime, Filename: path.Base(im.Path),
		}, objectKey); err != nil {
			return fmt.Errorf("upload image: %w", err)
		}
		r.c.inc("images_uploaded")
	}
	if _, err := r.q.MigratorInsertServiceImage(ctx, db.MigratorInsertServiceImageParams{
		Uuid: res.UUID, ServiceID: ref.ID, OrganizationID: ref.OrgID, BrandID: ref.BrandID, StorageKey: objectKey,
		Title: title, SortOrder: sortOrder, CreatedAt: pgTime(im.CreatedAt),
	}); err != nil {
		return fmt.Errorf("insert image: %w", err)
	}
	r.c.inc("images_created")
	return nil
}

// importLog appends a legacy status log once; service_status_logs is
// append-only, so a changed legacy log is reported, not rewritten.
func (r *serviceRun) importLog(ctx context.Context, ref *serviceRef, l legacyStatusLog) error {
	r.c.inc("logs_read")
	key := Key{System: r.step.system(), Table: "service_status_logs", ID: strconv.FormatInt(l.ID, 10),
		TargetTable: "service_status_logs"}
	sum := Checksum(l.ServiceID, l.FromDealerID.Int64, l.FromDealerID.Valid, l.ToDealerID.Int64, l.ToDealerID.Valid,
		l.UserID.Int64, l.UserID.Valid, l.Notes)
	_, mapped, err := r.m.Lookup(ctx, key.System, key.Table, key.ID)
	if err != nil {
		return err
	}
	if mapped {
		res, err := r.m.Upsert(ctx, key, sum)
		if err != nil {
			return err
		}
		if res.Changed {
			r.report("log_changed_kept", l.ID)
		} else {
			r.c.inc("logs_unchanged")
		}
		return nil
	}

	meta := map[string]any{"source": "legacy_hub.service_status_logs", "legacy_id": l.ID}
	dealerOrg := func(field string, id sql.NullInt64) (int64, error) {
		if !id.Valid {
			return 0, nil
		}
		org, err := r.dealerOrg(ctx, id.Int64)
		if err != nil || org == nil {
			if err == nil {
				r.report("log_dealer_unmapped", l.ID)
			}
			return 0, err
		}
		meta[field] = org.ID
		return org.ID, nil
	}
	if _, err := dealerOrg("from_organization_id", l.FromDealerID); err != nil {
		return err
	}
	to, err := dealerOrg("to_organization_id", l.ToDealerID)
	if err != nil {
		return err
	}
	// The hub writes the author's dealer as to_dealer_id; none is the center.
	actorOrg := pgtype.Int8{}
	switch {
	case to != 0:
		actorOrg = pgInt8(to)
	case !l.ToDealerID.Valid:
		actorOrg = pgInt8(r.tree.CenterID)
	}
	actor := int64(0)
	if l.UserID.Valid {
		if actor, err = r.user(ctx, l.UserID.Int64); err != nil {
			return err
		}
		if actor == 0 {
			r.report("log_user_unmapped", l.ID)
		}
	}
	metadata, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if _, err := r.m.Upsert(ctx, key, sum); err != nil {
		return err
	}
	status := ref.Target
	if status == "" {
		status = ref.Status
	}
	if err := r.q.MigratorInsertServiceStatusLog(ctx, db.MigratorInsertServiceStatusLogParams{
		ServiceID: ref.ID, OrganizationID: ref.OrgID, BrandID: ref.BrandID, ToStatus: status,
		ActorUserID: pgtype.Int8{Int64: actor, Valid: actor != 0}, ActorOrgID: actorOrg,
		Note: pgText(strings.TrimSpace(l.Notes)), Metadata: metadata, CreatedAt: pgTime(l.CreatedAt),
	}); err != nil {
		return fmt.Errorf("insert status log: %w", err)
	}
	r.c.inc("logs_created")
	return nil
}

// dealerOrg resolves a hub dealer to its organization (nil: not migrated).
func (r *serviceRun) dealerOrg(ctx context.Context, dealerID int64) (*db.MigratorOrganizationByUUIDRow, error) {
	if org, ok := r.orgs[dealerID]; ok {
		return org, nil
	}
	var out *db.MigratorOrganizationByUUIDRow
	target, ok, err := r.m.Lookup(ctx, r.step.system(), "dealers", strconv.FormatInt(dealerID, 10))
	if err != nil {
		return nil, err
	}
	if ok {
		row, err := r.q.MigratorOrganizationByUUID(ctx, target)
		switch {
		case err == nil:
			out = &row
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, fmt.Errorf("dealer organization: %w", err)
		}
	}
	r.orgs[dealerID] = out
	return out, nil
}

// customerUser resolves a hub customer to its user (0: not migrated).
func (r *serviceRun) customerUser(ctx context.Context, customerID int64) (int64, error) {
	target, ok, err := r.m.Lookup(ctx, r.step.system(), "customers", strconv.FormatInt(customerID, 10))
	if err != nil || !ok {
		return 0, err
	}
	id, err := r.q.MigratorCustomerUserByUUID(ctx, target)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("customer user: %w", err)
	}
	return id, nil
}

// user resolves a hub user to users.id (0: not migrated).
func (r *serviceRun) user(ctx context.Context, userID int64) (int64, error) {
	if id, ok := r.users[userID]; ok {
		return id, nil
	}
	id := int64(0)
	target, ok, err := r.m.Lookup(ctx, r.step.system(), "users", strconv.FormatInt(userID, 10))
	if err != nil {
		return 0, err
	}
	if ok {
		row, err := r.q.MigratorUserByUUID(ctx, target)
		switch {
		case err == nil:
			id = row.ID
		case !errors.Is(err, pgx.ErrNoRows):
			return 0, fmt.Errorf("user: %w", err)
		}
	}
	r.users[userID] = id
	return id, nil
}

// carModel resolves the legacy car brand / model. The model decides the
// brand (services_check_row); a model under another brand is reported. 0
// means the service is skipped (reported).
func (r *serviceRun) carModel(ctx context.Context, ls legacyService) (int64, int64, error) {
	target, ok, err := r.m.Lookup(ctx, r.step.system(), "car_models", strconv.FormatInt(ls.CarModelID, 10))
	if err != nil {
		return 0, 0, err
	}
	if !ok {
		r.report("service_skipped_car_model_unmapped", ls.ID)
		return 0, 0, nil
	}
	model, err := r.q.MigratorCarModelByUUID(ctx, target)
	if errors.Is(err, pgx.ErrNoRows) {
		r.report("service_skipped_car_model_unmapped", ls.ID)
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("car model: %w", err)
	}
	brandUUID, ok, err := r.m.Lookup(ctx, r.step.system(), "car_brands", strconv.FormatInt(ls.CarBrandID, 10))
	if err != nil {
		return 0, 0, err
	}
	if ok {
		brand, err := r.q.MigratorCarBrandByUUID(ctx, brandUUID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, fmt.Errorf("car brand: %w", err)
		}
		if err == nil && brand.ID != model.CarBrandID {
			r.report("car_brand_from_model", ls.ID)
		}
	}
	return model.CarBrandID, model.ID, nil
}
