package usecase

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

// customer is a customer user with a unique +90 5xx phone.
func (a *appFixture) customer(withPhone bool) db.User {
	a.t.Helper()
	nano := time.Now().UnixNano()
	arg := db.CreateUserParams{
		Email: pgtype.Text{String: fmt.Sprintf("tec639-%d@example.test", nano), Valid: true}, PasswordHash: "x",
		Name: "Ali", Surname: "Yılmaz", Status: "active",
	}
	if withPhone {
		arg.PhoneE164 = pgtype.Text{String: fmt.Sprintf("+905%09d", (nano/1000)%1_000_000_000), Valid: true}
	}
	u, err := a.q.CreateUser(a.ctx, arg)
	if err != nil {
		a.t.Fatalf("customer: %v", err)
	}
	return u
}

// service gives the customer a draft service at org.
func (a *appFixture) service(u db.User, org db.Organization) {
	a.t.Helper()
	var carBrand, carModel int64
	if err := a.tx.QueryRow(a.ctx, `INSERT INTO car_brands (name) VALUES ($1) RETURNING id`,
		fmt.Sprintf("T639 %d", time.Now().UnixNano())).Scan(&carBrand); err != nil {
		a.t.Fatal(err)
	}
	if err := a.tx.QueryRow(a.ctx, `INSERT INTO car_models (car_brand_id, name) VALUES ($1, 'T639 Model') RETURNING id`,
		carBrand).Scan(&carModel); err != nil {
		a.t.Fatal(err)
	}
	v, err := a.q.CreateVehicle(a.ctx, db.CreateVehicleParams{
		UserID: u.ID, BrandID: org.BrandID,
		CarBrandID: pgtype.Int8{Int64: carBrand, Valid: true}, CarModelID: pgtype.Int8{Int64: carModel, Valid: true},
	})
	if err != nil {
		a.t.Fatalf("vehicle: %v", err)
	}
	if _, err := a.q.CreateService(a.ctx, db.CreateServiceParams{
		ServiceNo: fmt.Sprintf("T639-%d", time.Now().UnixNano()), OrganizationID: org.ID, BrandID: org.BrandID,
		CustomerUserID: u.ID, VehicleID: v.ID, CarBrandID: carBrand, CarModelID: carModel, Status: "draft",
	}); err != nil {
		a.t.Fatalf("service: %v", err)
	}
}

// TEC-639: a customer's appointment request is a whatsapp customer lead
// linked to the user, at the dealer of their newest service; a second
// request within 30 days updates it.
func TestCustomerAppointmentLeadGoesToServiceDealer(t *testing.T) {
	a := newAppFixture(t)
	u := a.customer(true)
	a.service(u, a.dist)
	in := CustomerAppointmentInput{
		CustomerUserID: u.ID, PreferredTime: "15 Ekim 13:00", Vehicle: "34 ABC 123", Message: "PPF",
		Language: "tr", ConversationUUID: "c0ffee",
	}
	first, err := a.apps.UpsertCustomerAppointmentLead(a.ctx, a.brand, in)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	l := first.Lead
	if !first.Created || first.RoutedBy != RoutedByCustomerDealer || l.OrganizationID != a.dist.ID ||
		l.CustomerUserID.Int64 != u.ID || l.Source != SourceWhatsApp || l.TargetType != TargetCustomer ||
		l.CandidatePhoneE164.String != u.PhoneE164.String || l.CandidateContactName.String != "Ali Yılmaz" ||
		!strings.Contains(l.Notes, "15 Ekim 13:00") || !strings.Contains(l.Notes, "34 ABC 123") {
		t.Fatalf("first = %+v", first)
	}

	in.PreferredTime = "16 Ekim 10:00"
	second, err := a.apps.UpsertCustomerAppointmentLead(a.ctx, a.brand, in)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Created || second.Lead.ID != l.ID || !strings.Contains(second.Lead.Notes, "16 Ekim 10:00") {
		t.Fatalf("second = %+v, want an update of lead %d", second, l.ID)
	}
	var events int
	if err := a.tx.QueryRow(a.ctx, `SELECT COUNT(*) FROM lead_events WHERE lead_id = $1 AND event_type = 'message'
		AND payload->>'intent' = 'appointment' AND payload->>'customer' = 'true'`, l.ID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 2 {
		t.Fatalf("timeline events = %d, want 2", events)
	}
}

func TestCustomerAppointmentLeadWithoutServiceGoesToCenter(t *testing.T) {
	a := newAppFixture(t)
	u := a.customer(true)
	res, err := a.apps.UpsertCustomerAppointmentLead(a.ctx, a.brand, CustomerAppointmentInput{CustomerUserID: u.ID})
	if err != nil {
		t.Fatal(err)
	}
	if res.RoutedBy != RoutedByCenter || res.Lead.OrganizationID != a.center.ID {
		t.Fatalf("res = %+v, want the brand center", res)
	}
}

func TestCustomerAppointmentLeadNeedsPhone(t *testing.T) {
	a := newAppFixture(t)
	u := a.customer(false)
	_, err := a.apps.UpsertCustomerAppointmentLead(a.ctx, a.brand, CustomerAppointmentInput{CustomerUserID: u.ID})
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Field != "phone" {
		t.Fatalf("err = %v, want a phone validation error", err)
	}
}
