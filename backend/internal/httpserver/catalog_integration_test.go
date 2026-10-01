package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

type catalogItem struct {
	UUID     string `json:"uuid"`
	SKU      string `json:"sku"`
	Name     string `json:"name"`
	Active   bool   `json:"active"`
	Category struct {
		UUID string `json:"uuid"`
	} `json:"category"`
}

type catalogPage struct {
	Items []catalogItem `json:"items"`
	Total int64         `json:"total"`
}

// catalogLogin signs u in to org on host and returns the access token.
func (it *itest) catalogLogin(u db.User, pw string, org db.Organization, host string) string {
	it.t.Helper()
	return it.tokensFrom(it.do("POST", "/v1/auth/login", host, "", map[string]string{
		"email": u.Email.String, "password": pw, "organization_slug": org.Slug,
	})).AccessToken
}

func (it *itest) cleanupCatalog() {
	it.t.Cleanup(func() {
		ctx := context.Background()
		_, _ = it.pool.Exec(ctx, "DELETE FROM products WHERE sku LIKE '%' || $1", it.suffix)
		_, _ = it.pool.Exec(ctx, "DELETE FROM product_categories WHERE name LIKE '%' || $1", it.suffix)
	})
}

// createCatalogProduct opens a category and a product as the center.
func (it *itest) createCatalogProduct(token, host, sku string) catalogItem {
	it.t.Helper()
	code, env := it.do("POST", "/v1/catalog/categories", host, token, map[string]any{
		"name": "PPF " + sku, "available_parts": []string{"hood", "roof"},
	})
	if code != http.StatusCreated {
		it.t.Fatalf("create category on %s: %d %s", host, code, errCode(env))
	}
	var cat catalogItem
	_ = json.Unmarshal(env.Data, &cat)
	code, env = it.do("POST", "/v1/catalog/products", host, token, map[string]any{
		"category_uuid": cat.UUID, "sku": sku, "name": "Film " + sku,
		"unit_type": "roll_meter", "warranty_duration_months": 120, "micron_thickness": 190,
	})
	if code != http.StatusCreated {
		it.t.Fatalf("create product on %s: %d %s", host, code, errCode(env))
	}
	var p catalogItem
	_ = json.Unmarshal(env.Data, &p)
	if p.SKU != sku || p.Category.UUID != cat.UUID {
		it.t.Fatalf("created product = %+v", p)
	}
	return p
}

func (it *itest) listSKUs(path, host, token string) map[string]bool {
	t := it.t
	t.Helper()
	code, env := it.do("GET", path, host, token, nil)
	if code != http.StatusOK {
		t.Fatalf("list products: %d %s", code, errCode(env))
	}
	var page catalogPage
	if err := json.Unmarshal(env.Data, &page); err != nil {
		t.Fatalf("list payload: %s", env.Data)
	}
	out := map[string]bool{}
	for _, p := range page.Items {
		out[p.SKU] = true
	}
	return out
}

// TEC-145 acceptance: the center opens and reads a product; distributor and
// dealer read it but every write is 403 (K4); an Olex organization never
// lists a Glorian product (K1/K20).
func TestIntegrationCatalog(t *testing.T) {
	it := newIntegration(t)
	it.cleanupCatalog()
	olexCenter := it.brandCenter("olex")
	glorianCenter := it.brandCenter("glorian")
	dist := it.org("cat-dist", "distributor", olexCenter)
	dealer := it.org("cat-dealer", "dealer", dist)

	centerUser, centerPW := it.user("cat-center")
	it.member(olexCenter, centerUser, "staff")
	glorianUser, glorianPW := it.user("cat-glorian")
	it.member(glorianCenter, glorianUser, "staff")
	distUser, distPW := it.user("cat-dist")
	it.member(dist, distUser, "owner")
	dealerUser, dealerPW := it.user("cat-dealer")
	it.member(dealer, dealerUser, "owner")
	accUser, accPW := it.user("cat-acc")
	it.member(olexCenter, accUser, "staff", rbac.RoleCenterAccounting)

	center := it.catalogLogin(centerUser, centerPW, olexCenter, hostOlex)
	glorian := it.catalogLogin(glorianUser, glorianPW, glorianCenter, hostGlorian)
	distTok := it.catalogLogin(distUser, distPW, dist, hostOlex)
	dealerTok := it.catalogLogin(dealerUser, dealerPW, dealer, hostOlex)
	accTok := it.catalogLogin(accUser, accPW, olexCenter, hostOlex)

	// Center opens and reads a product.
	olexSKU := "OLX-" + it.suffix
	product := it.createCatalogProduct(center, hostOlex, olexSKU)
	code, env := it.do("GET", "/v1/catalog/products/"+product.UUID, hostOlex, center, nil)
	if code != http.StatusOK {
		t.Fatalf("center get product: %d %s", code, errCode(env))
	}
	var got catalogItem
	_ = json.Unmarshal(env.Data, &got)
	if got.SKU != olexSKU || !got.Active {
		t.Fatalf("center read product = %+v", got)
	}
	if code, env := it.do("POST", "/v1/catalog/products", hostOlex, center, map[string]any{
		"category_uuid": product.Category.UUID, "sku": olexSKU, "name": "dup",
	}); code != http.StatusConflict {
		t.Fatalf("duplicate sku: %d %s", code, errCode(env))
	}

	// Glorian product of the Glorian center.
	glorianSKU := "GLR-" + it.suffix
	glorianProduct := it.createCatalogProduct(glorian, hostGlorian, glorianSKU)

	// Distributor and dealer read the Olex catalog only.
	for name, tok := range map[string]string{"center": center, "distributor": distTok, "dealer": dealerTok} {
		skus := it.listSKUs("/v1/catalog/products?limit=100&q="+it.suffix, hostOlex, tok)
		if !skus[olexSKU] {
			t.Errorf("%s does not list the Olex product", name)
		}
		if skus[glorianSKU] {
			t.Errorf("%s lists the Glorian product on the Olex domain", name)
		}
		if code, _ := it.do("GET", "/v1/catalog/products/"+glorianProduct.UUID, hostOlex, tok, nil); code != http.StatusNotFound {
			t.Errorf("%s get Glorian product on Olex: %d, want 404", name, code)
		}
		if code, _ := it.do("GET", "/v1/catalog/categories/"+glorianProduct.Category.UUID, hostOlex, tok, nil); code != http.StatusNotFound {
			t.Errorf("%s get Glorian category on Olex: %d, want 404", name, code)
		}
	}
	// And the Glorian center does not see the Olex product.
	if skus := it.listSKUs("/v1/catalog/products?limit=100&q="+it.suffix, hostGlorian, glorian); skus[olexSKU] || !skus[glorianSKU] {
		t.Fatalf("glorian list = %v", skus)
	}

	// Distributor, dealer and a center role without catalog.write: 403.
	writes := []struct {
		method, path string
		body         any
	}{
		{"POST", "/v1/catalog/categories", map[string]any{"name": "X " + it.suffix}},
		{"PATCH", "/v1/catalog/categories/" + product.Category.UUID, map[string]any{"name": "Y " + it.suffix}},
		{"DELETE", "/v1/catalog/categories/" + product.Category.UUID, nil},
		{"POST", "/v1/catalog/products", map[string]any{"category_uuid": product.Category.UUID, "sku": "Z-" + it.suffix, "name": "Z"}},
		{"PATCH", "/v1/catalog/products/" + product.UUID, map[string]any{"name": "hacked"}},
		{"DELETE", "/v1/catalog/products/" + product.UUID, nil},
		{"POST", "/v1/catalog/products/bulk-active", map[string]any{"uuids": []string{product.UUID}, "active": false}},
	}
	for name, tok := range map[string]string{"distributor": distTok, "dealer": dealerTok, "center_accounting": accTok} {
		for _, w := range writes {
			if code, env := it.do(w.method, w.path, hostOlex, tok, w.body); code != http.StatusForbidden {
				t.Errorf("%s %s %s: %d %s, want 403", name, w.method, w.path, code, errCode(env))
			}
		}
	}

	// Center bulk deactivates; a Glorian uuid in the same request is ignored.
	code, env = it.do("POST", "/v1/catalog/products/bulk-active", hostOlex, center, map[string]any{
		"uuids": []string{product.UUID, glorianProduct.UUID}, "active": false,
	})
	if code != http.StatusOK {
		t.Fatalf("bulk deactivate: %d %s", code, errCode(env))
	}
	var bulk struct {
		Updated int `json:"updated"`
	}
	_ = json.Unmarshal(env.Data, &bulk)
	if bulk.Updated != 1 {
		t.Fatalf("bulk updated = %d, want 1", bulk.Updated)
	}
	if skus := it.listSKUs("/v1/catalog/products?active=true&q="+it.suffix, hostOlex, distTok); skus[olexSKU] {
		t.Fatal("deactivated product still listed as active")
	}
	code, env = it.do("GET", "/v1/catalog/products/"+glorianProduct.UUID, hostGlorian, glorian, nil)
	_ = json.Unmarshal(env.Data, &got)
	if code != http.StatusOK || !got.Active {
		t.Fatalf("glorian product touched by an Olex bulk: %d active=%v", code, got.Active)
	}

	// Center patches and clears an optional field with an explicit null.
	code, env = it.do("PATCH", "/v1/catalog/products/"+product.UUID, hostOlex, center, map[string]any{
		"name": "Renamed " + it.suffix, "warranty_duration_months": nil, "active": true,
	})
	if code != http.StatusOK {
		t.Fatalf("center patch: %d %s", code, errCode(env))
	}
	var patched struct {
		Name     string `json:"name"`
		Warranty *int   `json:"warranty_duration_months"`
	}
	_ = json.Unmarshal(env.Data, &patched)
	if patched.Name != "Renamed "+it.suffix || patched.Warranty != nil {
		t.Fatalf("patched = %+v", patched)
	}

	// A category with products cannot be deleted.
	if code, env := it.do("DELETE", "/v1/catalog/categories/"+product.Category.UUID, hostOlex, center, nil); code != http.StatusConflict {
		t.Fatalf("delete used category: %d %s", code, errCode(env))
	}
	if code, env := it.do("DELETE", "/v1/catalog/products/"+product.UUID, hostOlex, center, nil); code != http.StatusOK {
		t.Fatalf("center delete product: %d %s", code, errCode(env))
	}
	if code, env := it.do("DELETE", "/v1/catalog/categories/"+product.Category.UUID, hostOlex, center, nil); code != http.StatusOK {
		t.Fatalf("center delete category: %d %s", code, errCode(env))
	}

	// Validation.
	if code, env := it.do("POST", "/v1/catalog/products", hostOlex, center, map[string]any{
		"sku": " ", "name": "x", "unit_type": "box",
	}); code != http.StatusUnprocessableEntity || !strings.Contains(string(env.Data)+errCode(env), "VALIDATION_ERROR") {
		t.Fatalf("invalid product: %d %s", code, errCode(env))
	}
}
