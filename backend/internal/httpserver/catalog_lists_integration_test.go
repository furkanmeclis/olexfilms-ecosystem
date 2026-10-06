package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	bulkusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/bulk/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-369 (DT-BE-3): list contract of the catalog screens.

type listItem struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	SKU  string `json:"sku"`
}

// listNames GETs path and returns the names (or SKUs) in response order
// plus the total; want is the expected status.
func (it *itest) listNames(path, host, token string, want int, sku bool) ([]string, int64, string) {
	it.t.Helper()
	code, env := it.do("GET", path, host, token, nil)
	if code != want {
		it.t.Fatalf("GET %s: %d %s, want %d", path, code, errCode(env), want)
	}
	if code != http.StatusOK {
		return nil, 0, errCode(env)
	}
	var page struct {
		Items []listItem `json:"items"`
		Total int64      `json:"total"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		it.t.Fatalf("GET %s payload: %s", path, env.Data)
	}
	out := make([]string, 0, len(page.Items))
	for _, i := range page.Items {
		if sku {
			out = append(out, i.SKU)
		} else {
			out = append(out, i.Name)
		}
	}
	return out, page.Total, ""
}

func (it *itest) catalogCategory(token, name string) listItem {
	it.t.Helper()
	code, env := it.do("POST", "/v1/catalog/categories", hostOlex, token, map[string]any{"name": name})
	if code != http.StatusCreated {
		it.t.Fatalf("create category %s: %d %s", name, code, errCode(env))
	}
	var c listItem
	_ = json.Unmarshal(env.Data, &c)
	return c
}

func (it *itest) catalogProduct(token string, body map[string]any) listItem {
	it.t.Helper()
	code, env := it.do("POST", "/v1/catalog/products", hostOlex, token, body)
	if code != http.StatusCreated {
		it.t.Fatalf("create product %v: %d %s", body["sku"], code, errCode(env))
	}
	var p listItem
	_ = json.Unmarshal(env.Data, &p)
	return p
}

func wantOrder(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

func TestIntegrationCatalogListsProducts(t *testing.T) {
	it := newIntegration(t)
	it.cleanupCatalog()
	center := it.brandCenter("olex")
	dist := it.org("t369-dist", "distributor", center)
	staff, spw := it.user("t369-staff")
	it.member(center, staff, "staff")
	distUser, dpw := it.user("t369-dist")
	it.member(dist, distUser, "owner")
	tok := it.catalogLogin(staff, spw, center, hostOlex)
	distTok := it.catalogLogin(distUser, dpw, dist, hostOlex)
	s := it.suffix

	catA := it.catalogCategory(tok, "A-cat "+s)
	catB := it.catalogCategory(tok, "B-cat "+s)
	skuA, skuB, skuC := "A1-"+s, "B1-"+s, "C1-"+s
	pa := it.catalogProduct(tok, map[string]any{
		"category_uuid": catA.UUID, "sku": skuA, "name": "Zeta " + s, "unit_type": "piece",
		"warranty_duration_months": 12, "micron_thickness": 100, "uses_fixed_barcode": true,
	})
	pb := it.catalogProduct(tok, map[string]any{
		"category_uuid": catB.UUID, "sku": skuB, "name": "Alpha " + s, "unit_type": "roll_meter",
		"warranty_duration_months": 60, "micron_thickness": 200,
	})
	it.catalogProduct(tok, map[string]any{
		"category_uuid": catB.UUID, "sku": skuC, "name": "Mid " + s, "unit_type": "roll_meter",
	})

	base := "/v1/catalog/products?q=" + url.QueryEscape(s)
	list := func(extra string) []string {
		got, _, _ := it.listNames(base+extra, hostOlex, tok, http.StatusOK, true)
		return got
	}
	// Default sort is name; sku asc/desc really reorders.
	wantOrder(t, "default (name)", list(""), []string{skuB, skuC, skuA})
	wantOrder(t, "sort=sku", list("&sort=sku"), []string{skuA, skuB, skuC})
	wantOrder(t, "sort=-sku", list("&sort=-sku"), []string{skuC, skuB, skuA})
	// Nullable numbers sort empty values last in both directions.
	wantOrder(t, "sort=warranty", list("&sort=warranty_duration_months"), []string{skuA, skuB, skuC})
	wantOrder(t, "sort=-warranty", list("&sort=-warranty_duration_months"), []string{skuB, skuA, skuC})
	wantOrder(t, "sort=-micron", list("&sort=-micron_thickness"), []string{skuB, skuA, skuC})
	// Desc also flips the id tiebreak (C was created after B); extra fields are ignored.
	wantOrder(t, "sort=-category", list("&sort=-category,sku"), []string{skuC, skuB, skuA})
	// Filters.
	wantOrder(t, "unit_type CSV", list("&sort=sku&unit_type=piece,roll_meter"), []string{skuA, skuB, skuC})
	wantOrder(t, "unit_type", list("&sort=sku&unit_type=roll_meter"), []string{skuB, skuC})
	wantOrder(t, "barcode", list("&uses_fixed_barcode=true"), []string{skuA})
	wantOrder(t, "warranty range", list("&sort=sku&warranty_duration_months_min=24"), []string{skuB})
	wantOrder(t, "micron range", list("&sort=sku&micron_thickness_min=50&micron_thickness_max=150"), []string{skuA})
	wantOrder(t, "category CSV", list("&sort=sku&category_uuid="+catA.UUID+","+catB.UUID), []string{skuA, skuB, skuC})
	wantOrder(t, "category one", list("&sort=sku&category_uuid="+catA.UUID), []string{skuA})
	wantOrder(t, "created future", list("&created_from=2999-01-01"), []string{})
	wantOrder(t, "created to", list("&sort=sku&created_to=2999-12-31"), []string{skuA, skuB, skuC})
	for _, bad := range []string{"&sort=description_md", "&unit_type=box", "&uses_fixed_barcode=1",
		"&warranty_duration_months_min=x", "&category_uuid=nope", "&created_from=01.01.2026"} {
		it.listNames(base+bad, hostOlex, tok, http.StatusBadRequest, true)
	}

	// Bulk set_category (center only) + undo restores the old category.
	run := it.bulkRun("/v1/catalog/products/bulk", tok, map[string]any{
		"action": "set_category",
		"target": map[string]any{"scope": "ids", "ids": []string{pa.UUID, pb.UUID}, "params": map[string]string{"category_uuid": catB.UUID}},
	})
	if run.Summary.Succeeded != 2 || run.Operation.UndoStatus != bulkusecase.UndoAvailable {
		t.Fatalf("set_category run = %+v", run)
	}
	wantOrder(t, "after set_category", list("&sort=sku&category_uuid="+catB.UUID), []string{skuA, skuB, skuC})
	it.bulkUndo("/v1/tenant/bulk-operations/"+run.Operation.UUID.String()+"/undo", hostOlex, tok, http.StatusOK)
	wantOrder(t, "after undo", list("&sort=sku&category_uuid="+catA.UUID), []string{skuA})
	if code, env := it.do("POST", "/v1/catalog/products/bulk", hostOlex, tok, map[string]any{
		"action": "set_category",
		"target": map[string]any{"scope": "ids", "ids": []string{pa.UUID}, "params": map[string]string{"category_uuid": "00000000-0000-0000-0000-000000000000"}},
	}); code != http.StatusBadRequest {
		t.Fatalf("set_category unknown category: %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/catalog/products/bulk", hostOlex, distTok, map[string]any{
		"action": "set_category",
		"target": map[string]any{"scope": "ids", "ids": []string{pa.UUID}, "params": map[string]string{"category_uuid": catB.UUID}},
	}); code != http.StatusForbidden {
		t.Fatalf("distributor set_category: %d %s", code, errCode(env))
	}
}

func TestIntegrationCatalogListsCategories(t *testing.T) {
	it := newIntegration(t)
	it.cleanupCatalog()
	center := it.brandCenter("olex")
	dist := it.org("t369c-dist", "distributor", center)
	staff, spw := it.user("t369c-staff")
	it.member(center, staff, "staff")
	distUser, dpw := it.user("t369c-dist")
	it.member(dist, distUser, "owner")
	tok := it.catalogLogin(staff, spw, center, hostOlex)
	distTok := it.catalogLogin(distUser, dpw, dist, hostOlex)
	s := it.suffix

	a := it.catalogCategory(tok, "Aa "+s)
	b := it.catalogCategory(tok, "Bb "+s)
	c := it.catalogCategory(tok, "Cc "+s)
	nA, nB, nC := a.Name, b.Name, c.Name
	base := "/v1/catalog/categories?q=" + url.QueryEscape(s)
	list := func(extra string) []string {
		got, _, _ := it.listNames(base+extra, hostOlex, tok, http.StatusOK, false)
		return got
	}
	wantOrder(t, "sort=name", list("&sort=name"), []string{nA, nB, nC})
	wantOrder(t, "sort=-name", list("&sort=-name"), []string{nC, nB, nA})
	it.listNames(base+"&sort=available_parts", hostOlex, tok, http.StatusBadRequest, false)
	it.listNames(base+"&active=maybe", hostOlex, tok, http.StatusBadRequest, false)

	// Reorder: the full brand list is renumbered; ours come back C, A, B.
	code, env := it.do("PUT", "/v1/catalog/categories/order", hostOlex, tok, map[string]any{"uuids": []string{c.UUID, a.UUID, b.UUID}})
	if code != http.StatusOK {
		t.Fatalf("reorder: %d %s", code, errCode(env))
	}
	wantOrder(t, "sort=sort after reorder", list("&sort=sort"), []string{nC, nA, nB})
	wantOrder(t, "sort=-sort after reorder", list("&sort=-sort"), []string{nB, nA, nC})
	// A subset swaps inside its own slots.
	if code, env := it.do("PUT", "/v1/catalog/categories/order", hostOlex, tok, map[string]any{"uuids": []string{b.UUID, c.UUID}}); code != http.StatusOK {
		t.Fatalf("subset reorder: %d %s", code, errCode(env))
	}
	wantOrder(t, "default after subset", list(""), []string{nB, nA, nC})
	for name, body := range map[string]any{
		"empty":     map[string]any{"uuids": []string{}},
		"duplicate": map[string]any{"uuids": []string{a.UUID, a.UUID}},
		"unknown":   map[string]any{"uuids": []string{"00000000-0000-0000-0000-000000000001"}},
	} {
		if code, env := it.do("PUT", "/v1/catalog/categories/order", hostOlex, tok, body); code != http.StatusBadRequest {
			t.Fatalf("reorder %s: %d %s", name, code, errCode(env))
		}
	}
	if code, _ := it.do("PUT", "/v1/catalog/categories/order", hostOlex, distTok, map[string]any{"uuids": []string{a.UUID}}); code != http.StatusForbidden {
		t.Fatalf("distributor reorder: %d", code)
	}

	// Bulk deactivate + undo; active filter.
	run := it.bulkRun("/v1/catalog/categories/bulk", tok, map[string]any{
		"action": "deactivate", "target": map[string]any{"scope": "ids", "ids": []string{a.UUID, b.UUID}},
	})
	if run.Summary.Succeeded != 2 || run.Operation.UndoStatus != bulkusecase.UndoAvailable {
		t.Fatalf("deactivate run = %+v", run)
	}
	wantOrder(t, "active=false", list("&active=false&sort=name"), []string{nA, nB})
	it.bulkUndo("/v1/tenant/bulk-operations/"+run.Operation.UUID.String()+"/undo", hostOlex, tok, http.StatusOK)
	wantOrder(t, "active=false after undo", list("&active=false"), []string{})

	// Bulk delete: a category with products fails per item.
	it.catalogProduct(tok, map[string]any{"category_uuid": a.UUID, "sku": "CAT-" + s, "name": "In use " + s, "unit_type": "piece"})
	del := it.bulkRun("/v1/catalog/categories/bulk", tok, map[string]any{
		"action": "delete", "target": map[string]any{"scope": "ids", "ids": []string{a.UUID, c.UUID}},
	})
	if del.Summary.Succeeded != 1 || del.Summary.Failed != 1 || del.Operation.UndoStatus == bulkusecase.UndoAvailable {
		t.Fatalf("delete run = %+v", del)
	}
	wantOrder(t, "after delete", list("&sort=name"), []string{nA, nB})
	if code, _ := it.do("POST", "/v1/catalog/categories/bulk", hostOlex, distTok, map[string]any{
		"action": "activate", "target": map[string]any{"scope": "ids", "ids": []string{a.UUID}},
	}); code != http.StatusForbidden {
		t.Fatalf("distributor category bulk: %d", code)
	}
}

func TestIntegrationCatalogListsDistributorPrices(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	d1 := it.org("t369p-alpha", "distributor", center)
	d2 := it.org("t369p-beta", "distributor", center)
	acc, apw := it.user("t369p-acc")
	it.member(center, acc, "staff", rbac.RoleCenterAccounting)
	tok := it.loginOrg(acc, apw, center)
	it.stepUp(acc.Uuid)
	p := it.product(center, "T369P")
	pid := p.Uuid.String()
	for _, set := range []struct {
		org      db.Organization
		currency string
		price    string
	}{{d1, "TRY", "30"}, {d2, "TRY", "10"}, {d1, "EUR", "20"}} {
		code, env := it.do("PUT", "/v1/tenant/pricing/products/"+pid+"/distributor-prices/"+set.org.Uuid.String()+"/"+set.currency,
			hostOlex, tok, map[string]string{"price": set.price})
		if code != http.StatusOK {
			t.Fatalf("distributor price: %d %s", code, errCode(env))
		}
	}
	type row struct {
		Currency        string `json:"currency"`
		Price           string `json:"price"`
		DistributorName string `json:"distributor_name"`
	}
	list := func(extra string, want int) ([]string, int64) {
		code, env := it.do("GET", "/v1/tenant/pricing/distributor-prices?product_uuid="+pid+extra, hostOlex, tok, nil)
		if code != want {
			t.Fatalf("list %s: %d %s", extra, code, errCode(env))
		}
		var page struct {
			Items []row `json:"items"`
			Total int64 `json:"total"`
		}
		_ = json.Unmarshal(env.Data, &page)
		out := []string{}
		for _, r := range page.Items {
			out = append(out, r.Currency+":"+r.Price)
		}
		return out, page.Total
	}
	norm := func(v []string) []string {
		out := make([]string, len(v))
		for i, x := range v {
			cur, amount, _ := strings.Cut(x, ":")
			f, _ := strconv.ParseFloat(amount, 64)
			out[i] = cur + ":" + strconv.FormatFloat(f, 'f', -1, 64)
		}
		return out
	}
	got, _ := list("&sort=price", http.StatusOK)
	wantOrder(t, "sort=price", norm(got), []string{"TRY:10", "EUR:20", "TRY:30"})
	got, _ = list("&sort=-price", http.StatusOK)
	wantOrder(t, "sort=-price", norm(got), []string{"TRY:30", "EUR:20", "TRY:10"})
	got, total := list("&currency=eur", http.StatusOK)
	if total != 1 || len(got) != 1 {
		t.Fatalf("currency filter = %v (%d)", got, total)
	}
	got, _ = list("&currency=TRY,EUR&q="+url.QueryEscape(d2.Name), http.StatusOK)
	wantOrder(t, "q distributor", norm(got), []string{"TRY:10"})
	list("&sort=distributor_uuid", http.StatusBadRequest)
	list("&currency=EURO", http.StatusBadRequest)
}

func TestIntegrationCatalogListsVehicleCatalog(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = it.pool.Exec(ctx, "DELETE FROM car_models WHERE name LIKE '%' || $1", it.suffix)
		_, _ = it.pool.Exec(ctx, "DELETE FROM car_brands WHERE name LIKE '%' || $1", it.suffix)
	})
	admin, apw := it.user("t369v-admin", rbac.RoleSuperAdmin)
	adminTok := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": admin.Email.String, "password": apw,
	})).AccessToken
	center := it.brandCenter("olex")
	staff, spw := it.user("t369v-staff")
	it.member(center, staff, "staff")
	staffTok := it.catalogLogin(staff, spw, center, hostOlex)
	s := it.suffix

	brand := func(name string) listItem {
		code, env := it.do("POST", "/v1/platform/vehicle-catalog/brands", hostOlex, adminTok, map[string]any{"name": name})
		if code != http.StatusCreated {
			t.Fatalf("brand %s: %d %s", name, code, errCode(env))
		}
		var b listItem
		_ = json.Unmarshal(env.Data, &b)
		return b
	}
	model := func(brandUUID, name string, extra map[string]any) listItem {
		body := map[string]any{"brand_uuid": brandUUID, "name": name}
		for k, v := range extra {
			body[k] = v
		}
		code, env := it.do("POST", "/v1/platform/vehicle-catalog/models", hostOlex, adminTok, body)
		if code != http.StatusCreated {
			t.Fatalf("model %s: %d %s", name, code, errCode(env))
		}
		var m listItem
		_ = json.Unmarshal(env.Data, &m)
		return m
	}
	ba, bb := brand("Aaa "+s), brand("Bbb "+s)
	m1 := model(bb.UUID, "M1 "+s, map[string]any{"body_type": "SUV", "powertrain": "Hybrid", "year_start": 2010, "year_stop": 2014})
	m2 := model(bb.UUID, "M2 "+s, map[string]any{"body_type": "Sedan", "powertrain": "Diesel", "year_start": 2018})
	m3 := model(ba.UUID, "M3 "+s, map[string]any{"body_type": "SUV"})
	if _, err := it.pool.Exec(ctx, "UPDATE car_brands SET logo_object_key = 'logos/x.png' WHERE uuid = $1", ba.UUID); err != nil {
		t.Fatal(err)
	}

	bbase := "/v1/vehicle-catalog/brands?q=" + url.QueryEscape(s)
	blist := func(extra string) []string {
		got, _, _ := it.listNames(bbase+extra, hostOlex, adminTok, http.StatusOK, false)
		return got
	}
	wantOrder(t, "brands default", blist(""), []string{ba.Name, bb.Name})
	wantOrder(t, "brands -name", blist("&sort=-name"), []string{bb.Name, ba.Name})
	wantOrder(t, "brands -model_count", blist("&sort=-model_count"), []string{bb.Name, ba.Name})
	wantOrder(t, "brands model_count", blist("&sort=model_count"), []string{ba.Name, bb.Name})
	wantOrder(t, "has_logo", blist("&has_logo=true"), []string{ba.Name})
	wantOrder(t, "no logo", blist("&has_logo=false"), []string{bb.Name})
	it.listNames(bbase+"&sort=logo_height", hostOlex, adminTok, http.StatusBadRequest, false)
	it.listNames(bbase+"&has_logo=yes", hostOlex, adminTok, http.StatusBadRequest, false)

	mbase := "/v1/vehicle-catalog/models?q=" + url.QueryEscape(s)
	mlist := func(extra string) []string {
		got, _, _ := it.listNames(mbase+extra, hostOlex, adminTok, http.StatusOK, false)
		return got
	}
	// Default: brand name, then model name.
	wantOrder(t, "models default", mlist(""), []string{m3.Name, m1.Name, m2.Name})
	wantOrder(t, "models -year_start", mlist("&sort=-year_start"), []string{m2.Name, m1.Name, m3.Name})
	wantOrder(t, "models body_type", mlist("&sort=name&body_type=SUV"), []string{m1.Name, m3.Name})
	wantOrder(t, "models body_type CSV", mlist("&sort=name&body_type=SUV,Sedan"), []string{m1.Name, m2.Name, m3.Name})
	wantOrder(t, "models powertrain", mlist("&powertrain=Diesel"), []string{m2.Name})
	// Year span overlap: m1 2010-2014, m2 2018-open, m3 open.
	wantOrder(t, "models year 2015-2016", mlist("&sort=name&year_min=2015&year_max=2016"), []string{m3.Name})
	wantOrder(t, "models year 2012", mlist("&sort=name&year_min=2012&year_max=2012"), []string{m1.Name, m3.Name})
	for _, bad := range []string{"&sort=external_id", "&year_min=1800", "&year_min=2020&year_max=2010", "&year_max=2020.5"} {
		it.listNames(mbase+bad, hostOlex, adminTok, http.StatusBadRequest, false)
	}

	// Facets inside brand bb.
	code, env := it.do("GET", "/v1/vehicle-catalog/models/facets?brand_uuid="+bb.UUID, hostOlex, adminTok, nil)
	if code != http.StatusOK {
		t.Fatalf("facets: %d %s", code, errCode(env))
	}
	var facets struct {
		BodyType []struct {
			Value string `json:"value"`
			Count int64  `json:"count"`
		} `json:"body_type"`
		Powertrain []struct {
			Value string `json:"value"`
		} `json:"powertrain"`
	}
	_ = json.Unmarshal(env.Data, &facets)
	if len(facets.BodyType) != 2 || facets.BodyType[0].Value != "Sedan" || facets.BodyType[1].Value != "SUV" ||
		facets.BodyType[1].Count != 1 || len(facets.Powertrain) != 2 {
		t.Fatalf("facets = %s", env.Data)
	}

	// Platform bulk deactivate + undo (vehicle_catalog.write only).
	if code, _ := it.do("POST", "/v1/platform/vehicle-catalog/brands/bulk", hostOlex, staffTok, map[string]any{
		"action": "deactivate", "target": map[string]any{"scope": "ids", "ids": []string{ba.UUID}},
	}); code != http.StatusForbidden {
		t.Fatalf("staff vehicle bulk: %d", code)
	}
	run := it.bulkRun("/v1/platform/vehicle-catalog/brands/bulk", adminTok, map[string]any{
		"action": "deactivate", "target": map[string]any{"scope": "ids", "ids": []string{ba.UUID, bb.UUID}},
	})
	if run.Summary.Succeeded != 2 || run.Operation.UndoStatus != bulkusecase.UndoAvailable {
		t.Fatalf("brands deactivate = %+v", run)
	}
	wantOrder(t, "inactive brands", blist("&active=false"), []string{ba.Name, bb.Name})
	it.bulkUndo("/v1/platform/bulk-operations/"+run.Operation.UUID.String()+"/undo", hostOlex, adminTok, http.StatusOK)
	wantOrder(t, "inactive brands after undo", blist("&active=false"), []string{})
	mrun := it.bulkRun("/v1/platform/vehicle-catalog/models/bulk", adminTok, map[string]any{
		"action": "deactivate", "target": map[string]any{"scope": "ids", "ids": []string{m1.UUID}},
	})
	if mrun.Summary.Succeeded != 1 {
		t.Fatalf("models deactivate = %+v", mrun)
	}
	wantOrder(t, "inactive models", mlist("&active=false"), []string{m1.Name})
}

func TestIntegrationCatalogListsServiceCatalogAndContracts(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	staff, spw := it.user("t369s-staff")
	it.member(center, staff, "staff")
	tok := it.loginOrg(staff, spw, center)
	s := it.suffix
	t.Cleanup(func() {
		_, _ = it.pool.Exec(ctx, "DELETE FROM service_catalog_items WHERE name LIKE '%' || $1", s)
		_, _ = it.pool.Exec(ctx, "DELETE FROM contract_templates WHERE name LIKE '%' || $1", s)
	})
	price := pgtype.Numeric{}
	_ = price.Scan("100")
	for _, item := range []struct {
		name, category, recurrence string
		active                     bool
	}{
		{"Ads " + s, "advertising", "monthly", true},
		{"Course " + s, "training", "one_time", true},
		{"Old " + s, "software", "yearly", false},
	} {
		if _, err := it.q.CreateServiceCatalogItem(ctx, db.CreateServiceCatalogItemParams{
			OrganizationID: center.ID, BrandID: center.BrandID, Name: item.name, Description: "desc " + item.name,
			Category: item.category, DefaultPrice: price, Currency: "TRY", Recurrence: item.recurrence,
			CancellationFee: price, IsActive: item.active,
		}); err != nil {
			t.Fatal(err)
		}
	}
	q := "?q=" + url.QueryEscape(s)
	names := func(path string, want int) []string {
		got, _, _ := it.listNames(path, hostOlex, tok, want, false)
		return got
	}
	wantOrder(t, "service q", names("/v1/platform/service-catalog"+q, http.StatusOK), []string{"Ads " + s, "Course " + s, "Old " + s})
	wantOrder(t, "service category CSV", names("/v1/platform/service-catalog"+q+"&category=advertising,software", http.StatusOK), []string{"Ads " + s, "Old " + s})
	wantOrder(t, "service recurrence", names("/v1/platform/service-catalog"+q+"&recurrence=one_time", http.StatusOK), []string{"Course " + s})
	wantOrder(t, "service inactive", names("/v1/platform/service-catalog"+q+"&active=false", http.StatusOK), []string{"Old " + s})
	names("/v1/platform/service-catalog"+q+"&category=food", http.StatusBadRequest)
	names("/v1/platform/service-catalog"+q+"&recurrence=weekly", http.StatusBadRequest)
	// Tenant list: active only, same filters.
	wantOrder(t, "tenant service q", names("/v1/service-catalog"+q, http.StatusOK), []string{"Ads " + s, "Course " + s})
	wantOrder(t, "tenant service category", names("/v1/service-catalog"+q+"&category=training", http.StatusOK), []string{"Course " + s})

	// Contract templates: active / is_default true|false|omitted.
	mk := func(name string, active bool) {
		code, env := it.do("POST", "/v1/platform/contract-templates", hostOlex, tok, map[string]any{
			"name": name, "kind": "service_sale", "is_active": active,
		})
		if code != http.StatusCreated {
			t.Fatalf("template %s: %d %s", name, code, errCode(env))
		}
	}
	mk("On "+s, true)
	mk("Off "+s, false)
	tpl := func(extra string, want int) []string {
		got := names("/v1/platform/contract-templates?kind=service_sale"+extra, want)
		out := []string{}
		for _, n := range got {
			if strings.Contains(n, s) {
				out = append(out, n)
			}
		}
		slices.Sort(out)
		return out
	}
	wantOrder(t, "templates all", tpl("", http.StatusOK), []string{"Off " + s, "On " + s})
	wantOrder(t, "templates active", tpl("&active=true", http.StatusOK), []string{"On " + s})
	wantOrder(t, "templates inactive", tpl("&active=false", http.StatusOK), []string{"Off " + s})
	wantOrder(t, "templates kind CSV", tpl("&kind=vehicle_intake", http.StatusOK), []string{"Off " + s, "On " + s})
	if got := tpl("&active=false&is_default=true", http.StatusOK); len(got) != 0 {
		t.Fatalf("inactive defaults = %v", got)
	}
	names("/v1/platform/contract-templates?active=yes", http.StatusBadRequest)
	names("/v1/platform/contract-templates?is_default=2", http.StatusBadRequest)
	names("/v1/platform/contract-templates?kind=lease", http.StatusBadRequest)
}
