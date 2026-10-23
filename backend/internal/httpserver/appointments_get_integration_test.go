//go:build integration

package httpserver

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	appointmentsuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/appointments/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-961: GET /v1/appointments/{uuid} reaches exactly what the list reaches
// (appointments.read scope inside the active brand); everything else is 404,
// and a caller without appointments.read is 403. The mobile panel mirror
// serves the same route.
func TestIntegrationAppointmentGet(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dealerA := it.org("t961-a", "dealer", center)
	dealerB := it.org("t961-b", "dealer", center)
	glorian := it.org("t961-g", "dealer", it.brandCenter("glorian"))

	owner, ownerPW := it.user("t961-owner")
	it.member(dealerA, owner, "owner")
	acc, accPW := it.user("t961-acc")
	it.member(dealerA, acc, "staff", rbac.RoleDealerAccounting)
	staff, staffPW := it.user("t961-center")
	it.member(center, staff, "staff")
	customer, _ := it.user("t961-customer")

	starts := time.Now().Add(48 * time.Hour).Truncate(time.Minute)
	mk := func(org db.Organization) db.Appointment {
		a, err := it.q.CreateAppointment(ctx, db.CreateAppointmentParams{
			OrganizationID: org.ID, BrandID: org.BrandID, CustomerUserID: customer.ID,
			StartsAt:         pgtype.Timestamptz{Time: starts, Valid: true},
			EndsAt:           pgtype.Timestamptz{Time: starts.Add(time.Hour), Valid: true},
			EstimatedMinutes: 60, Source: "panel", Status: "scheduled", Note: "t961",
		})
		if err != nil {
			t.Fatalf("create appointment: %v", err)
		}
		return a
	}
	inA, inB, inG := mk(dealerA), mk(dealerB), mk(glorian)
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM appointments WHERE organization_id = ANY($1)`,
			[]int64{dealerA.ID, dealerB.ID, glorian.ID})
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM refresh_tokens WHERE user_id = ANY($1)`,
			[]int64{owner.ID, acc.ID, staff.ID})
	})

	path := func(a db.Appointment) string { return "/v1/appointments/" + a.Uuid.String() }
	tok := it.loginOrg(owner, ownerPW, dealerA)
	got := decodeData[appointmentsuc.Appointment](t, it.custDo("GET", path(inA), tok, nil, http.StatusOK))
	if got.UUID != inA.Uuid || got.Note != "t961" || got.Status != "scheduled" {
		t.Fatalf("appointment = %+v", got)
	}
	// Managed scope: a sibling dealer of the same brand is out of reach,
	// another brand is never visible, an unknown or malformed id is 404.
	it.custDo("GET", path(inB), tok, nil, http.StatusNotFound)
	it.custDo("GET", path(inG), tok, nil, http.StatusNotFound)
	it.custDo("GET", "/v1/appointments/"+uuid.NewString(), tok, nil, http.StatusNotFound)
	it.custDo("GET", "/v1/appointments/not-a-uuid", tok, nil, http.StatusNotFound)

	// No appointments.read grant -> 403 before any lookup.
	accTok := it.loginOrg(acc, accPW, dealerA)
	it.custDo("GET", path(inA), accTok, nil, http.StatusForbidden)
	it.custDo("GET", "/v1/appointments?from=2026-01-01&to=2030-01-01", accTok, nil, http.StatusForbidden)

	// Center staff reads the whole brand but still not another brand.
	staffTok := it.loginOrg(staff, staffPW, center)
	it.custDo("GET", path(inB), staffTok, nil, http.StatusOK)
	it.custDo("GET", path(inG), staffTok, nil, http.StatusNotFound)

	// Mobile panel mirror serves the same route with the same gates.
	mobile := it.mobileLogin(owner.Email.String, ownerPW, dealerA.Slug, "t961-device")
	if code, env, _ := it.doMobile("GET", "/v1/mobile/panel"+path(inA)[3:], mobile.AccessToken, "1", nil); code != http.StatusOK {
		t.Fatalf("mobile mirror get = %d %s", code, errCode(env))
	}
	if code, env, _ := it.doMobile("GET", "/v1/mobile/panel"+path(inB)[3:], mobile.AccessToken, "1", nil); code != http.StatusNotFound {
		t.Fatalf("mobile mirror sibling = %d %s", code, errCode(env))
	}
}
