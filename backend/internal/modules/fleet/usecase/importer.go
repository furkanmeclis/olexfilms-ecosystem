package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ImportResource is the fleet vehicle import (TEC-473): CSV / XLSX rows
// with plate, VIN, car brand, car model and model year. It follows the
// staged import pattern (TEC-158): preview is a dry run that classifies
// every row, confirm applies the new / link rows, undo removes the created
// vehicles and unlinks the linked ones within the undo window. The staged
// rows live in the job's preview (import_jobs.preview_json), the applied
// changes in import_changes.
const ImportResource = "tenant.fleet.vehicles"

// ImportDefaultFleetUUID is the job default naming the target fleet (set by
// POST /v1/fleets/{uuid}/vehicles/import; re-checked on every step).
const ImportDefaultFleetUUID = "fleet_uuid"

// maxImportRows bounds one fleet import file.
const maxImportRows = 1000

// Row statuses of the fleet vehicle import.
const (
	ImportRowNew          = "new"
	ImportRowLink         = "link"
	ImportRowDuplicate    = "duplicate"
	ImportRowInvalid      = "invalid"
	ImportRowConflict     = "conflict"
	ImportRowApplied      = "applied"
	ImportRowUndone       = "undone"
	ImportRowUndoRejected = "undo_rejected"
)

// Row error codes of the fleet vehicle import.
const (
	ImportErrPlateRequired   = "FLEET_IMPORT_PLATE_REQUIRED"
	ImportErrPlateInvalid    = "FLEET_IMPORT_PLATE_INVALID"
	ImportErrVINInvalid      = "FLEET_IMPORT_VIN_INVALID"
	ImportErrYearInvalid     = "FLEET_IMPORT_YEAR_INVALID"
	ImportErrBrandNotFound   = "FLEET_IMPORT_BRAND_NOT_FOUND"
	ImportErrModelNotFound   = "FLEET_IMPORT_MODEL_NOT_FOUND"
	ImportErrDuplicateInFile = "FLEET_IMPORT_DUPLICATE_IN_FILE"
	ImportErrInFleet         = "FLEET_IMPORT_ALREADY_IN_FLEET"
	ImportErrOtherOwner      = "FLEET_IMPORT_OTHER_OWNER"
	ImportErrUndoUsed        = "FLEET_IMPORT_UNDO_VEHICLE_USED"
)

var importErrorField = map[string]string{
	ImportErrPlateRequired: "plate", ImportErrPlateInvalid: "plate", ImportErrVINInvalid: "vin",
	ImportErrYearInvalid: "model_year", ImportErrBrandNotFound: "car_brand", ImportErrModelNotFound: "car_model",
	ImportErrDuplicateInFile: "plate", ImportErrInFleet: "plate", ImportErrOtherOwner: "plate",
}

// ImportErrorKey is the i18n catalog key of a row error code.
func ImportErrorKey(code string) string {
	return "fleet_import.error." + strings.ToLower(strings.TrimPrefix(code, "FLEET_IMPORT_"))
}

// ImportStatusKey is the i18n catalog key of a row status.
func ImportStatusKey(status string) string { return "fleet_import.status." + status }

// Importer is the staged fleet vehicle importer.
type Importer struct{ svc *Service }

// NewImporter creates the fleet vehicle importer.
func NewImporter(svc *Service) *Importer { return &Importer{svc: svc} }

var (
	_ ioengine.ResourceAdapter = (*Importer)(nil)
	_ ioengine.StagedImporter  = (*Importer)(nil)
)

// Resource implements ioengine.ResourceAdapter.
func (im *Importer) Resource() string { return ImportResource }

// ExportColumns implements ioengine.ResourceAdapter (import only).
func (im *Importer) ExportColumns() []ioengine.Column { return nil }

// Export implements ioengine.ResourceAdapter (import only).
func (im *Importer) Export(context.Context, ioengine.ExportQuery, i18n.Locale) (ioengine.Dataset, error) {
	return ioengine.Dataset{}, errors.New("fleet vehicle import is import only")
}

// ImportSchema implements ioengine.ResourceAdapter.
func (im *Importer) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "plate", LabelKey: "fleet_import.plate", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "vin", LabelKey: "fleet_import.vin", Type: ioengine.ColumnTypeString},
		{Key: "car_brand", LabelKey: "fleet_import.car_brand", Type: ioengine.ColumnTypeString},
		{Key: "car_model", LabelKey: "fleet_import.car_model", Type: ioengine.ColumnTypeString},
		{Key: "model_year", LabelKey: "fleet_import.model_year", Type: ioengine.ColumnTypeString},
		{Key: "plate_country", LabelKey: "fleet_import.plate_country", Type: ioengine.ColumnTypeString},
	}
}

// SampleRows is the sample file content.
func (im *Importer) SampleRows() []map[string]any {
	return []map[string]any{
		{"plate": "34 ABC 123", "vin": "WVWZZZ1KZAW000001", "car_brand": "Volkswagen", "car_model": "Golf", "model_year": "2021", "plate_country": "TR"},
		{"plate": "06 XYZ 45", "vin": "", "car_brand": "Renault", "car_model": "Clio", "model_year": "2019", "plate_country": ""},
	}
}

// ApplyRow implements ioengine.ResourceAdapter (staged; never called).
func (im *Importer) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "fleet vehicle import is staged"}, nil
}

// RevertRow implements ioengine.ResourceAdapter (staged; never called).
func (im *Importer) RevertRow(context.Context, string, string, map[string]any) error {
	return errors.New("fleet vehicle import is staged")
}

// importFleet is the target of a job: the fleet the job organization holds
// an active link to (or reaches with the brand center), with a primary user.
type importFleet struct {
	access  access
	primary int64
	caller  Caller
}

func rejected(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ioengine.ErrImportRejected}, args...)...)
}

func (im *Importer) target(ctx context.Context, q *db.Queries, job ioengine.ImportJob, raw any) (importFleet, error) {
	id, err := uuid.Parse(strings.TrimSpace(fmt.Sprint(raw)))
	if err != nil || raw == nil {
		return importFleet{}, rejected("the import has no fleet")
	}
	org, err := q.GetOrganizationByID(ctx, job.OrganizationID)
	if err != nil {
		return importFleet{}, rejected("the job organization is unknown")
	}
	c := Caller{UserID: job.ActorID, OrgID: org.ID, BrandID: org.BrandID, OrgType: org.Type}
	c.Filter.OrgIDs = []int64{org.ID}
	a, err := im.svc.resolve(ctx, q, c, id)
	if err != nil {
		return importFleet{}, rejected("the fleet is not linked to the organization")
	}
	if !a.fleet.FleetProfile.PrimaryUserID.Valid {
		return importFleet{}, rejected("invite the first fleet user (the vehicle owner) first")
	}
	return importFleet{access: a, primary: a.fleet.FleetProfile.PrimaryUserID.Int64, caller: c}, nil
}

// cell reads one mapped cell as trimmed text (XLSX numbers included).
func cell(row map[string]any, key string) string {
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

// stageRow validates one row: the vehicle data, or the error code.
func (im *Importer) stageRow(ctx context.Context, q *db.Queries, orgID int64, row map[string]any) (vehicleData, map[string]any, string) {
	data := map[string]any{
		"plate": cell(row, "plate"), "vin": cell(row, "vin"), "car_brand": cell(row, "car_brand"),
		"car_model": cell(row, "car_model"), "model_year": cell(row, "model_year"),
		"plate_country": cell(row, "plate_country"),
	}
	var d vehicleData
	if data["plate"] == "" {
		return d, data, ImportErrPlateRequired
	}
	plate, norm, country, err := im.svc.preparePlate(ctx, q, orgID, data["plate"].(string), data["plate_country"].(string))
	if err != nil {
		return d, data, ImportErrPlateInvalid
	}
	d.plate, d.normalized, d.country = plate, norm, country
	data["plate"], data["plate_country"] = plate, country
	if d.vin, err = normalizeVIN(data["vin"].(string)); err != nil {
		return d, data, ImportErrVINInvalid
	}
	data["vin"] = d.vin
	if y := data["model_year"].(string); y != "" {
		n, err := strconv.Atoi(strings.TrimSuffix(y, ".0"))
		if err != nil {
			return d, data, ImportErrYearInvalid
		}
		if d.year, err = modelYear(&n, im.svc.now().UTC().Year()); err != nil {
			return d, data, ImportErrYearInvalid
		}
	}
	if b := data["car_brand"].(string); b != "" {
		brand, err := q.FindCarBrandByName(ctx, b)
		if err != nil {
			return d, data, ImportErrBrandNotFound
		}
		d.brandID = pgtype.Int8{Int64: brand.ID, Valid: true}
		data["car_brand"] = brand.Name
		if m := data["car_model"].(string); m != "" {
			model, err := q.FindCarModelByName(ctx, db.FindCarModelByNameParams{CarBrandID: brand.ID, Name: m})
			if err != nil {
				return d, data, ImportErrModelNotFound
			}
			d.modelID = pgtype.Int8{Int64: model.ID, Valid: true}
			data["car_model"] = model.Name
		}
	} else if data["car_model"].(string) != "" {
		return d, data, ImportErrBrandNotFound
	}
	return d, data, ""
}

// Stage implements ioengine.StagedImporter: the dry run. Every row is
// classified new (a vehicle is created), link (an existing vehicle of the
// fleet's users joins the fleet), duplicate (in the file twice or already
// in the fleet), conflict (another customer's vehicle) or invalid; nothing
// is written.
func (im *Importer) Stage(ctx context.Context, job ioengine.ImportJob, rows []map[string]any, defaults map[string]any) (ioengine.PreviewSummary, error) {
	if len(rows) > maxImportRows {
		return ioengine.PreviewSummary{}, rejected("at most %d rows per file", maxImportRows)
	}
	q := db.New(im.svc.conn)
	t, err := im.target(ctx, q, job, defaults[ImportDefaultFleetUUID])
	if err != nil {
		return ioengine.PreviewSummary{}, err
	}
	fleet := t.access.fleet.Organization
	loc := i18n.Normalize(job.Locale)
	sum := ioengine.PreviewSummary{Total: len(rows), BatchUUID: job.UUID.String(), Counts: map[string]int{}}
	seen := map[string]bool{}
	for i, row := range rows {
		d, data, code := im.stageRow(ctx, q, job.OrganizationID, row)
		status := ImportRowNew
		target := map[string]any{"fleet_uuid": fleet.Uuid.String()}
		if code == "" {
			keys := []string{"p:" + d.country + d.normalized}
			if d.vin != "" {
				keys = append(keys, "v:"+d.vin)
			}
			for _, k := range keys {
				if seen[k] {
					code = ImportErrDuplicateInFile
				}
			}
			for _, k := range keys {
				seen[k] = true
			}
		}
		if code == "" {
			m, err := findMatch(ctx, q, fleet, d)
			if err != nil {
				return ioengine.PreviewSummary{}, err
			}
			switch {
			case m == nil:
			case m.inFleet:
				code = ImportErrInFleet
			case m.attachable:
				status = ImportRowLink
				target["vehicle_uuid"] = m.vehicle.Uuid.String()
			default:
				code = ImportErrOtherOwner
			}
		}
		switch code {
		case "":
		case ImportErrDuplicateInFile, ImportErrInFleet:
			status = ImportRowDuplicate
		case ImportErrOtherOwner:
			status = ImportRowConflict
		default:
			status = ImportRowInvalid
		}
		if code != "" {
			sum.Errors = append(sum.Errors, ioengine.RowError{
				Index: i + 1, Field: importErrorField[code], Code: code, Error: i18n.Translate(loc, ImportErrorKey(code)),
			})
		}
		if status == ImportRowNew || status == ImportRowLink {
			sum.Valid++
		} else {
			sum.Invalid++
		}
		sum.Counts[status]++
		sum.Rows = append(sum.Rows, ioengine.RowPreview{
			Index: i + 1, Data: data, Status: status, StatusLabel: i18n.Translate(loc, ImportStatusKey(status)),
			Target: target,
		})
	}
	return sum, nil
}

// stagedSummary reads the job's stored preview (the staged rows).
func stagedSummary(ctx context.Context, q *db.Queries, jobID int64) (db.ImportJob, ioengine.PreviewSummary, error) {
	job, err := q.GetImportJobByID(ctx, jobID)
	if err != nil {
		return db.ImportJob{}, ioengine.PreviewSummary{}, fmt.Errorf("fleet import: job: %w", err)
	}
	var sum ioengine.PreviewSummary
	if len(job.PreviewJson) == 0 || json.Unmarshal(job.PreviewJson, &sum) != nil || sum.BatchUUID == "" {
		return job, sum, rejected("the import has no preview")
	}
	return job, sum, nil
}

func jobDefaults(job db.ImportJob) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal(job.DefaultsJson, &out)
	return out
}

func recount(sum *ioengine.PreviewSummary, loc i18n.Locale) {
	sum.Counts = map[string]int{}
	for i := range sum.Rows {
		sum.Counts[sum.Rows[i].Status]++
		sum.Rows[i].StatusLabel = i18n.Translate(loc, ImportStatusKey(sum.Rows[i].Status))
	}
}

// Apply implements ioengine.StagedImporter: in one transaction the new rows
// become fleet vehicles and the link rows join the fleet; every change is
// logged in import_changes for the undo. A row whose state changed since the
// preview becomes a conflict. A second run returns the applied summary.
func (im *Importer) Apply(ctx context.Context, job ioengine.ImportJob) (ioengine.PreviewSummary, error) {
	var out ioengine.PreviewSummary
	err := im.svc.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		row, sum, err := stagedSummary(ctx, q, job.ID)
		if err != nil {
			return err
		}
		if changes, err := q.ListImportChangesForJob(ctx, job.ID); err != nil {
			return err
		} else if len(changes) > 0 {
			out = sum
			return nil
		}
		t, err := im.target(ctx, q, job, jobDefaults(row)[ImportDefaultFleetUUID])
		if err != nil {
			return err
		}
		fleet := t.access.fleet.Organization
		loc := i18n.Normalize(job.Locale)
		for i := range sum.Rows {
			r := &sum.Rows[i]
			if r.Status != ImportRowNew && r.Status != ImportRowLink {
				continue
			}
			v, op, code, err := im.applyRow(ctx, q, tx, t, *r)
			if err != nil {
				return err
			}
			if code != "" {
				r.Status = ImportRowConflict
				sum.Errors = append(sum.Errors, ioengine.RowError{
					Index: r.Index, Field: importErrorField[code], Code: code, Error: i18n.Translate(loc, ImportErrorKey(code)),
				})
				continue
			}
			prev, _ := json.Marshal(map[string]any{"fleet_org_id": nil, "fleet_id": fleet.ID})
			if _, err := q.InsertImportChange(ctx, db.InsertImportChangeParams{
				JobID: job.ID, EntityType: "vehicle", EntityUuid: v.Uuid, Op: op, PreviousJson: prev,
			}); err != nil {
				return fmt.Errorf("fleet import: change log: %w", err)
			}
			r.Status = ImportRowApplied
			r.Target["vehicle_uuid"] = v.Uuid.String()
		}
		recount(&sum, loc)
		out = sum
		return nil
	})
	return out, err
}

// applyRow writes one staged row; code is set when the row no longer fits
// (the vehicle appeared or changed owner since the preview).
func (im *Importer) applyRow(ctx context.Context, q *db.Queries, tx pgx.Tx, t importFleet, r ioengine.RowPreview) (db.Vehicle, string, string, error) {
	fleet := t.access.fleet.Organization
	d, _, code := im.stageRow(ctx, q, t.caller.OrgID, r.Data)
	if code != "" {
		return db.Vehicle{}, "", code, nil
	}
	m, err := findMatch(ctx, q, fleet, d)
	if err != nil {
		return db.Vehicle{}, "", "", err
	}
	switch {
	case m == nil:
		v, err := q.CreateVehicle(ctx, db.CreateVehicleParams{
			UserID: t.primary, OrganizationID: pgtype.Int8{Int64: t.caller.OrgID, Valid: true},
			BrandID: fleet.BrandID, CarBrandID: d.brandID, CarModelID: d.modelID, ModelYear: d.year,
			Plate: optText(d.plate), PlateNormalized: optText(d.normalized), PlateCountry: optText(d.country),
			Vin: optText(d.vin),
		})
		if err != nil {
			return db.Vehicle{}, "", "", fmt.Errorf("fleet import: create vehicle: %w", err)
		}
		if err := im.svc.vehicleEvent(ctx, tx, t.caller, events.VehicleCreated, v); err != nil {
			return db.Vehicle{}, "", "", err
		}
		v, err = im.svc.attach(ctx, q, tx, t.caller, fleet, v)
		return v, "create", "", err
	case m.inFleet:
		return db.Vehicle{}, "", ImportErrInFleet, nil
	case m.attachable:
		v, err := q.GetVehicleByIDForUpdate(ctx, m.vehicle.ID)
		if err != nil {
			return db.Vehicle{}, "", "", fmt.Errorf("fleet import: vehicle: %w", err)
		}
		v, err = im.svc.attach(ctx, q, tx, t.caller, fleet, v)
		return v, "update", "", err
	default:
		return db.Vehicle{}, "", ImportErrOtherOwner, nil
	}
}

// Undo implements ioengine.StagedImporter: in one transaction a created
// vehicle is deleted again (refused once it has a service) and a linked
// vehicle leaves the fleet (refused when it is no longer in this fleet).
func (im *Importer) Undo(ctx context.Context, job ioengine.ImportJob) (ioengine.PreviewSummary, error) {
	var out ioengine.PreviewSummary
	err := im.svc.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		_, sum, err := stagedSummary(ctx, q, job.ID)
		if err != nil {
			return err
		}
		changes, err := q.ListImportChangesForJob(ctx, job.ID)
		if err != nil {
			return err
		}
		org, err := q.GetOrganizationByID(ctx, job.OrganizationID)
		if err != nil {
			return fmt.Errorf("fleet import: organization: %w", err)
		}
		c := Caller{UserID: job.ActorID, OrgID: org.ID, BrandID: org.BrandID, OrgType: org.Type}
		loc := i18n.Normalize(job.Locale)
		result := map[string]string{}
		for i := len(changes) - 1; i >= 0; i-- {
			ch := changes[i]
			status, err := im.undoChange(ctx, q, tx, c, ch)
			if err != nil {
				return err
			}
			result[ch.EntityUuid.String()] = status
		}
		for i := range sum.Rows {
			r := &sum.Rows[i]
			if r.Status != ImportRowApplied {
				continue
			}
			vid, _ := r.Target["vehicle_uuid"].(string)
			status, ok := result[vid]
			if !ok {
				continue
			}
			r.Status = status
			if status == ImportRowUndoRejected {
				sum.Errors = append(sum.Errors, ioengine.RowError{
					Index: r.Index, Code: ImportErrUndoUsed, Error: i18n.Translate(loc, ImportErrorKey(ImportErrUndoUsed)),
				})
			}
		}
		recount(&sum, loc)
		out = sum
		return nil
	})
	return out, err
}

func (im *Importer) undoChange(ctx context.Context, q *db.Queries, tx pgx.Tx, c Caller, ch db.ImportChange) (string, error) {
	var prev struct {
		FleetID int64 `json:"fleet_id"`
	}
	_ = json.Unmarshal(ch.PreviousJson, &prev)
	v, err := q.GetVehicleByUUIDForUpdate(ctx, ch.EntityUuid)
	if errors.Is(err, pgx.ErrNoRows) {
		return ImportRowUndoRejected, nil
	}
	if err != nil {
		return "", fmt.Errorf("fleet import: vehicle: %w", err)
	}
	if !v.FleetOrgID.Valid || v.FleetOrgID.Int64 != prev.FleetID {
		return ImportRowUndoRejected, nil
	}
	if ch.Op == "create" {
		used, err := q.VehicleHasServices(ctx, v.ID)
		if err != nil {
			return "", fmt.Errorf("fleet import: vehicle services: %w", err)
		}
		if used {
			return ImportRowUndoRejected, nil
		}
		if _, err := q.SoftDeleteVehicle(ctx, v.ID); err != nil {
			return "", fmt.Errorf("fleet import: delete vehicle: %w", err)
		}
		return ImportRowUndone, im.svc.vehicleEvent(ctx, tx, c, events.VehicleDeleted, v)
	}
	row, err := q.SetVehicleFleet(ctx, db.SetVehicleFleetParams{ID: v.ID})
	if err != nil {
		return "", fmt.Errorf("fleet import: unlink vehicle: %w", err)
	}
	return ImportRowUndone, im.svc.vehicleEvent(ctx, tx, c, events.VehicleUpdated, row)
}
