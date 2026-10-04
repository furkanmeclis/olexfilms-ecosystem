package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/google/uuid"
)

type contractRequiredView struct {
	UUID             string `json:"uuid"`
	Status           string `json:"status"`
	ContractRequired bool   `json:"contract_required"`
	Contract         *struct {
		UUID       string `json:"uuid"`
		Status     string `json:"status"`
		ContractNo int64  `json:"contract_no"`
	} `json:"contract"`
}

// TEC-289: with contracts.intake_required on, the wired HTTP transition
// (panel and mobile share it) refuses draft/pending -> processing and a
// direct completion until the linked contract is executed; an organization
// with intake_contracts off is free.
func TestIntegrationServicesContractRequired(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	reset := func() { _, _ = it.srv.sysconfig.Reset(ctx, sysconfig.KeyContractsIntakeRequired) }
	reset()
	t.Cleanup(reset)

	center := it.brandCenter("olex")
	dist := it.org("t289-dist", "distributor", center)
	dealerA := it.org("t289-dealer-a", "dealer", dist)
	dealerB := it.org("t289-dealer-b", "dealer", dist)
	ownerA, apw := it.user("t289-owner-a")
	it.member(dealerA, ownerA, "owner")
	ownerB, bpw := it.user("t289-owner-b")
	it.member(dealerB, ownerB, "owner")
	admin, _ := it.user("t289-admin", rbac.RoleSuperAdmin)
	if _, err := it.srv.features.SetByAdmin(ctx, admin.ID, dealerB.ID, features.ModuleIntakeContracts, false); err != nil {
		t.Fatalf("disable intake_contracts for dealer B: %v", err)
	}
	custA, vehA := it.svcCustomer(dealerA, "t289-cust-a", "34T289"+it.suffix[len(it.suffix)-4:])
	custB, vehB := it.svcCustomer(dealerB, "t289-cust-b", "06T289"+it.suffix[len(it.suffix)-4:])
	aTok := it.loginOrg(ownerA, apw, dealerA)
	bTok := it.loginOrg(ownerB, bpw, dealerB)

	call := func(method, path, tok string, body any, want int) (contractRequiredView, string) {
		t.Helper()
		code, env := it.do(method, path, hostOlex, tok, body)
		if code != want {
			t.Fatalf("%s %s = %d %s, want %d", method, path, code, errCode(env), want)
		}
		var v contractRequiredView
		if code < 300 {
			if err := json.Unmarshal(env.Data, &v); err != nil {
				t.Fatal(err)
			}
		}
		return v, errCode(env)
	}
	move := func(id, tok, to string, want int) (contractRequiredView, string) {
		return call("POST", "/v1/services/"+id+"/transitions", tok, map[string]string{"status": to}, want)
	}
	create := func(tok, cust, veh string) contractRequiredView {
		v, _ := call("POST", "/v1/services", tok, map[string]any{"customer_uuid": cust, "vehicle_uuid": veh}, http.StatusCreated)
		return v
	}

	// Setting off (default): no requirement is reported.
	s1 := create(aTok, custA.Uuid.String(), vehA.Uuid.String())
	if s1.ContractRequired || s1.Contract != nil {
		t.Fatalf("setting off: %+v", s1)
	}

	if _, err := it.srv.sysconfig.Set(ctx, sysconfig.KeyContractsIntakeRequired, json.RawMessage("true"), admin.ID); err != nil {
		t.Fatal(err)
	}

	// Setting on + module on: draft -> pending is free, pending ->
	// processing and draft -> completed answer 422 CONTRACT_REQUIRED (the
	// gate runs before the item check of the completion).
	if v, _ := move(s1.UUID, aTok, "pending", http.StatusOK); !v.ContractRequired || v.Contract != nil {
		t.Fatalf("pending view = %+v", v)
	}
	if _, ec := move(s1.UUID, aTok, "processing", http.StatusUnprocessableEntity); ec != "CONTRACT_REQUIRED" {
		t.Fatalf("pending -> processing without contract = %s", ec)
	}
	s2 := create(aTok, custA.Uuid.String(), vehA.Uuid.String())
	if _, ec := move(s2.UUID, aTok, "completed", http.StatusUnprocessableEntity); ec != "CONTRACT_REQUIRED" {
		t.Fatalf("draft -> completed without contract = %s", ec)
	}
	// The list reports the requirement too.
	code, env := it.do("GET", "/v1/services", hostOlex, aTok, nil)
	if code != http.StatusOK {
		t.Fatalf("list = %d", code)
	}
	var page struct {
		Items []contractRequiredView `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) == 0 || !page.Items[0].ContractRequired {
		t.Fatalf("list items = %+v", page.Items)
	}

	// A pending contract is not enough.
	svcID := func(id string) int64 {
		var n int64
		if err := it.pool.QueryRow(ctx, "SELECT id FROM services WHERE uuid = $1", uuid.MustParse(id)).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	var tplID, contractID int64
	if err := it.pool.QueryRow(ctx, `INSERT INTO contract_templates (organization_id, brand_id, name, kind)
		VALUES ($1, $2, $3, 'vehicle_intake') RETURNING id`, center.ID, center.BrandID, "T289 "+it.suffix).Scan(&tplID); err != nil {
		t.Fatalf("template: %v", err)
	}
	var contractUUID uuid.UUID
	if err := it.pool.QueryRow(ctx, `INSERT INTO contract_instances (organization_id, brand_id, contract_no, subject_type,
		subject_id, template_id, kind, locale, template_version, otp_required, signature_required, status,
		rendered_html, content_sha256)
		VALUES ($1, $2, 1, 'service', $3, $4, 'vehicle_intake', 'tr', 1, false, false, 'pending', '<p>x</p>', $5)
		RETURNING id, uuid`, dealerA.ID, dealerA.BrandID, svcID(s1.UUID), tplID, strings.Repeat("a", 64)).Scan(&contractID, &contractUUID); err != nil {
		t.Fatalf("contract: %v", err)
	}
	if _, err := it.pool.Exec(ctx, "UPDATE services SET contract_id = $1 WHERE uuid = $2", contractID, uuid.MustParse(s1.UUID)); err != nil {
		t.Fatalf("link contract: %v", err)
	}
	if _, ec := move(s1.UUID, aTok, "processing", http.StatusUnprocessableEntity); ec != "CONTRACT_REQUIRED" {
		t.Fatalf("pending contract = %s", ec)
	}

	// Executed: the move goes through and the detail carries the summary.
	if _, err := it.pool.Exec(ctx, "UPDATE contract_instances SET status = 'executed', executed_at = NOW() WHERE id = $1", contractID); err != nil {
		t.Fatalf("execute contract: %v", err)
	}
	v, _ := move(s1.UUID, aTok, "processing", http.StatusOK)
	if v.Status != "processing" || !v.ContractRequired || v.Contract == nil ||
		v.Contract.UUID != contractUUID.String() || v.Contract.Status != "executed" || v.Contract.ContractNo != 1 {
		t.Fatalf("processing view = %+v (contract %+v)", v, v.Contract)
	}

	// Module off for dealer B: no requirement even with the setting on.
	s3 := create(bTok, custB.Uuid.String(), vehB.Uuid.String())
	if s3.ContractRequired {
		t.Fatalf("module off view = %+v", s3)
	}
	move(s3.UUID, bTok, "pending", http.StatusOK)
	if v, _ := move(s3.UUID, bTok, "processing", http.StatusOK); v.Status != "processing" {
		t.Fatalf("module off processing = %+v", v)
	}
}
