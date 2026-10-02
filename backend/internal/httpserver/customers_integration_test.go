package httpserver

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// base64 of "0123456789abcdef0123456789abcdef" (32 bytes); test only.
const itCustomerPIIKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

type custOrgLink struct {
	UUID string `json:"uuid"`
}

type custView struct {
	UUID             string        `json:"uuid"`
	Name             string        `json:"name"`
	Surname          string        `json:"surname"`
	Email            *string       `json:"email"`
	Phone            *string       `json:"phone"`
	Status           string        `json:"status"`
	Anonymized       bool          `json:"anonymized"`
	NationalIDLast4  *string       `json:"national_id_last4"`
	NationalID       *string       `json:"national_id"`
	ExistingUser     bool          `json:"existing_user"`
	IgnoredFields    []string      `json:"ignored_fields"`
	Editable         bool          `json:"editable"`
	IdentityEditable bool          `json:"identity_editable"`
	Organizations    []custOrgLink `json:"organizations"`
}

type custPage struct {
	Items []custView `json:"items"`
	Total int64      `json:"total"`
}

type vehView struct {
	UUID            string   `json:"uuid"`
	CustomerUUID    string   `json:"customer_uuid"`
	Plate           *string  `json:"plate"`
	PlateNormalized *string  `json:"plate_normalized"`
	PlateCountry    *string  `json:"plate_country"`
	VIN             *string  `json:"vin"`
	Warnings        []string `json:"warnings"`
	CarBrand        *struct {
		UUID string `json:"uuid"`
	} `json:"car_brand"`
	CarModel *struct {
		UUID string `json:"uuid"`
	} `json:"car_model"`
}

type vehPage struct {
	Items []vehView `json:"items"`
	Total int64     `json:"total"`
}

func (it *itest) custDo(method, path, token string, body any, want int) envelope {
	it.t.Helper()
	code, env := it.do(method, path, hostOlex, token, body)
	if code != want {
		it.t.Fatalf("%s %s = %d %s, want %d", method, path, code, errCode(env), want)
	}
	return env
}

// cleanupCustomers removes the API-created customers and their vehicles and
// links before the organizations and users of the test are deleted
// (vehicles and links restrict both).
func (it *itest) cleanupCustomers(phones *[]string, orgs ...db.Organization) {
	it.t.Cleanup(func() {
		ctx := context.Background()
		ids := make([]int64, 0, len(orgs))
		for _, o := range orgs {
			ids = append(ids, o.ID)
		}
		_, _ = it.pool.Exec(ctx, `DELETE FROM vehicles WHERE organization_id = ANY($1)
			OR user_id IN (SELECT id FROM users WHERE phone_e164 = ANY($2))`, ids, *phones)
		_, _ = it.pool.Exec(ctx, `DELETE FROM customer_organizations WHERE organization_id = ANY($1)
			OR user_id IN (SELECT id FROM users WHERE phone_e164 = ANY($2))`, ids, *phones)
		_, _ = it.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id IN (SELECT id FROM users WHERE phone_e164 = ANY($1))`, *phones)
		_, _ = it.pool.Exec(ctx, `DELETE FROM users WHERE phone_e164 = ANY($1)`, *phones)
	})
}

func hasUUID(items []custView, id string) bool {
	return slices.ContainsFunc(items, func(c custView) bool { return c.UUID == id })
}

// TEC-160 acceptance 1 + 2: the same phone at two dealers is one user with
// two customer_organizations rows; dealer A never sees dealer B's customer
// or vehicle (404). Identity numbers are stored encrypted and read back as
// last4 only; a shared customer's name is not overwritten; anonymized
// customers are masked and read-only.
func TestIntegrationCustomersOnePhoneAndScope(t *testing.T) {
	it := newIntegrationWith(t, func(c *config.Config) { c.Encryption.CustomerPIIKey = itCustomerPIIKey })
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t160-dist", "distributor", center)
	dealerA := it.org("t160-a", "dealer", dist)
	dealerB := it.org("t160-b", "dealer", dist)
	var phones []string
	it.cleanupCustomers(&phones, dealerA, dealerB, dist)

	ownerA, pwA := it.user("t160-owner-a")
	it.member(dealerA, ownerA, "owner")
	ownerB, pwB := it.user("t160-owner-b")
	it.member(dealerB, ownerB, "owner")
	distOwner, pwD := it.user("t160-dist-owner")
	it.member(dist, distOwner, "owner")
	tokA := it.loginOrg(ownerA, pwA, dealerA)
	tokB := it.loginOrg(ownerB, pwB, dealerB)
	tokD := it.loginOrg(distOwner, pwD, dist)

	// 1. Dealer A creates the customer with a national-format phone.
	ph := itPhone()
	phones = append(phones, ph)
	national := "0" + ph[3:]
	email := fmt.Sprintf("t160-%s@example.test", it.suffix)
	env := it.custDo("POST", "/v1/customers", tokA, map[string]any{
		"phone": national, "name": "Ayşe", "surname": "Kaya", "email": email,
		"national_id": "12345678901", "type": "individual",
	}, http.StatusCreated)
	c1 := decodeData[custView](t, env)
	if c1.ExistingUser || c1.Phone == nil || *c1.Phone != ph {
		t.Fatalf("create: %+v (phone must be E.164 %s)", c1, ph)
	}
	if c1.NationalIDLast4 == nil || *c1.NationalIDLast4 != "8901" || c1.NationalID != nil {
		t.Fatalf("national id must come back as last4 only: %+v", c1)
	}
	if bytes.Contains(env.Data, []byte("12345678901")) {
		t.Fatal("full national id leaked in the response")
	}
	var enc []byte
	if err := it.pool.QueryRow(ctx, `SELECT p.national_id_enc FROM customer_profiles p
		JOIN users u ON u.id = p.user_id WHERE u.uuid = $1`, c1.UUID).Scan(&enc); err != nil {
		t.Fatal(err)
	}
	if len(enc) == 0 || bytes.Contains(enc, []byte("12345678901")) {
		t.Fatal("national id must be stored encrypted")
	}

	// 2. Dealer B creates the same phone: same user, name kept (fill-only).
	env = it.custDo("POST", "/v1/customers", tokB, map[string]any{
		"phone": ph, "name": "Başka", "surname": "Kaya",
	}, http.StatusOK)
	c1b := decodeData[custView](t, env)
	if !c1b.ExistingUser || c1b.UUID != c1.UUID || c1b.Name != "Ayşe" || !slices.Contains(c1b.IgnoredFields, "name") {
		t.Fatalf("second create must link the same user without renaming: %+v", c1b)
	}
	var users, links int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE phone_e164 = $1 AND deleted_at IS NULL`, ph).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM customer_organizations co JOIN users u ON u.id = co.user_id
		WHERE u.phone_e164 = $1`, ph).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if users != 1 || links != 2 {
		t.Fatalf("users = %d, links = %d; want 1 and 2", users, links)
	}
	// Each dealer sees only its own link; the shared identity is not fully
	// editable by either.
	if len(c1b.Organizations) != 1 || c1b.Organizations[0].UUID != dealerB.Uuid.String() || c1b.IdentityEditable {
		t.Fatalf("dealer B view: %+v", c1b)
	}
	env = it.custDo("PATCH", "/v1/customers/"+c1.UUID, tokA, map[string]any{"name": "Ayşegül"}, http.StatusOK)
	if got := decodeData[custView](t, env); got.Name != "Ayşe" || !slices.Contains(got.IgnoredFields, "name") {
		t.Fatalf("dealer A must not rename a shared customer: %+v", got)
	}

	// 3. Vehicles: plate validated (geo.ValidatePlate), VIN 17 characters.
	carBrand, err := it.q.CreateCarBrand(ctx, db.CreateCarBrandParams{Name: "T160 Brand " + it.suffix, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	carModel, err := it.q.CreateCarModel(ctx, db.CreateCarModelParams{CarBrandID: carBrand.ID, Name: "T160 Model", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM vehicles WHERE car_brand_id = $1", carBrand.ID)
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM car_models WHERE id = $1", carModel.ID)
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM car_brands WHERE id = $1", carBrand.ID)
	})
	if code, env := it.do("POST", "/v1/vehicles", hostOlex, tokA, map[string]any{
		"customer_uuid": c1.UUID, "plate": "ABC",
	}); code != http.StatusBadRequest || errCode(env) != "INVALID_PLATE" {
		t.Fatalf("invalid plate: %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/vehicles", hostOlex, tokA, map[string]any{
		"customer_uuid": c1.UUID, "plate": "34 ABC 123", "vin": "SHORTVIN",
	}); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("invalid vin: %d %s", code, errCode(env))
	}
	env = it.custDo("POST", "/v1/vehicles", tokA, map[string]any{
		"customer_uuid": c1.UUID, "plate": "34 abc 123", "vin": "wvwzzz1jzxw000001",
		"car_model_uuid": carModel.Uuid.String(), "model_year": 2020,
	}, http.StatusCreated)
	v1 := decodeData[vehView](t, env)
	if v1.PlateNormalized == nil || *v1.PlateNormalized != "34ABC123" || v1.PlateCountry == nil || *v1.PlateCountry != "TR" ||
		v1.VIN == nil || *v1.VIN != "WVWZZZ1JZXW000001" || v1.CarBrand == nil || v1.CarBrand.UUID != carBrand.Uuid.String() ||
		v1.CarModel == nil || v1.CarModel.UUID != carModel.Uuid.String() {
		t.Fatalf("vehicle: %+v", v1)
	}

	// 4. A customer and vehicle of dealer B only.
	ph2 := itPhone()
	phones = append(phones, ph2)
	c2 := decodeData[custView](t, it.custDo("POST", "/v1/customers", tokB, map[string]any{
		"phone": ph2, "name": "Mehmet", "surname": "Demir",
	}, http.StatusCreated))
	v2 := decodeData[vehView](t, it.custDo("POST", "/v1/vehicles", tokB, map[string]any{
		"customer_uuid": c2.UUID, "plate": "06 XYZ 42",
	}, http.StatusCreated))

	// Dealer A cannot see or touch them.
	it.custDo("GET", "/v1/customers/"+c2.UUID, tokA, nil, http.StatusNotFound)
	it.custDo("PATCH", "/v1/customers/"+c2.UUID, tokA, map[string]any{"surname": "X"}, http.StatusNotFound)
	it.custDo("GET", "/v1/vehicles/"+v2.UUID, tokA, nil, http.StatusNotFound)
	it.custDo("PATCH", "/v1/vehicles/"+v2.UUID, tokA, map[string]any{"model_year": 2001}, http.StatusNotFound)
	it.custDo("DELETE", "/v1/vehicles/"+v2.UUID, tokA, nil, http.StatusNotFound)
	it.custDo("GET", "/v1/vehicles?customer_uuid="+c2.UUID, tokA, nil, http.StatusNotFound)
	it.custDo("POST", "/v1/vehicles", tokA, map[string]any{"customer_uuid": c2.UUID, "plate": "34 ABC 124"}, http.StatusNotFound)
	pageA := decodeData[custPage](t, it.custDo("GET", "/v1/customers?limit=100", tokA, nil, http.StatusOK))
	if hasUUID(pageA.Items, c2.UUID) || !hasUUID(pageA.Items, c1.UUID) {
		t.Fatalf("dealer A list: %+v", pageA.Items)
	}
	vehA := decodeData[vehPage](t, it.custDo("GET", "/v1/vehicles?limit=100", tokA, nil, http.StatusOK))
	for _, v := range vehA.Items {
		if v.UUID == v2.UUID {
			t.Fatal("dealer A lists dealer B's vehicle")
		}
	}
	// Dealer B serves the shared customer and sees its vehicle.
	it.custDo("GET", "/v1/vehicles/"+v1.UUID, tokB, nil, http.StatusOK)
	// The distributor sees its whole subtree.
	pageD := decodeData[custPage](t, it.custDo("GET", "/v1/customers?limit=100", tokD, nil, http.StatusOK))
	if !hasUUID(pageD.Items, c1.UUID) || !hasUUID(pageD.Items, c2.UUID) {
		t.Fatalf("distributor list: %+v", pageD.Items)
	}

	// 5. Anonymized customers are masked and read-only.
	if _, err := it.pool.Exec(ctx, `UPDATE users SET status = 'anonymized' WHERE uuid = $1`, c2.UUID); err != nil {
		t.Fatal(err)
	}
	pageB := decodeData[custPage](t, it.custDo("GET", "/v1/customers?limit=100", tokB, nil, http.StatusOK))
	found := false
	for _, c := range pageB.Items {
		if c.UUID == c2.UUID {
			found = true
			if !c.Anonymized || c.Name == "Mehmet" || c.Surname != "" || c.Phone != nil || c.Email != nil {
				t.Fatalf("anonymized customer not masked: %+v", c)
			}
		}
	}
	if !found {
		t.Fatal("anonymized customer missing from the list")
	}
	if code, env := it.do("PATCH", "/v1/customers/"+c2.UUID, hostOlex, tokB, map[string]any{"surname": "Y"}); code != http.StatusConflict ||
		errCode(env) != "CUSTOMER_ANONYMIZED" {
		t.Fatalf("anonymized patch: %d %s", code, errCode(env))
	}
	if code, env := it.do("PATCH", "/v1/vehicles/"+v2.UUID, hostOlex, tokB, map[string]any{"model_year": 2001}); code != http.StatusConflict ||
		errCode(env) != "CUSTOMER_ANONYMIZED" {
		t.Fatalf("anonymized vehicle patch: %d %s", code, errCode(env))
	}
}

// TEC-160 acceptance 3: upgrading a customer to dealer adds a membership in
// an existing dealer; the same user then signs in to the panel as a dealer
// member and to the portal with WhatsApp OTP. Dealers cannot upgrade; the
// distributor only into its own subtree.
func TestIntegrationCustomerUpgradeToDealer(t *testing.T) {
	it, fw := newWhatsAppIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t160u-dist", "distributor", center)
	otherDist := it.org("t160u-dist2", "distributor", center)
	dealer := it.org("t160u-dealer", "dealer", dist)
	foreign := it.org("t160u-foreign", "dealer", otherDist)
	var phones []string
	it.cleanupCustomers(&phones, dealer, foreign, dist, otherDist)

	dealerOwner, pwO := it.user("t160u-owner")
	it.member(dealer, dealerOwner, "owner")
	distOwner, pwD := it.user("t160u-dist-owner")
	it.member(dist, distOwner, "owner")
	staff, pwC := it.user("t160u-center")
	it.member(center, staff, "staff")
	tokO := it.loginOrg(dealerOwner, pwO, dealer)
	tokD := it.loginOrg(distOwner, pwD, dist)
	tokC := it.loginOrg(staff, pwC, center)

	ph := itPhone()
	phones = append(phones, ph)
	email := fmt.Sprintf("t160u-%s@example.test", it.suffix)
	cust := decodeData[custView](t, it.custDo("POST", "/v1/customers", tokO, map[string]any{
		"phone": ph, "name": "Can", "surname": "Usta", "email": email,
	}, http.StatusCreated))
	path := "/v1/customers/" + cust.UUID + "/upgrade-to-dealer"

	// A dealer cannot upgrade; a distributor cannot reach another subtree.
	it.custDo("POST", path, tokO, map[string]any{"organization_uuid": dealer.Uuid, "role": rbac.RoleDealerStaff}, http.StatusForbidden)
	it.custDo("POST", path, tokD, map[string]any{"organization_uuid": foreign.Uuid, "role": rbac.RoleDealerStaff}, http.StatusNotFound)
	it.custDo("POST", path, tokD, map[string]any{"organization_uuid": dealer.Uuid, "role": "center_staff"}, http.StatusBadRequest)

	// The distributor above the dealer upgrades; a repeat is a conflict.
	it.custDo("POST", path, tokD, map[string]any{"organization_uuid": dealer.Uuid, "role": rbac.RoleDealerStaff}, http.StatusCreated)
	if code, env := it.do("POST", path, hostOlex, tokC, map[string]any{"organization_uuid": dealer.Uuid, "role": rbac.RoleDealerOwner}); code != http.StatusConflict ||
		errCode(env) != "ALREADY_MEMBER" {
		t.Fatalf("repeat upgrade: %d %s", code, errCode(env))
	}

	// History kept: customer role and the dealer link stay.
	var hasCustomerRole bool
	if err := it.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id
		JOIN users u ON u.id = ur.user_id WHERE u.uuid = $1 AND r.slug = 'customer')`, cust.UUID).Scan(&hasCustomerRole); err != nil {
		t.Fatal(err)
	}
	if !hasCustomerRole {
		t.Fatal("upgrade must keep the customer role")
	}
	it.custDo("GET", "/v1/customers/"+cust.UUID, tokO, nil, http.StatusOK)

	// Panel: the user (password set as after a reset) is a dealer member.
	hash, err := password.Hash("Upgrade-Passw0rd!x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := it.pool.Exec(ctx, `UPDATE users SET password_hash = $2, email_verified_at = NOW() WHERE uuid = $1`, cust.UUID, hash); err != nil {
		t.Fatal(err)
	}
	panel := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": email, "password": "Upgrade-Passw0rd!x", "organization_slug": dealer.Slug,
	}))
	if it.realm(panel.AccessToken) != jwt.AudiencePanel || it.oid(panel.AccessToken) != dealer.Uuid.String() {
		t.Fatal("upgraded customer must get a panel session in the dealer")
	}
	if g := it.me(panel.AccessToken); !slices.Contains(g.OrganizationRoles, rbac.RoleDealerStaff) {
		t.Fatalf("panel roles: %+v", g)
	}

	// Portal: WhatsApp OTP still signs the same user in.
	if code, env := it.do("POST", "/v1/auth/otp/request", hostOlex, "", map[string]string{"phone": ph}); code != http.StatusAccepted {
		t.Fatalf("otp request: %d %s", code, errCode(env))
	}
	otp, _ := sentCode(t, fw, ph)
	portal := it.tokensFrom(it.do("POST", "/v1/auth/otp/verify", hostOlex, "", map[string]string{"phone": ph, "code": otp}))
	if it.realm(portal.AccessToken) != jwt.AudiencePortal || mustSubject(t, it, portal.AccessToken) != cust.UUID {
		t.Fatal("portal sign-in must keep working for the upgraded customer")
	}
	var members int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM organization_members om JOIN users u ON u.id = om.user_id
		WHERE u.uuid = $1`, cust.UUID).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if members != 1 {
		t.Fatalf("memberships = %d, want 1", members)
	}
}
