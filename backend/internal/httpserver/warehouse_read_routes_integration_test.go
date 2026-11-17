package httpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// TEC-1224: the supplier list and the import mapping templates are reads;
// a member holding warehouse.read only lists them but cannot add one.
func TestIntegrationWarehouseReadOnlyLists(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	slug := "t1224-wh-read-" + it.suffix
	it.exec("INSERT INTO roles (name, slug, description, is_system) VALUES ($1, $1, NULL, false)", slug)
	t.Cleanup(func() { _, _ = it.pool.Exec(context.Background(), "DELETE FROM roles WHERE slug = $1", slug) })
	it.exec(`INSERT INTO role_permissions (role_id, permission_id, scope)
		SELECT r.id, p.id, 'managed' FROM roles r, permissions p WHERE r.slug = $1 AND p.slug = $2`, slug, rbac.PermWarehouseRead)
	u, pw := it.user("t1224-reader")
	it.member(center, u, "staff", slug)
	token := it.loginOrg(u, pw, center)

	for _, path := range []string{"/v1/warehouse/suppliers", "/v1/warehouse/entry-import-templates"} {
		if code, env := it.do("GET", path, hostOlex, token, nil); code != http.StatusOK {
			t.Fatalf("GET %s with warehouse.read = %d %s", path, code, errCode(env))
		}
	}
	if code, _ := it.do("POST", "/v1/warehouse/suppliers", hostOlex, token, map[string]string{"name": "x"}); code != http.StatusForbidden {
		t.Fatalf("POST suppliers with warehouse.read = %d, want 403", code)
	}
}
