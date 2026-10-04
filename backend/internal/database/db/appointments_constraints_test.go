package db_test

import (
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-322: database-level guards of the appointment schema (migration
// 000090). Reuses the service fixture (rolled-back transaction, savepoint
// per failure).

func (f *serviceFixture) appointment(t *testing.T, org db.Organization, leadID, serviceID *int64) db.Appointment {
	t.Helper()
	start := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	arg := db.CreateAppointmentParams{
		OrganizationID: org.ID, BrandID: org.BrandID, CustomerUserID: f.customer.ID,
		VehicleID: int8p(f.vehicle.ID), StartsAt: timestamptz(start),
		EndsAt: timestamptz(start.Add(2 * time.Hour)), EstimatedMinutes: 120,
		Source: "panel", Status: "scheduled", Note: "",
	}
	if leadID != nil {
		arg.LeadID = int8p(*leadID)
	}
	if serviceID != nil {
		arg.ServiceID = int8p(*serviceID)
	}
	row, err := f.q.CreateAppointment(f.ctx, arg)
	if err != nil {
		t.Fatalf("appointment: %v", err)
	}
	return row
}

func int8p(id int64) pgtype.Int8 {
	return pgtype.Int8{Int64: id, Valid: true}
}

func timestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func TestAppointmentsSchemaConstraints(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.ctx
	dealer := f.dealer

	lead, err := db.New(f.tx).CreateLead(ctx, db.CreateLeadParams{
		OrganizationID: dealer.ID, BrandID: dealer.BrandID,
		TargetType: "customer", Source: "walk_in", Temperature: "warm", Status: "new", Notes: "",
	})
	if err != nil {
		t.Fatalf("lead: %v", err)
	}
	leadID := lead.ID
	serviceID := f.service(t, dealer).ID
	appt := f.appointment(t, dealer, &leadID, &serviceID)
	if appt.Status != "scheduled" || appt.OrganizationID != dealer.ID {
		t.Fatalf("appointment = %+v", appt)
	}

	t.Run("ends after starts", func(t *testing.T) {
		f.expectCode(t, "end before start", func(sp pgx.Tx) error {
			start := time.Date(2026, 10, 5, 11, 0, 0, 0, time.UTC)
			_, err := db.New(sp).CreateAppointment(ctx, db.CreateAppointmentParams{
				OrganizationID: dealer.ID, BrandID: dealer.BrandID, CustomerUserID: f.customer.ID,
				StartsAt: timestamptz(start), EndsAt: timestamptz(start.Add(-time.Hour)),
				EstimatedMinutes: 60, Source: "panel", Status: "scheduled", Note: "",
			})
			return err
		}, "23514")
	})

	t.Run("status enum", func(t *testing.T) {
		f.expectCode(t, "bad status", func(sp pgx.Tx) error {
			start := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
			_, err := db.New(sp).CreateAppointment(ctx, db.CreateAppointmentParams{
				OrganizationID: dealer.ID, BrandID: dealer.BrandID, CustomerUserID: f.customer.ID,
				StartsAt: timestamptz(start), EndsAt: timestamptz(start.Add(time.Hour)),
				EstimatedMinutes: 60, Source: "panel", Status: "bogus", Note: "",
			})
			return err
		}, "23514")
	})

	t.Run("lead and service scope", func(t *testing.T) {
		otherDealer := f.org(t, "appt-other", "dealer", f.dist.ID)
		otherLead, err := db.New(f.tx).CreateLead(ctx, db.CreateLeadParams{
			OrganizationID: otherDealer.ID, BrandID: otherDealer.BrandID,
			TargetType: "customer", Source: "walk_in", Temperature: "warm", Status: "new", Notes: "",
		})
		if err != nil {
			t.Fatalf("other lead: %v", err)
		}
		otherService := f.service(t, otherDealer)
		f.expectCode(t, "lead from another org", func(sp pgx.Tx) error {
			start := time.Date(2026, 10, 5, 13, 0, 0, 0, time.UTC)
			_, err := db.New(sp).CreateAppointment(ctx, db.CreateAppointmentParams{
				OrganizationID: dealer.ID, BrandID: dealer.BrandID, CustomerUserID: f.customer.ID,
				StartsAt: timestamptz(start), EndsAt: timestamptz(start.Add(time.Hour)),
				EstimatedMinutes: 60, Source: "lead", Status: "scheduled", Note: "", LeadID: int8p(otherLead.ID),
			})
			return err
		}, "23503")
		f.expectCode(t, "service from another org", func(sp pgx.Tx) error {
			start := time.Date(2026, 10, 5, 14, 0, 0, 0, time.UTC)
			_, err := db.New(sp).CreateAppointment(ctx, db.CreateAppointmentParams{
				OrganizationID: dealer.ID, BrandID: dealer.BrandID, CustomerUserID: f.customer.ID,
				StartsAt: timestamptz(start), EndsAt: timestamptz(start.Add(time.Hour)),
				EstimatedMinutes: 60, Source: "panel", Status: "scheduled", Note: "", ServiceID: int8p(otherService.ID),
			})
			return err
		}, "23503")
	})

	t.Run("settings are one row per organization", func(t *testing.T) {
		if _, err := f.tx.Exec(ctx, `INSERT INTO appointment_settings
			(organization_id, brand_id, daily_vehicle_capacity, default_estimated_minutes,
			 slot_interval_minutes, working_hours, portal_appointments_enabled)
			VALUES ($1, $2, 4, 120, 30, '{}'::jsonb, true)`,
			dealer.ID, dealer.BrandID); err != nil {
			t.Fatalf("settings: %v", err)
		}
		f.expectCode(t, "second settings row", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `INSERT INTO appointment_settings
				(organization_id, brand_id, daily_vehicle_capacity, default_estimated_minutes,
				 slot_interval_minutes, working_hours, portal_appointments_enabled)
				VALUES ($1, $2, 5, 90, 15, '{}'::jsonb, false)`,
				dealer.ID, dealer.BrandID)
			return err
		}, "23505")
	})
}
