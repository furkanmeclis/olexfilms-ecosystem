package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	customersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type mergeResultView struct {
	SourceUUID string `json:"source_uuid"`
	TargetUUID string `json:"target_uuid"`
	DryRun     bool   `json:"dry_run"`
	Moved      struct {
		Vehicles          int64 `json:"vehicles"`
		Services          int64 `json:"services"`
		Warranties        int64 `json:"warranties"`
		OrganizationLinks int64 `json:"organization_links"`
		Consents          int64 `json:"consents"`
	} `json:"moved"`
	Conflicts struct {
		OrganizationLinks int64 `json:"organization_links"`
		ConsentsKept      int64 `json:"consents_kept"`
		ServicesKept      int64 `json:"services_kept"`
		CariAccountsKept  int64 `json:"cari_accounts_kept"`
	} `json:"conflicts"`
	Profile         string     `json:"profile"`
	PhoneMoved      bool       `json:"phone_moved"`
	EmailMoved      bool       `json:"email_moved"`
	SessionsRevoked int64      `json:"sessions_revoked"`
	MergedAt        *time.Time `json:"merged_at"`
}

// TEC-193 acceptance: the preview writes nothing and reports the right
// counts; the merge moves the source's vehicle, service and warranty to the
// target (visible there), folds the duplicate organization link, closes the
// source (access token 401, refresh revoked) and writes the audit row and
// the customer.merged event; self, chain, anonymized, panel account and
// pending transfer refusals have their own codes.
func TestIntegrationCustomerMerge(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t193-dist", "distributor", center)
	dealerA := it.org("t193-dealer-a", "dealer", dist)
	dealerB := it.org("t193-dealer-b", "dealer", dist)

	owner, pwO := it.user("t193-owner")
	it.member(dealerA, owner, "owner")
	staff, pwC := it.user("t193-center")
	it.member(center, staff, "staff")
	tokO := it.loginOrg(owner, pwO, dealerA)
	tokC := it.loginOrg(staff, pwC, center)

	tail := it.suffix[len(it.suffix)-4:]
	src, vSrc := it.svcCustomer(dealerA, "t193-src", "34S193"+tail)
	dst, vDst := it.svcCustomer(dealerA, "t193-dst", "34D193"+tail)
	// Source also served by dealer B (moved); dealer A link exists on both
	// (folded into the target's).
	if _, err := it.q.LinkCustomerOrganization(ctx, db.LinkCustomerOrganizationParams{
		UserID: src.ID, OrganizationID: dealerB.ID, BrandID: dealerB.BrandID,
	}); err != nil {
		t.Fatalf("link dealer b: %v", err)
	}
	// The source has a phone the target lacks (handed over).
	srcPhone := itPhone()
	if _, err := it.pool.Exec(ctx, `UPDATE users SET phone_e164 = $2 WHERE id = $1`, src.ID, srcPhone); err != nil {
		t.Fatalf("source phone: %v", err)
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM vehicle_transfers WHERE organization_id = $1`, dealerA.ID)
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM consents WHERE user_id = ANY($1)`, []int64{src.ID, dst.ID})
		_, _ = it.pool.Exec(context.Background(), `UPDATE users SET merged_into_user_id = NULL WHERE id = $1`, src.ID)
	})

	// A completed service of the source with a warranty (piece unit shipped
	// center -> distributor -> dealer A).
	piece := it.product(center, "T193P")
	c := it.stockChain()
	cLoc := c.location(center, "C193")
	dLoc := c.location(dist, "D193")
	u := c.unit(center, piece, 1930)
	c.post(ledger.TypeEntry, u, c.nextRef(), cLoc)
	c.ship(u, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, dLoc)
	c.ship(u, ledger.TypeOrderOut, ledger.TypeReceived, dealerA,
		ledger.Owner{Type: ledger.OwnerOrganization, ID: dealerA.ID, OrgID: dealerA.ID})
	svc, _ := it.svcCall("POST", "/v1/services", tokO,
		map[string]any{"customer_uuid": src.Uuid.String(), "vehicle_uuid": vSrc.Uuid.String()}, http.StatusCreated)
	it.svcCall("POST", "/v1/services/"+svc.UUID+"/items", tokO, map[string]any{"barcode": u.Barcode, "kind": "full"}, http.StatusCreated)
	if done, _ := it.svcCall("POST", "/v1/services/"+svc.UUID+"/transitions", tokO,
		map[string]string{"status": "completed"}, http.StatusOK); done.Status != "completed" {
		t.Fatalf("service = %+v", done)
	}
	var itemID int64
	if err := it.pool.QueryRow(ctx, `SELECT si.id FROM service_items si JOIN services s ON s.id = si.service_id
		WHERE s.uuid = $1`, svc.UUID).Scan(&itemID); err != nil {
		t.Fatalf("service item: %v", err)
	}
	start := time.Now().Add(-time.Hour)
	if _, err := it.q.CreateWarrantyForServiceItem(ctx, db.CreateWarrantyForServiceItemParams{
		ServiceItemID: itemID, HolderUserID: src.ID,
		StartAt: pgtype.Timestamptz{Time: start, Valid: true}, EndAt: pgtype.Timestamptz{Time: start.AddDate(1, 0, 0), Valid: true},
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("warranty: %v", err)
	}
	// A consent decision of the source (moved).
	if _, err := it.pool.Exec(ctx, `INSERT INTO consents (user_id, legal_text_id, kind, locale, text_version, accepted)
		SELECT $1, id, kind, locale, version, true FROM legal_texts
		WHERE kind = 'ai_guidelines' AND locale = 'tr' ORDER BY version DESC LIMIT 1`, src.ID); err != nil {
		t.Fatalf("consent: %v", err)
	}
	// A live session of the source: portal access token + refresh token.
	portalTok, _, err := it.tokens.IssueAccess(jwt.AccessInput{
		UserID: src.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal,
	})
	if err != nil {
		t.Fatal(err)
	}
	rawRefresh := "t193-refresh-" + it.suffix
	sum := sha256.Sum256([]byte(rawRefresh))
	if _, err := it.pool.Exec(ctx, `INSERT INTO refresh_tokens (user_id, token_hash, expires_at, realm)
		VALUES ($1, $2, NOW() + interval '1 hour', 'portal')`, src.ID, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	if code, _ := it.do("GET", "/v1/auth/me", hostOlex, portalTok, nil); code == http.StatusUnauthorized {
		t.Fatal("source access token refused before the merge")
	}

	srcPath := "/v1/customers/" + src.Uuid.String()
	toDst := map[string]any{"target_uuid": dst.Uuid.String()}
	refused := func(path, tok string, body any, wantStatus int, wantCode string) {
		t.Helper()
		if code, env := it.do("POST", path, hostOlex, tok, body); code != wantStatus || errCode(env) != wantCode {
			t.Fatalf("POST %s = %d %s, want %d %s", path, code, errCode(env), wantStatus, wantCode)
		}
	}

	// 1. Permissions: a dealer cannot merge; the apply needs a step-up.
	it.custDo("POST", srcPath+"/merge/preview", tokO, toDst, http.StatusForbidden)
	refused(srcPath+"/merge", tokC, toDst, http.StatusForbidden, "STEP_UP_REQUIRED")
	it.custDo("POST", srcPath+"/merge/preview", tokC, map[string]any{"target_uuid": "x"}, http.StatusBadRequest)

	// 2. Dry run: right counts, nothing written.
	type snapshot struct{ vehicles, services, warranties, links, consents, liveTokens, audits, events int }
	snap := func() snapshot {
		t.Helper()
		var s snapshot
		if err := it.pool.QueryRow(ctx, `SELECT
			(SELECT COUNT(*) FROM vehicles WHERE user_id = $1),
			(SELECT COUNT(*) FROM services WHERE customer_user_id = $1),
			(SELECT COUNT(*) FROM warranties WHERE holder_user_id = $1),
			(SELECT COUNT(*) FROM customer_organizations WHERE user_id = $1),
			(SELECT COUNT(*) FROM consents WHERE user_id = $1),
			(SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL),
			(SELECT COUNT(*) FROM activity_events WHERE action = $3 AND resource_uuid = $2),
			(SELECT COUNT(*) FROM outbox_events WHERE event_name = $4 AND payload->>'entity_uuid' = $2::text)`,
			src.ID, src.Uuid, customersusecase.ActionMerged, events.CustomerMerged).
			Scan(&s.vehicles, &s.services, &s.warranties, &s.links, &s.consents, &s.liveTokens, &s.audits, &s.events); err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		return s
	}
	before := snap()
	if before != (snapshot{1, 1, 1, 2, 1, 1, 0, 0}) {
		t.Fatalf("fixture = %+v", before)
	}
	check := func(r mergeResultView, dry bool) {
		t.Helper()
		if r.DryRun != dry || r.SourceUUID != src.Uuid.String() || r.TargetUUID != dst.Uuid.String() ||
			r.Moved.Vehicles != 1 || r.Moved.Services != 1 || r.Moved.Warranties != 1 ||
			r.Moved.OrganizationLinks != 1 || r.Moved.Consents != 1 ||
			r.Conflicts.OrganizationLinks != 1 || r.Conflicts.ServicesKept != 0 || r.Conflicts.ConsentsKept != 0 ||
			!r.PhoneMoved || r.EmailMoved || r.SessionsRevoked != 1 || r.Profile != customersusecase.ProfileNone {
			t.Fatalf("merge result (dry=%v) = %+v", dry, r)
		}
	}
	preview := decodeData[mergeResultView](t, it.custDo("POST", srcPath+"/merge/preview", tokC, toDst, http.StatusOK))
	check(preview, true)
	if preview.MergedAt != nil {
		t.Fatal("preview reports merged_at")
	}
	if after := snap(); after != before {
		t.Fatalf("preview wrote: before %+v after %+v", before, after)
	}
	var status string
	var merged *int64
	var phone *string
	if err := it.pool.QueryRow(ctx, `SELECT status, merged_into_user_id, phone_e164 FROM users WHERE id = $1`, src.ID).
		Scan(&status, &merged, &phone); err != nil || status != "active" || merged != nil || phone == nil || *phone != srcPhone {
		t.Fatalf("source after preview: %s %v %v (%v)", status, merged, phone, err)
	}

	// 3. Refusals.
	refused(srcPath+"/merge/preview", tokC, map[string]any{"target_uuid": src.Uuid.String()}, http.StatusConflict, "CUSTOMER_MERGE_SELF")
	if _, err := it.q.LinkCustomerOrganization(ctx, db.LinkCustomerOrganizationParams{
		UserID: owner.ID, OrganizationID: dealerA.ID, BrandID: dealerA.BrandID,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM customer_organizations WHERE user_id = $1`, owner.ID)
	})
	refused("/v1/customers/"+owner.Uuid.String()+"/merge/preview", tokC, toDst, http.StatusConflict, "CUSTOMER_HAS_PANEL_ACCOUNT")
	refused(srcPath+"/merge/preview", tokC, map[string]any{"target_uuid": owner.Uuid.String()}, http.StatusConflict, "CUSTOMER_HAS_PANEL_ACCOUNT")

	anon, _ := it.user("t193-anon")
	if _, err := it.q.LinkCustomerOrganization(ctx, db.LinkCustomerOrganizationParams{
		UserID: anon.ID, OrganizationID: dealerA.ID, BrandID: dealerA.BrandID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := it.pool.Exec(ctx, `UPDATE users SET status = 'anonymized' WHERE id = $1`, anon.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM customer_organizations WHERE user_id = $1`, anon.ID)
	})
	refused("/v1/customers/"+anon.Uuid.String()+"/merge/preview", tokC, toDst, http.StatusConflict, "CUSTOMER_MERGE_ANONYMIZED")
	refused(srcPath+"/merge/preview", tokC, map[string]any{"target_uuid": anon.Uuid.String()}, http.StatusConflict, "CUSTOMER_MERGE_ANONYMIZED")

	pend, vPend := it.svcCustomer(dealerA, "t193-pend", "34P193"+tail)
	if _, err := it.pool.Exec(ctx, `INSERT INTO vehicle_transfers
		(organization_id, brand_id, vehicle_id, from_user_id, to_phone, from_code_hash, to_code_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5, 'h1', 'h2', NOW() + interval '1 day')`,
		dealerA.ID, dealerA.BrandID, vPend.ID, pend.ID, itPhone()); err != nil {
		t.Fatalf("pending transfer: %v", err)
	}
	refused("/v1/customers/"+pend.Uuid.String()+"/merge/preview", tokC, toDst, http.StatusConflict, "CUSTOMER_MERGE_PENDING_TRANSFER")

	// 4. Apply.
	it.stepUp(staff.Uuid)
	res := decodeData[mergeResultView](t, it.custDo("POST", srcPath+"/merge", tokC, toDst, http.StatusOK))
	check(res, false)
	if res.MergedAt == nil {
		t.Fatal("merge without merged_at")
	}
	if after := snap(); after != (snapshot{0, 0, 0, 0, 0, 0, 1, 1}) {
		t.Fatalf("source after merge = %+v", after)
	}
	var dstVehicles, dstServices, dstWarranties, dstLinks int
	if err := it.pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM vehicles WHERE user_id = $1),
		(SELECT COUNT(*) FROM services WHERE customer_user_id = $1 AND uuid = $2),
		(SELECT COUNT(*) FROM warranties WHERE holder_user_id = $1),
		(SELECT COUNT(*) FROM customer_organizations WHERE user_id = $1)`, dst.ID, svc.UUID).
		Scan(&dstVehicles, &dstServices, &dstWarranties, &dstLinks); err != nil {
		t.Fatal(err)
	}
	if dstVehicles != 2 || dstServices != 1 || dstWarranties != 1 || dstLinks != 2 {
		t.Fatalf("target after merge: vehicles %d services %d warranties %d links %d", dstVehicles, dstServices, dstWarranties, dstLinks)
	}
	// Visible through the API at the target (both dealers now see it).
	vs := decodeData[vehPage](t, it.custDo("GET", "/v1/vehicles?customer_uuid="+dst.Uuid.String(), tokO, nil, http.StatusOK))
	got := map[string]bool{}
	for _, v := range vs.Items {
		got[v.UUID] = true
	}
	if !got[vSrc.Uuid.String()] || !got[vDst.Uuid.String()] {
		t.Fatalf("target vehicles via api = %+v", vs.Items)
	}
	d := decodeData[custView](t, it.custDo("GET", "/v1/customers/"+dst.Uuid.String(), tokC, nil, http.StatusOK))
	if d.Phone == nil || *d.Phone != srcPhone || len(d.Organizations) != 2 {
		t.Fatalf("target after merge = %+v", d)
	}

	// 5. The source is closed: merged pointer, disabled, no phone; access
	// token 401, refresh refused.
	if err := it.pool.QueryRow(ctx, `SELECT status, merged_into_user_id, phone_e164 FROM users WHERE id = $1`, src.ID).
		Scan(&status, &merged, &phone); err != nil || status != "disabled" || merged == nil || *merged != dst.ID || phone != nil {
		t.Fatalf("source after merge: %s %v %v (%v)", status, merged, phone, err)
	}
	if code, _ := it.do("GET", "/v1/auth/me", hostOlex, portalTok, nil); code != http.StatusUnauthorized {
		t.Fatalf("source access token after merge: %d", code)
	}
	if code, _ := it.do("POST", "/v1/auth/refresh", hostOlex, "", map[string]string{"refresh_token": rawRefresh}); code == http.StatusOK {
		t.Fatal("source refresh token still works after the merge")
	}

	// 6. Audit row and event content (no personal data).
	var auditPayload, eventPayload string
	if err := it.pool.QueryRow(ctx, `SELECT payload::text FROM activity_events WHERE action = $1 AND resource_uuid = $2`,
		customersusecase.ActionMerged, src.Uuid).Scan(&auditPayload); err != nil {
		t.Fatalf("audit row: %v", err)
	}
	if err := it.pool.QueryRow(ctx, `SELECT payload::text FROM outbox_events WHERE event_name = $1 AND payload->>'entity_uuid' = $2`,
		events.CustomerMerged, src.Uuid.String()).Scan(&eventPayload); err != nil {
		t.Fatalf("outbox event: %v", err)
	}
	for _, p := range []string{auditPayload, eventPayload} {
		if !containsAll(p, dst.Uuid.String(), `"warranties": 1`, `"vehicles": 1`, `"services": 1`) {
			t.Fatalf("payload misses target or counts: %s", p)
		}
		if containsAll(p, srcPhone) {
			t.Fatalf("payload carries the phone: %s", p)
		}
	}

	// 7. Chain: the merged source can be neither source nor target again.
	refused(srcPath+"/merge/preview", tokC, toDst, http.StatusConflict, "CUSTOMER_ALREADY_MERGED")
	third, _ := it.svcCustomer(dealerA, "t193-third", "34T193"+tail)
	refused("/v1/customers/"+third.Uuid.String()+"/merge/preview", tokC, map[string]any{"target_uuid": src.Uuid.String()},
		http.StatusConflict, "CUSTOMER_ALREADY_MERGED")
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
