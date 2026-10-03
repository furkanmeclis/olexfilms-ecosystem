package migrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
)

// ---------------------------------------------------------------- step 8

// MeasurementsStep imports the hub NexPTG data (TEC-262, step 8). Only data
// moves: the NexPTG device sync API is dropped (design §9) and the device
// passwords are never read.
//
//   - nexptg_api_users -> measurement_devices: one device per legacy API
//     account in the organization of its user's dealer (else the Olex
//     center). The serial is the one device serial the account's reports
//     carry; an account with none (or several) keeps its username as the
//     serial (reported). A device the organization already holds under the
//     serial is linked, not duplicated. The username is the label.
//   - nexptg_reports with their nexptg_report_measurements and the
//     service_nexptg_report pivot -> one measurement_results row per report,
//     source legacy_import. raw holds the report, its measurement rows (all
//     of them, in id order) and the service link. The service and vehicle
//     come from the services step (migration_map "services" and
//     "services.vehicle"); the organization is the service's, else the
//     dealer of the account's / report's user, else the Olex center. A
//     report without a usable VIN is vin_pending ("tamamlanacak").
//
// Every legacy row is recorded in migration_map: the measurement rows and
// the pivot rows point at the result of their report, so the cutover delta
// report (TEC-276) does not list migrated results. A report is written as a
// whole, so the step always reads the full NexPTG tables (122 reports /
// 7,081 rows in production); the checksum keeps a rerun from writing
// anything. Nothing is written to the outbox.
type MeasurementsStep struct {
	// System is the migration_map source system; empty means SourceHub.
	System string
}

// Name implements Step.
func (MeasurementsStep) Name() string { return "measurements" }

func (s MeasurementsStep) system() string {
	if s.System == "" {
		return SourceHub
	}
	return s.System
}

// Column limits of measurement_results / measurement_devices (000076).
const (
	measurementSerialMax = 64
	measurementLabelMax  = 255
)

// Measurement statuses (chk_measurement_results_status).
const (
	measurementAccepted   = "accepted"
	measurementVINPending = "vin_pending"
)

// measurementVINRe is the VIN rule of the mobile measurement upload
// (measurements usecase): 11 to 17 letters or digits.
var measurementVINRe = regexp.MustCompile(`^[A-Z0-9]{11,17}$`)

// legacyTimeLayout writes the legacy (zone-less) timestamps into raw as the
// hub stored them.
const legacyTimeLayout = "2006-01-02 15:04:05"

// The password column is never read.
const nexptgAPIUsersQuery = `SELECT a.id, a.user_id, a.username, a.is_active, a.last_used_at, a.created_at, a.updated_at
FROM nexptg_api_users a ORDER BY a.id`

// The source guard refuses the word COMMENT (a column of the table), and
// quoting differs between the engines: the report is read with r.* and
// scanned by column name (scanByName).
const nexptgReportsQuery = `SELECT r.* FROM nexptg_reports r ORDER BY r.id`

// "timestamp" and "position" are keywords; qualified, they need no quoting.
const nexptgMeasurementsQuery = `SELECT m.id, m.report_id, m.is_inside, m.place_id, m.part_type, m.value, m.interpretation,
	m.substrate_type, m.timestamp, m.position, m.created_at, m.updated_at
FROM nexptg_report_measurements m ORDER BY m.report_id, m.id`

const nexptgServiceLinksQuery = `SELECT p.id, p.service_id, p.nexptg_report_id, p.match_type, p.created_at, p.updated_at
FROM service_nexptg_report p ORDER BY p.id`

type legacyAPIUser struct {
	ID                   int64
	UserID               sql.NullInt64
	Username             string
	Active               bool
	LastUsedAt           sql.NullTime
	CreatedAt, UpdatedAt sql.NullTime
}

type legacyReport struct {
	ID                             int64
	APIUserID, UserID, ExternalID  sql.NullInt64
	Name                           string
	Date, CalibrationDate          sql.NullTime
	DeviceSerial, Model            sql.NullString
	CarModelID                     sql.NullInt64
	Brand                          sql.NullString
	CarBrandID                     sql.NullInt64
	TypeOfBody, BodyType           sql.NullString
	Capacity, Power, VIN, FuelType sql.NullString
	Year, UnitOfMeasure            sql.NullString
	ExtraFields                    []byte
	Comment                        sql.NullString
	CreatedAt, UpdatedAt           sql.NullTime
	measurements                   []legacyMeasurement
	link                           *legacyServiceLink
}

type legacyMeasurement struct {
	ID, ReportID         int64
	Inside               bool
	PlaceID, PartType    string
	Value                sql.NullString
	Interpretation       sql.NullInt64
	Substrate            sql.NullString
	Timestamp            sql.NullTime
	Position             sql.NullInt64
	CreatedAt, UpdatedAt sql.NullTime
}

type legacyServiceLink struct {
	ID, ServiceID, ReportID int64
	MatchType               string
	CreatedAt, UpdatedAt    sql.NullTime
}

type measurementRun struct {
	step       MeasurementsStep
	q          *db.Queries
	m          *Mapper
	c          counts
	brandID    int64
	centerID   int64
	userDealer map[int64]int64 // hub user id -> hub dealer id
	orgs       map[int64]int64 // hub dealer id -> organization id (0: unmapped)
	users      map[int64]int64 // hub user id -> users.id (0: unmapped)
}

// Run implements Step.
func (s MeasurementsStep) Run(ctx context.Context, src Sources, dst *Target, m *Mapper) (StepResult, error) {
	c := counts{}
	hub, err := src.Get(SourceHub)
	if err != nil {
		return StepResult{}, err
	}
	brand, err := dst.Q.GetBrandBySlug(ctx, OlexBrandSlug)
	if err != nil {
		return StepResult{}, fmt.Errorf("brand %q: %w", OlexBrandSlug, err)
	}
	center, err := dst.Q.GetBrandCenter(ctx, brand.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return StepResult{}, errors.New("olex center is missing; run the organizations step first")
	}
	if err != nil {
		return StepResult{}, fmt.Errorf("olex center: %w", err)
	}
	r := &measurementRun{step: s, q: dst.Q, m: m, c: c, brandID: brand.ID, centerID: center.ID,
		orgs: map[int64]int64{}, users: map[int64]int64{}}
	if r.userDealer, err = readDealerLinks(ctx, hub, legacyUserDealersQuery); err != nil {
		return StepResult{Counts: c}, fmt.Errorf("user dealers: %w", err)
	}

	apiUsers, err := readAPIUsers(ctx, hub)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	reports, err := readReports(ctx, hub)
	if err != nil {
		return StepResult{Counts: c}, err
	}

	var watermark time.Time
	mark := func(ts ...sql.NullTime) {
		if t := latest(ts...); t.After(watermark) {
			watermark = t
		}
	}
	// The device serials each API account's reports carry.
	serials := map[int64]map[string]bool{}
	for _, rep := range reports {
		mark(rep.CreatedAt, rep.UpdatedAt)
		for _, ms := range rep.measurements {
			mark(ms.CreatedAt, ms.UpdatedAt)
		}
		if rep.link != nil {
			mark(rep.link.CreatedAt, rep.link.UpdatedAt)
		}
		serial := truncate(strings.TrimSpace(rep.DeviceSerial.String), measurementSerialMax)
		if rep.APIUserID.Valid && serial != "" {
			if serials[rep.APIUserID.Int64] == nil {
				serials[rep.APIUserID.Int64] = map[string]bool{}
			}
			serials[rep.APIUserID.Int64][serial] = true
		}
	}

	for _, u := range apiUsers {
		c.inc("devices_read")
		mark(u.CreatedAt, u.UpdatedAt, u.LastUsedAt)
		if err := r.importDevice(ctx, u, serials[u.ID]); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("nexptg api user %d: %w", u.ID, err)
		}
	}
	apiUserOf := map[int64]legacyAPIUser{}
	for _, u := range apiUsers {
		apiUserOf[u.ID] = u
	}
	for _, rep := range reports {
		c.inc("reports_read")
		c.add("measurements_read", int64(len(rep.measurements)))
		if err := r.importReport(ctx, rep, apiUserOf); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("nexptg report %d: %w", rep.ID, err)
		}
	}
	return StepResult{Counts: c, Watermark: watermark}, nil
}

func readAPIUsers(ctx context.Context, hub source.LegacySource) ([]legacyAPIUser, error) {
	rows, err := hub.Query(ctx, nexptgAPIUsersQuery)
	if err != nil {
		return nil, err
	}
	var out []legacyAPIUser
	for rows.Next() {
		var u legacyAPIUser
		if err := rows.Scan(&u.ID, &u.UserID, &u.Username, &u.Active, &u.LastUsedAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan nexptg api user: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read nexptg api users: %w", err)
	}
	return out, nil
}

// readReports reads the reports with their measurement rows and service
// link attached.
func readReports(ctx context.Context, hub source.LegacySource) ([]*legacyReport, error) {
	rows, err := hub.Query(ctx, nexptgReportsQuery)
	if err != nil {
		return nil, err
	}
	cols, err := rows.Columns()
	if err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("nexptg report columns: %w", err)
	}
	var out []*legacyReport
	byID := map[int64]*legacyReport{}
	for rows.Next() {
		rep := &legacyReport{}
		var extra []byte
		if err := scanByName(rows, cols, map[string]any{
			"id": &rep.ID, "api_user_id": &rep.APIUserID, "user_id": &rep.UserID, "external_id": &rep.ExternalID,
			"name": &rep.Name, "date": &rep.Date, "calibration_date": &rep.CalibrationDate,
			"device_serial_number": &rep.DeviceSerial, "model": &rep.Model, "car_model_id": &rep.CarModelID,
			"brand": &rep.Brand, "car_brand_id": &rep.CarBrandID, "type_of_body": &rep.TypeOfBody,
			"body_type": &rep.BodyType, "capacity": &rep.Capacity, "power": &rep.Power, "vin": &rep.VIN,
			"fuel_type": &rep.FuelType, "year": &rep.Year, "unit_of_measure": &rep.UnitOfMeasure,
			"extra_fields": &extra, "comment": &rep.Comment, "created_at": &rep.CreatedAt, "updated_at": &rep.UpdatedAt,
		}); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan nexptg report: %w", err)
		}
		rep.ExtraFields = jsonOrNil(extra)
		out = append(out, rep)
		byID[rep.ID] = rep
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read nexptg reports: %w", err)
	}

	rows, err = hub.Query(ctx, nexptgMeasurementsQuery)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var ms legacyMeasurement
		if err := rows.Scan(&ms.ID, &ms.ReportID, &ms.Inside, &ms.PlaceID, &ms.PartType, &ms.Value,
			&ms.Interpretation, &ms.Substrate, &ms.Timestamp, &ms.Position, &ms.CreatedAt, &ms.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan nexptg measurement: %w", err)
		}
		// The foreign key cascades; an orphan cannot exist.
		if rep := byID[ms.ReportID]; rep != nil {
			rep.measurements = append(rep.measurements, ms)
		}
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read nexptg measurements: %w", err)
	}

	rows, err = hub.Query(ctx, nexptgServiceLinksQuery)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		l := &legacyServiceLink{}
		if err := rows.Scan(&l.ID, &l.ServiceID, &l.ReportID, &l.MatchType, &l.CreatedAt, &l.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan service nexptg report: %w", err)
		}
		// nexptg_report_id is unique: a report has at most one service.
		if rep := byID[l.ReportID]; rep != nil {
			rep.link = l
		}
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read service nexptg reports: %w", err)
	}
	return out, nil
}

// scanByName scans the current row into dest by column name; a column dest
// does not name is discarded, a dest column the row lacks is an error.
func scanByName(rows source.Rows, cols []string, dest map[string]any) error {
	targets := make([]any, len(cols))
	seen := 0
	for i, c := range cols {
		if d, ok := dest[strings.ToLower(c)]; ok {
			targets[i] = d
			seen++
			continue
		}
		var sink any
		targets[i] = &sink
	}
	if seen != len(dest) {
		var missing []string
		for name := range dest {
			if !slices.ContainsFunc(cols, func(c string) bool { return strings.EqualFold(c, name) }) {
				missing = append(missing, name)
			}
		}
		slices.Sort(missing)
		return fmt.Errorf("missing columns %s", strings.Join(missing, ", "))
	}
	return rows.Scan(targets...)
}

func (r *measurementRun) report(key string, id int64) {
	r.c.inc(key)
	r.c.inc(key + ":" + strconv.FormatInt(id, 10))
}

// DeviceSerial picks the serial of a legacy NexPTG API account: the one
// device serial its reports carry, else the username. multiple is true when
// the reports carry several serials.
func DeviceSerial(username string, reportSerials map[string]bool) (serial string, fromUsername, multiple bool) {
	if len(reportSerials) == 1 {
		for s := range reportSerials {
			return s, false, false
		}
	}
	return truncate(strings.TrimSpace(username), measurementSerialMax), true, len(reportSerials) > 1
}

func (r *measurementRun) importDevice(ctx context.Context, u legacyAPIUser, reportSerials map[string]bool) error {
	serial, fromUsername, multiple := DeviceSerial(u.Username, reportSerials)
	if multiple {
		r.report("device_multiple_serials", u.ID)
	}
	if fromUsername {
		r.c.inc("device_serial_from_username")
	}
	if serial == "" {
		r.report("device_skipped_no_serial", u.ID)
		return nil
	}
	if !u.Active {
		r.c.inc("devices_inactive")
	}
	label := pgText(truncate(strings.TrimSpace(u.Username), measurementLabelMax))
	orgID, err := r.orgOfUsers(ctx, u.UserID)
	if err != nil {
		return err
	}

	key := Key{System: r.step.system(), Table: "nexptg_api_users", ID: strconv.FormatInt(u.ID, 10),
		TargetTable: "measurement_devices"}
	sum := Checksum(u.Username, u.UserID.Int64, u.UserID.Valid, orgID, serial)
	_, mapped, err := r.m.Lookup(ctx, key.System, key.Table, key.ID)
	if err != nil {
		return err
	}
	if !mapped {
		// The organization already registered the device: link it.
		existing, err := r.q.MigratorMeasurementDeviceBySerial(ctx, db.MigratorMeasurementDeviceBySerialParams{
			OrganizationID: orgID, Serial: serial,
		})
		switch {
		case err == nil:
			linked, err := r.m.Link(ctx, key, existing.Uuid, sum)
			if err != nil {
				return err
			}
			if linked {
				r.c.inc("devices_linked")
				return nil
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("find device: %w", err)
		}
	}
	res, err := r.m.Upsert(ctx, key, sum)
	if err != nil {
		return err
	}
	current, err := r.q.MigratorMeasurementDeviceByUUID(ctx, res.UUID)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := r.q.MigratorInsertMeasurementDevice(ctx, db.MigratorInsertMeasurementDeviceParams{
			Uuid: res.UUID, OrganizationID: orgID, BrandID: r.brandID, Serial: serial, Label: label,
			CreatedAt: pgTime(u.CreatedAt),
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				r.report("device_serial_taken", u.ID)
				return nil
			}
			return fmt.Errorf("insert device: %w", err)
		}
		r.c.inc("devices_created")
		return nil
	}
	if err != nil {
		return fmt.Errorf("read device: %w", err)
	}
	if !res.Changed {
		r.c.inc("devices_unchanged")
		return nil
	}
	if current.OrganizationID != orgID || current.Serial != serial {
		other, err := r.q.MigratorMeasurementDeviceBySerial(ctx, db.MigratorMeasurementDeviceBySerialParams{
			OrganizationID: orgID, Serial: serial,
		})
		switch {
		case err == nil && other.ID != current.ID:
			// Another device holds the serial there; the stored one is kept.
			r.report("device_serial_taken", u.ID)
			return nil
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("find device: %w", err)
		}
	}
	if err := r.q.MigratorUpdateMeasurementDevice(ctx, db.MigratorUpdateMeasurementDeviceParams{
		ID: current.ID, OrganizationID: orgID, BrandID: r.brandID, Serial: serial, Label: label,
	}); err != nil {
		return fmt.Errorf("update device: %w", err)
	}
	r.c.inc("devices_updated")
	return nil
}

// MeasurementVIN normalizes a legacy report VIN; ok is false for a value
// that is present but not a VIN (the row is then vin_pending).
func MeasurementVIN(raw string) (vin string, ok bool) {
	v := strings.ToUpper(strings.TrimSpace(raw))
	if v == "" {
		return "", true
	}
	if !measurementVINRe.MatchString(v) {
		return "", false
	}
	return v, true
}

func legacyTimeValue(t sql.NullTime) any {
	if !t.Valid {
		return nil
	}
	return t.Time.Format(legacyTimeLayout)
}

func nullString(s sql.NullString) any {
	if !s.Valid {
		return nil
	}
	return s.String
}

func nullInt(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

// legacyDecimal keeps a decimal as a JSON number (its text, unrounded).
func legacyDecimal(s sql.NullString) any {
	if !s.Valid {
		return nil
	}
	v := strings.TrimSpace(s.String)
	if _, err := strconv.ParseFloat(v, 64); err != nil {
		return v
	}
	return json.Number(v)
}

// MeasurementRaw is the raw document of a migrated report: the report, all
// of its measurement rows and its service link, values as the hub stored
// them.
func MeasurementRaw(system string, rep *legacyReport) ([]byte, error) {
	report := map[string]any{
		"id": rep.ID, "api_user_id": nullInt(rep.APIUserID), "user_id": nullInt(rep.UserID),
		"external_id": nullInt(rep.ExternalID), "name": rep.Name, "date": legacyTimeValue(rep.Date),
		"calibration_date": legacyTimeValue(rep.CalibrationDate), "device_serial_number": nullString(rep.DeviceSerial),
		"model": nullString(rep.Model), "car_model_id": nullInt(rep.CarModelID), "brand": nullString(rep.Brand),
		"car_brand_id": nullInt(rep.CarBrandID), "type_of_body": nullString(rep.TypeOfBody),
		"body_type": nullString(rep.BodyType), "capacity": nullString(rep.Capacity), "power": nullString(rep.Power),
		"vin": nullString(rep.VIN), "fuel_type": nullString(rep.FuelType), "year": nullString(rep.Year),
		"unit_of_measure": nullString(rep.UnitOfMeasure), "comment": nullString(rep.Comment),
		"created_at": legacyTimeValue(rep.CreatedAt), "updated_at": legacyTimeValue(rep.UpdatedAt),
	}
	if rep.ExtraFields != nil {
		report["extra_fields"] = json.RawMessage(rep.ExtraFields)
	} else {
		report["extra_fields"] = nil
	}
	measurements := make([]map[string]any, 0, len(rep.measurements))
	for _, ms := range rep.measurements {
		measurements = append(measurements, map[string]any{
			"id": ms.ID, "is_inside": ms.Inside, "place_id": ms.PlaceID, "part_type": ms.PartType,
			"value": legacyDecimal(ms.Value), "interpretation": nullInt(ms.Interpretation),
			"substrate_type": nullString(ms.Substrate), "timestamp": legacyTimeValue(ms.Timestamp),
			"position": nullInt(ms.Position),
		})
	}
	doc := map[string]any{
		"legacy":       map[string]any{"system": system, "table": "nexptg_reports", "id": rep.ID},
		"report":       report,
		"measurements": measurements,
	}
	if l := rep.link; l != nil {
		doc["service_link"] = map[string]any{"id": l.ID, "service_id": l.ServiceID, "match_type": l.MatchType}
	}
	return json.Marshal(doc)
}

// resultTarget is where a report lands.
type resultTarget struct {
	OrgID, BrandID     int64
	ServiceID, Vehicle pgtype.Int8
	CreatedBy          pgtype.Int8
}

func (r *measurementRun) target(ctx context.Context, rep *legacyReport, apiUser legacyAPIUser, hasAPIUser bool) (resultTarget, error) {
	t := resultTarget{BrandID: r.brandID}
	if rep.link != nil {
		svc, err := r.service(ctx, rep.link.ServiceID)
		if err != nil {
			return t, err
		}
		if svc != nil {
			t.OrgID, t.BrandID = svc.OrganizationID, svc.BrandID
			t.ServiceID = pgtype.Int8{Int64: svc.ID, Valid: true}
			vehicle, err := r.serviceVehicle(ctx, rep.link.ServiceID)
			if err != nil {
				return t, err
			}
			if vehicle == 0 {
				vehicle = svc.VehicleID
			}
			t.Vehicle = pgtype.Int8{Int64: vehicle, Valid: vehicle != 0}
		} else {
			r.report("service_unmapped", rep.ID)
		}
	}
	deviceOwner := sql.NullInt64{}
	if hasAPIUser {
		deviceOwner = apiUser.UserID
	}
	if t.OrgID == 0 {
		// The dealer of the account's user (the device owner), else of the
		// report's user.
		var err error
		if t.OrgID, err = r.orgOfUsers(ctx, deviceOwner, rep.UserID); err != nil {
			return t, err
		}
	}
	// The report's user (a mobile upload), else the device owner.
	for _, o := range []sql.NullInt64{rep.UserID, deviceOwner} {
		if !o.Valid {
			continue
		}
		id, err := r.user(ctx, o.Int64)
		if err != nil {
			return t, err
		}
		if id != 0 {
			t.CreatedBy = pgtype.Int8{Int64: id, Valid: true}
			break
		}
	}
	return t, nil
}

func (r *measurementRun) importReport(ctx context.Context, rep *legacyReport, apiUsers map[int64]legacyAPIUser) error {
	var apiUser legacyAPIUser
	hasAPIUser := false
	if rep.APIUserID.Valid {
		apiUser, hasAPIUser = apiUsers[rep.APIUserID.Int64]
	}
	t, err := r.target(ctx, rep, apiUser, hasAPIUser)
	if err != nil {
		return err
	}
	vin, ok := MeasurementVIN(rep.VIN.String)
	if !ok {
		r.report("vin_invalid", rep.ID)
	}
	status, vinText := measurementVINPending, pgtype.Text{}
	if vin != "" {
		status, vinText = measurementAccepted, pgText(vin)
	} else {
		r.c.inc("vin_pending")
	}
	serial := pgText(truncate(strings.TrimSpace(rep.DeviceSerial.String), measurementSerialMax))
	raw, err := MeasurementRaw(r.step.system(), rep)
	if err != nil {
		return fmt.Errorf("encode raw: %w", err)
	}

	key := Key{System: r.step.system(), Table: "nexptg_reports", ID: strconv.FormatInt(rep.ID, 10),
		TargetTable: "measurement_results"}
	sum := Checksum(string(raw), t.OrgID, t.BrandID, t.ServiceID.Int64, t.Vehicle.Int64, t.CreatedBy.Int64)
	res, err := r.m.Upsert(ctx, key, sum)
	if err != nil {
		return err
	}
	id, err := r.q.MigratorMeasurementResultIDByUUID(ctx, res.UUID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		createdAt := rep.CreatedAt
		if !createdAt.Valid {
			createdAt = rep.Date
		}
		if _, err := r.q.MigratorInsertMeasurementResult(ctx, db.MigratorInsertMeasurementResultParams{
			Uuid: res.UUID, OrganizationID: t.OrgID, BrandID: t.BrandID, ServiceID: t.ServiceID,
			VehicleID: t.Vehicle, Vin: vinText, Status: status, Raw: raw, DeviceSerial: serial,
			CreatedBy: t.CreatedBy, CreatedAt: pgTime(createdAt),
		}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("insert measurement result: %w", err)
		}
		r.c.inc("reports_created")
	case err != nil:
		return fmt.Errorf("read measurement result: %w", err)
	case !res.Changed:
		r.c.inc("reports_unchanged")
		return nil
	default:
		if err := r.q.MigratorUpdateMeasurementResult(ctx, db.MigratorUpdateMeasurementResultParams{
			ID: id, OrganizationID: t.OrgID, BrandID: t.BrandID, ServiceID: t.ServiceID, VehicleID: t.Vehicle,
			Vin: vinText, Status: status, Raw: raw, DeviceSerial: serial, CreatedBy: t.CreatedBy,
		}); err != nil {
			return fmt.Errorf("update measurement result: %w", err)
		}
		r.c.inc("reports_updated")
	}

	// The measurement rows and the service link live in the result's raw:
	// they map to the same result.
	for _, ms := range rep.measurements {
		if _, err := r.m.Link(ctx, Key{System: key.System, Table: "nexptg_report_measurements",
			ID: strconv.FormatInt(ms.ID, 10), TargetTable: "measurement_results"}, res.UUID, ""); err != nil {
			return err
		}
	}
	if l := rep.link; l != nil {
		if _, err := r.m.Link(ctx, Key{System: key.System, Table: "service_nexptg_report",
			ID: strconv.FormatInt(l.ID, 10), TargetTable: "measurement_results"}, res.UUID, ""); err != nil {
			return err
		}
	}
	return nil
}

// service resolves a hub service to its row here (nil: not migrated).
func (r *measurementRun) service(ctx context.Context, legacyID int64) (*db.MigratorServiceForMeasurementRow, error) {
	target, ok, err := r.m.Lookup(ctx, r.step.system(), "services", strconv.FormatInt(legacyID, 10))
	if err != nil || !ok {
		return nil, err
	}
	row, err := r.q.MigratorServiceForMeasurement(ctx, target)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read service: %w", err)
	}
	return &row, nil
}

// serviceVehicle is the vehicle the services step mapped for a hub service
// (0: none).
func (r *measurementRun) serviceVehicle(ctx context.Context, legacyID int64) (int64, error) {
	target, ok, err := r.m.Lookup(ctx, r.step.system(), "services.vehicle", strconv.FormatInt(legacyID, 10))
	if err != nil || !ok {
		return 0, err
	}
	row, err := r.q.MigratorVehicleByUUID(ctx, target)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read vehicle: %w", err)
	}
	return row.ID, nil
}

// user resolves a hub user to users.id (0: not migrated).
func (r *measurementRun) user(ctx context.Context, userID int64) (int64, error) {
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
			return 0, fmt.Errorf("read user: %w", err)
		}
	}
	r.users[userID] = id
	return id, nil
}

// orgOfUsers is the organization of the first hub user (in order) whose
// dealer is migrated, else the Olex center.
func (r *measurementRun) orgOfUsers(ctx context.Context, users ...sql.NullInt64) (int64, error) {
	for _, u := range users {
		if !u.Valid {
			continue
		}
		dealer, ok := r.userDealer[u.Int64]
		if !ok {
			continue
		}
		orgID, ok := r.orgs[dealer]
		if !ok {
			target, found, err := r.m.Lookup(ctx, r.step.system(), "dealers", strconv.FormatInt(dealer, 10))
			if err != nil {
				return 0, err
			}
			if found {
				orgID, err = r.q.MigratorOrganizationIDByUUID(ctx, target)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return 0, fmt.Errorf("read organization: %w", err)
				}
			}
			r.orgs[dealer] = orgID
		}
		if orgID != 0 {
			return orgID, nil
		}
	}
	r.c.inc("organization_center")
	return r.centerID, nil
}
