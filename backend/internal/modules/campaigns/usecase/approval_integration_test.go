package usecase

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// recordingOutbox keeps the enqueued events (TEC-406 notifications).
type recordingOutbox struct{ events []events.Event }

func (r *recordingOutbox) Enqueue(_ context.Context, _ pgx.Tx, ev events.Event) error {
	r.events = append(r.events, ev)
	return nil
}

func (r *recordingOutbox) last(t *testing.T, name string) events.Event {
	t.Helper()
	for i := len(r.events) - 1; i >= 0; i-- {
		if r.events[i].Name == name {
			return r.events[i]
		}
	}
	t.Fatalf("no %s event in %d events", name, len(r.events))
	return events.Event{}
}

func notifyIDs(ev events.Event) []int64 {
	ids, _ := ev.Payload["notify_user_ids"].([]int64)
	return ids
}

// approver adds a member of org holding campaigns.approve (through the
// owner role of the organization type).
func (f *fixture) approver(t *testing.T, org db.Organization, name, role string) db.User {
	t.Helper()
	u := f.user(t, name, "")
	m, err := f.q.CreateOrganizationMember(f.ctx, db.CreateOrganizationMemberParams{
		OrganizationID: org.ID, UserID: u.ID, Role: "owner",
	})
	if err != nil {
		t.Fatalf("member: %v", err)
	}
	f.exec(t, `INSERT INTO organization_member_roles (member_id, role_id) SELECT $1, id FROM roles WHERE slug = $2`, m.ID, role)
	return u
}

// ready creates a campaign of c with a complete tr content (the audience is
// empty unless the test adds customers).
func (f *fixture) ready(t *testing.T, c Caller) Campaign {
	t.Helper()
	camp := f.create(t, c, AudienceFilter{AudienceType: AudienceCustomers})
	if _, err := f.svc.PutContent(f.ctx, c, camp.UUID, "tr", ContentInput{Body: "Merhaba"}); err != nil {
		t.Fatalf("content: %v", err)
	}
	return camp
}

func (f *fixture) campaignRow(t *testing.T, id uuid.UUID) db.Campaign {
	t.Helper()
	row, err := f.q.GetCampaignByUUID(f.ctx, db.GetCampaignByUUIDParams{Uuid: id, BrandID: f.brandID})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func (f *fixture) events(t *testing.T, id uuid.UUID) []db.CampaignEvent {
	t.Helper()
	evs, err := f.q.ListCampaignEvents(f.ctx, f.campaignRow(t, id).ID)
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func (f *fixture) withOutbox() *recordingOutbox {
	out := &recordingOutbox{}
	f.svc.SetOutbox(out)
	return out
}

func asCaller(base Caller, u db.User) Caller {
	base.UserID = u.ID
	return base
}

func approvalUUIDs(t *testing.T, f *fixture, c Caller) []uuid.UUID {
	t.Helper()
	page, err := f.svc.Approvals(f.ctx, c, ApprovalFilter{SortKey: "created_at", Limit: 50})
	if err != nil {
		t.Fatalf("approvals: %v", err)
	}
	out := []uuid.UUID{}
	for _, it := range page.Items {
		out = append(out, it.UUID)
	}
	return out
}

// TEC-406: a dealer campaign waits in its distributor's queue (not the
// center's); another distributor neither sees nor decides it (404); the
// distributor's approvers are notified on submission, the creator on the
// decision.
func TestCampaignApprovalDealerToDistributor(t *testing.T) {
	f := newFixture(t)
	out := f.withOutbox()
	distApprover := f.approver(t, f.dist, "dist-owner", "distributor_owner")
	f.approver(t, f.otherDist, "other-owner", "distributor_owner")
	f.member(t, f.dist, "dist-plain") // no campaigns.approve: not notified

	camp := f.ready(t, f.dealerC)
	got, err := f.svc.Submit(f.ctx, f.dealerC, camp.UUID)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if got.Status != StatusPendingApproval || got.ApproverOrganizationUUID == nil || *got.ApproverOrganizationUUID != f.dist.Uuid {
		t.Fatalf("submitted = %s approver %v, want pending at the distributor", got.Status, got.ApproverOrganizationUUID)
	}
	ev := out.last(t, events.CampaignsSubmitted)
	if ids := notifyIDs(ev); !slices.Equal(ids, []int64{distApprover.ID}) {
		t.Fatalf("submitted notify = %v, want the distributor approver %d", ids, distApprover.ID)
	}

	distC := asCaller(f.distC, distApprover)
	if q := approvalUUIDs(t, f, distC); !slices.Contains(q, camp.UUID) {
		t.Fatalf("distributor queue %v misses the dealer campaign", q)
	}
	if q := approvalUUIDs(t, f, f.centerC); slices.Contains(q, camp.UUID) {
		t.Fatal("the center queue must not hold the dealer campaign")
	}
	otherC := f.managed(distApprover, f.otherDist)
	if q := approvalUUIDs(t, f, otherC); slices.Contains(q, camp.UUID) {
		t.Fatal("another distributor's queue must not hold the campaign")
	}
	if _, err := f.svc.Approve(f.ctx, otherC, camp.UUID, DecisionInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other distributor approve err = %v, want not found", err)
	}
	if _, err := f.svc.Reject(f.ctx, otherC, camp.UUID, DecisionInput{Reason: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other distributor reject err = %v, want not found", err)
	}
	if _, err := f.svc.Get(f.ctx, otherC, camp.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other distributor get err = %v, want not found", err)
	}
	if _, err := f.svc.Approve(f.ctx, f.centerC, camp.UUID, DecisionInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("center approve err = %v, want not found (not the approver)", err)
	}
	if _, err := f.svc.Approve(f.ctx, f.dealerC, camp.UUID, DecisionInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("creator approve err = %v, want not found", err)
	}
	if v, err := f.svc.Get(f.ctx, distC, camp.UUID); err != nil || v.UUID != camp.UUID {
		t.Fatalf("approver get = %v, %v", v.UUID, err)
	}

	approved, err := f.svc.Approve(f.ctx, distC, camp.UUID, DecisionInput{Reason: "Uygun"})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != StatusApproved {
		t.Fatalf("status = %s", approved.Status)
	}
	ev = out.last(t, events.CampaignsApproved)
	if ids := notifyIDs(ev); !slices.Equal(ids, []int64{f.dealerC.UserID}) {
		t.Fatalf("approved notify = %v, want the creator", ids)
	}
	if q := approvalUUIDs(t, f, distC); slices.Contains(q, camp.UUID) {
		t.Fatal("a decided campaign must leave the queue")
	}
	if _, err := f.svc.Approve(f.ctx, distC, camp.UUID, DecisionInput{}); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("second approve err = %v, want invalid status", err)
	}
}

// TEC-406 (S4): a dealer without a distributor and a distributor go to the
// center; a center campaign is approved on submission with its event.
func TestCampaignApproverResolution(t *testing.T) {
	f := newFixture(t)
	out := f.withOutbox()
	centerApprover := f.approver(t, f.center, "center-social", "center_social")

	lone := f.org(t, "lone", rbac.OrgTypeDealer, f.center.ID)
	loneC := f.managed(f.user(t, "lone-author", ""), lone)
	loneCamp := f.ready(t, loneC)
	if got, err := f.svc.Submit(f.ctx, loneC, loneCamp.UUID); err != nil || got.ApproverOrganizationUUID == nil ||
		*got.ApproverOrganizationUUID != f.center.Uuid {
		t.Fatalf("dealer without distributor: %+v, %v", got.ApproverOrganizationUUID, err)
	}
	if ids := notifyIDs(out.last(t, events.CampaignsSubmitted)); !slices.Contains(ids, centerApprover.ID) {
		t.Fatalf("center approvers not notified: %v", ids)
	}

	// A dealer whose distributor is not active falls back to the center.
	f.exec(t, `UPDATE organizations SET status = 'suspended' WHERE id = $1`, f.otherDist.ID)
	susp := f.ready(t, f.otherDealerC)
	if got, err := f.svc.Submit(f.ctx, f.otherDealerC, susp.UUID); err != nil || *got.ApproverOrganizationUUID != f.center.Uuid {
		t.Fatalf("suspended distributor: %v, %v", got.ApproverOrganizationUUID, err)
	}

	distCamp := f.ready(t, f.distC)
	got, err := f.svc.Submit(f.ctx, f.distC, distCamp.UUID)
	if err != nil || got.Status != StatusPendingApproval || *got.ApproverOrganizationUUID != f.center.Uuid {
		t.Fatalf("distributor campaign: %s %v, %v", got.Status, got.ApproverOrganizationUUID, err)
	}
	centerQ := asCaller(f.centerC, centerApprover)
	q := approvalUUIDs(t, f, centerQ)
	if !slices.Contains(q, distCamp.UUID) || !slices.Contains(q, loneCamp.UUID) {
		t.Fatalf("center queue %v misses the distributor / lone dealer campaigns", q)
	}

	n := len(out.events)
	centerCamp := f.ready(t, f.centerC)
	got, err = f.svc.Submit(f.ctx, f.centerC, centerCamp.UUID)
	if err != nil || got.Status != StatusApproved {
		t.Fatalf("center submit = %s, %v; want approved", got.Status, err)
	}
	evs := f.events(t, centerCamp.UUID)
	if len(evs) != 1 || evs[0].EventType != EventApproved || evs[0].FromStatus.String != StatusDraft ||
		evs[0].ToStatus.String != StatusApproved || !bytes.Contains(evs[0].Payload, []byte(`"auto": true`)) {
		t.Fatalf("center events = %+v", evs)
	}
	if len(out.events) != n {
		t.Fatal("an auto approval notifies nobody")
	}
	if slices.Contains(approvalUUIDs(t, f, centerQ), centerCamp.UUID) {
		t.Fatal("an auto-approved campaign is not queued")
	}
}

// TEC-406: submission runs the localization gate (422) and is draft only.
func TestCampaignSubmitGate(t *testing.T) {
	f := newFixture(t)
	f.customer(t, f.dealer, "de", "de", ptr(true))
	camp := f.ready(t, f.dealerC)
	var lm *LocaleMissingError
	if _, err := f.svc.Submit(f.ctx, f.dealerC, camp.UUID); !errors.As(err, &lm) || !slices.Equal(lm.Locales, []string{"de"}) {
		t.Fatalf("submit err = %v, want missing de", err)
	}
	if evs := f.events(t, camp.UUID); len(evs) != 0 {
		t.Fatalf("a refused submit wrote %d events", len(evs))
	}
	if _, err := f.svc.PutContent(f.ctx, f.dealerC, camp.UUID, "de", ContentInput{Body: "Hallo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Submit(f.ctx, f.dealerC, camp.UUID); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, err := f.svc.Submit(f.ctx, f.dealerC, camp.UUID); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("second submit err = %v, want invalid status", err)
	}
	if _, err := f.svc.Submit(f.ctx, f.otherDealerC, camp.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign submit err = %v, want not found", err)
	}
}

// TEC-406: reject and request-changes need a reason; request-changes
// returns the campaign to draft, reject closes it; the creator hears both.
func TestCampaignRejectAndRequestChanges(t *testing.T) {
	f := newFixture(t)
	out := f.withOutbox()
	distC := asCaller(f.distC, f.approver(t, f.dist, "owner", "distributor_owner"))

	camp := f.ready(t, f.dealerC)
	if _, err := f.svc.Submit(f.ctx, f.dealerC, camp.UUID); err != nil {
		t.Fatal(err)
	}
	var ve *ValidationError
	if _, err := f.svc.Reject(f.ctx, distC, camp.UUID, DecisionInput{Reason: "  "}); !errors.As(err, &ve) || ve.Field != "reason" {
		t.Fatalf("reject without reason err = %v", err)
	}
	if _, err := f.svc.RequestChanges(f.ctx, distC, camp.UUID, DecisionInput{}); !errors.As(err, &ve) {
		t.Fatalf("request-changes without reason err = %v", err)
	}
	got, err := f.svc.RequestChanges(f.ctx, distC, camp.UUID, DecisionInput{Reason: "Görsel ekleyin"})
	if err != nil || got.Status != StatusDraft || got.ApproverOrganizationUUID != nil {
		t.Fatalf("request changes = %s %v, %v", got.Status, got.ApproverOrganizationUUID, err)
	}
	ev := out.last(t, events.CampaignsChangesRequested)
	if ev.Payload["reason"] != "Görsel ekleyin" || !slices.Equal(notifyIDs(ev), []int64{f.dealerC.UserID}) {
		t.Fatalf("changes requested event = %+v", ev.Payload)
	}
	if last := got.Events[len(got.Events)-1]; last.Reason == nil || *last.Reason != "Görsel ekleyin" {
		t.Fatalf("history misses the reason: %+v", last)
	}
	// Back in draft: editable again, then resubmitted and rejected.
	if _, err := f.svc.Update(f.ctx, f.dealerC, camp.UUID, PatchInput{Name: ptr("Düzeltilmiş")}); err != nil {
		t.Fatalf("edit after request changes: %v", err)
	}
	if _, err := f.svc.Submit(f.ctx, f.dealerC, camp.UUID); err != nil {
		t.Fatal(err)
	}
	got, err = f.svc.Reject(f.ctx, distC, camp.UUID, DecisionInput{Reason: "Uygunsuz içerik"})
	if err != nil || got.Status != StatusRejected {
		t.Fatalf("reject = %s, %v", got.Status, err)
	}
	if ev := out.last(t, events.CampaignsRejected); ev.Payload["reason"] != "Uygunsuz içerik" {
		t.Fatalf("rejected event = %+v", ev.Payload)
	}
	if _, err := f.svc.Update(f.ctx, f.dealerC, camp.UUID, PatchInput{Name: ptr("x")}); !errors.Is(err, ErrNotDraft) {
		t.Fatalf("edit of a rejected campaign err = %v, want not draft", err)
	}
	if _, err := f.svc.Submit(f.ctx, f.dealerC, camp.UUID); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("submit of a rejected campaign err = %v", err)
	}
}

// TEC-406: a change of content or audience after the approval returns the
// campaign to draft (new approval needed); a no-op edit keeps it.
func TestCampaignEditAfterApprovalReturnsToDraft(t *testing.T) {
	f := newFixture(t)
	distC := asCaller(f.distC, f.approver(t, f.dist, "owner", "distributor_owner"))
	approve := func(id uuid.UUID) {
		t.Helper()
		if _, err := f.svc.Submit(f.ctx, f.dealerC, id); err != nil {
			t.Fatalf("submit: %v", err)
		}
		if _, err := f.svc.Approve(f.ctx, distC, id, DecisionInput{}); err != nil {
			t.Fatalf("approve: %v", err)
		}
	}
	camp := f.ready(t, f.dealerC)
	approve(camp.UUID)

	// Same values: still approved, no event.
	if got, err := f.svc.Update(f.ctx, f.dealerC, camp.UUID, PatchInput{Name: ptr(camp.Name)}); err != nil || got.Status != StatusApproved {
		t.Fatalf("no-op patch = %s, %v", got.Status, err)
	}
	if got, err := f.svc.PutContent(f.ctx, f.dealerC, camp.UUID, "tr", ContentInput{Body: "Merhaba"}); err != nil {
		t.Fatalf("no-op content: %v (%+v)", err, got)
	}
	if st := f.campaignRow(t, camp.UUID).Status; st != StatusApproved {
		t.Fatalf("no-op content status = %s", st)
	}
	before := len(f.events(t, camp.UUID))

	// Content change → draft with one event.
	if _, err := f.svc.PutContent(f.ctx, f.dealerC, camp.UUID, "tr", ContentInput{Body: "Yeni metin"}); err != nil {
		t.Fatalf("content: %v", err)
	}
	row := f.campaignRow(t, camp.UUID)
	if row.Status != StatusDraft || row.ApproverOrgID.Valid {
		t.Fatalf("after content change status = %s approver %v", row.Status, row.ApproverOrgID)
	}
	evs := f.events(t, camp.UUID)
	if len(evs) != before+1 || evs[len(evs)-1].EventType != EventChangesRequested ||
		evs[len(evs)-1].FromStatus.String != StatusApproved || evs[len(evs)-1].ToStatus.String != StatusDraft {
		t.Fatalf("events after content change = %+v", evs[before:])
	}
	if _, err := f.svc.Schedule(f.ctx, f.dealerC, camp.UUID, ScheduleInput{ScheduledAt: time.Now().Add(time.Hour).Format(time.RFC3339)}); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("schedule after edit err = %v, want invalid status (needs a new approval)", err)
	}

	// Audience change of a scheduled campaign → draft, schedule cleared.
	approve(camp.UUID)
	if _, err := f.svc.Schedule(f.ctx, f.dealerC, camp.UUID, ScheduleInput{ScheduledAt: time.Now().Add(time.Hour).Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Update(f.ctx, f.dealerC, camp.UUID, PatchInput{
		AudienceFilter: &AudienceFilter{AudienceType: AudienceCustomers, Locales: []string{"tr"}},
	}); err != nil {
		t.Fatalf("audience change: %v", err)
	}
	if row := f.campaignRow(t, camp.UUID); row.Status != StatusDraft || row.ScheduledAt.Valid {
		t.Fatalf("after audience change status = %s scheduled %v", row.Status, row.ScheduledAt)
	}

	// Media change → draft.
	approve(camp.UUID)
	if _, err := f.svc.AddMedia(f.ctx, f.dealerC, camp.UUID, "tr", MediaInput{Body: bytes.NewReader(pngBytes(10)), Filename: "a.png"}); err != nil {
		t.Fatalf("media: %v", err)
	}
	if st := f.campaignRow(t, camp.UUID).Status; st != StatusDraft {
		t.Fatalf("after media status = %s", st)
	}

	// Pending approval stays locked (409) while the approver decides.
	if _, err := f.svc.Submit(f.ctx, f.dealerC, camp.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.PutContent(f.ctx, f.dealerC, camp.UUID, "tr", ContentInput{Body: "x"}); !errors.Is(err, ErrNotDraft) {
		t.Fatalf("pending content err = %v, want not draft", err)
	}
}

// TEC-406: scheduling needs an approved campaign and a time at least five
// minutes ahead (422 otherwise); it is stored in UTC, a local time is read
// in the organization's time zone; send-now schedules at now.
func TestCampaignSchedule(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	f.svc.SetClock(func() time.Time { return now })
	camp := f.ready(t, f.centerC)

	at := now.Add(time.Hour).Format(time.RFC3339)
	if _, err := f.svc.Schedule(f.ctx, f.centerC, camp.UUID, ScheduleInput{ScheduledAt: at}); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("draft schedule err = %v, want invalid status", err)
	}
	if _, err := f.svc.Submit(f.ctx, f.centerC, camp.UUID); err != nil {
		t.Fatal(err)
	}
	for _, past := range []time.Time{now.Add(-time.Hour), now.Add(4 * time.Minute)} {
		if _, err := f.svc.Schedule(f.ctx, f.centerC, camp.UUID, ScheduleInput{ScheduledAt: past.Format(time.RFC3339)}); !errors.Is(err, ErrScheduleSoon) {
			t.Fatalf("schedule %s err = %v, want too soon", past, err)
		}
	}
	var ve *ValidationError
	if _, err := f.svc.Schedule(f.ctx, f.centerC, camp.UUID, ScheduleInput{ScheduledAt: "yarın"}); !errors.As(err, &ve) {
		t.Fatalf("bad time err = %v, want validation", err)
	}
	if _, err := f.svc.Schedule(f.ctx, f.centerC, camp.UUID, ScheduleInput{}); !errors.As(err, &ve) {
		t.Fatalf("missing time err = %v, want validation", err)
	}
	// 2026-10-07T15:30 in Europe/Istanbul (UTC+3) is 12:30 UTC.
	f.exec(t, `UPDATE organizations SET timezone = 'Europe/Istanbul' WHERE id = $1`, f.center.ID)
	got, err := f.svc.Schedule(f.ctx, f.centerC, camp.UUID, ScheduleInput{ScheduledAt: "2026-10-07T15:30"})
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	want := time.Date(2026, 10, 7, 12, 30, 0, 0, time.UTC)
	if got.Status != StatusScheduled || got.ScheduledAt == nil || !got.ScheduledAt.Equal(want) || got.Timezone != "Europe/Istanbul" {
		t.Fatalf("scheduled = %s %v tz %s, want %s", got.Status, got.ScheduledAt, got.Timezone, want)
	}
	// Reschedule exactly at the 5 minute edge.
	edge := now.Add(MinScheduleLead)
	if got, err = f.svc.Schedule(f.ctx, f.centerC, camp.UUID, ScheduleInput{ScheduledAt: edge.Format(time.RFC3339)}); err != nil || !got.ScheduledAt.Equal(edge) {
		t.Fatalf("reschedule = %v, %v", got.ScheduledAt, err)
	}
	if got, err = f.svc.SendNow(f.ctx, f.centerC, camp.UUID); err != nil || got.Status != StatusScheduled || !got.ScheduledAt.Equal(now) {
		t.Fatalf("send now = %s %v, %v", got.Status, got.ScheduledAt, err)
	}
	evs := f.events(t, camp.UUID)
	last := evs[len(evs)-1]
	if last.EventType != EventScheduled || !bytes.Contains(last.Payload, []byte(`"send_now": true`)) {
		t.Fatalf("send-now event = %s %s", last.EventType, last.Payload)
	}
	if _, err := f.svc.Schedule(f.ctx, f.otherDealerC, camp.UUID, ScheduleInput{ScheduledAt: at}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign schedule err = %v", err)
	}
}

// TEC-406: an organization whose contract ended (read only) cannot submit,
// schedule or send (422).
func TestCampaignReadOnlyOrganization(t *testing.T) {
	f := newFixture(t)
	draft := f.ready(t, f.centerC)
	approved := f.ready(t, f.centerC)
	if _, err := f.svc.Submit(f.ctx, f.centerC, approved.UUID); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE organizations SET status = 'read_only' WHERE id = $1`, f.center.ID)
	if _, err := f.svc.Submit(f.ctx, f.centerC, draft.UUID); !errors.Is(err, ErrOrgReadOnly) {
		t.Fatalf("submit err = %v, want read only", err)
	}
	if _, err := f.svc.Schedule(f.ctx, f.centerC, approved.UUID, ScheduleInput{ScheduledAt: time.Now().Add(time.Hour).Format(time.RFC3339)}); !errors.Is(err, ErrOrgReadOnly) {
		t.Fatalf("schedule err = %v, want read only", err)
	}
	if _, err := f.svc.SendNow(f.ctx, f.centerC, approved.UUID); !errors.Is(err, ErrOrgReadOnly) {
		t.Fatalf("send now err = %v, want read only", err)
	}
	if st := f.campaignRow(t, approved.UUID).Status; st != StatusApproved {
		t.Fatalf("status = %s", st)
	}
}

// TEC-406: cancel stops a campaign before sending entirely; while sending
// the remaining pending recipients are skipped; a finished campaign stays.
func TestCampaignCancel(t *testing.T) {
	f := newFixture(t)
	camp := f.ready(t, f.centerC)
	if _, err := f.svc.Submit(f.ctx, f.centerC, camp.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SendNow(f.ctx, f.centerC, camp.UUID); err != nil {
		t.Fatal(err)
	}
	if got, err := f.svc.Cancel(f.ctx, f.centerC, camp.UUID); err != nil || got.Status != StatusCancelled || got.FinishedAt == nil {
		t.Fatalf("cancel scheduled = %s, %v", got.Status, err)
	}
	if _, err := f.svc.Cancel(f.ctx, f.centerC, camp.UUID); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("second cancel err = %v", err)
	}

	sending := f.ready(t, f.centerC)
	row := f.campaignRow(t, sending.UUID)
	f.exec(t, `UPDATE campaigns SET status = 'sending', started_at = NOW() WHERE id = $1`, row.ID)
	a, b := f.user(t, "a", ""), f.user(t, "b", "")
	for i, u := range []db.User{a, b} {
		status := "pending"
		if i == 0 {
			status = "failed"
		}
		f.exec(t, `INSERT INTO campaign_recipients (campaign_id, organization_id, brand_id, user_id, channel, locale, target_address, status)
			VALUES ($1, $2, $3, $4, 'whatsapp', 'tr', '+905551112233', $5)`, row.ID, row.OrganizationID, row.BrandID, u.ID, status)
	}
	got, err := f.svc.Cancel(f.ctx, f.centerC, sending.UUID)
	if err != nil || got.Status != StatusCancelled {
		t.Fatalf("cancel sending = %s, %v", got.Status, err)
	}
	if got.RecipientsSkipped != 1 || got.RecipientsFailed != 1 {
		t.Fatalf("counters skipped %d failed %d", got.RecipientsSkipped, got.RecipientsFailed)
	}
	evs := f.events(t, sending.UUID)
	if len(evs) != 1 || evs[0].EventType != EventCancelled || evs[0].FromStatus.String != StatusSending ||
		!bytes.Contains(evs[0].Payload, []byte(`"skipped": 1`)) {
		t.Fatalf("cancel events = %+v", evs)
	}

	done := f.ready(t, f.centerC)
	f.exec(t, `UPDATE campaigns SET status = 'sent', finished_at = NOW() WHERE uuid = $1`, done.UUID)
	if _, err := f.svc.Cancel(f.ctx, f.centerC, done.UUID); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("cancel sent err = %v", err)
	}
}

// TEC-406: every status change writes exactly one campaign_events row with
// the matching from/to statuses; refused actions write none.
func TestCampaignEveryTransitionWritesOneEvent(t *testing.T) {
	f := newFixture(t)
	distC := asCaller(f.distC, f.approver(t, f.dist, "owner", "distributor_owner"))
	camp := f.ready(t, f.dealerC)
	future := time.Now().Add(time.Hour).Format(time.RFC3339)
	steps := []struct {
		run      func() error
		event    string
		from, to string
	}{
		{func() error { _, err := f.svc.Submit(f.ctx, f.dealerC, camp.UUID); return err }, EventSubmitted, StatusDraft, StatusPendingApproval},
		{func() error {
			_, err := f.svc.RequestChanges(f.ctx, distC, camp.UUID, DecisionInput{Reason: "Başlık"})
			return err
		}, EventChangesRequested, StatusPendingApproval, StatusDraft},
		{func() error { _, err := f.svc.Submit(f.ctx, f.dealerC, camp.UUID); return err }, EventSubmitted, StatusDraft, StatusPendingApproval},
		{func() error { _, err := f.svc.Approve(f.ctx, distC, camp.UUID, DecisionInput{}); return err }, EventApproved, StatusPendingApproval, StatusApproved},
		{func() error {
			_, err := f.svc.Schedule(f.ctx, f.dealerC, camp.UUID, ScheduleInput{ScheduledAt: future})
			return err
		}, EventScheduled, StatusApproved, StatusScheduled},
		{func() error {
			_, err := f.svc.Update(f.ctx, f.dealerC, camp.UUID, PatchInput{Name: ptr("Yeni")})
			return err
		}, EventChangesRequested, StatusScheduled, StatusDraft},
		{func() error { _, err := f.svc.Submit(f.ctx, f.dealerC, camp.UUID); return err }, EventSubmitted, StatusDraft, StatusPendingApproval},
		{func() error { _, err := f.svc.Approve(f.ctx, distC, camp.UUID, DecisionInput{}); return err }, EventApproved, StatusPendingApproval, StatusApproved},
		{func() error { _, err := f.svc.SendNow(f.ctx, f.dealerC, camp.UUID); return err }, EventScheduled, StatusApproved, StatusScheduled},
		{func() error { _, err := f.svc.Cancel(f.ctx, f.dealerC, camp.UUID); return err }, EventCancelled, StatusScheduled, StatusCancelled},
	}
	for i, st := range steps {
		before := f.events(t, camp.UUID)
		if err := st.run(); err != nil {
			t.Fatalf("step %d (%s): %v", i, st.event, err)
		}
		after := f.events(t, camp.UUID)
		if len(after) != len(before)+1 {
			t.Fatalf("step %d (%s): %d events written, want 1", i, st.event, len(after)-len(before))
		}
		e := after[len(after)-1]
		if e.EventType != st.event || e.FromStatus.String != st.from || e.ToStatus.String != st.to {
			t.Fatalf("step %d: event %s %s→%s, want %s %s→%s", i, e.EventType, e.FromStatus.String, e.ToStatus.String,
				st.event, st.from, st.to)
		}
		if e.ActorUserID.Int64 == 0 || !e.ActorOrgID.Valid {
			t.Fatalf("step %d: actor missing: %+v", i, e)
		}
		if row := f.campaignRow(t, camp.UUID); row.Status != st.to {
			t.Fatalf("step %d: status %s, want %s", i, row.Status, st.to)
		}
	}
	// Refused actions write nothing.
	n := len(f.events(t, camp.UUID))
	_, _ = f.svc.Submit(f.ctx, f.dealerC, camp.UUID)
	_, _ = f.svc.Approve(f.ctx, distC, camp.UUID, DecisionInput{})
	_, _ = f.svc.Cancel(f.ctx, f.dealerC, camp.UUID)
	if got := len(f.events(t, camp.UUID)); got != n {
		t.Fatalf("refused actions wrote %d events", got-n)
	}
}
