package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

type vcBrand struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	HasLogo bool   `json:"has_logo"`
	LogoURL string `json:"logo_url"`
	HeroURL string `json:"hero_url"`
}

type vcModel struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	HeroURL string `json:"hero_url"`
	Brand   struct {
		UUID string `json:"uuid"`
	} `json:"brand"`
}

// pngBytes is enough of a PNG for content sniffing.
var pngBytes = append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), bytes.Repeat([]byte{1}, 64)...)

const svgBytes = `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`

// raw sends a request with an arbitrary body and headers.
func (it *itest) raw(method, path, bearer, contentType string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	it.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Host = "backend:8080"
	req.Header.Set("X-Forwarded-Host", hostOlex)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	it.handler.ServeHTTP(rec, req)
	return rec
}

// uploadImage PUTs a multipart image as field with a declared content type.
func (it *itest) uploadImage(path, bearer, field, declared string, data []byte) *httptest.ResponseRecorder {
	it.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="`+field+`"; filename="img"`)
	h.Set("Content-Type", declared)
	part, err := mw.CreatePart(h)
	if err != nil {
		it.t.Fatal(err)
	}
	_, _ = part.Write(data)
	_ = mw.Close()
	return it.raw("PUT", path, bearer, mw.FormDataContentType(), buf.Bytes(), nil)
}

func decodeEnv[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	var out T
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("payload: %s", rec.Body.String())
	}
	return out
}

// TEC-149 acceptance: only super_admin writes the vehicle catalog (403 for
// organization roles, which still read it); the public logo URL needs no
// auth and answers 200 + ETag + Cache-Control, 304 on If-None-Match and a
// placeholder without a logo; SVG uploads are refused; hero falls back
// model → brand → default; a brand with models cannot be deleted (409).
func TestIntegrationVehicleCatalog(t *testing.T) {
	store := storage.NewMemory()
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = store })
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = it.pool.Exec(ctx, "DELETE FROM car_models WHERE name LIKE '%' || $1", it.suffix)
		_, _ = it.pool.Exec(ctx, "DELETE FROM car_brands WHERE name LIKE '%' || $1", it.suffix)
	})

	admin, apw := it.user("vc-admin", rbac.RoleSuperAdmin)
	adminTok := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": admin.Email.String, "password": apw,
	})).AccessToken
	center := it.brandCenter("olex")
	dealerOrg := it.org("vc-dealer", "dealer", center)
	staff, spw := it.user("vc-staff")
	it.member(center, staff, "staff")
	dealer, dpw := it.user("vc-dealer")
	it.member(dealerOrg, dealer, "owner")
	staffTok := it.catalogLogin(staff, spw, center, hostOlex)
	dealerTok := it.catalogLogin(dealer, dpw, dealerOrg, hostOlex)

	// super_admin creates a brand and a model.
	code, env := it.do("POST", "/v1/platform/vehicle-catalog/brands", hostOlex, adminTok, map[string]any{
		"name": "BMW " + it.suffix, "external_id": "ext-b-" + it.suffix,
	})
	if code != http.StatusCreated {
		t.Fatalf("create brand: %d %s", code, errCode(env))
	}
	var brand vcBrand
	_ = json.Unmarshal(env.Data, &brand)
	if brand.HasLogo || brand.LogoURL != "/brand-logos/"+brand.UUID {
		t.Fatalf("new brand = %+v", brand)
	}
	code, env = it.do("POST", "/v1/platform/vehicle-catalog/models", hostOlex, adminTok, map[string]any{
		"brand_uuid": brand.UUID, "name": "X5 " + it.suffix, "body_type": "SUV",
		"powertrain": "Hybrid", "year_start": 2018, "year_stop": 2023,
	})
	if code != http.StatusCreated {
		t.Fatalf("create model: %d %s", code, errCode(env))
	}
	var mdl vcModel
	_ = json.Unmarshal(env.Data, &mdl)
	if mdl.Brand.UUID != brand.UUID || mdl.HeroURL != "/vehicle-heroes/default" {
		t.Fatalf("new model = %+v", mdl)
	}
	if code, env := it.do("POST", "/v1/platform/vehicle-catalog/models", hostOlex, adminTok, map[string]any{
		"brand_uuid": brand.UUID, "name": "x5 " + it.suffix,
	}); code != http.StatusConflict {
		t.Fatalf("duplicate model name: %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/platform/vehicle-catalog/models", hostOlex, adminTok, map[string]any{
		"brand_uuid": brand.UUID, "name": "Bad " + it.suffix, "year_start": 2020, "year_stop": 2010,
	}); code != http.StatusUnprocessableEntity {
		t.Fatalf("bad year range: %d %s", code, errCode(env))
	}

	// Organization roles read but never write.
	for name, tok := range map[string]string{"center_staff": staffTok, "dealer_owner": dealerTok} {
		code, env := it.do("GET", "/v1/vehicle-catalog/models?q="+it.suffix, hostOlex, tok, nil)
		if code != http.StatusOK || !strings.Contains(string(env.Data), mdl.UUID) {
			t.Fatalf("%s search models: %d %s", name, code, errCode(env))
		}
		writes := []struct {
			method, path string
			body         any
		}{
			{"POST", "/v1/platform/vehicle-catalog/brands", map[string]any{"name": "Hack " + it.suffix}},
			{"PATCH", "/v1/platform/vehicle-catalog/brands/" + brand.UUID, map[string]any{"name": "Hack " + it.suffix}},
			{"DELETE", "/v1/platform/vehicle-catalog/brands/" + brand.UUID, nil},
			{"POST", "/v1/platform/vehicle-catalog/models", map[string]any{"brand_uuid": brand.UUID, "name": "Hack " + it.suffix}},
			{"PATCH", "/v1/platform/vehicle-catalog/models/" + mdl.UUID, map[string]any{"name": "Hack " + it.suffix}},
			{"DELETE", "/v1/platform/vehicle-catalog/models/" + mdl.UUID, nil},
		}
		for _, w := range writes {
			if code, env := it.do(w.method, w.path, hostOlex, tok, w.body); code != http.StatusForbidden {
				t.Errorf("%s %s %s: %d %s, want 403", name, w.method, w.path, code, errCode(env))
			}
		}
		if rec := it.uploadImage("/v1/platform/vehicle-catalog/brands/"+brand.UUID+"/logo", tok, "logo", "image/png", pngBytes); rec.Code != http.StatusForbidden {
			t.Errorf("%s logo upload: %d, want 403", name, rec.Code)
		}
	}

	// No logo yet: the public URL serves the placeholder without auth.
	logoPath := "/v1/public/brand-logos/" + brand.UUID
	rec := it.raw("GET", logoPath, "", "", nil, nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/svg+xml" || rec.Header().Get("ETag") == "" {
		t.Fatalf("placeholder: %d %v", rec.Code, rec.Header())
	}

	// SVG is refused even when declared as PNG.
	for _, declared := range []string{"image/svg+xml", "image/png"} {
		if rec := it.uploadImage("/v1/platform/vehicle-catalog/brands/"+brand.UUID+"/logo", adminTok, "logo", declared, []byte(svgBytes)); rec.Code < 400 || rec.Code >= 500 {
			t.Fatalf("svg upload declared %s: %d, want 4xx", declared, rec.Code)
		}
	}

	// PNG logo: public 200 + ETag + Cache-Control, then 304.
	rec = it.uploadImage("/v1/platform/vehicle-catalog/brands/"+brand.UUID+"/logo", adminTok, "logo", "image/png", pngBytes)
	if rec.Code != http.StatusOK {
		t.Fatalf("png upload: %d %s", rec.Code, rec.Body.String())
	}
	if b := decodeEnv[vcBrand](t, rec); !b.HasLogo || !strings.HasPrefix(b.LogoURL, "/brand-logos/"+brand.UUID+"?v=") {
		t.Fatalf("brand after upload = %+v", b)
	}
	rec = it.raw("GET", logoPath, "", "", nil, nil)
	etag := rec.Header().Get("ETag")
	if rec.Code != http.StatusOK || etag == "" || rec.Header().Get("Cache-Control") != "public, max-age=86400" ||
		rec.Header().Get("Content-Type") != "image/png" || !bytes.Equal(rec.Body.Bytes(), pngBytes) {
		t.Fatalf("public logo: %d %v", rec.Code, rec.Header())
	}
	rec = it.raw("GET", logoPath, "", "", nil, map[string]string{"If-None-Match": etag})
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 || rec.Header().Get("ETag") != etag {
		t.Fatalf("if-none-match: %d len=%d", rec.Code, rec.Body.Len())
	}
	// A new upload changes the ETag; the old tag no longer matches.
	if rec := it.uploadImage("/v1/platform/vehicle-catalog/brands/"+brand.UUID+"/logo", adminTok, "logo", "image/png", append(pngBytes, 2)); rec.Code != http.StatusOK {
		t.Fatalf("second upload: %d", rec.Code)
	}
	rec = it.raw("GET", logoPath, "", "", nil, map[string]string{"If-None-Match": etag})
	if rec.Code != http.StatusOK || rec.Header().Get("ETag") == etag {
		t.Fatalf("stale etag after re-upload: %d %s", rec.Code, rec.Header().Get("ETag"))
	}
	if code, _ := it.do("GET", "/v1/public/brand-logos/00000000-0000-0000-0000-000000000000", hostOlex, "", nil); code != http.StatusNotFound {
		t.Fatalf("unknown brand logo: %d", code)
	}

	// Hero: default → brand hero (inherited by the model) → model hero.
	heroPath := "/v1/public/vehicle-heroes/models/" + mdl.UUID
	if rec := it.raw("GET", heroPath, "", "", nil, nil); rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("default hero: %d %v", rec.Code, rec.Header())
	}
	brandHero := append([]byte(nil), pngBytes...)
	brandHero = append(brandHero, 'b')
	if rec := it.uploadImage("/v1/platform/vehicle-catalog/brands/"+brand.UUID+"/hero", adminTok, "hero", "image/png", brandHero); rec.Code != http.StatusOK {
		t.Fatalf("brand hero upload: %d %s", rec.Code, rec.Body.String())
	}
	if rec := it.raw("GET", heroPath, "", "", nil, nil); rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), brandHero) {
		t.Fatalf("model hero from brand: %d", rec.Code)
	}
	modelHero := append([]byte(nil), pngBytes...)
	modelHero = append(modelHero, 'm')
	if rec := it.uploadImage("/v1/platform/vehicle-catalog/models/"+mdl.UUID+"/hero", adminTok, "hero", "image/png", modelHero); rec.Code != http.StatusOK {
		t.Fatalf("model hero upload: %d %s", rec.Code, rec.Body.String())
	}
	if rec := it.raw("GET", heroPath, "", "", nil, nil); rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), modelHero) {
		t.Fatalf("model hero: %d", rec.Code)
	}

	// A brand with models answers 409; after the model it deletes.
	if code, env := it.do("DELETE", "/v1/platform/vehicle-catalog/brands/"+brand.UUID, hostOlex, adminTok, nil); code != http.StatusConflict {
		t.Fatalf("delete brand with models: %d %s", code, errCode(env))
	}
	if code, env := it.do("DELETE", "/v1/platform/vehicle-catalog/models/"+mdl.UUID, hostOlex, adminTok, nil); code != http.StatusOK {
		t.Fatalf("delete model: %d %s", code, errCode(env))
	}
	if code, env := it.do("DELETE", "/v1/platform/vehicle-catalog/brands/"+brand.UUID, hostOlex, adminTok, nil); code != http.StatusOK {
		t.Fatalf("delete brand: %d %s", code, errCode(env))
	}
	objects, err := store.List(context.Background(), storage.ListObjectsInput{Prefix: "vehicle-"})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects.Objects) != 0 {
		t.Fatalf("images left in storage: %+v", objects.Objects)
	}
}
