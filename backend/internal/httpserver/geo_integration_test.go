package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-84 seed rows used by the acceptance tests.
type geoIDs struct {
	nl, tr                    db.Country
	noordHolland, zuidHolland db.Province
	amsterdam, haarlem, rdam  db.District
}

func (it *itest) geoIDs() geoIDs {
	it.t.Helper()
	ctx := context.Background()
	var g geoIDs
	var err error
	if g.nl, err = it.q.GetCountryByISO2(ctx, "NL"); err != nil {
		it.t.Fatalf("NL: %v", err)
	}
	if g.tr, err = it.q.GetCountryByISO2(ctx, "TR"); err != nil {
		it.t.Fatalf("TR: %v", err)
	}
	row := func(sql string, args ...any) int64 {
		var id int64
		if err := it.pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			it.t.Fatalf("%s %v: %v", sql, args, err)
		}
		return id
	}
	prov := func(code string) db.Province {
		p, err := it.q.GetProvinceByID(ctx, row("SELECT id FROM provinces WHERE country_id = $1 AND code = $2", g.nl.ID, code))
		if err != nil {
			it.t.Fatal(err)
		}
		return p
	}
	dist := func(p db.Province, name string) db.District {
		d, err := it.q.GetDistrictByID(ctx, row("SELECT id FROM districts WHERE province_id = $1 AND name = $2", p.ID, name))
		if err != nil {
			it.t.Fatal(err)
		}
		return d
	}
	g.noordHolland, g.zuidHolland = prov("NL-NH"), prov("NL-ZH")
	g.amsterdam, g.haarlem = dist(g.noordHolland, "Amsterdam"), dist(g.noordHolland, "Haarlem")
	g.rdam = dist(g.zuidHolland, "Rotterdam")
	return g
}

// Territories are brand wide; start from a clean NL so leftovers of an
// aborted run cannot fake a conflict.
func (it *itest) clearNLTerritories(g geoIDs) {
	it.t.Helper()
	clear := func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM territories WHERE country_id = $1", g.nl.ID)
	}
	clear()
	it.t.Cleanup(clear)
}

type orgOut struct {
	UUID       string `json:"uuid"`
	City       string `json:"city"`
	District   string `json:"district"`
	CountryID  *int64 `json:"country_id"`
	ProvinceID *int64 `json:"province_id"`
	DistrictID *int64 `json:"district_id"`
	Parent     *struct {
		UUID string `json:"uuid"`
	} `json:"parent"`
}

// Acceptance 1 + 2: Amsterdam assigned to the NL distributor; a dealer opened
// in Amsterdam lands under it; a second distributor cannot take Amsterdam
// (409) nor an ancestor/descendant of it.
func TestIntegrationTerritoryAmsterdam(t *testing.T) {
	it := newIntegration(t)
	g := it.geoIDs()
	it.clearNLTerritories(g)
	center := it.brandCenter("olex")
	distA := it.org("nl-dist-a", "distributor", center)
	distB := it.org("nl-dist-b", "distributor", center)
	owner, _ := it.user("geo-owner")
	admin := it.adminToken()

	assign := func(dist db.Organization, body map[string]any) (int, envelope) {
		body["distributor_uuid"] = dist.Uuid.String()
		return it.do("POST", "/v1/platform/territories", hostOlex, admin, body)
	}
	code, env := assign(distA, map[string]any{
		"country_id": g.nl.ID, "province_id": g.noordHolland.ID, "district_id": g.amsterdam.ID,
	})
	if code != http.StatusCreated {
		t.Fatalf("assign Amsterdam to A: %d %s", code, errCode(env))
	}
	var terr geo.Territory
	_ = json.Unmarshal(env.Data, &terr)
	if terr.Level != "district" || terr.OrganizationUUID != distA.Uuid {
		t.Fatalf("territory = %+v", terr)
	}

	for name, body := range map[string]map[string]any{
		"same district":           {"country_id": g.nl.ID, "province_id": g.noordHolland.ID, "district_id": g.amsterdam.ID},
		"ancestor province":       {"country_id": g.nl.ID, "province_id": g.noordHolland.ID},
		"ancestor country":        {"country_id": g.nl.ID},
		"same district, same org": {"country_id": g.nl.ID, "province_id": g.noordHolland.ID, "district_id": g.amsterdam.ID},
	} {
		target := distB
		if name == "same district, same org" {
			target = distA
		}
		code, env := assign(target, body)
		if code != http.StatusConflict || errCode(env) != "TERRITORY_CONFLICT" {
			t.Fatalf("%s: %d %s, want 409 TERRITORY_CONFLICT", name, code, errCode(env))
		}
	}
	// A sibling area is free.
	if code, env := assign(distB, map[string]any{
		"country_id": g.nl.ID, "province_id": g.zuidHolland.ID, "district_id": g.rdam.ID,
	}); code != http.StatusCreated {
		t.Fatalf("assign Rotterdam to B: %d %s", code, errCode(env))
	}
	// A dealer is not a distributor; a district outside its province is invalid.
	dealerOrg := it.org("nl-dealer-x", "dealer", distA)
	if code, _ := assign(dealerOrg, map[string]any{"country_id": g.nl.ID, "province_id": g.zuidHolland.ID}); code != http.StatusUnprocessableEntity {
		t.Fatalf("dealer territory: %d", code)
	}
	if code, _ := assign(distB, map[string]any{
		"country_id": g.nl.ID, "province_id": g.zuidHolland.ID, "district_id": g.haarlem.ID,
	}); code != http.StatusUnprocessableEntity {
		t.Fatalf("district outside province: %d", code)
	}

	create := func(name string, district db.District) orgOut {
		t.Helper()
		code, env := it.do("POST", "/v1/platform/organizations", hostOlex, admin, map[string]any{
			"name": name + " " + it.suffix, "city": "", "district": "", "phone": "", "address": "",
			"owner_user_uuid": owner.Uuid.String(), "type": "dealer",
			"country_id": g.nl.ID, "province_id": district.ProvinceID, "district_id": district.ID,
		})
		var o orgOut
		_ = json.Unmarshal(env.Data, &o)
		if code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", name, code, errCode(env))
		}
		t.Cleanup(func() {
			_, _ = it.pool.Exec(context.Background(), "DELETE FROM organizations WHERE uuid = $1", o.UUID)
		})
		return o
	}
	ams := create("Amsterdam Dealer", g.amsterdam)
	if ams.Parent == nil || ams.Parent.UUID != distA.Uuid.String() {
		t.Fatalf("Amsterdam dealer parent = %+v, want %s", ams.Parent, distA.Uuid)
	}
	if ams.City != "Noord-Holland" || ams.District != "Amsterdam" || ams.DistrictID == nil || *ams.DistrictID != g.amsterdam.ID {
		t.Fatalf("Amsterdam dealer address = %+v", ams)
	}
	// Haarlem is in Noord-Holland but no distributor covers it: the center.
	haarlem := create("Haarlem Dealer", g.haarlem)
	if haarlem.Parent == nil || haarlem.Parent.UUID != center.Uuid.String() {
		t.Fatalf("Haarlem dealer parent = %+v, want center", haarlem.Parent)
	}

	code, env = it.do("GET", fmt.Sprintf("/v1/platform/territories/resolve?country_id=%d&province_id=%d&district_id=%d",
		g.nl.ID, g.noordHolland.ID, g.amsterdam.ID), hostOlex, admin, nil)
	var res struct {
		Match *geo.Match `json:"match"`
	}
	_ = json.Unmarshal(env.Data, &res)
	if code != http.StatusOK || res.Match == nil || res.Match.OrganizationUUID != distA.Uuid {
		t.Fatalf("resolve: %d %+v", code, res)
	}

	code, env = it.do("GET", "/v1/platform/territories?distributor_uuid="+distA.Uuid.String(), hostOlex, admin, nil)
	var list struct {
		Items []geo.Territory `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &list)
	if code != http.StatusOK || len(list.Items) != 1 || list.Items[0].DistrictName == nil || *list.Items[0].DistrictName != "Amsterdam" {
		t.Fatalf("list: %d %+v", code, list)
	}
	// Removing the territory frees the area for B.
	if code, _ := it.do("DELETE", "/v1/platform/territories/"+terr.UUID.String(), hostOlex, admin, nil); code != http.StatusOK {
		t.Fatalf("delete territory: %d", code)
	}
	if code, env := assign(distB, map[string]any{"country_id": g.nl.ID, "province_id": g.noordHolland.ID}); code != http.StatusCreated {
		t.Fatalf("assign Noord-Holland to B after delete: %d %s", code, errCode(env))
	}
}

// The advisory lock serializes overlapping assignments: of "Amsterdam to A"
// and "NL to B" racing, exactly one wins.
func TestIntegrationTerritoryConcurrentOverlap(t *testing.T) {
	it := newIntegration(t)
	g := it.geoIDs()
	it.clearNLTerritories(g)
	center := it.brandCenter("olex")
	distA := it.org("nl-race-a", "distributor", center)
	distB := it.org("nl-race-b", "distributor", center)
	svc := geo.New(it.pool, it.q)
	for round := 0; round < 5; round++ {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM territories WHERE country_id = $1", g.nl.ID)
		var wg sync.WaitGroup
		errs := make([]error, 2)
		inputs := []geo.AssignInput{
			{BrandID: center.BrandID, DistributorID: distA.ID, CountryID: g.nl.ID, ProvinceID: &g.noordHolland.ID, DistrictID: &g.amsterdam.ID},
			{BrandID: center.BrandID, DistributorID: distB.ID, CountryID: g.nl.ID},
		}
		for i := range inputs {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, errs[i] = svc.Assign(context.Background(), inputs[i])
			}(i)
		}
		wg.Wait()
		ok, conflicts := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				ok++
			case errors.Is(err, geo.ErrTerritoryConflict):
				conflicts++
			default:
				t.Fatalf("round %d: %v", round, err)
			}
		}
		if ok != 1 || conflicts != 1 {
			t.Fatalf("round %d: ok=%d conflicts=%d", round, ok, conflicts)
		}
	}
}

// Acceptance 3: the plate country is independent of the customer; a German
// plate validates against DE, an invalid one answers 422.
func TestIntegrationPlateValidation(t *testing.T) {
	it := newIntegration(t)
	admin := it.adminToken()
	check := func(country, plate string) (int, string, geo.PlateCheck) {
		code, env := it.do("POST", "/v1/plate-formats/validate", hostOlex, admin, map[string]string{"country": country, "plate": plate})
		var c geo.PlateCheck
		_ = json.Unmarshal(env.Data, &c)
		return code, errCode(env), c
	}
	if code, ec, c := check("DE", "b-ab 1234"); code != http.StatusOK || !c.Valid || c.Normalized != "BAB1234" {
		t.Fatalf("DE valid: %d %s %+v", code, ec, c)
	}
	if code, ec, _ := check("DE", "B AB 0123"); code != http.StatusUnprocessableEntity || ec != "INVALID_PLATE" {
		t.Fatalf("DE invalid: %d %s", code, ec)
	}
	if code, ec, _ := check("TR", "34 ABC 123"); code != http.StatusOK {
		t.Fatalf("TR valid: %d %s", code, ec)
	}
	if code, ec, _ := check("NL", "AB-123-C"); code != http.StatusOK {
		t.Fatalf("NL valid: %d %s", code, ec)
	}
	if code, _, _ := check("FR", "AB-123-CD"); code != http.StatusNotFound {
		t.Fatalf("no FR format: %d", code)
	}

	code, env := it.do("GET", "/v1/plate-formats", hostOlex, admin, nil)
	var list struct {
		Items []geo.PlateFormat `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &list)
	if code != http.StatusOK || len(list.Items) < 3 {
		t.Fatalf("plate formats: %d %+v", code, list)
	}

	// Admin CRUD: a lookahead regex is not RE2 and is rejected.
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(),
			"DELETE FROM plate_formats WHERE country_id = (SELECT id FROM countries WHERE iso2 = 'FR')")
	})
	if code, _ := it.do("POST", "/v1/platform/plate-formats", hostOlex, admin, map[string]any{
		"country": "FR", "regex": "^(?=A)[A-Z]{2}$", "country_label": "F",
	}); code != http.StatusUnprocessableEntity {
		t.Fatalf("lookahead regex: %d", code)
	}
	if code, env := it.do("POST", "/v1/platform/plate-formats", hostOlex, admin, map[string]any{
		"country": "FR", "regex": "^[A-Z]{2}[0-9]{3}[A-Z]{2}$", "country_label": "F", "example": "AB-123-CD",
	}); code != http.StatusCreated {
		t.Fatalf("create FR: %d %s", code, errCode(env))
	}
	if code, _, _ := check("FR", "AB-123-CD"); code != http.StatusOK {
		t.Fatalf("FR valid after create: %d", code)
	}
	if code, _ := it.do("PATCH", "/v1/platform/plate-formats/FR", hostOlex, admin, map[string]any{"is_active": false}); code != http.StatusOK {
		t.Fatalf("deactivate FR: %d", code)
	}
	if code, _ := it.do("DELETE", "/v1/platform/plate-formats/FR", hostOlex, admin, nil); code != http.StatusOK {
		t.Fatalf("delete FR: %d", code)
	}
}

// Seed: 81 il / 973 ilçe for TR, ISO 3166-1 countries, pickers.
func TestIntegrationGeoSeedAndPickers(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	var countries, trProvinces, trDistricts int
	if err := it.pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM countries),
		(SELECT COUNT(*) FROM provinces p JOIN countries c ON c.id = p.country_id WHERE c.iso2 = 'TR'),
		(SELECT COUNT(*) FROM districts d JOIN provinces p ON p.id = d.province_id JOIN countries c ON c.id = p.country_id WHERE c.iso2 = 'TR')`,
	).Scan(&countries, &trProvinces, &trDistricts); err != nil {
		t.Fatal(err)
	}
	if countries < 249 || trProvinces != 81 || trDistricts != 973 {
		t.Fatalf("seed: countries=%d tr provinces=%d tr districts=%d", countries, trProvinces, trDistricts)
	}
	admin := it.adminToken()
	code, env := it.do("GET", "/v1/geo/countries/tr/provinces", hostOlex, admin, nil)
	var provs struct {
		Items []geo.Province `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &provs)
	if code != http.StatusOK || len(provs.Items) != 81 || provs.Items[33].Name != "İstanbul" || !provs.Items[33].HasDistricts {
		t.Fatalf("TR provinces: %d %d", code, len(provs.Items))
	}
	code, env = it.do("GET", fmt.Sprintf("/v1/geo/provinces/%d/districts", provs.Items[33].ID), hostOlex, admin, nil)
	var dists struct {
		Items []geo.District `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &dists)
	if code != http.StatusOK || len(dists.Items) != 39 {
		t.Fatalf("İstanbul districts: %d %d", code, len(dists.Items))
	}
	if code, _ := it.do("GET", "/v1/geo/countries", hostOlex, "", nil); code != http.StatusUnauthorized {
		t.Fatalf("pickers need a session: %d", code)
	}

	// TEC-320: the public form reads the same pickers without a session.
	code, env = it.do("GET", "/v1/public/geo/countries?all=true", hostOlex, "", nil)
	var pubCountries struct {
		Items []geo.Country `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &pubCountries)
	if code != http.StatusOK || len(pubCountries.Items) == 0 {
		t.Fatalf("public countries: %d %d", code, len(pubCountries.Items))
	}
	for _, c := range pubCountries.Items {
		if !c.IsActive {
			t.Fatalf("public countries list hidden %s", c.ISO2)
		}
	}
	code, env = it.do("GET", "/v1/public/geo/countries/tr/provinces", hostOlex, "", nil)
	_ = json.Unmarshal(env.Data, &provs)
	if code != http.StatusOK || len(provs.Items) != 81 {
		t.Fatalf("public TR provinces: %d %d", code, len(provs.Items))
	}
	code, env = it.do("GET", fmt.Sprintf("/v1/public/geo/provinces/%d/districts", provs.Items[33].ID), hostOlex, "", nil)
	_ = json.Unmarshal(env.Data, &dists)
	if code != http.StatusOK || len(dists.Items) != 39 {
		t.Fatalf("public İstanbul districts: %d %d", code, len(dists.Items))
	}
}

// Override priority and previous-day fallback on the real SQL.
func TestIntegrationExchangeRates(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	// A fixed past day no provider fetch ever writes.
	d := time.Date(2001, 3, 14, 0, 0, 0, 0, time.UTC)
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM exchange_rates WHERE rate_date BETWEEN '2001-03-01' AND '2001-03-31'")
	})
	for _, r := range []db.UpsertExchangeRateParams{
		{RateDate: pgtype.Date{Time: d, Valid: true}, Base: "EUR", Quote: "TRY", Rate: "30.5", Source: fxrates.SourceECB},
		{RateDate: pgtype.Date{Time: d, Valid: true}, Base: "EUR", Quote: "TRY", Rate: "31.25", Source: fxrates.SourceTCMB},
	} {
		if err := it.q.UpsertExchangeRate(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	admin := it.adminToken()
	resolve := func(date string) (int, fxrates.Snapshot) {
		code, env := it.do("GET", "/v1/platform/exchange-rates/resolve?date="+date+"&base=EUR&quote=TRY", hostOlex, admin, nil)
		var s fxrates.Snapshot
		_ = json.Unmarshal(env.Data, &s)
		return code, s
	}
	if code, s := resolve("2001-03-14"); code != http.StatusOK || s.Source != "tcmb" || s.Rate != "31.25" {
		t.Fatalf("tcmb over ecb: %d %+v", code, s)
	}
	if code, env := it.do("PUT", "/v1/platform/exchange-rates/override", hostOlex, admin, map[string]any{
		"date": "2001-03-14", "base": "EUR", "quote": "TRY", "rate": "32", "note": "test",
	}); code != http.StatusOK {
		t.Fatalf("override: %d %s", code, errCode(env))
	}
	if code, s := resolve("2001-03-14"); code != http.StatusOK || s.Source != "manual" || s.Rate != "32" {
		t.Fatalf("manual wins: %d %+v", code, s)
	}
	// Weekend / missing day: the previous day's rate.
	if code, s := resolve("2001-03-17"); code != http.StatusOK || s.RateDate != "2001-03-14" {
		t.Fatalf("previous day: %d %+v", code, s)
	}
	// Inverse direction.
	code, env := it.do("GET", "/v1/platform/exchange-rates/resolve?date=2001-03-14&base=TRY&quote=EUR", hostOlex, admin, nil)
	var inv fxrates.Snapshot
	_ = json.Unmarshal(env.Data, &inv)
	if code != http.StatusOK || inv.Rate != "0.03125" {
		t.Fatalf("inverse: %d %+v", code, inv)
	}
	code, env = it.do("GET", "/v1/platform/exchange-rates?date=2001-03-14", hostOlex, admin, nil)
	var day struct {
		Items []fxrates.StoredRate `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &day)
	if code != http.StatusOK || len(day.Items) != 3 || day.Items[0].Source != "manual" || !day.Items[0].Effective || day.Items[1].Effective {
		t.Fatalf("day list: %d %+v", code, day)
	}
	if code, _ := it.do("DELETE", "/v1/platform/exchange-rates/override?date=2001-03-14&base=EUR&quote=TRY", hostOlex, admin, nil); code != http.StatusOK {
		t.Fatalf("clear override: %d", code)
	}
	if code, s := resolve("2001-03-14"); code != http.StatusOK || s.Source != "tcmb" {
		t.Fatalf("after clear: %d %+v", code, s)
	}
	if code, env := it.do("GET", "/v1/platform/exchange-rates/resolve?date=2001-03-14&base=EUR&quote=CNY", hostOlex, admin, nil); code != http.StatusNotFound || errCode(env) != "RATE_NOT_FOUND" {
		t.Fatalf("missing pair: %d %s", code, errCode(env))
	}
}
