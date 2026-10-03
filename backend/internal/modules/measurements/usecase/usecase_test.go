package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// fakeStore keeps rows in memory and enforces the two partial unique
// indexes of 000076 like ON CONFLICT DO NOTHING does.
type fakeStore struct {
	rows     []db.MeasurementResult
	services map[uuid.UUID]db.GetServiceForMeasurementRow
	svcOrg   map[uuid.UUID]int64
	// skipFind makes the pre-insert lookup miss, as a concurrent upload
	// that inserts between the lookup and the insert would.
	skipFind int
}

func (f *fakeStore) InsertMeasurementResult(_ context.Context, a db.InsertMeasurementResultParams) (db.MeasurementResult, error) {
	for _, r := range f.rows {
		if r.OrganizationID != a.OrganizationID {
			continue
		}
		if (a.IdempotencyKey.Valid && r.IdempotencyKey == a.IdempotencyKey) ||
			(a.ClientMeasurementID.Valid && r.ClientMeasurementID == a.ClientMeasurementID) {
			return db.MeasurementResult{}, pgx.ErrNoRows
		}
	}
	row := db.MeasurementResult{
		ID: int64(len(f.rows) + 1), Uuid: uuid.New(), OrganizationID: a.OrganizationID, BrandID: a.BrandID,
		ServiceID: a.ServiceID, VehicleID: a.VehicleID, Vin: a.Vin, Status: a.Status, Raw: a.Raw,
		ClientMeasurementID: a.ClientMeasurementID, IdempotencyKey: a.IdempotencyKey,
		DeviceSerial: a.DeviceSerial, Source: a.Source, CreatedBy: a.CreatedBy,
	}
	f.rows = append(f.rows, row)
	return row, nil
}

func (f *fakeStore) FindMeasurementResultByKeys(_ context.Context, a db.FindMeasurementResultByKeysParams) (db.MeasurementResult, error) {
	if f.skipFind > 0 {
		f.skipFind--
		return db.MeasurementResult{}, pgx.ErrNoRows
	}
	for _, r := range f.rows {
		if r.OrganizationID != a.OrganizationID {
			continue
		}
		if (a.IdempotencyKey.Valid && r.IdempotencyKey == a.IdempotencyKey) ||
			(a.ClientMeasurementID.Valid && r.ClientMeasurementID == a.ClientMeasurementID) {
			return r, nil
		}
	}
	return db.MeasurementResult{}, pgx.ErrNoRows
}

func (f *fakeStore) GetServiceForMeasurement(_ context.Context, a db.GetServiceForMeasurementParams) (db.GetServiceForMeasurementRow, error) {
	s, ok := f.services[a.Uuid]
	if !ok || f.svcOrg[a.Uuid] != a.OrganizationID {
		return db.GetServiceForMeasurementRow{}, pgx.ErrNoRows
	}
	return s, nil
}

var caller = Caller{UserID: 3, OrganizationID: 10, BrandID: 1}

func body(t *testing.T, m map[string]any) []byte {
	t.Helper()
	if _, ok := m["raw"]; !ok {
		m["raw"] = map[string]any{"report": "nexptg"}
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCreateStoresAcceptedRow(t *testing.T) {
	st := &fakeStore{}
	s := New(st)
	in := Input{Body: body(t, map[string]any{
		"vin": " wvwzzz1jz3w386752 ", "device": map[string]any{"serial": "NX-1"}, "parts": []any{},
	})}
	res, err := s.Create(context.Background(), caller, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusAccepted || res.Replayed || len(st.rows) != 1 {
		t.Fatalf("result = %+v rows = %d", res, len(st.rows))
	}
	r := st.rows[0]
	if r.Vin.String != "WVWZZZ1JZ3W386752" || r.DeviceSerial.String != "NX-1" || r.Source != SourceMobile ||
		r.CreatedBy.Int64 != caller.UserID || r.OrganizationID != caller.OrganizationID || r.BrandID != caller.BrandID {
		t.Fatalf("row = %+v", r)
	}
	var stored map[string]any
	if err := json.Unmarshal(r.Raw, &stored); err != nil || stored["raw"] == nil || stored["parts"] == nil {
		t.Fatalf("raw keeps the body as received: %s", r.Raw)
	}
}

func TestCreateWithoutVINIsPending(t *testing.T) {
	st := &fakeStore{}
	res, err := New(st).Create(context.Background(), caller, Input{Body: body(t, map[string]any{"vin": "  "})})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusVINPending || st.rows[0].Vin.Valid {
		t.Fatalf("result = %+v row = %+v", res, st.rows[0])
	}
	if _, err := New(st).Create(context.Background(), caller, Input{Body: body(t, map[string]any{})}); err != nil {
		t.Fatal(err)
	}
	if st.rows[1].Status != StatusVINPending {
		t.Fatalf("absent vin = %s", st.rows[1].Status)
	}
}

func TestCreateIsIdempotent(t *testing.T) {
	st := &fakeStore{}
	s := New(st)
	ctx := context.Background()
	first, err := s.Create(ctx, caller, Input{IdempotencyKey: "k-1", Body: body(t, map[string]any{"vin": "WVWZZZ1JZ3W386752"})})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Create(ctx, caller, Input{IdempotencyKey: "k-1", Body: body(t, map[string]any{})})
	if err != nil {
		t.Fatal(err)
	}
	if second.UUID != first.UUID || second.Status != StatusAccepted || !second.Replayed || len(st.rows) != 1 {
		t.Fatalf("first %+v second %+v rows %d", first, second, len(st.rows))
	}

	// client_measurement_id in the body works the same way.
	a, _ := s.Create(ctx, caller, Input{Body: body(t, map[string]any{"client_measurement_id": "c-9"})})
	b, _ := s.Create(ctx, caller, Input{Body: body(t, map[string]any{"client_measurement_id": "c-9"})})
	if a.UUID != b.UUID || len(st.rows) != 2 {
		t.Fatalf("client id replay: %v %v rows %d", a.UUID, b.UUID, len(st.rows))
	}

	// The same key in another organization is a new row.
	other := caller
	other.OrganizationID = 11
	c, _ := s.Create(ctx, other, Input{IdempotencyKey: "k-1", Body: body(t, map[string]any{})})
	if c.UUID == first.UUID || len(st.rows) != 3 {
		t.Fatal("idempotency keys are per organization")
	}

	// A concurrent upload that wins the insert is returned, not an error.
	st.skipFind = 1
	d, err := s.Create(ctx, caller, Input{IdempotencyKey: "k-1", Body: body(t, map[string]any{})})
	if err != nil || d.UUID != first.UUID || len(st.rows) != 3 {
		t.Fatalf("race replay = %+v %v", d, err)
	}
}

func TestCreateServiceOfAnotherOrgIsNotFound(t *testing.T) {
	own, foreign := uuid.New(), uuid.New()
	st := &fakeStore{
		services: map[uuid.UUID]db.GetServiceForMeasurementRow{own: {ID: 5, VehicleID: 7}, foreign: {ID: 6, VehicleID: 8}},
		svcOrg:   map[uuid.UUID]int64{own: caller.OrganizationID, foreign: 99},
	}
	s := New(st)
	ctx := context.Background()
	if _, err := s.Create(ctx, caller, Input{Body: body(t, map[string]any{"service_uuid": foreign.String()})}); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("foreign service: %v", err)
	}
	if len(st.rows) != 0 {
		t.Fatal("nothing is written for a foreign service")
	}
	if _, err := s.Create(ctx, caller, Input{Body: body(t, map[string]any{"service_uuid": own.String()})}); err != nil {
		t.Fatal(err)
	}
	if st.rows[0].ServiceID.Int64 != 5 || st.rows[0].VehicleID.Int64 != 7 {
		t.Fatalf("row = %+v", st.rows[0])
	}
}

func TestCreateValidation(t *testing.T) {
	s := New(&fakeStore{})
	long := strings.Repeat("x", 129)
	cases := map[string]Input{
		"body":                  {Body: []byte(`[1]`)},
		"raw":                   {Body: []byte(`{"vin":"WVWZZZ1JZ3W386752"}`)},
		"vin":                   {Body: body(t, map[string]any{"vin": "SHORT"})},
		"service_uuid":          {Body: body(t, map[string]any{"service_uuid": "nope"})},
		"measured_at":           {Body: body(t, map[string]any{"measured_at": "yesterday"})},
		"client_measurement_id": {Body: body(t, map[string]any{"client_measurement_id": long})},
		"Idempotency-Key":       {IdempotencyKey: long, Body: body(t, map[string]any{})},
		"device.serial":         {Body: body(t, map[string]any{"device": map[string]any{"serial": long}})},
	}
	for field, in := range cases {
		_, err := s.Create(context.Background(), caller, in)
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != field {
			t.Fatalf("%s: got %v", field, err)
		}
	}
	// A raw that is not an object.
	if _, err := s.Create(context.Background(), caller, Input{Body: []byte(`{"raw":[1]}`)}); err == nil {
		t.Fatal("raw array accepted")
	}
}
