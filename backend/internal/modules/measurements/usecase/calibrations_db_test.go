package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TEC-939: the ledger against PostgreSQL: organization scope, client_id
// idempotency, latest by calibrated_at, append-only and the brand check.
func TestDBGaugeCalibrations(t *testing.T) {
	f := newDBFixture(t)
	other := f.newOrg("b")
	c := NewCalibrations(f.q)
	ctx := f.ctx
	caller := f.caller(f.org)

	t0 := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Microsecond)
	t1 := t0.Add(time.Hour)
	client := uuid.New()
	first, replayed, err := c.Record(ctx, caller, CalibrationInput{
		Serial: 32475, FeOffsetHz: ip(-35), AlOffsetHz: ip(10), TargetUm: 90, ClientID: &client, CalibratedAt: &t0,
		Samples: &CalibrationSamples{Fe: []int{18275, 18276}, Al: []int{26180}},
	})
	if err != nil || replayed {
		t.Fatalf("record = %v %v", err, replayed)
	}
	if first.CreatedBy.UUID != f.user.Uuid || first.Samples.Al[0] != 26180 || !first.CalibratedAt.Equal(t0) {
		t.Fatalf("first = %+v", first)
	}
	again, replayed, err := c.Record(ctx, caller, CalibrationInput{Serial: 32475, FeOffsetHz: ip(5), TargetUm: 150, ClientID: &client})
	if err != nil || !replayed || again.UUID != first.UUID {
		t.Fatalf("replay = %+v %v %v", again, replayed, err)
	}
	// The same client_id in another organization is another calibration.
	if _, replayed, err := c.Record(ctx, f.caller(other), CalibrationInput{Serial: 32475, FeOffsetHz: ip(7), TargetUm: 150, ClientID: &client}); err != nil || replayed {
		t.Fatalf("other org = %v %v", err, replayed)
	}

	// Recorded later but calibrated earlier does not become the current one.
	early := t0.Add(-time.Hour)
	if _, _, err := c.Record(ctx, caller, CalibrationInput{Serial: 32475, FeOffsetHz: ip(-99), TargetUm: 90, CalibratedAt: &early}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Record(ctx, caller, CalibrationInput{Serial: 32475, FeOffsetHz: ip(-20), TargetUm: 90, CalibratedAt: &t1}); err != nil {
		t.Fatal(err)
	}
	latest, err := c.Latest(ctx, f.org.ID, 32475)
	if err != nil || *latest.FeOffsetHz != -20 || latest.AlOffsetHz != nil {
		t.Fatalf("latest = %+v %v", latest, err)
	}
	if l, err := c.Latest(ctx, other.ID, 32475); err != nil || *l.FeOffsetHz != 7 || l.TargetUm != 150 {
		t.Fatalf("other latest = %+v %v", l, err)
	}
	if _, err := c.Latest(ctx, f.org.ID, 1355); !errors.Is(err, ErrCalibrationNotFound) {
		t.Fatalf("unknown serial = %v", err)
	}

	// Append-only: updates and deletes are refused (savepoints keep the tx usable).
	for _, stmt := range []string{
		`UPDATE gauge_calibrations SET fe_offset_hz = 1 WHERE organization_id = $1`,
		`DELETE FROM gauge_calibrations WHERE organization_id = $1`,
	} {
		if _, err := f.tx.Exec(ctx, "SAVEPOINT sp"); err != nil {
			t.Fatal(err)
		}
		if _, err := f.tx.Exec(ctx, stmt, f.org.ID); err == nil {
			t.Fatalf("%s accepted", stmt)
		}
		if _, err := f.tx.Exec(ctx, "ROLLBACK TO SAVEPOINT sp"); err != nil {
			t.Fatal(err)
		}
	}
	// The brand must be the organization's.
	if _, err := f.tx.Exec(ctx, "SAVEPOINT sp"); err != nil {
		t.Fatal(err)
	}
	_, err = f.tx.Exec(ctx, `INSERT INTO gauge_calibrations (organization_id, brand_id, serial, fe_offset_hz, target_um, calibrated_at, created_by)
		VALUES ($1, $2, 1, 1, 90, now(), $3)`, f.org.ID, f.org.BrandID+100000, f.user.ID)
	if err == nil {
		t.Fatal("foreign brand accepted")
	}
	if _, err := f.tx.Exec(context.Background(), "ROLLBACK TO SAVEPOINT sp"); err != nil {
		t.Fatal(err)
	}
}
