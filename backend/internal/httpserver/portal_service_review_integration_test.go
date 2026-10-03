package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

type portalServiceReviewState struct {
	Review *struct {
		UUID           string  `json:"uuid"`
		PlatformRating int     `json:"platform_rating"`
		ProductRating  int     `json:"product_rating"`
		Comment        *string `json:"comment"`
	} `json:"review"`
	CanReview         bool    `json:"can_review"`
	GoogleBusinessURL *string `json:"google_business_url"`
}

// TEC-244 acceptance: the owner customer reviews a completed service once
// (a second attempt is 409); an out-of-range rating is 400
// VALIDATION_ERROR; another customer gets 404 on GET and POST; a service
// that is not completed is 422; the review state carries the dealer's
// Google review link only when it is set.
func TestIntegrationPortalServiceReview(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()

	center := it.brandCenter("olex")
	dist := it.org("t244-dist", "distributor", center)
	dealer := it.org("t244-dealer", "dealer", dist)

	plate := "34T244" + it.suffix[len(it.suffix)-4:]
	cust, veh := it.svcCustomer(dealer, "t244-cust", plate)
	if err := it.q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{UserID: cust.ID, Slug: rbac.RoleCustomer}); err != nil {
		t.Fatalf("customer role: %v", err)
	}
	other, _ := it.user("t244-other", rbac.RoleCustomer)

	svc := it.directService(dealer, cust, veh, 244)
	draft, err := it.q.CreateService(ctx, db.CreateServiceParams{
		ServiceNo:      fmt.Sprintf("T244-%s-draft", it.suffix[len(it.suffix)-10:]),
		OrganizationID: dealer.ID, BrandID: dealer.BrandID, CustomerUserID: cust.ID, VehicleID: veh.ID,
		CarBrandID: veh.CarBrandID.Int64, CarModelID: veh.CarModelID.Int64, Status: "draft",
	})
	if err != nil {
		t.Fatalf("draft service: %v", err)
	}

	portalTok := func(u db.User) string {
		tok, _, err := it.tokens.IssueAccess(jwt.AccessInput{UserID: u.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	path := "/v1/portal/services/" + svc.Uuid.String() + "/review"
	good := map[string]any{"platform_rating": 5, "product_rating": 4, "comment": "  Çok memnun kaldım  "}

	// 1. Before the review: no review, can_review, no Google link.
	st := decodeData[portalServiceReviewState](t, it.accDo("GET", path, portalTok(cust), nil, http.StatusOK))
	if st.Review != nil || !st.CanReview || st.GoogleBusinessURL != nil {
		t.Fatalf("initial state = %+v", st)
	}

	// 2. Another customer: 404 on GET and POST (existence does not leak).
	it.accDo("GET", path, portalTok(other), nil, http.StatusNotFound)
	it.accDo("POST", path, portalTok(other), good, http.StatusNotFound)

	// 2b. A fleet session is read only (TEC-245): 403 PORTAL_READ_ONLY.
	fleet, fpw := it.user("t244-fleet", rbac.RoleFleet)
	fleetTok := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": fleet.Email.String, "password": fpw, "realm": "portal",
	})).AccessToken
	if code, env := it.do("POST", path, hostOlex, fleetTok, good); code != http.StatusForbidden || errCode(env) != "PORTAL_READ_ONLY" {
		t.Fatalf("fleet POST review = %d %s, want 403 PORTAL_READ_ONLY", code, errCode(env))
	}

	// 3. Out-of-range or missing rating: 400 VALIDATION_ERROR.
	for _, body := range []map[string]any{
		{"platform_rating": 0, "product_rating": 3},
		{"platform_rating": 3, "product_rating": 6},
		{"platform_rating": 3},
		{"platform_rating": 3, "product_rating": 3, "comment": strings.Repeat("a", 2001)},
	} {
		env := it.accDo("POST", path, portalTok(cust), body, http.StatusBadRequest)
		if errCode(env) != "VALIDATION_ERROR" {
			t.Fatalf("body %v: code = %s", body, errCode(env))
		}
	}

	// 4. A service that is not completed cannot be reviewed.
	it.accDo("POST", "/v1/portal/services/"+draft.Uuid.String()+"/review", portalTok(cust), good, http.StatusUnprocessableEntity)

	// 5. The owner reviews; the comment is trimmed.
	created := decodeData[portalServiceReviewState](t, it.accDo("POST", path, portalTok(cust), good, http.StatusCreated))
	if created.Review == nil || created.Review.PlatformRating != 5 || created.Review.ProductRating != 4 ||
		created.Review.Comment == nil || *created.Review.Comment != "Çok memnun kaldım" || created.CanReview {
		t.Fatalf("created = %+v", created)
	}
	var stored db.ServiceReview
	stored, err = it.q.GetServiceReviewByService(ctx, svc.ID)
	if err != nil || stored.CustomerUserID != cust.ID || stored.OrganizationID != dealer.ID || stored.BrandID != dealer.BrandID {
		t.Fatalf("stored = %+v (%v)", stored, err)
	}

	// 6. A second attempt: 409 SERVICE_ALREADY_REVIEWED.
	env := it.accDo("POST", path, portalTok(cust), map[string]any{"platform_rating": 1, "product_rating": 1}, http.StatusConflict)
	if errCode(env) != "SERVICE_ALREADY_REVIEWED" {
		t.Fatalf("second review code = %s", errCode(env))
	}

	// 7. The dealer's Google review link is returned once it is set.
	const gurl = "https://g.page/r/t244-review"
	if _, err := it.pool.Exec(ctx, `UPDATE organizations SET google_business_url = $1 WHERE id = $2`, gurl, dealer.ID); err != nil {
		t.Fatalf("google url: %v", err)
	}
	st = decodeData[portalServiceReviewState](t, it.accDo("GET", path, portalTok(cust), nil, http.StatusOK))
	if st.Review == nil || st.Review.UUID != created.Review.UUID || st.CanReview ||
		st.GoogleBusinessURL == nil || *st.GoogleBusinessURL != gurl {
		t.Fatalf("final state = %+v", st)
	}
}
