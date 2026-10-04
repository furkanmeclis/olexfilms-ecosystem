package httpserver

import (
	"net/http"
	"testing"
)

// TEC-286: contract template editor is center-only behind
// contracts.templates.manage and the intake_contracts feature.
func TestIntegrationContractTemplatePermissions(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	dealer := it.org("tec286-dealer", "dealer", center)
	centerUser, centerPW := it.user("tec286-center")
	dealerUser, dealerPW := it.user("tec286-dealer")
	it.member(center, centerUser, "staff")
	it.member(dealer, dealerUser, "owner")
	centerTok := it.loginOrg(centerUser, centerPW, center)
	dealerTok := it.loginOrg(dealerUser, dealerPW, dealer)

	body := map[string]any{"name": "TEC286 HTTP", "kind": "vehicle_intake"}
	if code, ec := it.status("POST", "/v1/platform/contract-templates", dealerTok); code != http.StatusForbidden || ec != "FORBIDDEN" {
		t.Fatalf("dealer write = %d %s, want 403 FORBIDDEN", code, ec)
	}
	if code, env := it.do("POST", "/v1/platform/contract-templates", hostOlex, centerTok, body); code != http.StatusCreated {
		t.Fatalf("center write = %d %s", code, errCode(env))
	}
}
