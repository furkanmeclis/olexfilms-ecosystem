package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// postPhoto POSTs a multipart showcase photo with a declared content type.
func (it *itest) postPhoto(path, bearer, declared string, data []byte) *httptest.ResponseRecorder {
	it.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="photo"; filename="file"`)
	h.Set("Content-Type", declared)
	part, err := mw.CreatePart(h)
	if err != nil {
		it.t.Fatal(err)
	}
	_, _ = part.Write(data)
	_ = mw.WriteField("caption", `{"tr":"Atölye"}`)
	_ = mw.Close()
	return it.raw("POST", path, bearer, mw.FormDataContentType(), buf.Bytes(), nil)
}

// TEC-467 (F5-01b): the showcase panel API behind RequireFeature, photo
// sniffing, the center review queue and the public dealer extension.
func TestIntegrationDealerShowcaseAPI(t *testing.T) {
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = storage.NewMemory() })
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("scdist", "distributor", center)
	dealer := it.org("scdealer", "dealer", dist)
	for _, o := range []db.Organization{dist, dealer} {
		id := o.ID
		t.Cleanup(func() {
			_, _ = it.pool.Exec(context.Background(), "DELETE FROM dealer_showcases WHERE organization_id = $1", id)
			_, _ = it.pool.Exec(context.Background(), "DELETE FROM module_flags WHERE organization_id = $1", id)
		})
	}
	if _, err := it.pool.Exec(ctx, `UPDATE organizations SET latitude = 39.92, longitude = 32.85, phone = '+905321112233',
		city = 'Ankara' WHERE id = $1`, dealer.ID); err != nil {
		t.Fatal(err)
	}
	owner, pw := it.user("scowner")
	it.member(dealer, owner, "owner")
	reviewer, rpw := it.user("screviewer")
	it.member(center, reviewer, "staff", "center_staff")
	ownerTok := it.loginOrg(owner, pw, dealer)
	reviewerTok := it.loginOrg(reviewer, rpw, center)

	public := func() (int, string) {
		t.Helper()
		rec := it.raw("GET", "/v1/public/dealers/"+dealer.Slug, "", "", nil, nil)
		var env envelope
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		return rec.Code, string(env.Data)
	}
	code, skeleton := public()
	if code != http.StatusOK || strings.Contains(skeleton, "showcase") {
		t.Fatalf("skeleton: %d %s", code, skeleton)
	}

	// Module off: every panel route is 403 FEATURE_DISABLED.
	for _, r := range [][2]string{{"GET", "/v1/showcase"}, {"PUT", "/v1/showcase"}, {"POST", "/v1/showcase/submit"},
		{"GET", "/v1/showcase/services"}, {"POST", "/v1/showcase/photos"}} {
		if code, env := it.do(r[0], r[1], hostOlex, ownerTok, map[string]any{}); code != http.StatusForbidden || errCode(env) != "FEATURE_DISABLED" {
			t.Fatalf("%s %s with module off: %d %s", r[0], r[1], code, errCode(env))
		}
	}

	if _, err := it.srv.features.SetByAdmin(ctx, reviewer.ID, dealer.ID, features.ModuleDealerShowcase, true); err != nil {
		t.Fatal(err)
	}
	if code, env := it.do("PUT", "/v1/showcase", hostOlex, ownerTok, map[string]any{
		"content":       map[string]any{"tr": map[string]string{"headline": "Ankara PPF", "about": "Hakkımızda"}},
		"working_hours": map[string]any{"monday": []map[string]string{{"start": "09:00", "end": "18:00"}}},
		"social_links":  map[string]string{"instagram": "https://instagram.com/scdealer"},
		"seo_keywords":  []string{"ankara ppf"},
	}); code != http.StatusOK {
		t.Fatalf("save: %d %s", code, errCode(env))
	}
	if code, env := it.do("PUT", "/v1/showcase", hostOlex, ownerTok, map[string]any{"price": 1}); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("unknown field: %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/showcase/services", hostOlex, ownerTok, map[string]any{
		"kind": "custom", "title": map[string]string{"tr": "Seramik kaplama"},
	}); code != http.StatusCreated {
		t.Fatalf("service: %d %s", code, errCode(env))
	}
	// Byte sniffing: a PDF declared as JPEG is 415; a PNG declared as text is accepted.
	if rec := it.postPhoto("/v1/showcase/photos", ownerTok, "image/jpeg", []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n")); rec.Code != http.StatusUnsupportedMediaType ||
		!strings.Contains(rec.Body.String(), "UNSUPPORTED_MEDIA_TYPE") {
		t.Fatalf("pdf upload: %d %s", rec.Code, rec.Body.String())
	}
	if rec := it.postPhoto("/v1/showcase/photos", ownerTok, "image/png", bytes.Repeat([]byte{0xff}, 5<<20+10)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("6 MB upload: %d %s", rec.Code, rec.Body.String())
	}
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{1}, 64)...)
	rec := it.postPhoto("/v1/showcase/photos", ownerTok, "text/plain", png)
	if rec.Code != http.StatusCreated {
		t.Fatalf("png upload: %d %s", rec.Code, rec.Body.String())
	}
	photo := decodeEnv[struct {
		UUID string `json:"uuid"`
		Mime string `json:"mime"`
	}](t, rec)
	if photo.Mime != "image/png" {
		t.Fatalf("photo mime = %s", photo.Mime)
	}
	if rec := it.postPhoto("/v1/showcase/photos", ownerTok, "image/png", png); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate photo: %d", rec.Code)
	}
	if rec := it.raw("GET", "/v1/showcase/photos/"+photo.UUID+"/file", ownerTok, "", nil, nil); rec.Code != http.StatusOK ||
		rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("panel photo: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}

	// Saved but not published: the public body is still the skeleton.
	if code, body := public(); code != http.StatusOK || body != skeleton {
		t.Fatalf("draft leaks to public:\n%s\n%s", skeleton, body)
	}
	if code, env := it.do("POST", "/v1/showcase/submit", hostOlex, ownerTok, nil); code != http.StatusOK ||
		!strings.Contains(string(env.Data), `"status":"published"`) {
		t.Fatalf("submit: %d %s %s", code, errCode(env), env.Data)
	}
	code, body := public()
	var withShowcase struct {
		Showcase struct {
			Headline string `json:"headline"`
			Photos   []struct {
				URL string `json:"url"`
			} `json:"photos"`
			Services        []map[string]any `json:"services"`
			LeadFormEnabled bool             `json:"lead_form_enabled"`
			WhatsAppChatURL string           `json:"whatsapp_chat_url"`
		} `json:"showcase"`
	}
	if err := json.Unmarshal([]byte(body), &withShowcase); code != http.StatusOK || err != nil ||
		withShowcase.Showcase.Headline != "Ankara PPF" || len(withShowcase.Showcase.Photos) != 1 || len(withShowcase.Showcase.Services) != 1 {
		t.Fatalf("published public: %d %s", code, body)
	}
	if rec := it.raw("GET", withShowcase.Showcase.Photos[0].URL, "", "", nil, nil); rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("public photo: %d", rec.Code)
	}
	if rec := it.raw("GET", "/v1/public/dealers/"+dealer.Slug+"/photos/"+dealer.Uuid.String(), "", "", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown public photo: %d", rec.Code)
	}
	// The organizations index document carries has_showcase; the sitemap
	// list dates the page at least to the publish.
	if row, err := it.q.GetOrganizationForIndex(ctx, dealer.Uuid); err != nil || !row.HasShowcase {
		t.Fatalf("index row has_showcase: %v %v", row.HasShowcase, err)
	}
	var publishedAt time.Time
	if err := it.pool.QueryRow(ctx, "SELECT published_at FROM dealer_showcases WHERE organization_id = $1", dealer.ID).Scan(&publishedAt); err != nil {
		t.Fatal(err)
	}
	if code, env := it.do("GET", "/v1/public/dealers", hostOlex, "", nil); code != http.StatusOK {
		t.Fatalf("codes: %d", code)
	} else {
		var codes struct {
			Items []struct {
				Code      string    `json:"code"`
				UpdatedAt time.Time `json:"updated_at"`
			} `json:"items"`
		}
		_ = json.Unmarshal(env.Data, &codes)
		found := false
		for _, c := range codes.Items {
			if c.Code == dealer.Slug {
				found = !c.UpdatedAt.Before(publishedAt.UTC())
			}
		}
		if !found {
			t.Fatalf("sitemap code missing or older than the publish: %s", env.Data)
		}
	}
	// Nearby carries has_showcase.
	if code, env := it.do("GET", "/v1/public/dealers/nearby?lat=39.92&lng=32.85&radius_km=5", hostOlex, "", nil); code != http.StatusOK ||
		!strings.Contains(string(env.Data), `"slug":"`+dealer.Slug+`"`) || !strings.Contains(string(env.Data), `"has_showcase":true`) {
		t.Fatalf("nearby: %d %s", code, env.Data)
	}

	// Module switched off again: the public body is the skeleton, byte for byte.
	if _, err := it.srv.features.SetByAdmin(ctx, reviewer.ID, dealer.ID, features.ModuleDealerShowcase, false); err != nil {
		t.Fatal(err)
	}
	if code, body := public(); code != http.StatusOK || body != skeleton {
		t.Fatalf("module off public differs from the skeleton:\n%s\n%s", skeleton, body)
	}
	if rec := it.raw("GET", withShowcase.Showcase.Photos[0].URL, "", "", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("public photo with module off: %d", rec.Code)
	}

	// Center review queue: list contract and a rejection without a note.
	if _, err := it.pool.Exec(ctx, `UPDATE dealer_showcases SET status = 'pending_review', submitted_at = NOW()
		WHERE organization_id = $1`, dealer.ID); err != nil {
		t.Fatal(err)
	}
	if code, env := it.do("GET", "/v1/platform/showcases?status=pending_review,published&sort=-updated_at&q="+url.QueryEscape(dealer.Name), hostOlex, reviewerTok, nil); code != http.StatusOK ||
		!strings.Contains(string(env.Data), dealer.Uuid.String()) || !strings.Contains(string(env.Data), `"total":1`) {
		t.Fatalf("queue: %d %s %s", code, errCode(env), env.Data)
	}
	for _, q := range []string{"sort=organization_id", "status=archived"} {
		if code, env := it.do("GET", "/v1/platform/showcases?"+q, hostOlex, reviewerTok, nil); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
			t.Fatalf("%s: %d %s", q, code, errCode(env))
		}
	}
	if code, env := it.do("GET", "/v1/platform/showcases", hostOlex, ownerTok, nil); code != http.StatusForbidden {
		t.Fatalf("dealer owner on the queue: %d %s", code, errCode(env))
	}
	path := "/v1/platform/showcases/" + dealer.Uuid.String() + "/review"
	if code, env := it.do("POST", path, hostOlex, reviewerTok, map[string]string{"decision": "reject"}); code != http.StatusUnprocessableEntity ||
		errCode(env) != "SHOWCASE_REVIEW_NOTE_REQUIRED" {
		t.Fatalf("reject without note: %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", path, hostOlex, reviewerTok, map[string]string{"decision": "reject", "note": "Logo eksik"}); code != http.StatusOK ||
		!strings.Contains(string(env.Data), `"status":"rejected"`) {
		t.Fatalf("reject: %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", path, hostOlex, reviewerTok, map[string]string{"decision": "approve"}); code != http.StatusConflict ||
		errCode(env) != "INVALID_TRANSITION" {
		t.Fatalf("approve rejected: %d %s", code, errCode(env))
	}
}
