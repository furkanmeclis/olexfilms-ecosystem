package db_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-293: database-level guards of the measurement schema extension
// (migration 000085). Reuses the service fixture (rolled-back transaction,
// savepoint per failure).

func (f *serviceFixture) measurement(t *testing.T, org db.Organization) db.MeasurementResult {
	t.Helper()
	f.seq++
	// legacy_import: no uploading user is required; none of the 000085
	// columns is set, as the migrator (TEC-262) inserts it.
	r, err := f.q.InsertMeasurementResult(f.ctx, db.InsertMeasurementResultParams{
		OrganizationID: org.ID, BrandID: org.BrandID, Status: "vin_pending",
		Raw:                 []byte(`{"raw":{}}`),
		ClientMeasurementID: text(fmt.Sprintf("t293-%d-%d", time.Now().UnixNano(), f.seq)),
		Source:              "legacy_import",
	})
	if err != nil {
		t.Fatalf("measurement: %v", err)
	}
	if r.ParsedAt.Valid || r.DeviceID.Valid || r.MeasuredAt.Valid {
		t.Fatalf("new measurement has 000085 fields set: %+v", r)
	}
	return r
}

func TestMeasurementSchemaConstraints(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.ctx
	dealer, dealer2 := f.dealer, f.dealer2

	link := func(org db.Organization, svc db.Service, res db.MeasurementResult, phase string) func(sp pgx.Tx) error {
		return func(sp pgx.Tx) error {
			_, err := db.New(sp).LinkServiceMeasurement(ctx, db.LinkServiceMeasurementParams{
				OrganizationID: org.ID, BrandID: org.BrandID, ServiceID: svc.ID,
				MeasurementResultID: res.ID, Phase: phase, LinkSource: "manual",
			})
			return err
		}
	}

	svc := f.service(t, dealer)
	if svc.MeasurementCheckRequired || svc.MeasurementCheckedAt.Valid {
		t.Fatalf("new service measurement check = %+v", svc)
	}
	before := f.measurement(t, dealer)
	after := f.measurement(t, dealer)
	if _, err := f.q.LinkServiceMeasurement(ctx, db.LinkServiceMeasurementParams{
		OrganizationID: dealer.ID, BrandID: dealer.BrandID, ServiceID: svc.ID,
		MeasurementResultID: before.ID, Phase: "before", LinkSource: "auto",
	}); err != nil {
		t.Fatalf("link before: %v", err)
	}
	sm, err := f.q.LinkServiceMeasurement(ctx, db.LinkServiceMeasurementParams{
		OrganizationID: dealer.ID, BrandID: dealer.BrandID, ServiceID: svc.ID,
		MeasurementResultID: after.ID, Phase: "after", LinkSource: "manual",
		ConfirmedBy: pgtype.Int8{Int64: f.customer.ID, Valid: true},
	})
	if err != nil {
		t.Fatalf("link after: %v", err)
	}
	if !sm.ConfirmedAt.Valid {
		t.Fatalf("confirmed link without confirmed_at: %+v", sm)
	}

	t.Run("two before measurements on a service", func(t *testing.T) {
		other := f.measurement(t, dealer)
		f.expectConstraint(t, "second before", "23505", "uq_service_measurements_service_phase",
			link(dealer, svc, other, "before"))
	})

	t.Run("one measurement on two services", func(t *testing.T) {
		svc2 := f.service(t, dealer)
		f.expectConstraint(t, "measurement reused", "23505", "uq_service_measurements_result",
			link(dealer, svc2, before, "before"))
	})

	t.Run("measurement of another organization", func(t *testing.T) {
		foreign := f.measurement(t, dealer2)
		svc3 := f.service(t, dealer)
		f.expectConstraint(t, "foreign measurement", "23503", "fk_service_measurements_result_org",
			link(dealer, svc3, foreign, "before"))
		// Claiming the measurement's organization does not help: the service
		// is checked against the link's organization by a trigger.
		f.expectTrigger(t, "foreign service", link(dealer2, svc3, foreign, "before"))
	})

	t.Run("unknown phase", func(t *testing.T) {
		svc4 := f.service(t, dealer)
		res := f.measurement(t, dealer)
		f.expectConstraint(t, "bad phase", "23514", "chk_service_measurements_phase",
			link(dealer, svc4, res, "during"))
	})

	t.Run("negative value_um", func(t *testing.T) {
		res := f.measurement(t, dealer)
		value := func(v string) func(sp pgx.Tx) error {
			return func(sp pgx.Tx) error {
				_, err := db.New(sp).InsertMeasurementValue(ctx, db.InsertMeasurementValueParams{
					OrganizationID: dealer.ID, BrandID: dealer.BrandID, ResultID: res.ID,
					PlaceID: "left", PartType: "LEFT_FRONT_DOOR", ValueUm: numeric(t, v),
					Interpretation: pgtype.Int2{Int16: 1, Valid: true},
				})
				return err
			}
		}
		f.expectConstraint(t, "negative value", "23514", "chk_measurement_values_value", value("-0.01"))
		if err := value("112.50")(f.tx); err != nil {
			t.Fatalf("valid value: %v", err)
		}
		// A reading belongs to its result's organization.
		f.expectConstraint(t, "reading of another org", "23503", "fk_measurement_values_result_org",
			func(sp pgx.Tx) error {
				_, err := db.New(sp).InsertMeasurementValue(ctx, db.InsertMeasurementValueParams{
					OrganizationID: dealer2.ID, BrandID: dealer2.BrandID, ResultID: res.ID,
					PlaceID: "left", PartType: "HOOD",
				})
				return err
			})
	})

	t.Run("device of another organization", func(t *testing.T) {
		dev, err := f.q.UpsertMeasurementDevice(ctx, db.UpsertMeasurementDeviceParams{
			OrganizationID: dealer2.ID, BrandID: dealer2.BrandID,
			Serial: fmt.Sprintf("T293-%d", time.Now().UnixNano()), IsActive: true,
		})
		if err != nil {
			t.Fatalf("device: %v", err)
		}
		if !dev.IsActive {
			t.Fatalf("device not active: %+v", dev)
		}
		res := f.measurement(t, dealer)
		f.expectConstraint(t, "foreign device", "23503", "fk_measurement_results_device_org",
			func(sp pgx.Tx) error {
				return db.New(sp).MarkMeasurementResultParsed(ctx, db.MarkMeasurementResultParsedParams{
					ID: res.ID, OrganizationID: dealer.ID,
					DeviceID: pgtype.Int8{Int64: dev.ID, Valid: true},
				})
			})
	})

	t.Run("negative tread depth", func(t *testing.T) {
		res := f.measurement(t, dealer)
		f.expectConstraint(t, "negative depth", "23514", "chk_measurement_tires_depth",
			func(sp pgx.Tx) error {
				_, err := db.New(sp).InsertMeasurementTire(ctx, db.InsertMeasurementTireParams{
					OrganizationID: dealer.ID, BrandID: dealer.BrandID, ResultID: res.ID,
					TreadDepth1Mm: numeric(t, "-1"),
				})
				return err
			})
	})
}
