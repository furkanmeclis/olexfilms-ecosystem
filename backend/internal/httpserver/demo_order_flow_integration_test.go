package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// fakeExpo records the messages posted to the Expo push API.
type fakeExpo struct {
	mu   sync.Mutex
	msgs []map[string]any
}

func (f *fakeExpo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var batch []map[string]any
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.msgs = append(f.msgs, batch...)
	f.mu.Unlock()
	tickets := make([]map[string]string, len(batch))
	for i := range batch {
		tickets[i] = map[string]string{"status": "ok", "id": fmt.Sprintf("ticket-%d", i)}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": tickets})
}

// sentTo lists the push messages sent to token.
func (f *fakeExpo) sentTo(token string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []map[string]any
	for _, m := range f.msgs {
		if m["to"] == token {
			out = append(out, m)
		}
	}
	return out
}

// Demo order flow end to end (center Olex Merkez, warehouse Merkez Depo):
//
//  1. The center owner opens the accounts: a warehouse worker
//     (center_warehouse, member of the primary warehouse) and a new dealer
//     with its owner. Both sign in on the phone with a WhatsApp OTP and
//     register an Expo push token.
//  2. The center owner opens an order for the dealer: it starts approved in
//     the primary warehouse; the worker is told in-app and by Expo push,
//     the dealer in-app and by Expo push.
//  3. The worker (mobile session) starts preparing, assigns the unit by its
//     barcode, marks it ready and ships it with a delivery method and a
//     tracking number; the dealer is told.
//  4. The dealer (mobile session) receives it: the center owner is told and
//     the order is booked (seller income on the dealer's cari, buyer
//     expense on the center's cari).
func TestIntegrationDemoOrderFlow(t *testing.T) {
	fw := &fakeWuzapi{}
	wsrv := httptest.NewServer(fw)
	t.Cleanup(wsrv.Close)
	expo := &fakeExpo{}
	esrv := httptest.NewServer(expo)
	t.Cleanup(esrv.Close)
	bus := events.NewBus(nil)
	it := newIntegrationWithDeps(t, func(c *config.Config) {
		c.Wuzapi = config.WuzapiConfig{URL: wsrv.URL, AdminToken: itWuzapiAdmin, WebhookSecret: itWebhookKey, WebhookURL: itWebhookURL}
		c.Notify.ExpoPushURL = esrv.URL
	}, func(d *Deps) { d.Events = bus })
	it.lockWhatsAppSettings()
	ctx := context.Background()
	if err := notifusecase.SyncCatalog(ctx, it.q); err != nil {
		t.Fatalf("notification catalog: %v", err)
	}

	center := it.brandCenter("olex")
	brand, err := it.q.GetBrandByID(ctx, center.BrandID)
	if err != nil {
		t.Fatal(err)
	}
	cur := strings.TrimSpace(brand.Currency)
	if cur != "TRY" {
		today := time.Now().UTC().Truncate(24 * time.Hour)
		if err := it.q.UpsertExchangeRate(ctx, db.UpsertExchangeRateParams{
			RateDate: pgtype.Date{Time: today, Valid: true}, Base: cur, Quote: "TRY", Rate: "35", Source: "manual",
		}); err != nil {
			t.Fatalf("rate: %v", err)
		}
	}

	// The center's primary warehouse "Merkez Depo" with one bin (the
	// previous primary comes back after the test).
	var prevPrimary pgtype.Int8
	_ = it.pool.QueryRow(ctx, `SELECT id FROM warehouses WHERE organization_id = $1 AND is_primary`, center.ID).Scan(&prevPrimary)
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `UPDATE warehouses SET is_primary = FALSE WHERE organization_id = $1`, center.ID)
		if prevPrimary.Valid {
			_, _ = it.pool.Exec(context.Background(), `UPDATE warehouses SET is_primary = TRUE WHERE id = $1`, prevPrimary.Int64)
		}
	})
	code := fmt.Sprintf("AFM%d", time.Now().UnixNano()%1_000_000_000)
	wh, err := it.q.CreateWarehouse(ctx, db.CreateWarehouseParams{
		OrganizationID: center.ID, Code: code, Name: "Merkez Depo " + it.suffix, Active: true,
	})
	if err != nil {
		t.Fatalf("warehouse: %v", err)
	}
	room, err := it.q.CreateRoom(ctx, db.CreateRoomParams{OrganizationID: center.ID, WarehouseID: wh.ID, Code: "R1", Name: "Oda", Active: true})
	if err != nil {
		t.Fatalf("room: %v", err)
	}
	bin, err := it.q.CreateTypedLocation(ctx, db.CreateTypedLocationParams{
		OrganizationID: center.ID, RoomID: pgtype.Int8{Int64: room.ID, Valid: true}, Type: pgtype.Text{String: "aisle", Valid: true},
		Code: "A1", Name: "Koridor", Active: true,
	})
	if err != nil {
		t.Fatalf("location: %v", err)
	}
	if _, err := it.pool.Exec(ctx, `UPDATE warehouses SET is_primary = (id = $2) WHERE organization_id = $1`, center.ID, wh.ID); err != nil {
		t.Fatal(err)
	}

	// The center owner (Zorbey) signs in on the panel.
	zorbey, zpw := it.user("t-demo-zorbey")
	it.member(center, zorbey, "owner")
	zTok := it.loginOrg(zorbey, zpw, center)

	// 1a. The warehouse worker: center_warehouse, member of the warehouse.
	workerPhone := itPhone()
	code1, env := it.do("POST", "/v1/tenant/members", hostOlex, zTok, map[string]any{
		"name": "Depo", "surname": "Çalışanı", "phone": workerPhone,
		"role_slugs": []string{"center_warehouse"}, "warehouse_uuids": []string{wh.Uuid.String()},
	})
	if code1 != http.StatusCreated {
		t.Fatalf("create worker = %d %s %s", code1, errCode(env), env.Data)
	}
	var worker struct {
		UserUUID   string   `json:"user_uuid"`
		Phone      string   `json:"phone"`
		RoleSlugs  []string `json:"role_slugs"`
		Warehouses []string `json:"warehouse_uuids"`
		Existing   bool     `json:"existing"`
	}
	if err := json.Unmarshal(env.Data, &worker); err != nil {
		t.Fatal(err)
	}
	if worker.Phone != workerPhone || worker.Existing || len(worker.Warehouses) != 1 || worker.Warehouses[0] != wh.Uuid.String() ||
		strings.Join(worker.RoleSlugs, ",") != "center_warehouse" {
		t.Fatalf("worker = %+v", worker)
	}
	workerUser, err := it.q.GetUserByUUID(ctx, uuid.MustParse(worker.UserUUID))
	if err != nil {
		t.Fatal(err)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM warehouse_members WHERE warehouse_id = $1 AND user_id = $2`, wh.ID, workerUser.ID); n != 1 {
		t.Fatalf("warehouse memberships = %d", n)
	}
	// The same person again is a conflict, an owner role is refused.
	if c, env := it.do("POST", "/v1/tenant/members", hostOlex, zTok, map[string]any{
		"name": "Depo", "phone": workerPhone, "role_slugs": []string{"center_warehouse"},
	}); c != http.StatusConflict {
		t.Fatalf("duplicate member = %d %s", c, errCode(env))
	}
	if c, env := it.do("POST", "/v1/tenant/members", hostOlex, zTok, map[string]any{
		"name": "Patron", "phone": itPhone(), "role_slugs": []string{"center_owner"},
	}); c != http.StatusBadRequest {
		t.Fatalf("owner role = %d %s", c, errCode(env))
	}

	// 1b. The dealer with its owner, directly under the center.
	dealerPhone := itPhone()
	code1, env = it.do("POST", "/v1/tenant/dealers", hostOlex, zTok, map[string]any{
		"name": "Demo Bayi " + it.suffix, "city": "İstanbul",
		"owner": map[string]any{"name": "Bayi", "surname": "Sahibi", "phone": dealerPhone},
	})
	if code1 != http.StatusCreated {
		t.Fatalf("create dealer = %d %s %s", code1, errCode(env), env.Data)
	}
	var dealerRes struct {
		Organization struct {
			UUID   string `json:"uuid"`
			Slug   string `json:"slug"`
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"organization"`
		Owner struct {
			UserUUID  string   `json:"user_uuid"`
			RoleSlugs []string `json:"role_slugs"`
		} `json:"owner"`
	}
	if err := json.Unmarshal(env.Data, &dealerRes); err != nil {
		t.Fatal(err)
	}
	dealer, err := it.q.GetOrganizationByUUID(ctx, uuid.MustParse(dealerRes.Organization.UUID))
	if err != nil {
		t.Fatal(err)
	}
	if dealer.Type != "dealer" || dealer.Status != "active" || !dealer.ParentID.Valid || dealer.ParentID.Int64 != center.ID ||
		dealer.AccessEndsAt.Valid || strings.Join(dealerRes.Owner.RoleSlugs, ",") != "dealer_owner" {
		t.Fatalf("dealer = %+v owner %+v", dealer, dealerRes.Owner)
	}
	dealerOwner, err := it.q.GetUserByUUID(ctx, uuid.MustParse(dealerRes.Owner.UserUUID))
	if err != nil {
		t.Fatal(err)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM organization_member_roles mr JOIN organization_members om ON om.id = mr.member_id
		JOIN roles r ON r.id = mr.role_id WHERE om.organization_id = $1 AND om.user_id = $2 AND r.slug = 'dealer_owner'`,
		dealer.ID, dealerOwner.ID); n != 1 {
		t.Fatalf("dealer_owner memberships = %d", n)
	}
	// A worker cannot open accounts.
	// (checked below with the worker's mobile session)

	// 1c. Phone OTP sign-in on the mobile app, the organization picked as
	// the app does, and the Expo push token registered.
	mobileSignIn := func(phone, orgSlug, device, pushToken string) string {
		t.Helper()
		status, env, _ := it.doMobile("POST", "/v1/mobile/auth/identify", "", "1", map[string]string{"identifier": phone})
		if status != http.StatusOK || !strings.Contains(string(env.Data), `"next":"otp"`) {
			t.Fatalf("identify %s: %d %s", phone, status, errCode(env))
		}
		otp, _ := sentCode(t, fw, phone)
		status, env, _ = it.doMobile("POST", "/v1/mobile/auth/otp/verify", "", "1", map[string]any{
			"phone": phone, "code": otp, "device": map[string]string{"id": device, "platform": "ios", "name": "iPhone"},
		})
		tp := it.tokensFrom(status, env)
		if !strings.Contains(string(env.Data), `"session_kind":"staff"`) {
			t.Fatalf("otp session %s: %s", phone, env.Data)
		}
		status, env, _ = it.doMobile("POST", "/v1/mobile/auth/organization-context", tp.AccessToken, "1",
			map[string]string{"organization_slug": orgSlug})
		tp = it.tokensFrom(status, env)
		status, env, _ = it.doMobile("PUT", "/v1/mobile/push-token", tp.AccessToken, "1", map[string]string{
			"expo_push_token": pushToken, "platform": "ios",
		})
		if status != http.StatusOK {
			t.Fatalf("push token %s: %d %s", phone, status, errCode(env))
		}
		return tp.AccessToken
	}
	workerPush := "ExponentPushToken[demo-worker-" + it.suffix + "]"
	dealerPush := "ExponentPushToken[demo-dealer-" + it.suffix + "]"
	zorbeyPush := "ExponentPushToken[demo-zorbey-" + it.suffix + "]"
	wTok := mobileSignIn(workerPhone, center.Slug, "demo-worker-"+it.suffix, workerPush)
	dTok := mobileSignIn(dealerPhone, dealer.Slug, "demo-dealer-"+it.suffix, dealerPush)
	if _, err := it.pool.Exec(ctx, `INSERT INTO device_push_tokens (user_id, expo_token, device_id, platform)
		VALUES ($1, $2, $3, 'ios')`, zorbey.ID, zorbeyPush, "demo-zorbey-"+it.suffix); err != nil {
		t.Fatalf("zorbey push token: %v", err)
	}
	mobile := func(method, path, tok string, body any) (int, envelope) {
		t.Helper()
		status, env, _ := it.doMobile(method, "/v1/mobile/panel"+strings.TrimPrefix(path, "/v1"), tok, "1", body)
		return status, env
	}
	if c, env := mobile("POST", "/v1/tenant/members", wTok, map[string]any{"name": "X", "phone": itPhone()}); c != http.StatusForbidden && c != http.StatusNotFound {
		t.Fatalf("worker opens accounts = %d %s", c, errCode(env))
	}

	// Stock: one serial unit in the warehouse bin.
	p := it.product(center, "TDEMO")
	it.setListPrice(p, cur, "100")
	chain := it.stockChain()
	unit := chain.unit(center, p, 20261)
	chain.post(ledger.TypeEntry, unit, chain.nextRef(), ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: bin.ID, OrgID: center.ID})

	// publish hands the order's outbox events to the in-process bus (what
	// the outbox publisher of the API/worker does), so the notification
	// handlers write the in-app rows and send the pushes.
	published := map[int64]bool{}
	publish := func(orderUUID string) {
		t.Helper()
		rows, err := it.pool.Query(ctx, `SELECT id, event_name, payload FROM outbox_events
			WHERE payload->'data'->>'order_uuid' = $1 ORDER BY id`, orderUUID)
		if err != nil {
			t.Fatal(err)
		}
		type row struct {
			id   int64
			name string
			raw  []byte
		}
		var list []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.name, &r.raw); err != nil {
				t.Fatal(err)
			}
			list = append(list, r)
		}
		rows.Close()
		for _, r := range list {
			if published[r.id] {
				continue
			}
			published[r.id] = true
			var env struct {
				EventID     uuid.UUID      `json:"event_id"`
				ActorUserID *int64         `json:"actor_user_id"`
				EntityType  string         `json:"entity_type"`
				Data        map[string]any `json:"data"`
			}
			if err := json.Unmarshal(r.raw, &env); err != nil {
				t.Fatal(err)
			}
			ev := events.Event{Name: r.name, ActorUserID: env.ActorUserID, EntityType: env.EntityType,
				Payload: env.Data, EventID: env.EventID, OccurredAt: time.Now().UTC()}
			if err := bus.Publish(ctx, ev); err != nil {
				t.Fatalf("publish %s: %v", r.name, err)
			}
		}
	}
	inbox := func(u db.User, eventCode, orderUUID string) int {
		return it.countRows(`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND channel = 'inapp'
			AND template_code = $2 AND payload->>'order_uuid' = $3`, u.ID, eventCode, orderUUID)
	}
	pushed := func(token, eventCode, orderUUID string) bool {
		for _, m := range expo.sentTo(token) {
			data, _ := m["data"].(map[string]any)
			if data["event_code"] == eventCode && data["order_uuid"] == orderUUID {
				return true
			}
		}
		return false
	}

	// 2. Zorbey opens the order for the dealer: approved in the primary
	// warehouse.
	o := it.orderCall("POST", "/v1/orders", zTok, map[string]any{
		"buyer_org_uuid": dealer.Uuid.String(),
		"items":          []any{map[string]any{"product_uuid": p.Uuid.String(), "quantity": 1}},
	}, http.StatusCreated)
	if o.Status != "approved" {
		t.Fatalf("created order status = %s", o.Status)
	}
	if row := it.orderRow(o.UUID, center.BrandID); !row.SellerWarehouseID.Valid || row.SellerWarehouseID.Int64 != wh.ID {
		t.Fatalf("order warehouse = %v", row.SellerWarehouseID)
	}
	publish(o.UUID)
	if inbox(workerUser, "ORDER_WAREHOUSE_NEW", o.UUID) != 1 || !pushed(workerPush, "ORDER_WAREHOUSE_NEW", o.UUID) {
		t.Fatalf("worker not told: inbox %d push %v", inbox(workerUser, "ORDER_WAREHOUSE_NEW", o.UUID),
			pushed(workerPush, "ORDER_WAREHOUSE_NEW", o.UUID))
	}
	if inbox(dealerOwner, "ORDER_CREATED_BY_SELLER", o.UUID) != 1 || !pushed(dealerPush, "ORDER_CREATED_BY_SELLER", o.UUID) {
		t.Fatalf("dealer not told of the new order")
	}
	if inbox(zorbey, "ORDER_WAREHOUSE_NEW", o.UUID) != 0 {
		t.Fatal("the actor must not be told")
	}
	// The worker sees it in the warehouse queue of the app.
	c, env := mobile("GET", "/v1/orders?side=seller&status=approved,processing,preparing,ready", wTok, nil)
	if c != http.StatusOK || !strings.Contains(string(env.Data), o.UUID) {
		t.Fatalf("worker queue = %d %s", c, errCode(env))
	}

	// 3. The worker prepares on the phone: preparing, barcode, ready,
	// shipped with a delivery method and a tracking number.
	transition := func(tok string, body map[string]any) orderView {
		t.Helper()
		c, env := mobile("POST", "/v1/orders/"+o.UUID+"/transitions", tok, body)
		if c != http.StatusOK {
			t.Fatalf("transition %v = %d %s", body, c, errCode(env))
		}
		var v orderView
		if err := json.Unmarshal(env.Data, &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	transition(wTok, map[string]any{"status": "preparing"})
	if c, env := mobile("POST", "/v1/orders/"+o.UUID+"/items/"+o.Items[0].UUID+"/units", wTok,
		map[string]any{"barcode": unit.Barcode}); c != http.StatusOK {
		t.Fatalf("assign %s = %d %s", unit.Barcode, c, errCode(env))
	}
	transition(wTok, map[string]any{"status": "ready"})
	c, env = mobile("POST", "/v1/delivery-methods", wTok, map[string]any{"code": "ARAS" + it.suffix[len(it.suffix)-6:], "name": "Aras Kargo"})
	if c != http.StatusCreated {
		t.Fatalf("delivery method = %d %s", c, errCode(env))
	}
	var method struct {
		UUID string `json:"uuid"`
	}
	if err := json.Unmarshal(env.Data, &method); err != nil {
		t.Fatal(err)
	}
	if v := transition(wTok, map[string]any{"status": "shipped", "delivery_method_uuid": method.UUID, "tracking_no": "TR123456"}); v.Status != "shipped" {
		t.Fatalf("shipped status = %s", v.Status)
	}
	publish(o.UUID)
	if inbox(dealerOwner, "ORDER_SHIPPED", o.UUID) != 1 || !pushed(dealerPush, "ORDER_SHIPPED", o.UUID) {
		t.Fatalf("dealer not told of the shipment: inbox %d", inbox(dealerOwner, "ORDER_SHIPPED", o.UUID))
	}
	if n := it.countRows(`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND template_code = 'ORDER_SHIPPED'
		AND payload->>'order_uuid' = $2 AND payload->>'tracking_no' = 'TR123456' AND payload->>'delivery_method' = 'Aras Kargo'`,
		dealerOwner.ID, o.UUID); n == 0 {
		t.Fatal("shipment notification misses the tracking number")
	}
	if inbox(workerUser, "ORDER_SHIPPED", o.UUID) != 0 {
		t.Fatal("the shipping worker must not be told")
	}

	// 4. The dealer receives on the phone; the center owner is told and the
	// order is booked on both caris.
	if v := transition(dTok, map[string]any{"status": "received"}); v.Status != "received" {
		t.Fatalf("received status = %s", v.Status)
	}
	publish(o.UUID)
	if inbox(zorbey, "ORDER_RECEIVED", o.UUID) != 1 || !pushed(zorbeyPush, "ORDER_RECEIVED", o.UUID) {
		t.Fatalf("center owner not told of the receipt: inbox %d", inbox(zorbey, "ORDER_RECEIVED", o.UUID))
	}
	if st, err := it.q.GetUnitCurrentState(ctx, unit.ID); err != nil || st.HolderOrgID != dealer.ID || st.Status != "available" {
		t.Fatalf("unit after receipt = %+v %v", st, err)
	}
	rows := it.orderEntries(o.UUID)
	if len(rows) != 2 {
		t.Fatalf("finance entries = %+v", rows)
	}
	var seller, buyer *orderEntry
	for i := range rows {
		switch rows[i].OrgID {
		case center.ID:
			seller = &rows[i]
		case dealer.ID:
			buyer = &rows[i]
		}
	}
	if seller == nil || buyer == nil || seller.Counterparty != dealer.ID || buyer.Counterparty != center.ID ||
		seller.Direction != "income" || buyer.Direction != "expense" || seller.OrigAmount != "100.00" || buyer.OrigAmount != "100.00" {
		t.Fatalf("finance entries = %+v", rows)
	}
}
