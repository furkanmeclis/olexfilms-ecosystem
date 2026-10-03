package legacymobile

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// okLoader accepts every valid token (the identity is the token's user).
type okLoader struct{}

func (okLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, err := c.UserUUID()
	if err != nil {
		return authctx.Principal{}, err
	}
	org, _ := c.OrganizationUUID()
	return authctx.Principal{UserID: id, SessionID: c.SessionUUID(), OrganizationUUID: org, Realm: c.Realm()}, nil
}

func newTokens(t *testing.T) *jwt.Manager {
	t.Helper()
	tokens, err := jwt.NewManager("test-secret-test-secret-test-secret", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tokens
}

// serve runs one adapter behind the old envelope, as RegisterRoutes mounts
// it (without the gates).
func serve(name string, fn http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	envelope(name, fn).ServeHTTP(rec, req)
	return rec
}

func check(t *testing.T, name string, rec *httptest.ResponseRecorder) LegacyBody {
	t.Helper()
	f, err := LoadFixture(FixtureDir, name, testVars)
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.Check(rec.Code, rec.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func fixtureReq(t *testing.T, name string) *http.Request {
	t.Helper()
	f, err := LoadFixture(FixtureDir, name, testVars)
	if err != nil {
		t.Fatal(err)
	}
	return fixtureRequest(f, "t")
}

type sampleOrg struct {
	UUID   uuid.UUID `json:"uuid"`
	Slug   string    `json:"slug"`
	Name   string    `json:"name"`
	Role   string    `json:"role"`
	Status string    `json:"status"`
	Type   string    `json:"type"`
}

func sampleMe(user uuid.UUID, org sampleOrg, active bool) map[string]any {
	me := map[string]any{
		"user": map[string]any{
			"uuid": user, "email": "legacy@example.com", "name": "Ali", "surname": "Veli",
			"is_super_admin": false,
		},
		"organizations": []sampleOrg{org}, "effective_locale": "zh-CN",
		"roles": []string{}, "permissions": []string{},
	}
	if active {
		me["active_organization_uuid"] = org.UUID
	}
	return me
}

// Login: the old body (email, password, device_name) becomes the new
// login with a device; the session gets the only membership through the
// organization switch, and the answer is {token, token_type, user} in the
// old envelope with the hub's roles.
func TestLoginAdapter(t *testing.T) {
	tokens := newTokens(t)
	user := uuid.New()
	org := sampleOrg{UUID: uuid.New(), Slug: "bayi-1", Name: "Olex İstanbul", Role: "owner", Status: "active", Type: "dealer"}
	first, _, _ := tokens.IssueAccess(jwt.AccessInput{UserID: user, Audience: jwt.AudienceMobile})
	second, _, _ := tokens.IssueAccess(jwt.AccessInput{UserID: user, OrganizationID: &org.UUID, Audience: jwt.AudienceMobile})
	var loginBody, switchBody map[string]any
	h := Handlers{
		Login: func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&loginBody)
			response.JSON(w, r, http.StatusOK, map[string]any{"access_token": first, "me": sampleMe(user, org, false)})
		},
		SwitchOrganization: func(w http.ResponseWriter, r *http.Request) {
			if p := authctx.MustPrincipal(r.Context()); p.UserID != user {
				t.Errorf("switch principal = %v", p.UserID)
			}
			_ = json.NewDecoder(r.Body).Decode(&switchBody)
			response.JSON(w, r, http.StatusOK, map[string]any{"access_token": second})
		},
		Me: func(w http.ResponseWriter, r *http.Request) {
			if p := authctx.MustPrincipal(r.Context()); p.OrganizationUUID == nil || *p.OrganizationUUID != org.UUID {
				t.Errorf("me is not on the switched token")
			}
			response.JSON(w, r, http.StatusOK, sampleMe(user, org, true))
		},
	}
	a := &adapters{h: h, authn: authnFor(tokens)}
	req := fixtureReq(t, "login")
	req.Header.Set("User-Agent", "Olex/1.9 CFNetwork/1410 Darwin/22.6.0")
	rec := serve("login", a.login, req)
	b := check(t, "login", rec)

	dev, _ := loginBody["device"].(map[string]any)
	if loginBody["email"] != "legacy@example.com" || loginBody["password"] != "x" || dev["name"] != "Legacy Phone" ||
		dev["platform"] != "ios" || !strings.HasPrefix(dev["id"].(string), "legacy-") {
		t.Fatalf("new login body = %v", loginBody)
	}
	if switchBody["organization_slug"] != "bayi-1" {
		t.Fatalf("switch body = %v", switchBody)
	}
	var data struct {
		Token     string     `json:"token"`
		TokenType string     `json:"token_type"`
		User      legacyUser `json:"user"`
	}
	if err := json.Unmarshal(b.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Token != second || data.TokenType != "Bearer" || data.User.Name != "Ali Veli" || data.User.Locale != "zh_CN" ||
		len(data.User.Roles) != 1 || data.User.Roles[0] != roleDealerOwner || *data.User.Dealer.Name != org.Name ||
		!*data.User.Dealer.IsActive || data.User.UUID != user {
		t.Fatalf("login data = %+v", data)
	}
}

func authnFor(tokens *jwt.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			c, err := tokens.ParseAccess(raw)
			if err != nil {
				response.Unauthorized(w, r, "")
				return
			}
			p, _ := okLoader{}.LoadPrincipal(r, c)
			next.ServeHTTP(w, r.WithContext(authctx.WithPrincipal(r.Context(), p)))
		})
	}
}

// Login errors keep the old contract: wrong credentials are 422
// errors.email, a missing field is 422 on that field.
func TestLoginErrors(t *testing.T) {
	a := &adapters{h: Handlers{Login: func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, r, http.StatusUnauthorized, response.CodeInvalidCredentials, "Email or password is incorrect")
	}}}
	rec := serve("login", a.login, fixtureReq(t, "login"))
	e := decodeErr(t, rec)
	if rec.Code != http.StatusUnprocessableEntity || *e.Message != "Girdiğiniz bilgiler kayıtlarımızla eşleşmiyor." ||
		len(e.Errors["email"]) != 1 {
		t.Fatalf("wrong credentials = %d %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest(http.MethodPost, Prefix+"/auth/login?locale=en", strings.NewReader(`{"email":"a@b.c"}`))
	rec = serve("login", a.login, req)
	e = decodeErr(t, rec)
	if rec.Code != http.StatusUnprocessableEntity || e.Errors["password"][0] != "This field is required." {
		t.Fatalf("missing password = %d %s", rec.Code, rec.Body.String())
	}
}

// me answers UserResource; a center member is center_staff with the dealer
// key present and null (the hub always loaded the relation).
func TestMeAdapter(t *testing.T) {
	user := uuid.New()
	org := sampleOrg{UUID: uuid.New(), Slug: "merkez", Name: "Merkez", Role: "staff", Status: "active", Type: "center"}
	a := &adapters{h: Handlers{Me: func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, r, http.StatusOK, sampleMe(user, org, true))
	}}}
	b := check(t, "me", serve("me", a.me, fixtureReq(t, "me")))
	var u legacyUser
	if err := json.Unmarshal(b.Data, &u); err != nil {
		t.Fatal(err)
	}
	if len(u.Roles) != 1 || u.Roles[0] != roleCenterStaff || u.DealerID != nil || u.Dealer.ID != nil || u.Email == nil {
		t.Fatalf("me = %s", b.Data)
	}
}

func sampleService() svcuc.ServiceView {
	plate, year := "34ABC123", int16(2022)
	now := time.Date(2026, 3, 10, 14, 30, 0, 0, time.UTC)
	return svcuc.ServiceView{
		UUID: uuid.New(), ServiceNo: "OLX-1", Status: svcuc.StatusCompleted, StatusLabel: "Tamamlandı",
		Organization: svcuc.OrgRef{UUID: uuid.New(), Name: "Olex İstanbul", Type: "dealer"},
		Customer:     svcuc.CustomerRef{UUID: uuid.New(), Name: "Ayşe", Surname: "Kaya"},
		CarBrand:     svcuc.Ref{UUID: uuid.New(), Name: "BMW"}, CarModel: svcuc.Ref{UUID: uuid.New(), Name: "320i"},
		ModelYear: &year, Plate: &plate, CompletedAt: &now, CreatedAt: now, UpdatedAt: now,
	}
}

// The list: the old query (search, page, per_page) becomes q/limit/offset,
// the answer is the Laravel paginator {data, links, meta}.
func TestListServicesAdapter(t *testing.T) {
	var got url.Values
	a := &adapters{h: Handlers{ListServices: func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		response.JSON(w, r, http.StatusOK, apiquery.NewPage([]svcuc.ServiceView{sampleService()}, 41, 20, 20))
	}}}
	b := check(t, "services_list", serve("services_list", a.listServices, fixtureReq(t, "services_list")))
	if got.Get("limit") != "20" || got.Get("offset") != "0" {
		t.Fatalf("new query = %v", got)
	}
	req := httptest.NewRequest(http.MethodGet, Prefix+"/services?page=2&search=34ABC&status=completed", nil)
	req.Host = "api.example.com"
	rec := serve("services_list", a.listServices, req)
	b = check(t, "services_list", rec)
	if got.Get("offset") != "20" || got.Get("q") != "34ABC" || got.Get("status") != "completed" {
		t.Fatalf("new query = %v", got)
	}
	var pg legacyPage
	if err := json.Unmarshal(b.Data, &pg); err != nil {
		t.Fatal(err)
	}
	s := pg.Data[0]
	if pg.Meta.CurrentPage != 2 || pg.Meta.LastPage != 3 || pg.Meta.Total != 41 || *pg.Meta.From != 21 ||
		pg.Links.Prev == nil || pg.Links.Next == nil || !strings.Contains(*pg.Links.Next, "page=3") ||
		!strings.HasPrefix(pg.Meta.Path, "http://api.example.com"+Prefix+"/services") ||
		s.Customer.Name == nil || *s.Customer.Name != "Ayşe Kaya" || *s.Year != 2022 || s.CompletedAt == nil ||
		!s.CanSendReviewRequest || s.ItemsCount == nil || s.Items != nil {
		t.Fatalf("page = %s", b.Data)
	}
	// per_page outside 1..100 is the old 422.
	rec = serve("services_list", a.listServices, httptest.NewRequest(http.MethodGet, Prefix+"/services?per_page=500", nil))
	if e := decodeErr(t, rec); rec.Code != http.StatusUnprocessableEntity || len(e.Errors["per_page"]) != 1 {
		t.Fatalf("per_page = %d %s", rec.Code, rec.Body.String())
	}
}

// The detail: {service} may be a uuid; the answer is MobileServiceResource
// with items, images and warranties. An id that cannot be resolved, or a
// 404 of the adapted handler, is the old 404 envelope.
func TestGetServiceAdapter(t *testing.T) {
	v := sampleService()
	start := time.Now().Add(-24 * time.Hour)
	itemUUID := uuid.New()
	v.Items = []svcuc.ItemView{{UUID: itemUUID, Product: svcuc.ProductRef{SKU: "PPF-1", Name: "PPF"}, Barcode: "OLX-00000001", Kind: svcuc.KindFull}}
	v.Images = []svcuc.ImageView{{UUID: uuid.New(), SortOrder: 2, URL: "/b"}, {UUID: uuid.New(), SortOrder: 1, URL: "/a"}}
	v.Warranties = []svcuc.WarrantyView{{UUID: uuid.New(), ServiceItemUUID: itemUUID, ProductName: "PPF", Status: "active",
		StartAt: start, EndAt: start.Add(365 * 24 * time.Hour)}}
	var gotUUID string
	a := &adapters{h: Handlers{GetService: func(w http.ResponseWriter, r *http.Request) {
		gotUUID = r.PathValue("uuid")
		if gotUUID != v.UUID.String() {
			response.NotFound(w, r, "Service not found")
			return
		}
		response.JSON(w, r, http.StatusOK, v)
	}}}
	req := fixtureReq(t, "service_detail")
	req.SetPathValue("service", v.UUID.String())
	b := check(t, "service_detail", serve("service_detail", a.getService, req))
	var s legacyService
	if err := json.Unmarshal(b.Data, &s); err != nil {
		t.Fatal(err)
	}
	if gotUUID != v.UUID.String() || len(*s.Items) != 1 || (*s.Items)[0].UsageTypeLabel != "Tamamı" ||
		*(*s.Items)[0].StockItem.Barcode != "OLX-00000001" || (*s.Images)[0].URL != "/a" ||
		len(*s.Warranties) != 1 || !(*s.Warranties)[0].IsActive || (*s.Warranties)[0].TotalDays != 365 ||
		!*s.HasWarrantyRecords || s.ItemsCount != nil {
		t.Fatalf("detail = %s", b.Data)
	}
	for _, id := range []string{"42", uuid.NewString()} {
		req := fixtureReq(t, "service_detail")
		req.SetPathValue("service", id)
		rec := serve("service_detail", a.getService, req)
		if e := decodeErr(t, rec); rec.Code != http.StatusNotFound || e.Code != response.CodeNotFound {
			t.Fatalf("service %s = %d %s", id, rec.Code, rec.Body.String())
		}
	}
}

// The NexPTG report upload becomes a measurement upload: the old body is
// kept whole as raw, vin / date / serial fill the columns, the answer is
// 201 with the report fields echoed.
func TestStoreReportAdapter(t *testing.T) {
	var got map[string]json.RawMessage
	id := uuid.New()
	a := &adapters{h: Handlers{CreateMeasurement: func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = nil
		_ = json.Unmarshal(b, &got)
		response.JSON(w, r, http.StatusAccepted, map[string]any{"uuid": id, "status": "accepted"})
	}}}
	b := check(t, "measurement", serve("measurement", a.storeReport, fixtureReq(t, "measurement")))
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(got["raw"], &raw)
	if string(got["vin"]) != `"WBA1234567890ABCD"` || string(got["measured_at"]) != `"2026-03-10T14:30:00Z"` ||
		string(got["device"]) != `{"serial":"NEXPTG-001"}` || !strings.HasPrefix(string(got["client_measurement_id"]), `"legacy-`) ||
		string(raw["body_type"]) != `"sedan"` || len(raw["measurements"]) == 0 {
		t.Fatalf("new body = %v", got)
	}
	var data map[string]json.RawMessage
	_ = json.Unmarshal(b.Data, &data)
	if string(data["uuid"]) != `"`+id.String()+`"` || string(data["measurements_count"]) != "1" ||
		string(data["is_matched"]) != "false" || string(data["name"]) != `"BMW 320i Ölçüm"` {
		t.Fatalf("report = %s", b.Data)
	}
	// A VIN the new column refuses is left out (vin_pending), the upload
	// still succeeds; no measurement is the old 422.
	req := httptest.NewRequest(http.MethodPost, Prefix+"/nexptg-reports",
		strings.NewReader(`{"vin":"abc","date":"2026-03-10 14:30:00","measurements":[{"part_type":"HOOD"}]}`))
	rec := serve("measurement", a.storeReport, req)
	if _, has := got["vin"]; rec.Code != http.StatusCreated || has || string(got["measured_at"]) != `"2026-03-10T14:30:00Z"` {
		t.Fatalf("invalid vin = %d %v", rec.Code, got)
	}
	rec = serve("measurement", a.storeReport, httptest.NewRequest(http.MethodPost, Prefix+"/nexptg-reports", strings.NewReader(`{"measurements":[]}`)))
	if e := decodeErr(t, rec); rec.Code != http.StatusUnprocessableEntity || len(e.Errors["measurements"]) != 1 {
		t.Fatalf("no measurements = %d %s", rec.Code, rec.Body.String())
	}
}

// Push token: the old body passes through, the answer is data null with
// the hub's message; the hub's 422 rules hold.
func TestPushTokenAdapters(t *testing.T) {
	var put, del map[string]string
	a := &adapters{h: Handlers{
		PutPushToken: func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&put)
			response.JSON(w, r, http.StatusOK, map[string]string{"uuid": uuid.NewString()})
		},
		DeletePushToken: func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&del)
			response.JSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
		},
	}}
	check(t, "push_token", serve("push_token", a.putPushToken, fixtureReq(t, "push_token")))
	if put["expo_push_token"] != "ExponentPushToken[x]" || put["platform"] != "android" || put["device_name"] != "Legacy Phone" {
		t.Fatalf("put body = %v", put)
	}
	check(t, "push_token_delete", serve("push_token_delete", a.deletePushToken, fixtureReq(t, "push_token_delete")))
	if del["expo_push_token"] != "ExponentPushToken[x]" {
		t.Fatalf("delete body = %v", del)
	}
	for body, field := range map[string]string{
		`{"platform":"ios"}`:                      "expo_push_token",
		`{"expo_push_token":"x"}`:                 "platform",
		`{"expo_push_token":"x","platform":"wp"}`: "platform",
	} {
		rec := serve("push_token", a.putPushToken, httptest.NewRequest(http.MethodPut, Prefix+"/push-token", strings.NewReader(body)))
		if e := decodeErr(t, rec); rec.Code != http.StatusUnprocessableEntity || len(e.Errors[field]) != 1 {
			t.Fatalf("%s = %d %s", body, rec.Code, rec.Body.String())
		}
	}
}

// A new 400 VALIDATION_ERROR is the old 422 with errors per field.
func TestMapError(t *testing.T) {
	status, e := mapError("push_token", "tr", http.StatusBadRequest, response.ErrorBody{
		Code: response.CodeValidationError, Message: "bad",
		Details: []response.Detail{{Field: "expo_push_token", Message: "invalid"}, {Message: "x"}},
	})
	if status != http.StatusUnprocessableEntity || e.Errors["expo_push_token"][0] != "invalid" || e.Errors["body"][0] != "x" {
		t.Fatalf("validation = %d %+v", status, e)
	}
	status, e = mapError("me", "tr", http.StatusForbidden, response.ErrorBody{Code: response.CodeForbidden, Message: "no"})
	if status != http.StatusForbidden || e.Message != "no" || e.Code != response.CodeForbidden {
		t.Fatalf("forbidden = %d %+v", status, e)
	}
	status, e = mapError("login", "en", http.StatusForbidden, response.ErrorBody{Code: response.CodeForbidden, Message: "User is disabled"})
	if status != http.StatusForbidden || !strings.HasPrefix(e.Message, "Your account") {
		t.Fatalf("disabled = %d %+v", status, e)
	}
}

func TestPickOrganizationAndRoles(t *testing.T) {
	dealer := newOrg{Slug: "d", Type: "dealer", Status: "active", Role: "staff"}
	center := newOrg{Slug: "c", Type: "center", Status: "active", Role: "owner"}
	suspended := newOrg{Slug: "s", Type: "dealer", Status: "suspended"}
	if pickOrganization([]newOrg{suspended}) != "s" || pickOrganization([]newOrg{dealer, center}) != "c" ||
		pickOrganization([]newOrg{suspended, dealer}) != "d" || pickOrganization(nil) != "" {
		t.Fatal("pickOrganization")
	}
	var m newMe
	m.Organizations = []newOrg{dealer}
	m.ActiveOrganization = &m.Organizations[0].UUID
	if r := legacyRoles(m); r[0] != roleDealerStaff {
		t.Fatalf("roles = %v", r)
	}
	m.User.IsSuperAdmin = true
	if r := legacyRoles(m); r[0] != roleSuperAdmin {
		t.Fatalf("roles = %v", r)
	}
	if platformFromUA("okhttp/4.9") != "android" || platformFromUA("Olex/1 (iPhone; iOS 17)") != "ios" {
		t.Fatal("platformFromUA")
	}
}
