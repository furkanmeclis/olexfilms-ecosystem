package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	bulkusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/bulk/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// convIT is the conversation API fixture: a platform admin, a dealer owner
// and conversations named after the run suffix (so list filters only see
// this run's rows).
type convIT struct {
	*itest
	adminTok  string
	dealerTok string
	tag       string
}

func newConvIT(t *testing.T) *convIT {
	t.Helper()
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = storage.NewMemory() })
	c := &convIT{itest: it, tag: "tec398-" + it.suffix}
	admin, adminPW := it.user("conv-admin", rbac.RoleSuperAdmin)
	c.adminTok = it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": admin.Email.String, "password": adminPW,
	})).AccessToken
	dist := it.org("conv-dist", "distributor", it.brandCenter("olex"))
	dealer := it.org("conv-dealer", "dealer", dist)
	dealerUser, dealerPW := it.user("conv-dealer")
	it.member(dealer, dealerUser, "owner")
	c.dealerTok = it.catalogLogin(dealerUser, dealerPW, dealer, hostOlex)
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM conversations WHERE contact_name LIKE $1 || '%'", c.tag)
	})
	return c
}

var convSeq int

// conv inserts a conversation with the given state.
func (c *convIT) conv(status string, unread int, assignedOrg *int64) string {
	c.t.Helper()
	convSeq++
	var id string
	err := c.pool.QueryRow(context.Background(), `
		INSERT INTO conversations (channel, contact_e164, contact_name, status, unread_count, assigned_org_id, last_message_at)
		VALUES ('whatsapp', $1, $2, $3, $4, $5, NOW() - make_interval(mins => $6::int))
		RETURNING uuid::text`,
		fmt.Sprintf("+90544%07d", time.Now().UnixNano()%10_000_000+int64(convSeq)), fmt.Sprintf("%s-%02d", c.tag, convSeq),
		status, unread, assignedOrg, convSeq).Scan(&id)
	if err != nil {
		c.t.Fatal(err)
	}
	return id
}

type convPage struct {
	Items []struct {
		UUID        string `json:"uuid"`
		Status      string `json:"status"`
		UnreadCount int    `json:"unread_count"`
	} `json:"items"`
	Total int `json:"total"`
}

func (c *convIT) list(query string) (int, convPage, string) {
	c.t.Helper()
	code, env := c.do("GET", "/v1/conversations?q="+c.tag+"&"+query, hostOlex, c.adminTok, nil)
	var p convPage
	_ = json.Unmarshal(env.Data, &p)
	return code, p, errCode(env)
}

func (c *convIT) row(uuid string) (status, aiMode string, pausedUntil *time.Time) {
	c.t.Helper()
	if err := c.pool.QueryRow(context.Background(),
		"SELECT status, ai_mode, ai_paused_until FROM conversations WHERE uuid = $1", uuid).Scan(&status, &aiMode, &pausedUntil); err != nil {
		c.t.Fatal(err)
	}
	return
}

// TEC-398 acceptance: list contract (unknown sort 400, multi-valued status,
// unread=true, default sort in /meta).
func TestIntegrationConversationsListContract(t *testing.T) {
	c := newConvIT(t)
	open1 := c.conv("open", 2, nil)
	pending := c.conv("pending", 0, nil)
	c.conv("closed", 1, nil)

	code, env := c.do("GET", "/v1/conversations/meta", hostOlex, c.adminTok, nil)
	var meta struct {
		DefaultSort    string   `json:"default_sort"`
		SortableFields []string `json:"sortable_fields"`
		BulkActions    []struct {
			ID string `json:"id"`
		} `json:"bulk_actions"`
	}
	_ = json.Unmarshal(env.Data, &meta)
	if code != http.StatusOK || meta.DefaultSort != "-last_message_at" || len(meta.BulkActions) != 3 {
		t.Fatalf("meta = %d %+v", code, meta)
	}

	if code, _, ec := c.list("sort=bogus"); code != http.StatusBadRequest || ec != "VALIDATION_ERROR" {
		t.Fatalf("unknown sort = %d %s", code, ec)
	}
	if code, _, ec := c.list("status=open,archived"); code != http.StatusBadRequest || ec != "VALIDATION_ERROR" {
		t.Fatalf("unknown status = %d %s", code, ec)
	}
	if code, _, _ := c.list("unread=maybe"); code != http.StatusBadRequest {
		t.Fatalf("bad unread = %d", code)
	}

	code, p, ec := c.list("status=open,pending")
	if code != http.StatusOK || p.Total != 2 || len(p.Items) != 2 {
		t.Fatalf("status=open,pending = %d %s %+v", code, ec, p)
	}
	// Default sort: newest message first (open1 is the newest).
	if p.Items[0].UUID != open1 || p.Items[1].UUID != pending {
		t.Fatalf("default order = %+v", p.Items)
	}
	code, p, _ = c.list("status=open&status=pending&sort=last_message_at")
	if code != http.StatusOK || p.Total != 2 || p.Items[0].UUID != pending {
		t.Fatalf("repeated status, asc sort = %d %+v", code, p)
	}
	code, p, _ = c.list("unread=true")
	if code != http.StatusOK || p.Total != 2 {
		t.Fatalf("unread=true = %d %+v", code, p)
	}
	for _, it := range p.Items {
		if it.UnreadCount == 0 {
			t.Fatalf("unread=true returned a read conversation: %+v", it)
		}
	}
	code, p, _ = c.list("unread=true&sort=-unread_count")
	if code != http.StatusOK || p.Items[0].UUID != open1 {
		t.Fatalf("unread sort = %d %+v", code, p)
	}
}

// TEC-398 acceptance: a dealer does not see a conversation assigned to
// another organization (404) and without conversations.read / .reply the
// list and a reply are 403.
func TestIntegrationConversationsVisibility(t *testing.T) {
	c := newConvIT(t)
	other := c.org("conv-other", "dealer", c.brandCenter("olex"))
	id := c.conv("open", 1, &other.ID)

	for _, path := range []string{"/v1/conversations/" + id, "/v1/conversations/" + id + "/messages"} {
		if code, env := c.do("GET", path, hostOlex, c.dealerTok, nil); code != http.StatusNotFound {
			t.Fatalf("dealer GET %s = %d %s, want 404", path, code, errCode(env))
		}
	}
	if code, _ := c.do("POST", "/v1/conversations/"+id+"/read", hostOlex, c.dealerTok, nil); code != http.StatusNotFound {
		t.Fatalf("dealer read = %d, want 404", code)
	}
	if code, _ := c.do("GET", "/v1/conversations", hostOlex, c.dealerTok, nil); code != http.StatusForbidden {
		t.Fatalf("dealer list = %d, want 403", code)
	}
	if code, _ := c.do("POST", "/v1/conversations/"+id+"/messages", hostOlex, c.dealerTok, map[string]string{"body": "x"}); code != http.StatusForbidden {
		t.Fatalf("dealer reply = %d, want 403", code)
	}
	if code, _ := c.do("POST", "/v1/conversations", hostOlex, c.dealerTok, map[string]string{"body": "x"}); code != http.StatusForbidden {
		t.Fatalf("dealer start = %d, want 403", code)
	}
	if code, _ := c.do("PATCH", "/v1/conversations/"+id, hostOlex, c.dealerTok, map[string]string{"status": "closed"}); code != http.StatusForbidden {
		t.Fatalf("dealer patch = %d, want 403", code)
	}
	if code, _ := c.do("POST", "/v1/conversations/bulk", hostOlex, c.dealerTok, map[string]any{
		"action": "close", "target": map[string]any{"scope": "ids", "ids": []string{id}},
	}); code != http.StatusForbidden {
		t.Fatalf("dealer bulk = %d, want 403", code)
	}
	// The platform admin sees it.
	if code, env := c.do("GET", "/v1/conversations/"+id, hostOlex, c.adminTok, nil); code != http.StatusOK {
		t.Fatalf("admin GET = %d %s", code, errCode(env))
	}
}

// TEC-398 acceptance: a staff reply is queued with sender_type = staff and
// pauses the AI for 30 minutes; an attachment goes as multipart.
func TestIntegrationConversationsStaffReply(t *testing.T) {
	c := newConvIT(t)
	id := c.conv("open", 1, nil)
	before := time.Now()
	code, env := c.do("POST", "/v1/conversations/"+id+"/messages", hostOlex, c.adminTok, map[string]string{"body": "Merhaba"})
	if code != http.StatusCreated {
		t.Fatalf("reply = %d %s", code, errCode(env))
	}
	var res struct {
		Message struct {
			UUID       string `json:"uuid"`
			SenderType string `json:"sender_type"`
			Status     string `json:"status"`
		} `json:"message"`
	}
	_ = json.Unmarshal(env.Data, &res)
	var sender, status string
	var senderUser *int64
	if err := c.pool.QueryRow(context.Background(),
		"SELECT sender_type, status, sender_user_id FROM messages WHERE uuid = $1", res.Message.UUID).Scan(&sender, &status, &senderUser); err != nil {
		t.Fatal(err)
	}
	if sender != "staff" || status != "queued" || senderUser == nil || res.Message.SenderType != "staff" {
		t.Fatalf("message = %s %s %v", sender, status, senderUser)
	}
	_, aiMode, until := c.row(id)
	if aiMode != "paused" || until == nil || until.Before(before.Add(29*time.Minute)) || until.After(time.Now().Add(31*time.Minute)) {
		t.Fatalf("ai = %s until %v", aiMode, until)
	}

	// Multipart: caption + PNG.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("body", "Fotoğraf")
	fw, _ := mw.CreateFormFile("file", "foto.png")
	_, _ = fw.Write(append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{2}, 64)...))
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/v1/conversations/"+id+"/messages", &buf)
	req.Host = "backend:8080"
	req.Header.Set("X-Forwarded-Host", hostOlex)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.adminTok)
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("multipart reply = %d %s", rec.Code, rec.Body.String())
	}
	var menv envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &menv)
	var mres struct {
		Message struct {
			UUID           string `json:"uuid"`
			HasStoredMedia bool   `json:"has_stored_media"`
		} `json:"message"`
	}
	_ = json.Unmarshal(menv.Data, &mres)
	if !mres.Message.HasStoredMedia {
		t.Fatalf("attachment not stored: %s", menv.Data)
	}
	req = httptest.NewRequest("GET", "/v1/conversations/"+id+"/messages/"+mres.Message.UUID+"/media", nil)
	req.Host = "backend:8080"
	req.Header.Set("X-Forwarded-Host", hostOlex)
	req.Header.Set("Authorization", "Bearer "+c.adminTok)
	rec = httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("media = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}

	// Timeline: newest first.
	code, env = c.do("GET", "/v1/conversations/"+id+"/messages?limit=1", hostOlex, c.adminTok, nil)
	var page struct {
		Items []struct {
			UUID string `json:"uuid"`
		} `json:"items"`
		NextCursor *string `json:"next_cursor"`
	}
	_ = json.Unmarshal(env.Data, &page)
	if code != http.StatusOK || len(page.Items) != 1 || page.Items[0].UUID != mres.Message.UUID || page.NextCursor == nil {
		t.Fatalf("timeline = %d %+v", code, page)
	}
	code, env = c.do("GET", "/v1/conversations/"+id+"/messages?before="+*page.NextCursor, hostOlex, c.adminTok, nil)
	_ = json.Unmarshal(env.Data, &page)
	if code != http.StatusOK || len(page.Items) != 1 || page.Items[0].UUID != res.Message.UUID || page.NextCursor != nil {
		t.Fatalf("older page = %d %+v", code, page)
	}

	// Read mark.
	code, env = c.do("POST", "/v1/conversations/"+id+"/read", hostOlex, c.adminTok, nil)
	var view struct {
		UnreadCount int `json:"unread_count"`
	}
	_ = json.Unmarshal(env.Data, &view)
	if code != http.StatusOK || view.UnreadCount != 0 {
		t.Fatalf("read = %d %+v", code, view)
	}
	// PATCH validation: unknown field / status.
	if code, _ := c.do("PATCH", "/v1/conversations/"+id, hostOlex, c.adminTok, map[string]string{"status": "archived"}); code != http.StatusBadRequest {
		t.Fatalf("bad status = %d", code)
	}
	if code, _ := c.do("PATCH", "/v1/conversations/"+id, hostOlex, c.adminTok, map[string]string{"color": "red"}); code != http.StatusBadRequest {
		t.Fatalf("unknown field = %d", code)
	}
	code, env = c.do("PATCH", "/v1/conversations/"+id, hostOlex, c.adminTok, map[string]any{"ai_mode": "off", "status": "pending"})
	if st, mode, _ := c.row(id); code != http.StatusOK || st != "pending" || mode != "off" {
		t.Fatalf("patch = %d %s, row %s %s", code, errCode(env), st, mode)
	}
}

// TEC-398 acceptance: bulk close is logged and undo restores the previous
// status.
func TestIntegrationConversationsBulkCloseUndo(t *testing.T) {
	c := newConvIT(t)
	a := c.conv("open", 0, nil)
	b := c.conv("pending", 0, nil)
	run := c.bulkRun("/v1/conversations/bulk", c.adminTok, map[string]any{
		"action": "close", "target": map[string]any{"scope": "query", "query": map[string]string{"q": c.tag}},
	})
	if run.Summary.Succeeded != 2 || run.Operation.UndoStatus != bulkusecase.UndoAvailable {
		t.Fatalf("bulk close = %+v", run)
	}
	for _, id := range []string{a, b} {
		if st, _, _ := c.row(id); st != "closed" {
			t.Fatalf("%s status = %s", id, st)
		}
	}
	undone, _ := c.bulkUndo("/v1/platform/bulk-operations/"+run.Operation.UUID.String()+"/undo", hostOlex, c.adminTok, http.StatusOK)
	if undone.UndoStatus != bulkusecase.UndoUndone || undone.UndoResult == nil || undone.UndoResult.Restored != 2 {
		t.Fatalf("undo = %+v", undone)
	}
	if st, _, _ := c.row(a); st != "open" {
		t.Fatalf("a restored to %s", st)
	}
	if st, _, _ := c.row(b); st != "pending" {
		t.Fatalf("b restored to %s", st)
	}
}
