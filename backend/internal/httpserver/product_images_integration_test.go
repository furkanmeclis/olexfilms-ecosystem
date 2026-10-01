package httpserver

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
)

type productWithImages struct {
	UUID   string `json:"uuid"`
	Images []struct {
		Key  string `json:"key"`
		Sort int    `json:"sort"`
	} `json:"images"`
}

// postProductImage POSTs a multipart "image" part with a declared type.
func (it *itest) postProductImage(productUUID, bearer, declared string, data []byte) *httptest.ResponseRecorder {
	it.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="image"; filename="img"`)
	h.Set("Content-Type", declared)
	part, err := mw.CreatePart(h)
	if err != nil {
		it.t.Fatal(err)
	}
	_, _ = part.Write(data)
	_ = mw.Close()
	return it.raw("POST", "/v1/catalog/products/"+productUUID+"/images", bearer, mw.FormDataContentType(), buf.Bytes(), nil)
}

// TEC-152 acceptance: the center uploads a product image (201) that the
// public URL serves with ETag + Cache-Control (304 on If-None-Match); SVG is
// 400, a dealer/distributor 403, > 5 MiB 413 and the 11th image 422; delete
// and reorder work and an inactive product's images are not served.
func TestIntegrationProductImages(t *testing.T) {
	store := storage.NewMemory()
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = store })
	it.cleanupCatalog()
	olexCenter := it.brandCenter("olex")
	dist := it.org("img-dist", "distributor", olexCenter)
	dealerOrg := it.org("img-dealer", "dealer", dist)
	centerUser, centerPW := it.user("img-center")
	it.member(olexCenter, centerUser, "staff")
	distUser, distPW := it.user("img-dist")
	it.member(dist, distUser, "owner")
	dealerUser, dealerPW := it.user("img-dealer")
	it.member(dealerOrg, dealerUser, "owner")
	center := it.catalogLogin(centerUser, centerPW, olexCenter, hostOlex)
	distTok := it.catalogLogin(distUser, distPW, dist, hostOlex)
	dealerTok := it.catalogLogin(dealerUser, dealerPW, dealerOrg, hostOlex)

	product := it.createCatalogProduct(center, hostOlex, "IMG-"+it.suffix)

	// Center uploads a PNG: 201 with the new image on the product.
	rec := it.postProductImage(product.UUID, center, "image/png", pngBytes)
	if rec.Code != http.StatusCreated {
		t.Fatalf("center upload: %d %s", rec.Code, rec.Body.String())
	}
	got := decodeEnv[productWithImages](t, rec)
	if len(got.Images) != 1 || got.Images[0].Sort != 0 {
		t.Fatalf("after upload = %+v", got)
	}
	first := got.Images[0].Key

	// The product detail lists it.
	rec = it.raw("GET", "/v1/catalog/products/"+product.UUID, center, "", nil, nil)
	if detail := decodeEnv[productWithImages](t, rec); len(detail.Images) != 1 || detail.Images[0].Key != first {
		t.Fatalf("detail images = %+v", detail.Images)
	}

	// Public GET without auth: 200 + ETag + Cache-Control, then 304.
	publicPath := "/v1/public/product-images/" + first
	rec = it.raw("GET", publicPath, "", "", nil, nil)
	etag := rec.Header().Get("ETag")
	if rec.Code != http.StatusOK || etag == "" || rec.Header().Get("Content-Type") != "image/png" ||
		rec.Header().Get("Cache-Control") == "" || !bytes.Equal(rec.Body.Bytes(), pngBytes) {
		t.Fatalf("public image: %d %v", rec.Code, rec.Header())
	}
	rec = it.raw("GET", publicPath, "", "", nil, map[string]string{"If-None-Match": etag})
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatalf("conditional public image: %d", rec.Code)
	}
	if rec := it.raw("GET", "/v1/public/product-images/"+uuid.NewString(), "", "", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown key: %d", rec.Code)
	}

	// SVG (declared as SVG or disguised as PNG): 400.
	for _, declared := range []string{"image/svg+xml", "image/png"} {
		if rec := it.postProductImage(product.UUID, center, declared, []byte(svgBytes)); rec.Code != http.StatusBadRequest {
			t.Fatalf("svg upload declared %s: %d, want 400", declared, rec.Code)
		}
	}
	// Larger than 5 MiB: 413.
	big := append(append([]byte{}, pngBytes...), bytes.Repeat([]byte{0}, storage.MaxProductImageBytes)...)
	if rec := it.postProductImage(product.UUID, center, "image/png", big); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized upload: %d, want 413", rec.Code)
	}

	// Distributor and dealer: 403 on every image write.
	for name, tok := range map[string]string{"distributor": distTok, "dealer": dealerTok} {
		if rec := it.postProductImage(product.UUID, tok, "image/png", pngBytes); rec.Code != http.StatusForbidden {
			t.Errorf("%s upload: %d, want 403", name, rec.Code)
		}
		if code, _ := it.do("DELETE", "/v1/catalog/products/"+product.UUID+"/images/"+first, hostOlex, tok, nil); code != http.StatusForbidden {
			t.Errorf("%s delete image: %d, want 403", name, code)
		}
		if code, _ := it.do("PUT", "/v1/catalog/products/"+product.UUID+"/images/order", hostOlex, tok,
			map[string]any{"keys": []string{first}}); code != http.StatusForbidden {
			t.Errorf("%s reorder: %d, want 403", name, code)
		}
	}

	// Fill up to 10; the 11th is 422.
	for i := 1; i < 10; i++ {
		if rec := it.postProductImage(product.UUID, center, "image/png", pngBytes); rec.Code != http.StatusCreated {
			t.Fatalf("upload %d: %d %s", i+1, rec.Code, rec.Body.String())
		}
	}
	rec = it.postProductImage(product.UUID, center, "image/png", pngBytes)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("11th upload: %d, want 422", rec.Code)
	}

	// Reorder: move the first image last.
	rec = it.raw("GET", "/v1/catalog/products/"+product.UUID, center, "", nil, nil)
	full := decodeEnv[productWithImages](t, rec)
	keys := make([]string, 0, len(full.Images))
	for _, img := range full.Images[1:] {
		keys = append(keys, img.Key)
	}
	keys = append(keys, first)
	code, env := it.do("PUT", "/v1/catalog/products/"+product.UUID+"/images/order", hostOlex, center, map[string]any{"keys": keys})
	if code != http.StatusOK {
		t.Fatalf("reorder: %d %s", code, errCode(env))
	}
	if code, _ := it.do("PUT", "/v1/catalog/products/"+product.UUID+"/images/order", hostOlex, center,
		map[string]any{"keys": keys[:3]}); code != http.StatusUnprocessableEntity {
		t.Fatalf("partial reorder: %d, want 422", code)
	}

	// Delete removes the image and its object; the public URL is 404.
	code, env = it.do("DELETE", "/v1/catalog/products/"+product.UUID+"/images/"+first, hostOlex, center, nil)
	if code != http.StatusOK {
		t.Fatalf("delete image: %d %s", code, errCode(env))
	}
	if rec := it.raw("GET", publicPath, "", "", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("deleted image public: %d, want 404", rec.Code)
	}
	if ok, _ := store.Exists(context.Background(), storage.ProductImageObjectKey(uuid.MustParse(product.UUID), first)); ok {
		t.Fatal("deleted image object still in storage")
	}

	// An inactive product's images are not served.
	second := keys[0]
	if rec := it.raw("GET", "/v1/public/product-images/"+second, "", "", nil, nil); rec.Code != http.StatusOK {
		t.Fatalf("active product image: %d", rec.Code)
	}
	if code, env := it.do("PATCH", "/v1/catalog/products/"+product.UUID, hostOlex, center, map[string]any{"active": false}); code != http.StatusOK {
		t.Fatalf("deactivate: %d %s", code, errCode(env))
	}
	if rec := it.raw("GET", "/v1/public/product-images/"+second, "", "", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("inactive product image: %d, want 404", rec.Code)
	}

	// Deleting the product removes its objects.
	if code, env := it.do("DELETE", "/v1/catalog/products/"+product.UUID, hostOlex, center, nil); code != http.StatusOK {
		t.Fatalf("delete product: %d %s", code, errCode(env))
	}
	objects, err := store.List(context.Background(), storage.ListObjectsInput{Prefix: "products/" + product.UUID + "/"})
	if err != nil || len(objects.Objects) != 0 {
		t.Fatalf("objects left after product delete: %v %+v", err, objects.Objects)
	}
}
