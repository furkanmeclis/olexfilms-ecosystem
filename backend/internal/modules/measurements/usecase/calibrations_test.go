package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type memCalibrations struct {
	rows     []db.InsertGaugeCalibrationParams
	inserted int
}

func (m *memCalibrations) InsertGaugeCalibration(_ context.Context, arg db.InsertGaugeCalibrationParams) (int64, error) {
	for _, r := range m.rows {
		if arg.ClientID.Valid && r.OrganizationID == arg.OrganizationID && r.ClientID == arg.ClientID {
			return 0, pgx.ErrNoRows
		}
	}
	m.rows = append(m.rows, arg)
	m.inserted++
	return int64(len(m.rows)), nil
}

func (m *memCalibrations) FindGaugeCalibrationByClientID(_ context.Context, arg db.FindGaugeCalibrationByClientIDParams) (int64, error) {
	for i, r := range m.rows {
		if r.OrganizationID == arg.OrganizationID && r.ClientID == arg.ClientID {
			return int64(i + 1), nil
		}
	}
	return 0, pgx.ErrNoRows
}

func (m *memCalibrations) row(i int) db.GetGaugeCalibrationRow {
	r := m.rows[i]
	return db.GetGaugeCalibrationRow{
		Serial: r.Serial, FeOffsetHz: r.FeOffsetHz, AlOffsetHz: r.AlOffsetHz, TargetUm: r.TargetUm,
		Samples: r.Samples, CalibratedAt: r.CalibratedAt, CreatedByName: "Ada", CreatedBySurname: "Usta",
	}
}

func (m *memCalibrations) GetGaugeCalibration(_ context.Context, arg db.GetGaugeCalibrationParams) (db.GetGaugeCalibrationRow, error) {
	i := int(arg.ID) - 1
	if i < 0 || i >= len(m.rows) || m.rows[i].OrganizationID != arg.OrganizationID {
		return db.GetGaugeCalibrationRow{}, pgx.ErrNoRows
	}
	return m.row(i), nil
}

func (m *memCalibrations) GetLatestGaugeCalibration(_ context.Context, arg db.GetLatestGaugeCalibrationParams) (db.GetLatestGaugeCalibrationRow, error) {
	best := -1
	for i, r := range m.rows {
		if r.OrganizationID == arg.OrganizationID && r.Serial == arg.Serial &&
			(best < 0 || !r.CalibratedAt.Time.Before(m.rows[best].CalibratedAt.Time)) {
			best = i
		}
	}
	if best < 0 {
		return db.GetLatestGaugeCalibrationRow{}, pgx.ErrNoRows
	}
	return db.GetLatestGaugeCalibrationRow(m.row(best)), nil
}

func ip(n int) *int { return &n }

func TestCalibrationValidation(t *testing.T) {
	c := NewCalibrations(&memCalibrations{})
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	future := now.Add(time.Hour)
	ok := CalibrationInput{Serial: 32475, FeOffsetHz: ip(-40), TargetUm: 90}
	cases := map[string]struct {
		mut   func(*CalibrationInput)
		field string
	}{
		"serial zero":   {func(in *CalibrationInput) { in.Serial = 0 }, "serial"},
		"serial long":   {func(in *CalibrationInput) { in.Serial = 1_000_000_000 }, "serial"},
		"no offsets":    {func(in *CalibrationInput) { in.FeOffsetHz = nil }, "fe_offset_hz"},
		"fe range":      {func(in *CalibrationInput) { in.FeOffsetHz = ip(20001) }, "fe_offset_hz"},
		"al range":      {func(in *CalibrationInput) { in.AlOffsetHz = ip(-20001) }, "al_offset_hz"},
		"target":        {func(in *CalibrationInput) { in.TargetUm = 100 }, "target_um"},
		"samples range": {func(in *CalibrationInput) { in.Samples = &CalibrationSamples{Fe: []int{0}} }, "samples"},
		"samples many":  {func(in *CalibrationInput) { in.Samples = &CalibrationSamples{Al: make([]int, 65)} }, "samples"},
		"future":        {func(in *CalibrationInput) { in.CalibratedAt = &future }, "calibrated_at"},
	}
	for name, tc := range cases {
		in := ok
		tc.mut(&in)
		_, _, err := c.Record(context.Background(), Caller{UserID: 1, OrganizationID: 1, BrandID: 1}, in)
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != tc.field {
			t.Fatalf("%s: err = %v, want field %s", name, err, tc.field)
		}
	}
}

func TestCalibrationRecordAndLatest(t *testing.T) {
	m := &memCalibrations{}
	c := NewCalibrations(m)
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	ctx := context.Background()
	caller := Caller{UserID: 3, OrganizationID: 10, BrandID: 2}

	if _, err := c.Latest(ctx, 10, 32475); !errors.Is(err, ErrCalibrationNotFound) {
		t.Fatalf("empty latest = %v", err)
	}

	client := uuid.New()
	earlier := now.Add(-time.Hour)
	v, replayed, err := c.Record(ctx, caller, CalibrationInput{
		Serial: 32475, FeOffsetHz: ip(-35), TargetUm: 90, ClientID: &client, CalibratedAt: &earlier,
		Samples: &CalibrationSamples{Fe: []int{18275, 18276}},
	})
	if err != nil || replayed {
		t.Fatalf("record = %v replayed=%v", err, replayed)
	}
	if v.Serial != 32475 || v.FeOffsetHz == nil || *v.FeOffsetHz != -35 || v.AlOffsetHz != nil || v.TargetUm != 90 {
		t.Fatalf("view = %+v", v)
	}
	if len(v.Samples.Fe) != 2 || v.Samples.Al == nil || !v.CalibratedAt.Equal(earlier) || v.CreatedBy.Name != "Ada Usta" {
		t.Fatalf("view = %+v", v)
	}
	if r := m.rows[0]; r.OrganizationID != 10 || r.BrandID != 2 || r.CreatedBy != 3 {
		t.Fatalf("row scope = %+v", r)
	}

	// The same client_id again: no new row, the first one answers.
	if _, replayed, err := c.Record(ctx, caller, CalibrationInput{Serial: 32475, FeOffsetHz: ip(1), TargetUm: 90, ClientID: &client}); err != nil || !replayed {
		t.Fatalf("replay = %v %v", err, replayed)
	}
	if m.inserted != 1 {
		t.Fatalf("inserted = %d", m.inserted)
	}

	// A later calibration becomes the current one; another organization sees none.
	if _, _, err := c.Record(ctx, caller, CalibrationInput{Serial: 32475, FeOffsetHz: ip(-20), AlOffsetHz: ip(15), TargetUm: 90}); err != nil {
		t.Fatal(err)
	}
	latest, err := c.Latest(ctx, 10, 32475)
	if err != nil || *latest.FeOffsetHz != -20 || *latest.AlOffsetHz != 15 || !latest.CalibratedAt.Equal(now) {
		t.Fatalf("latest = %+v %v", latest, err)
	}
	if _, err := c.Latest(ctx, 11, 32475); !errors.Is(err, ErrCalibrationNotFound) {
		t.Fatalf("other org = %v", err)
	}
}

func TestParseGaugeSerial(t *testing.T) {
	for in, want := range map[string]int{"32475": 32475, "0032475": 32475, " 7 ": 7, "999999999": 999999999} {
		if got, ok := ParseGaugeSerial(in); !ok || got != want {
			t.Fatalf("%q = %d %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "0", "000", "1234567890", "12a", "-1", "+1"} {
		if _, ok := ParseGaugeSerial(in); ok {
			t.Fatalf("%q accepted", in)
		}
	}
}
