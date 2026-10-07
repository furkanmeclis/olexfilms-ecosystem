package httpserver

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"testing"

	campaignsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/campaigns/usecase"
)

// TEC-407: the public unsubscribe of the campaign e-mail link writes the
// marketing opt-out of the user's phone without authentication; a forged
// token is 404, a missing one 400.
func TestIntegrationCampaignUnsubscribe(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	u, _ := it.user("unsub")
	phone := fmt.Sprintf("+4916%09d", rand.IntN(1_000_000_000))
	if _, err := it.pool.Exec(ctx, `UPDATE users SET phone_e164 = $2 WHERE id = $1`, u.ID, phone); err != nil {
		t.Fatal(err)
	}
	token := campaignsusecase.UnsubscribeToken([]byte(strings.Repeat("a", 40)), u.Uuid)
	const path = "/v1/public/campaigns/unsubscribe"

	for i := 0; i < 2; i++ {
		if code, env := it.do(http.MethodPost, path, hostOlex, "", map[string]string{"token": token}); code != http.StatusOK ||
			!strings.Contains(string(env.Data), `"unsubscribed":true`) {
			t.Fatalf("unsubscribe #%d: %d %s", i, code, env.Data)
		}
	}
	var optedOut bool
	if err := it.pool.QueryRow(ctx, `SELECT opted_out FROM contact_opt_out_state WHERE contact_e164 = $1 AND scope = 'marketing'`,
		phone).Scan(&optedOut); err != nil || !optedOut {
		t.Fatalf("opt-out state = %v, %v", optedOut, err)
	}
	var n int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM contact_opt_outs WHERE contact_e164 = $1 AND source = 'campaign'`,
		phone).Scan(&n); err != nil || n != 1 {
		t.Fatalf("opt-out rows = %d, %v; want one (repeat is a no-op)", n, err)
	}

	forged := campaignsusecase.UnsubscribeToken([]byte("another-secret"), u.Uuid)
	if code, _ := it.do(http.MethodPost, path, hostOlex, "", map[string]string{"token": forged}); code != http.StatusNotFound {
		t.Fatalf("forged token: %d", code)
	}
	if code, _ := it.do(http.MethodPost, path, hostOlex, "", map[string]string{"token": ""}); code != http.StatusBadRequest {
		t.Fatalf("empty token: %d", code)
	}
}
