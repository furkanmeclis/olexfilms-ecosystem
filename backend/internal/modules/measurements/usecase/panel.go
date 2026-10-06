package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	maxLabel = 128
	maxModel = 64
)

type DeviceInput struct {
	Serial   string
	Label    *string
	Model    *string
	IsActive *bool
}

type DeviceView struct {
	UUID      uuid.UUID `json:"uuid"`
	Serial    string    `json:"serial"`
	Type      string    `json:"type"`
	Label     *string   `json:"label"`
	Model     *string   `json:"model"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListSort is the sort contract of GET /v1/measurements (TEC-299,
// docs/list-contract.md): measured_at is COALESCE(measured_at, created_at);
// vin and plate are nullable and sort blanks last in both directions.
var ListSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"measured_at": "measured_at", "created_at": "created_at", "vin": "vin",
		"status": "status", "plate": "plate",
	},
	Default: apiquery.SortField{Field: "measured_at", Desc: true},
}

// Statuses are the measurement result statuses (list filter values).
var Statuses = []string{StatusAccepted, StatusVINPending}

// MeasurementFilter is the panel list filter. Statuses and DeviceUUIDs are
// multi-value (nil = no filter); Q searches the VIN, the vehicle plate and
// the device serial; a zero Sort is the ListSort default.
type MeasurementFilter struct {
	VIN          string
	DeviceUUIDs  []uuid.UUID
	Statuses     []string
	Q            string
	Linked       *bool
	MeasuredFrom *time.Time
	MeasuredTo   *time.Time
	Sort         apiquery.ResolvedSort
	Limit        int32
	Offset       int32
}

type OrganizationRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

type ServiceRef struct {
	UUID        uuid.UUID  `json:"uuid"`
	ServiceNo   string     `json:"service_no"`
	Phase       string     `json:"phase"`
	LinkSource  string     `json:"link_source"`
	ConfirmedAt *time.Time `json:"confirmed_at"`
}

type MeasurementSummary struct {
	UUID         uuid.UUID       `json:"uuid"`
	Organization OrganizationRef `json:"organization"`
	VIN          *string         `json:"vin"`
	Status       string          `json:"status"`
	Source       string          `json:"source"`
	DeviceSerial *string         `json:"device_serial"`
	Device       *DeviceView     `json:"device"`
	Service      *ServiceRef     `json:"service"`
	Plate        *string         `json:"plate"`
	MeasuredAt   *time.Time      `json:"measured_at"`
	CreatedAt    time.Time       `json:"created_at"`
}

type MeasurementValueView struct {
	PlaceID        string     `json:"place_id"`
	PartType       string     `json:"part_type"`
	IsInside       bool       `json:"is_inside"`
	Position       *int32     `json:"position"`
	ValueUM        *string    `json:"value_um"`
	Interpretation *int16     `json:"interpretation"`
	SubstrateType  *string    `json:"substrate_type"`
	MeasuredAt     *time.Time `json:"measured_at"`
}

type MeasurementTireView struct {
	Section       *string `json:"section"`
	Width         *string `json:"width"`
	Profile       *string `json:"profile"`
	Diameter      *string `json:"diameter"`
	Maker         *string `json:"maker"`
	Season        *string `json:"season"`
	TreadDepth1MM *string `json:"tread_depth_1_mm"`
	TreadDepth2MM *string `json:"tread_depth_2_mm"`
}

type MeasurementDetail struct {
	MeasurementSummary
	BodyType *string                `json:"body_type"`
	PDFKey   *string                `json:"pdf_key"`
	Raw      map[string]any         `json:"raw"`
	Values   []MeasurementValueView `json:"values"`
	Tires    []MeasurementTireView  `json:"tires"`
}

func (s *Service) ListDevices(ctx context.Context, c PanelCaller) ([]DeviceView, error) {
	rows, err := s.store.ListMeasurementDevices(ctx, c.Org.InternalID)
	if err != nil {
		return nil, err
	}
	out := make([]DeviceView, 0, len(rows))
	for _, row := range rows {
		out = append(out, deviceView(row))
	}
	return out, nil
}

func (s *Service) CreateDevice(ctx context.Context, c PanelCaller, in DeviceInput) (DeviceView, error) {
	serial := strings.TrimSpace(in.Serial)
	if err := validateDevice(serial, in.Label, in.Model); err != nil {
		return DeviceView{}, err
	}
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	row, err := s.store.CreateMeasurementDevice(ctx, db.CreateMeasurementDeviceParams{
		OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, Serial: serial,
		Label: textPtr(in.Label), Model: textPtr(in.Model), IsActive: active,
	})
	if err != nil {
		return DeviceView{}, mapPanelDBError(err)
	}
	return deviceView(row), nil
}

func (s *Service) UpdateDevice(ctx context.Context, c PanelCaller, id uuid.UUID, in DeviceInput) (DeviceView, error) {
	cur, err := s.store.GetMeasurementDeviceByUUID(ctx, db.GetMeasurementDeviceByUUIDParams{
		Uuid: id, OrganizationID: c.Org.InternalID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return DeviceView{}, ErrNotFound
	}
	if err != nil {
		return DeviceView{}, err
	}
	label := textOut(cur.Label)
	model := textOut(cur.Model)
	active := cur.IsActive
	if in.Label != nil {
		label = in.Label
	}
	if in.Model != nil {
		model = in.Model
	}
	if in.IsActive != nil {
		active = *in.IsActive
	}
	if err := validateDevice(cur.Serial, label, model); err != nil {
		return DeviceView{}, err
	}
	row, err := s.store.UpdateMeasurementDevice(ctx, db.UpdateMeasurementDeviceParams{
		Uuid: id, OrganizationID: c.Org.InternalID, Label: textPtr(label), Model: textPtr(model), IsActive: active,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return DeviceView{}, ErrNotFound
	}
	if err != nil {
		return DeviceView{}, mapPanelDBError(err)
	}
	return deviceView(row), nil
}

func (s *Service) ListMeasurements(ctx context.Context, c PanelCaller, f MeasurementFilter) ([]MeasurementSummary, int64, error) {
	if err := validateMeasurementFilter(f); err != nil {
		return nil, 0, err
	}
	sort := f.Sort
	if sort.Key == "" {
		sort = apiquery.ResolvedSort{Key: ListSort.Columns[ListSort.Default.Field], Desc: ListSort.Default.Desc}
	}
	var q pgtype.Text
	if term := strings.TrimSpace(f.Q); term != "" {
		q = pgtype.Text{String: escapeLike(term), Valid: true}
	}
	rows, err := s.store.ListMeasurementResultsPanel(ctx, db.ListMeasurementResultsPanelParams{
		BrandID: c.Org.BrandID, OrgIds: c.Filter.OrgIDsArg(), Vin: text(strings.ToUpper(strings.TrimSpace(f.VIN))),
		DeviceUuids: f.DeviceUUIDs, Statuses: f.Statuses, Linked: boolPtr(f.Linked),
		MeasuredFrom: timePtr(f.MeasuredFrom), MeasuredTo: timePtr(f.MeasuredTo), Q: q,
		SortKey: sort.Key, SortDesc: sort.Desc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]MeasurementSummary, 0, len(rows))
	var total int64
	for _, row := range rows {
		total = row.TotalCount
		out = append(out, summaryFromList(row))
	}
	return out, total, nil
}

func (s *Service) GetMeasurement(ctx context.Context, c PanelCaller, id uuid.UUID) (MeasurementDetail, error) {
	row, err := s.store.GetMeasurementResultPanel(ctx, db.GetMeasurementResultPanelParams{
		Uuid: id, BrandID: c.Org.BrandID, OrgIds: c.Filter.OrgIDsArg(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return MeasurementDetail{}, ErrNotFound
	}
	if err != nil {
		return MeasurementDetail{}, err
	}
	values, err := s.store.ListMeasurementValues(ctx, db.ListMeasurementValuesParams{
		ResultID: row.ID, OrganizationID: row.OrganizationID,
	})
	if err != nil {
		return MeasurementDetail{}, err
	}
	tires, err := s.store.ListMeasurementTires(ctx, db.ListMeasurementTiresParams{
		ResultID: row.ID, OrganizationID: row.OrganizationID,
	})
	if err != nil {
		return MeasurementDetail{}, err
	}
	var raw map[string]any
	_ = json.Unmarshal(row.Raw, &raw)
	return MeasurementDetail{
		MeasurementSummary: summaryFromDetail(row),
		BodyType:           textOut(row.BodyType),
		PDFKey:             textOut(row.PdfKey),
		Raw:                raw,
		Values:             valueViews(values),
		Tires:              tireViews(tires),
	}, nil
}

func validateDevice(serial string, label, model *string) error {
	if serial == "" {
		return invalid("serial", "is required")
	}
	if utf8.RuneCountInString(serial) > maxSerial {
		return invalid("serial", "must be at most 64 characters")
	}
	if label != nil && utf8.RuneCountInString(strings.TrimSpace(*label)) > maxLabel {
		return invalid("label", "must be at most 128 characters")
	}
	if model != nil && utf8.RuneCountInString(strings.TrimSpace(*model)) > maxModel {
		return invalid("model", "must be at most 64 characters")
	}
	return nil
}

func validateMeasurementFilter(f MeasurementFilter) error {
	vin := strings.ToUpper(strings.TrimSpace(f.VIN))
	if vin != "" && !vinRe.MatchString(vin) {
		return invalid("vin", "must be 11 to 17 letters or digits")
	}
	for _, st := range f.Statuses {
		if st != StatusAccepted && st != StatusVINPending {
			return invalid("status", "must be accepted or vin_pending")
		}
	}
	if f.Sort.Key != "" {
		known := false
		for _, k := range ListSort.Columns {
			known = known || k == f.Sort.Key
		}
		if !known {
			return invalid("sort", "is not a sortable field")
		}
	}
	if f.MeasuredFrom != nil && f.MeasuredTo != nil && f.MeasuredTo.Before(*f.MeasuredFrom) {
		return invalid("measured_to", "must not be before measured_from")
	}
	return nil
}

// escapeLike escapes the LIKE wildcards of a search term.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func boolPtr(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

func mapPanelDBError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_measurement_devices_org_serial" {
		return ErrSerialExists
	}
	return err
}

func deviceView(row db.MeasurementDevice) DeviceView {
	return DeviceView{
		UUID: row.Uuid, Serial: row.Serial, Type: "NexPTG", Label: textOut(row.Label),
		Model: textOut(row.Model), IsActive: row.IsActive,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func rowDevice(id pgtype.UUID, serial, label, model pgtype.Text, active pgtype.Bool) *DeviceView {
	uid := uuidOut(id)
	if uid == nil {
		return nil
	}
	return &DeviceView{UUID: *uid, Serial: serial.String, Type: "NexPTG", Label: textOut(label), Model: textOut(model), IsActive: active.Bool}
}

func rowService(id pgtype.UUID, no, phase, source pgtype.Text, confirmed pgtype.Timestamptz) *ServiceRef {
	uid := uuidOut(id)
	if uid == nil {
		return nil
	}
	return &ServiceRef{UUID: *uid, ServiceNo: no.String, Phase: phase.String, LinkSource: source.String, ConfirmedAt: timeOut(confirmed)}
}

func orgRef(id pgtype.UUID, name pgtype.Text) OrganizationRef {
	uid := uuidOut(id)
	if uid == nil {
		return OrganizationRef{}
	}
	return OrganizationRef{UUID: *uid, Name: name.String}
}

func summaryFromList(row db.ListMeasurementResultsPanelRow) MeasurementSummary {
	return MeasurementSummary{
		UUID: row.Uuid, Organization: orgRef(row.OrganizationUuid, row.OrganizationName),
		VIN: textOut(row.Vin), Status: row.Status, Source: row.Source,
		DeviceSerial: textOut(row.DeviceSerial),
		Device:       rowDevice(row.DeviceUuid, row.RegistryDeviceSerial, row.DeviceLabel, row.DeviceModel, row.DeviceIsActive),
		Service:      rowService(row.ServiceUuid, row.ServiceNo, row.ServicePhase, row.ServiceLinkSource, row.ServiceConfirmedAt),
		Plate:        textOut(row.VehiclePlate),
		MeasuredAt:   timeOut(row.MeasuredAt), CreatedAt: row.CreatedAt.Time,
	}
}

func summaryFromDetail(row db.GetMeasurementResultPanelRow) MeasurementSummary {
	return MeasurementSummary{
		UUID: row.Uuid, Organization: orgRef(row.OrganizationUuid, row.OrganizationName),
		VIN: textOut(row.Vin), Status: row.Status, Source: row.Source,
		DeviceSerial: textOut(row.DeviceSerial),
		Device:       rowDevice(row.DeviceUuid, row.RegistryDeviceSerial, row.DeviceLabel, row.DeviceModel, row.DeviceIsActive),
		Service:      rowService(row.ServiceUuid, row.ServiceNo, row.ServicePhase, row.ServiceLinkSource, row.ServiceConfirmedAt),
		Plate:        textOut(row.VehiclePlate),
		MeasuredAt:   timeOut(row.MeasuredAt), CreatedAt: row.CreatedAt.Time,
	}
}

func valueViews(rows []db.MeasurementValue) []MeasurementValueView {
	out := make([]MeasurementValueView, 0, len(rows))
	for _, r := range rows {
		var pos *int32
		if r.Position.Valid {
			pos = &r.Position.Int32
		}
		var interp *int16
		if r.Interpretation.Valid {
			interp = &r.Interpretation.Int16
		}
		out = append(out, MeasurementValueView{
			PlaceID: r.PlaceID, PartType: r.PartType, IsInside: r.IsInside, Position: pos,
			ValueUM: numericOut(r.ValueUm), Interpretation: interp, SubstrateType: textOut(r.SubstrateType),
			MeasuredAt: timeOut(r.MeasuredAt),
		})
	}
	return out
}

func tireViews(rows []db.MeasurementTire) []MeasurementTireView {
	out := make([]MeasurementTireView, 0, len(rows))
	for _, r := range rows {
		out = append(out, MeasurementTireView{
			Section: textOut(r.Section), Width: textOut(r.Width), Profile: textOut(r.Profile),
			Diameter: textOut(r.Diameter), Maker: textOut(r.Maker), Season: textOut(r.Season),
			TreadDepth1MM: numericOut(r.TreadDepth1Mm), TreadDepth2MM: numericOut(r.TreadDepth2Mm),
		})
	}
	return out
}
