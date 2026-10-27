package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// portalLoader resolves the token subject to a user id with no
// permissions, as a customer / fleet portal session has.
type portalLoader struct{ ids map[uuid.UUID]int64 }

func (l portalLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	u, err := c.UserUUID()
	if err != nil {
		return authctx.Principal{}, err
	}
	id, ok := l.ids[u]
	if !ok {
		return authctx.Principal{}, fmt.Errorf("unknown user")
	}
	return authctx.Principal{UserID: u, UserInternal: id, Realm: c.Realm()}, nil
}

// TEC-1021: the portal inbox lists only the session user's own in-app rows
// (another channel and another user's rows stay out), filters unread,
// counts unread, marks one read (another user's row is 404) and all read; a
// panel token cannot use it and a portal token still cannot use the panel
// inbox.
func TestIntegrationPortalInbox(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	loader := portalLoader{ids: map[uuid.UUID]int64{}}
	user := func(name string) uuid.UUID {
		t.Helper()
		var id int64
		var u uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname, status)
			VALUES ($1, 'x', $2, 'T1021', 'active') RETURNING id, uuid`,
			name+"-"+suffix+"@t1021.example.com", name).Scan(&id, &u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), "DELETE FROM notifications WHERE user_id = $1", id)
			_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id)
		})
		loader.ids[u] = id
		return u
	}
	notify := func(owner uuid.UUID, channel, title string, read bool, at time.Time) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO notifications (user_id, channel, status, priority, title, read_at, created_at)
			VALUES ($1, $2, 'sent', 'normal', $3, CASE WHEN $4 THEN NOW() END, $5) RETURNING uuid::text`,
			loader.ids[owner], channel, title, read, at).Scan(&id); err != nil {
			t.Fatalf("create notification: %v", err)
		}
		return id
	}
	me, other := user("me"), user("other")
	day := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	older := notify(me, "inapp", "older", false, day)
	readOne := notify(me, "inapp", "read", true, day.Add(time.Hour))
	newer := notify(me, "inapp", "newer", false, day.Add(2*time.Hour))
	notify(me, "whatsapp", "wa", false, day.Add(3*time.Hour))
	foreign := notify(other, "inapp", "foreign", false, day)

	tokens, err := jwt.NewManager("tec-1021-test-secret-tec-1021-test-secret", time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	issue := func(u uuid.UUID, aud string) string {
		tok, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: u, SessionID: uuid.New(), Audience: aud})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	mux := http.NewServeMux()
	svc := usecase.New(db.New(pool), nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	RegisterRoutes(mux, handler.New(svc), tokens, loader)
	portal := issue(me, jwt.AudiencePortal)

	do := func(method, path, tok string, want int) []byte {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("%s %s = %d, want %d: %s", method, path, rec.Code, want, rec.Body.String())
		}
		return rec.Body.Bytes()
	}
	type page struct {
		Data struct {
			Items []struct {
				UUID    string  `json:"uuid"`
				Channel string  `json:"channel"`
				ReadAt  *string `json:"read_at"`
			} `json:"items"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	list := func(query string) ([]string, int64) {
		t.Helper()
		var p page
		if err := json.Unmarshal(do("GET", "/v1/portal/notifications"+query, portal, http.StatusOK), &p); err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, it := range p.Data.Items {
			if it.Channel != "inapp" {
				t.Fatalf("portal inbox returned channel %q", it.Channel)
			}
			ids = append(ids, it.UUID)
		}
		return ids, p.Data.Total
	}
	unread := func() int64 {
		t.Helper()
		var c struct {
			Data struct {
				Count int64 `json:"count"`
			} `json:"data"`
		}
		if err := json.Unmarshal(do("GET", "/v1/portal/notifications/unread-count", portal, http.StatusOK), &c); err != nil {
			t.Fatal(err)
		}
		return c.Data.Count
	}

	// Own in-app rows only, newest first; ?channel / ?status cannot widen it.
	if ids, total := list("?channel=whatsapp"); total != 3 || !slices.Equal(ids, []string{newer, readOne, older}) {
		t.Fatalf("list = %v (total %d)", ids, total)
	}
	if ids, total := list("?unread=true"); total != 2 || !slices.Equal(ids, []string{newer, older}) {
		t.Fatalf("unread list = %v (total %d)", ids, total)
	}
	if ids, total := list("?unread=false"); total != 1 || !slices.Equal(ids, []string{readOne}) {
		t.Fatalf("read list = %v (total %d)", ids, total)
	}
	if ids, total := list("?limit=1&offset=1"); total != 3 || !slices.Equal(ids, []string{readOne}) {
		t.Fatalf("paged list = %v (total %d)", ids, total)
	}
	do("GET", "/v1/portal/notifications?unread=maybe", portal, http.StatusBadRequest)
	if n := unread(); n != 2 {
		t.Fatalf("unread count = %d", n)
	}

	// Another user's row is 404 and stays unread.
	do("POST", "/v1/portal/notifications/"+foreign+"/read", portal, http.StatusNotFound)
	do("POST", "/v1/portal/notifications/"+uuid.NewString()+"/read", portal, http.StatusNotFound)
	var foreignRead bool
	if err := pool.QueryRow(ctx, "SELECT read_at IS NOT NULL FROM notifications WHERE uuid = $1", foreign).Scan(&foreignRead); err != nil || foreignRead {
		t.Fatalf("foreign row read = %v (%v)", foreignRead, err)
	}

	do("POST", "/v1/portal/notifications/"+older+"/read", portal, http.StatusOK)
	if n := unread(); n != 1 {
		t.Fatalf("unread count after read = %d", n)
	}
	do("POST", "/v1/portal/notifications/read-all", portal, http.StatusOK)
	if n := unread(); n != 0 {
		t.Fatalf("unread count after read-all = %d", n)
	}
	if err := pool.QueryRow(ctx, "SELECT read_at IS NOT NULL FROM notifications WHERE uuid = $1", foreign).Scan(&foreignRead); err != nil || foreignRead {
		t.Fatalf("read-all touched another user's row: %v (%v)", foreignRead, err)
	}

	// Realms never cross.
	do("GET", "/v1/portal/notifications", issue(me, jwt.AudiencePanel), http.StatusForbidden)
	do("GET", "/v1/notifications", portal, http.StatusForbidden)
}
