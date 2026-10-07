package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// fakeOutbox records outbox events.
type fakeOutbox struct {
	mu  sync.Mutex
	got []events.Event
}

func (o *fakeOutbox) Enqueue(_ context.Context, _ pgx.Tx, ev events.Event) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.got = append(o.got, ev)
	return nil
}

// convPublisher records conversation realtime events.
type convPublisher struct {
	mu  sync.Mutex
	got []map[string]any
}

func (p *convPublisher) Publish(_ context.Context, channel string, data any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	m := data.(map[string]any)
	m["_channel"] = channel
	p.got = append(p.got, m)
	return nil
}

type inboxFixture struct {
	*msgFixture
	t      *testing.T
	inbox  *usecase.Inbox
	outbox *fakeOutbox
	cpub   *convPublisher
	now    time.Time
	admin  db.User
	dealer db.User
}

func newInboxFixture(t *testing.T) *inboxFixture {
	t.Helper()
	f := &inboxFixture{msgFixture: newMsgFixture(t), t: t, outbox: &fakeOutbox{}, cpub: &convPublisher{}}
	f.now = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	f.inbox = usecase.NewInbox(usecase.InboxDeps{
		Queries: f.q, Tx: f.tx, Messaging: f.m, Storage: f.store, Outbox: f.outbox, Publisher: f.cpub,
	})
	f.inbox.SetClock(func() time.Time { return f.now })
	f.admin = f.user(true, true)
	f.dealer = f.user(false, true)
	return f
}

func (f *inboxFixture) user(superAdmin, withPhone bool) db.User {
	f.t.Helper()
	f.seq++
	p := db.CreateUserParams{
		PasswordHash: "x", Name: "TEC398", Surname: fmt.Sprint(f.seq), Status: "active",
		Email: pgtype.Text{String: fmt.Sprintf("tec398-%s-%d@example.test", f.phone[1:], f.seq), Valid: true},
	}
	if withPhone {
		p.PhoneE164 = pgtype.Text{String: fmt.Sprintf("%s%02d", f.phone, f.seq), Valid: true}
	}
	u, err := f.q.CreateUser(f.ctx, p)
	if err != nil {
		f.t.Fatal(err)
	}
	if superAdmin {
		if err := f.q.AssignUserRoleBySlug(f.ctx, db.AssignUserRoleBySlugParams{UserID: u.ID, Slug: rbac.RoleSuperAdmin}); err != nil {
			f.t.Fatal(err)
		}
	}
	return u
}

func (f *inboxFixture) adminViewer() usecase.Viewer {
	return usecase.Viewer{UserID: f.admin.ID, PlatformAdmin: true}
}

func (f *inboxFixture) conv(t *testing.T) db.Conversation {
	t.Helper()
	return f.conversation(t)
}

func (f *inboxFixture) reload(t *testing.T, c db.Conversation) db.Conversation {
	t.Helper()
	got, err := f.q.GetConversationByID(f.ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// Acceptance: a staff reply is queued as sender_type = staff with the
// replying user and pauses the AI for 30 minutes; a longer pause and AI off
// stay as they are.
func TestInboxStaffReplyQueuesStaffAndPausesAI(t *testing.T) {
	f := newInboxFixture(t)
	c := f.conv(t)
	res, err := f.inbox.Reply(f.ctx, f.adminViewer(), c.Uuid, "Merhaba, yardımcı olayım", nil)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := f.q.GetMessageByUUID(f.ctx, res.Message.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if msg.SenderType != model.SenderStaff || msg.SenderUserID.Int64 != f.admin.ID || msg.Status != "queued" || msg.Direction != "out" {
		t.Fatalf("staff message = %+v", msg)
	}
	if f.queue.count("send", msg.ID) != 1 {
		t.Fatal("send task not enqueued")
	}
	if res.Message.SenderUser == nil || res.Message.SenderUser.UUID != f.admin.Uuid {
		t.Fatalf("sender user = %+v", res.Message.SenderUser)
	}
	got := f.reload(t, c)
	want := f.now.Add(usecase.StaffReplyAIPause)
	if got.AiMode != model.AIModePaused || !got.AiPausedUntil.Time.Equal(want) {
		t.Fatalf("ai = %s until %v, want paused until %v", got.AiMode, got.AiPausedUntil.Time, want)
	}
	if res.Conversation.AIMode != model.AIModePaused {
		t.Fatalf("view ai mode = %s", res.Conversation.AIMode)
	}

	// A longer pause is kept.
	later := f.now.Add(3 * time.Hour)
	if _, err := f.q.SetConversationAIMode(f.ctx, db.SetConversationAIModeParams{
		ID: c.ID, AiMode: model.AIModePaused, AiPausedUntil: pgtype.Timestamptz{Time: later, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.inbox.Reply(f.ctx, f.adminViewer(), c.Uuid, "İkinci", nil); err != nil {
		t.Fatal(err)
	}
	if got := f.reload(t, c); !got.AiPausedUntil.Time.Equal(later) {
		t.Fatalf("longer pause shortened to %v", got.AiPausedUntil.Time)
	}
	// AI off stays off.
	if _, err := f.q.SetConversationAIMode(f.ctx, db.SetConversationAIModeParams{ID: c.ID, AiMode: model.AIModeOff}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.inbox.Reply(f.ctx, f.adminViewer(), c.Uuid, "Üçüncü", nil); err != nil {
		t.Fatal(err)
	}
	if got := f.reload(t, c); got.AiMode != model.AIModeOff {
		t.Fatalf("ai off became %s", got.AiMode)
	}
	// An empty reply is a validation error.
	if _, err := f.inbox.Reply(f.ctx, f.adminViewer(), c.Uuid, "  ", nil); !errors.Is(err, usecase.ErrInvalidRequest) {
		t.Fatalf("empty reply err = %v", err)
	}
}

// Acceptance (visibility, S2): a dealer user does not see a conversation
// assigned to another organization, nor one assigned to its own (only the
// platform admin reads conversations): 404 everywhere, empty list.
func TestInboxDealerCannotSeeConversations(t *testing.T) {
	f := newInboxFixture(t)
	brand, err := f.q.GetBrandBySlug(f.ctx, "olex")
	if err != nil {
		t.Fatal(err)
	}
	center, err := f.q.GetBrandCenter(f.ctx, brand.ID)
	if err != nil {
		t.Fatal(err)
	}
	mkOrg := func(name string) db.Organization {
		f.seq++
		o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
			Slug: fmt.Sprintf("tec398-%s-%d", f.phone[1:], f.seq), Name: name, Status: "active",
			AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
			Type:           "dealer", ParentID: pgtype.Int8{Int64: center.ID, Valid: true},
			BrandID: brand.ID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
		})
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	own, other := mkOrg("Own dealer"), mkOrg("Other dealer")
	if _, err := f.q.CreateOrganizationMember(f.ctx, db.CreateOrganizationMemberParams{
		OrganizationID: own.ID, UserID: f.dealer.ID, Role: "owner",
	}); err != nil {
		t.Fatal(err)
	}
	dealer := usecase.Viewer{UserID: f.dealer.ID}
	for _, org := range []db.Organization{other, own} {
		c := f.conv(t)
		if _, err := f.q.AssignConversation(f.ctx, db.AssignConversationParams{
			ID: c.ID, AssignedOrgID: pgtype.Int8{Int64: org.ID, Valid: true},
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.inbox.Get(f.ctx, dealer, c.Uuid); !errors.Is(err, usecase.ErrNotFound) {
			t.Fatalf("%s: get err = %v, want not found", org.Name, err)
		}
		if _, err := f.inbox.Messages(f.ctx, dealer, c.Uuid, nil, 10); !errors.Is(err, usecase.ErrNotFound) {
			t.Fatalf("%s: messages err = %v", org.Name, err)
		}
		if _, err := f.inbox.Reply(f.ctx, dealer, c.Uuid, "x", nil); !errors.Is(err, usecase.ErrNotFound) {
			t.Fatalf("%s: reply err = %v", org.Name, err)
		}
		status := model.StatusClosed
		if _, err := f.inbox.Patch(f.ctx, dealer, c.Uuid, usecase.PatchInput{Status: &status}); !errors.Is(err, usecase.ErrNotFound) {
			t.Fatalf("%s: patch err = %v", org.Name, err)
		}
		if _, err := f.inbox.MarkRead(f.ctx, dealer, c.Uuid); !errors.Is(err, usecase.ErrNotFound) {
			t.Fatalf("%s: read err = %v", org.Name, err)
		}
		// The platform admin sees it.
		if _, err := f.inbox.Get(f.ctx, f.adminViewer(), c.Uuid); err != nil {
			t.Fatalf("admin get: %v", err)
		}
	}
	items, total, err := f.inbox.List(f.ctx, dealer, repository.ConversationFilter{Limit: 20})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("dealer list = %d items, total %d, err %v", len(items), total, err)
	}
}

// Assignment: only a platform admin can be assigned; a new assignee other
// than the actor gets conversations.assigned; status and AI mode change;
// the change is published.
func TestInboxPatchAssignStatusAndAIMode(t *testing.T) {
	f := newInboxFixture(t)
	c := f.conv(t)
	other := f.user(true, false)
	if _, err := f.inbox.Patch(f.ctx, f.adminViewer(), c.Uuid, usecase.PatchInput{
		AssignUser: true, AssignedUser: &f.dealer.Uuid,
	}); !errors.Is(err, usecase.ErrAssigneeNotAllowed) {
		t.Fatalf("dealer assignee err = %v", err)
	}
	closed, paused := model.StatusClosed, model.AIModePaused
	until := f.now.Add(2 * time.Hour)
	view, err := f.inbox.Patch(f.ctx, f.adminViewer(), c.Uuid, usecase.PatchInput{
		Status: &closed, AIMode: &paused, AIPausedUntil: &until, AssignUser: true, AssignedUser: &other.Uuid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != closed || view.AIMode != paused || view.AIPausedUntil == nil || !view.AIPausedUntil.Equal(until) ||
		view.AssignedUser == nil || view.AssignedUser.UUID != other.Uuid {
		t.Fatalf("view = %+v", view)
	}
	if len(f.outbox.got) != 1 || f.outbox.got[0].Name != events.ConversationsAssigned ||
		f.outbox.got[0].Payload["assigned_user_id"] != other.ID {
		t.Fatalf("outbox = %+v", f.outbox.got)
	}
	found := false
	for _, p := range f.cpub.got {
		if p["_channel"] == realtime.UserChannel(other.Uuid) && p["type"] == usecase.EventConversationUpdated {
			found = true
		}
	}
	if !found {
		t.Fatal("assignee channel not published")
	}
	// Self-assignment notifies nobody; null unassigns.
	if _, err := f.inbox.Patch(f.ctx, f.adminViewer(), c.Uuid, usecase.PatchInput{AssignUser: true, AssignedUser: &f.admin.Uuid}); err != nil {
		t.Fatal(err)
	}
	if len(f.outbox.got) != 1 {
		t.Fatalf("self assignment notified: %+v", f.outbox.got)
	}
	view, err = f.inbox.Patch(f.ctx, f.adminViewer(), c.Uuid, usecase.PatchInput{AssignUser: true})
	if err != nil || view.AssignedUser != nil {
		t.Fatalf("unassign = %+v, %v", view.AssignedUser, err)
	}
	// Validation.
	bad := "archived"
	if _, err := f.inbox.Patch(f.ctx, f.adminViewer(), c.Uuid, usecase.PatchInput{Status: &bad}); !errors.Is(err, usecase.ErrInvalidRequest) {
		t.Fatalf("bad status err = %v", err)
	}
	auto := model.AIModeAuto
	if _, err := f.inbox.Patch(f.ctx, f.adminViewer(), c.Uuid, usecase.PatchInput{AIMode: &auto, AIPausedUntil: &until}); !errors.Is(err, usecase.ErrInvalidRequest) {
		t.Fatalf("until without paused err = %v", err)
	}
}

// The timeline pages newest first with an opaque before= cursor.
func TestInboxMessagesCursor(t *testing.T) {
	f := newInboxFixture(t)
	c := f.conv(t)
	var sent []string
	for i := range 3 {
		res, err := f.inbox.Reply(f.ctx, f.adminViewer(), c.Uuid, fmt.Sprintf("m%d", i), nil)
		if err != nil {
			t.Fatal(err)
		}
		sent = append(sent, res.Message.UUID.String())
	}
	page, err := f.inbox.Messages(f.ctx, f.adminViewer(), c.Uuid, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.NextCursor == nil || page.Items[0].UUID.String() != sent[2] {
		t.Fatalf("first page = %d items, cursor %v", len(page.Items), page.NextCursor)
	}
	cur, err := usecase.DecodeCursor(*page.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	page, err = f.inbox.Messages(f.ctx, f.adminViewer(), c.Uuid, &cur, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.NextCursor != nil || page.Items[0].UUID.String() != sent[0] {
		t.Fatalf("second page = %+v", page)
	}
	if _, err := usecase.DecodeCursor("not-a-cursor"); !errors.Is(err, usecase.ErrInvalidRequest) {
		t.Fatalf("bad cursor err = %v", err)
	}
}

// A new outgoing conversation with a user: the conversation is opened on
// the user's number with the first staff message; the marketing opt-out
// does not block a one-to-one message; a user without a phone is refused.
func TestInboxStartConversation(t *testing.T) {
	f := newInboxFixture(t)
	customer := f.user(false, true)
	if _, _, err := repository.FromQueries(f.q).RecordOptOut(f.ctx, repository.OptOut{
		ContactE164: customer.PhoneE164.String, Scope: model.OptOutScopeMarketing,
		Action: model.OptOutActionOut, Source: model.OptOutSourcePanel,
	}); err != nil {
		t.Fatal(err)
	}
	res, err := f.inbox.Start(f.ctx, f.adminViewer(), usecase.StartInput{UserUUID: customer.Uuid, Body: "Merhaba"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := f.q.GetConversationByUUID(f.ctx, res.Conversation.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if c.ContactE164 != customer.PhoneE164.String || c.UserID.Int64 != customer.ID || !c.LastMessageAt.Valid {
		t.Fatalf("conversation = %+v", c)
	}
	if res.Message.SenderType != model.SenderStaff || res.Conversation.AIMode != model.AIModePaused {
		t.Fatalf("start result = %+v", res)
	}
	// Again: the same conversation continues.
	again, err := f.inbox.Start(f.ctx, f.adminViewer(), usecase.StartInput{UserUUID: customer.Uuid, Body: "Tekrar"})
	if err != nil || again.Conversation.UUID != c.Uuid {
		t.Fatalf("second start = %v, %v", again.Conversation.UUID, err)
	}
	// An unsupported file is refused before a conversation is opened.
	other := f.user(false, true)
	if _, err := f.inbox.Start(f.ctx, f.adminViewer(), usecase.StartInput{
		UserUUID: other.Uuid, Media: &usecase.OutgoingMedia{Data: []byte("plain text"), FileName: "a.txt"},
	}); !errors.Is(err, usecase.ErrInvalidRequest) {
		t.Fatalf("bad file err = %v", err)
	}
	var n int
	if err := f.tx.QueryRow(f.ctx, "SELECT COUNT(*) FROM conversations WHERE contact_e164 = $1", other.PhoneE164.String).Scan(&n); err != nil || n != 0 {
		t.Fatalf("conversation opened for a refused file: %d, %v", n, err)
	}
	noPhone := f.user(false, false)
	if _, err := f.inbox.Start(f.ctx, f.adminViewer(), usecase.StartInput{UserUUID: noPhone.Uuid, Body: "x"}); !errors.Is(err, usecase.ErrContactNoPhone) {
		t.Fatalf("no phone err = %v", err)
	}
	if _, err := f.inbox.Start(f.ctx, usecase.Viewer{UserID: f.dealer.ID}, usecase.StartInput{UserUUID: customer.Uuid, Body: "x"}); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatalf("dealer start err = %v", err)
	}
}

// Bulk: close / set_ai_mode apply and revert to the snapshot; assign
// refuses a non-admin assignee.
func TestConversationBulkAdapterApplyAndRevert(t *testing.T) {
	f := newInboxFixture(t)
	c := f.conv(t)
	a := usecase.NewBulkAdapter(f.q)
	run := func(params map[string]string) context.Context {
		return bulkengine.WithRun(f.ctx, bulkengine.Run{Params: params})
	}
	res, err := a.ApplyItem(run(nil), usecase.BulkClose, c.Uuid.String())
	if err != nil || !res.OK {
		t.Fatalf("close = %+v, %v", res, err)
	}
	if got := f.reload(t, c); got.Status != model.StatusClosed {
		t.Fatalf("status = %s", got.Status)
	}
	if err := a.RevertItem(run(nil), usecase.BulkClose, c.Uuid.String(), map[string]any{"status": model.StatusOpen}); err != nil {
		t.Fatal(err)
	}
	if got := f.reload(t, c); got.Status != model.StatusOpen {
		t.Fatalf("reverted status = %s", got.Status)
	}
	res, err = a.ApplyItem(run(map[string]string{usecase.ParamAIMode: model.AIModeOff}), usecase.BulkSetAIMode, c.Uuid.String())
	if err != nil || !res.OK || f.reload(t, c).AiMode != model.AIModeOff {
		t.Fatalf("set ai mode = %+v, %v", res, err)
	}
	res, err = a.ApplyItem(run(map[string]string{usecase.ParamAssigneeUserUUID: f.dealer.Uuid.String()}), usecase.BulkAssign, c.Uuid.String())
	if err != nil || res.OK {
		t.Fatalf("dealer assignee = %+v, %v", res, err)
	}
	res, err = a.ApplyItem(run(map[string]string{usecase.ParamAssigneeUserUUID: f.admin.Uuid.String()}), usecase.BulkAssign, c.Uuid.String())
	if err != nil || !res.OK || f.reload(t, c).AssignedUserID.Int64 != f.admin.ID {
		t.Fatalf("admin assignee = %+v, %v", res, err)
	}
}
