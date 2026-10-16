package httpserver

import (
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

func TestStockCountApproveRequiresStepUp(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	u, pw := it.user("count-stepup")
	it.member(center, u, "staff", rbac.RoleCenterWarehouse)
	token := it.loginOrg(u, pw, center)
	path := "/v1/warehouse/stock-counts/" + uuid.NewString() + "/approve"
	code, env := it.do("POST", path, hostOlex, token, map[string]any{"resolutions": []any{}})
	if code != http.StatusForbidden || errCode(env) != "STEP_UP_REQUIRED" {
		t.Fatalf("approval without step-up = %d %s", code, errCode(env))
	}
}
