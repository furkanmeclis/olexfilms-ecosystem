package httpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// TEC-394: a new membership writes tenant.member_added to the outbox, which
// drops the WhatsApp identity cache in the worker.
func TestIntegrationMemberAddedEventForIdentityCache(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dealer := it.org("t394-dealer", "dealer", center)
	u, _ := it.user("t394-member")

	path := "/v1/platform/organizations/" + dealer.Uuid.String() + "/members"
	if code, env := it.do("POST", path, hostOlex, it.adminToken(), map[string]any{
		"user_uuid": u.Uuid.String(), "role": "staff",
	}); code != http.StatusCreated && code != http.StatusNoContent && code != http.StatusOK {
		t.Fatalf("add member: %d %s", code, errCode(env))
	}
	var n int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_name = $1
		AND (payload->'data'->>'user_id')::bigint = $2 AND (payload->'data'->>'organization_id')::bigint = $3`,
		events.TenantMemberAdded, u.ID, dealer.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("outbox tenant.member_added = %d %v", n, err)
	}
}
