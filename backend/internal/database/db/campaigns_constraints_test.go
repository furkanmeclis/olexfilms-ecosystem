package db_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-404: database-level guards of the campaign schema (migration 000107):
// channels, recipient snapshot uniqueness and its statistics projection,
// append-only events, contents/media, the marketing consent reachability
// read and the list queries. Reuses the order fixture (center > dist >
// dealer, dealer2; rolled-back transaction, savepoint per failure).

func (f *orderFixture) campaignUser(t *testing.T, phone string) db.User {
	t.Helper()
	f.seq++
	arg := db.CreateUserParams{
		PasswordHash: "x", Name: "Kampanya", Surname: fmt.Sprintf("T404-%d", f.seq), Status: "active",
		Email: text(fmt.Sprintf("t404-%d-%d@example.test", time.Now().UnixNano(), f.seq)),
	}
	if phone != "" {
		arg.PhoneE164 = text(phone)
	}
	u, err := f.q.CreateUser(f.ctx, arg)
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	return u
}

func (f *orderFixture) campaign(t *testing.T, org db.Organization, name string, channels ...string) db.Campaign {
	t.Helper()
	c, err := f.q.CreateCampaign(f.ctx, db.CreateCampaignParams{
		OrganizationID: org.ID, BrandID: org.BrandID, Name: name, Channels: channels,
		AudienceFilter: []byte(`{"audience":"customers"}`),
	})
	if err != nil {
		t.Fatalf("campaign %s: %v", name, err)
	}
	return c
}

func recipientParams(c db.Campaign, u db.User, channel string) db.InsertCampaignRecipientParams {
	arg := db.InsertCampaignRecipientParams{
		CampaignID: c.ID, OrganizationID: c.OrganizationID, BrandID: c.BrandID, UserID: u.ID,
		Channel: channel, Locale: "tr", Status: "pending",
	}
	switch channel {
	case "push":
		arg.PushTokenCount = pgtype.Int4{Int32: 2, Valid: true}
	case "whatsapp":
		arg.TargetAddress = u.PhoneE164
	case "email":
		arg.TargetAddress = u.Email
	}
	return arg
}

func (f *orderFixture) recipient(t *testing.T, c db.Campaign, u db.User, channel string) db.CampaignRecipient {
	t.Helper()
	r, err := f.q.InsertCampaignRecipient(f.ctx, recipientParams(c, u, channel))
	if err != nil {
		t.Fatalf("recipient %s: %v", channel, err)
	}
	return r
}

func phoneFor(seq int) string { return fmt.Sprintf("+90555%07d", (time.Now().UnixNano()/1000+int64(seq))%10000000) }

func TestCampaignSchemaConstraints(t *testing.T) {
	f := newOrderFixture(t)
	ctx := f.ctx

	t.Run("channels: known, non-empty, unique; no sms", func(t *testing.T) {
		for name, ch := range map[string][]string{
			"empty": {}, "sms": {"sms"}, "duplicate": {"push", "push"},
			"duplicate third": {"push", "email", "push"}, "four": {"push", "email", "whatsapp", "email"},
		} {
			f.expectConstraint(t, name, "23514", "chk_campaigns_channels", func(sp pgx.Tx) error {
				_, err := db.New(sp).CreateCampaign(ctx, db.CreateCampaignParams{
					OrganizationID: f.dealer.ID, BrandID: f.dealer.BrandID, Name: "x", Channels: ch,
					AudienceFilter: []byte(`{}`),
				})
				return err
			})
		}
		c := f.campaign(t, f.dealer, "all channels", "push", "whatsapp", "email")
		if c.Status != "draft" || len(c.Channels) != 3 || c.RecipientsTotal != 0 {
			t.Fatalf("new campaign = %+v", c)
		}
	})

	t.Run("pending_approval needs an approver, scheduled a time", func(t *testing.T) {
		c := f.campaign(t, f.dealer, "approval", "push")
		f.expectConstraint(t, "pending without approver", "23514", "chk_campaigns_approver", func(sp pgx.Tx) error {
			_, err := db.New(sp).SetCampaignStatus(ctx, db.SetCampaignStatusParams{
				ID: c.ID, FromStatus: "draft", Status: "pending_approval",
			})
			return err
		})
		f.expectConstraint(t, "scheduled without time", "23514", "chk_campaigns_scheduled", func(sp pgx.Tx) error {
			_, err := db.New(sp).SetCampaignStatus(ctx, db.SetCampaignStatusParams{
				ID: c.ID, FromStatus: "draft", Status: "scheduled",
			})
			return err
		})
		got, err := f.q.SetCampaignStatus(ctx, db.SetCampaignStatusParams{
			ID: c.ID, FromStatus: "draft", Status: "pending_approval",
			ApproverOrgID: pgtype.Int8{Int64: f.dist.ID, Valid: true},
		})
		if err != nil || got.ApproverOrgID.Int64 != f.dist.ID {
			t.Fatalf("submit = %+v, %v", got, err)
		}
		// A stale from_status moves nothing.
		if _, err := f.q.SetCampaignStatus(ctx, db.SetCampaignStatusParams{
			ID: c.ID, FromStatus: "draft", Status: "approved",
		}); err != pgx.ErrNoRows {
			t.Fatalf("stale transition: %v", err)
		}
	})

	t.Run("recipient unique per campaign, user and channel", func(t *testing.T) {
		c := f.campaign(t, f.dealer, "unique", "push", "whatsapp")
		u := f.campaignUser(t, phoneFor(f.seq))
		f.recipient(t, c, u, "push")
		f.expectConstraint(t, "same user and channel", "23505", "uq_campaign_recipients_user_channel", func(sp pgx.Tx) error {
			_, err := db.New(sp).InsertCampaignRecipient(ctx, recipientParams(c, u, "push"))
			return err
		})
		// Another channel of the same user and the same user in another
		// campaign are separate rows.
		f.recipient(t, c, u, "whatsapp")
		f.recipient(t, f.campaign(t, f.dealer, "unique 2", "push"), u, "push")
	})

	t.Run("recipient target matches the channel", func(t *testing.T) {
		c := f.campaign(t, f.dealer, "targets", "push", "whatsapp", "email")
		u := f.campaignUser(t, phoneFor(f.seq))
		for name, mut := range map[string]func(*db.InsertCampaignRecipientParams){
			"push without token count": func(a *db.InsertCampaignRecipientParams) {
				a.PushTokenCount = pgtype.Int4{}
			},
			"push with address": func(a *db.InsertCampaignRecipientParams) { a.TargetAddress = text("+905551112233") },
			"whatsapp not E.164": func(a *db.InsertCampaignRecipientParams) {
				a.Channel, a.PushTokenCount, a.TargetAddress = "whatsapp", pgtype.Int4{}, text("05551112233")
			},
			"email without @": func(a *db.InsertCampaignRecipientParams) {
				a.Channel, a.PushTokenCount, a.TargetAddress = "email", pgtype.Int4{}, text("nobody")
			},
		} {
			arg := recipientParams(c, u, "push")
			mut(&arg)
			f.expectConstraint(t, name, "23514", "chk_campaign_recipients_target", func(sp pgx.Tx) error {
				_, err := db.New(sp).InsertCampaignRecipient(ctx, arg)
				return err
			})
		}
		f.expectConstraint(t, "sms channel", "23514", "chk_campaign_recipients_channel", func(sp pgx.Tx) error {
			arg := recipientParams(c, u, "push")
			arg.Channel = "sms"
			_, err := db.New(sp).InsertCampaignRecipient(ctx, arg)
			return err
		})
		f.expectConstraint(t, "sent without sent_at", "23514", "chk_campaign_recipients_sent_at", func(sp pgx.Tx) error {
			arg := recipientParams(c, u, "push")
			arg.Status = "sent"
			_, err := db.New(sp).InsertCampaignRecipient(ctx, arg)
			return err
		})
		// The recipient must belong to the campaign's organization.
		f.expectCode(t, "foreign organization", func(sp pgx.Tx) error {
			arg := recipientParams(c, u, "push")
			arg.OrganizationID = f.dealer2.ID
			_, err := db.New(sp).InsertCampaignRecipient(ctx, arg)
			return err
		}, "23503")
	})

	t.Run("statistics projection follows the snapshot", func(t *testing.T) {
		c := f.campaign(t, f.dealer, "stats", "push", "email")
		var rs []db.CampaignRecipient
		for i := 0; i < 3; i++ {
			u := f.campaignUser(t, "")
			rs = append(rs, f.recipient(t, c, u, "email"))
		}
		// Bulk insert in one statement (snapshot shape).
		u4, u5 := f.campaignUser(t, ""), f.campaignUser(t, "")
		if _, err := f.tx.Exec(ctx, `
			INSERT INTO campaign_recipients (campaign_id, organization_id, brand_id, user_id, channel, locale,
			                                 push_token_count, status, reason)
			VALUES ($1, $2, $3, $4, 'push', 'en', 1, 'pending', NULL),
			       ($1, $2, $3, $5, 'push', 'de', 0, 'skipped', 'no_token')`,
			c.ID, c.OrganizationID, c.BrandID, u4.ID, u5.ID); err != nil {
			t.Fatal(err)
		}
		check := func(step string, total, sent, failed, skipped int32) {
			t.Helper()
			got, err := f.q.GetCampaignByUUID(ctx, db.GetCampaignByUUIDParams{Uuid: c.Uuid, BrandID: c.BrandID})
			if err != nil {
				t.Fatal(err)
			}
			if got.RecipientsTotal != total || got.RecipientsSent != sent ||
				got.RecipientsFailed != failed || got.RecipientsSkipped != skipped {
				t.Fatalf("%s: counters = %d/%d/%d/%d, want %d/%d/%d/%d", step,
					got.RecipientsTotal, got.RecipientsSent, got.RecipientsFailed, got.RecipientsSkipped,
					total, sent, failed, skipped)
			}
		}
		check("inserted", 5, 0, 0, 1)

		sent, err := f.q.SetCampaignRecipientStatus(ctx, db.SetCampaignRecipientStatusParams{ID: rs[0].ID, Status: "sent"})
		if err != nil || !sent.SentAt.Valid || sent.Attempts != 1 {
			t.Fatalf("sent = %+v, %v", sent, err)
		}
		if _, err := f.q.SetCampaignRecipientStatus(ctx, db.SetCampaignRecipientStatusParams{
			ID: rs[1].ID, Status: "failed", Reason: text("bounce"),
		}); err != nil {
			t.Fatal(err)
		}
		// An ended recipient does not move again.
		if _, err := f.q.SetCampaignRecipientStatus(ctx, db.SetCampaignRecipientStatusParams{
			ID: rs[0].ID, Status: "failed",
		}); err != pgx.ErrNoRows {
			t.Fatalf("second end: %v", err)
		}
		check("sent+failed", 5, 1, 1, 1)

		n, err := f.q.SkipPendingCampaignRecipients(ctx, db.SkipPendingCampaignRecipientsParams{
			CampaignID: c.ID, Reason: "cancelled",
		})
		if err != nil || n != 2 {
			t.Fatalf("skip pending = %d, %v", n, err)
		}
		check("cancelled", 5, 1, 1, 3)

		if _, err := f.tx.Exec(ctx, `DELETE FROM campaign_recipients WHERE id = $1`, rs[1].ID); err != nil {
			t.Fatal(err)
		}
		check("deleted failed", 4, 1, 0, 3)
	})

	t.Run("events are append-only and leave with their campaign", func(t *testing.T) {
		c := f.campaign(t, f.dealer, "events", "push")
		ev, err := f.q.InsertCampaignEvent(ctx, db.InsertCampaignEventParams{
			CampaignID: c.ID, OrganizationID: c.OrganizationID, BrandID: c.BrandID,
			EventType: "changes_requested", FromStatus: text("pending_approval"), ToStatus: text("draft"),
			ActorOrgID: pgtype.Int8{Int64: f.dist.ID, Valid: true}, Reason: text("Görseli değiştirin"),
			Payload: []byte(`{}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		f.expectConstraint(t, "rejected without reason", "23514", "chk_campaign_events_reason", func(sp pgx.Tx) error {
			_, err := db.New(sp).InsertCampaignEvent(ctx, db.InsertCampaignEventParams{
				CampaignID: c.ID, OrganizationID: c.OrganizationID, BrandID: c.BrandID,
				EventType: "rejected", Payload: []byte(`{}`),
			})
			return err
		})
		f.expectConstraint(t, "unknown type", "23514", "chk_campaign_events_type", func(sp pgx.Tx) error {
			_, err := db.New(sp).InsertCampaignEvent(ctx, db.InsertCampaignEventParams{
				CampaignID: c.ID, OrganizationID: c.OrganizationID, BrandID: c.BrandID,
				EventType: "edited", Payload: []byte(`{}`),
			})
			return err
		})
		f.expectCode(t, "update event", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE campaign_events SET reason = 'x' WHERE id = $1`, ev.ID)
			return err
		}, "23001")
		f.expectCode(t, "delete event", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `DELETE FROM campaign_events WHERE id = $1`, ev.ID)
			return err
		}, "23001")
		f.expectCode(t, "event of a foreign organization", func(sp pgx.Tx) error {
			_, err := db.New(sp).InsertCampaignEvent(ctx, db.InsertCampaignEventParams{
				CampaignID: c.ID, OrganizationID: f.dealer2.ID, BrandID: c.BrandID,
				EventType: "submitted", Payload: []byte(`{}`),
			})
			return err
		}, "23503")

		// Deleting the draft takes its events and contents along.
		if _, err := f.q.UpsertCampaignContent(ctx, db.UpsertCampaignContentParams{
			CampaignID: c.ID, Locale: "tr", Title: "Başlık", Body: "Metin",
		}); err != nil {
			t.Fatal(err)
		}
		n, err := f.q.DeleteDraftCampaign(ctx, c.ID)
		if err != nil || n != 1 {
			t.Fatalf("delete draft = %d, %v", n, err)
		}
		evs, err := f.q.ListCampaignEvents(ctx, c.ID)
		if err != nil || len(evs) != 0 {
			t.Fatalf("events after delete = %d, %v", len(evs), err)
		}
	})

	t.Run("contents unique per locale; media kind and mime", func(t *testing.T) {
		c := f.campaign(t, f.dealer, "contents", "push", "email")
		first, err := f.q.UpsertCampaignContent(ctx, db.UpsertCampaignContentParams{
			CampaignID: c.ID, Locale: "de", Title: "Titel", Body: "Text",
		})
		if err != nil {
			t.Fatal(err)
		}
		second, err := f.q.UpsertCampaignContent(ctx, db.UpsertCampaignContentParams{
			CampaignID: c.ID, Locale: "de", Title: "Titel 2", Body: "Text 2", Deeplink: text("olex://campaign"),
		})
		if err != nil || second.ID != first.ID || second.Title != "Titel 2" {
			t.Fatalf("upsert = %+v, %v", second, err)
		}
		f.expectConstraint(t, "unknown locale", "23514", "chk_campaign_contents_locale", func(sp pgx.Tx) error {
			_, err := db.New(sp).UpsertCampaignContent(ctx, db.UpsertCampaignContentParams{
				CampaignID: c.ID, Locale: "xx", Body: "x",
			})
			return err
		})
		f.expectConstraint(t, "pdf as image", "23514", "chk_campaign_media_mime", func(sp pgx.Tx) error {
			_, err := db.New(sp).InsertCampaignMedia(ctx, db.InsertCampaignMediaParams{
				ContentID: first.ID, Kind: "image", StorageKey: "campaigns/x.pdf",
				MimeType: "application/pdf", SizeBytes: 10,
			})
			return err
		})
		m, err := f.q.InsertCampaignMedia(ctx, db.InsertCampaignMediaParams{
			ContentID: first.ID, Kind: "document", StorageKey: "campaigns/x.pdf",
			MimeType: "application/pdf", SizeBytes: 10, FileName: text("katalog.pdf"),
		})
		if err != nil {
			t.Fatal(err)
		}
		media, err := f.q.ListCampaignMedia(ctx, c.ID)
		if err != nil || len(media) != 1 || media[0].Uuid != m.Uuid || media[0].Locale != "de" {
			t.Fatalf("media = %+v, %v", media, err)
		}
	})
}

// marketing_consent: seeded text, latest decision wins, phone opt-out read
// from contact_opt_out_state.
func TestCampaignMarketingReachability(t *testing.T) {
	f := newOrderFixture(t)
	ctx := f.ctx

	texts := map[string]db.LegalText{}
	for _, l := range []string{"tr", "en"} {
		lt, err := f.q.GetLatestLegalText(ctx, db.GetLatestLegalTextParams{Kind: "marketing_consent", Locale: l})
		if err != nil {
			t.Fatalf("marketing_consent %s seed: %v", l, err)
		}
		texts[l] = lt
	}
	v2, err := f.q.InsertLegalText(ctx, db.InsertLegalTextParams{
		Kind: "marketing_consent", Locale: "tr", Body: "v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	decide := func(u db.User, lt db.LegalText, accepted bool) {
		t.Helper()
		if _, err := f.q.InsertConsent(ctx, db.InsertConsentParams{
			UserID: u.ID, LegalTextID: lt.ID, Kind: "marketing_consent", Locale: lt.Locale,
			TextVersion: lt.Version, Accepted: accepted,
		}); err != nil {
			t.Fatal(err)
		}
	}

	accepted := f.campaignUser(t, phoneFor(1))
	decide(accepted, texts["tr"], true)
	optedOut := f.campaignUser(t, phoneFor(2))
	decide(optedOut, texts["en"], true)
	if _, err := f.q.InsertContactOptOut(ctx, db.InsertContactOptOutParams{
		ContactE164: optedOut.PhoneE164.String, Scope: "marketing", Action: "out", Source: "whatsapp",
	}); err != nil {
		t.Fatal(err)
	}
	withdrawn := f.campaignUser(t, phoneFor(3))
	decide(withdrawn, texts["tr"], true)
	if _, err := f.tx.Exec(ctx, `UPDATE consents SET decided_at = NOW() - interval '1 day' WHERE user_id = $1`, withdrawn.ID); err != nil {
		t.Fatal(err)
	}
	decide(withdrawn, v2, false)
	silent := f.campaignUser(t, "")
	// An ai_guidelines acceptance is not a marketing consent.
	aiText, err := f.q.GetLatestLegalText(ctx, db.GetLatestLegalTextParams{Kind: "ai_guidelines", Locale: "tr"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.InsertConsent(ctx, db.InsertConsentParams{
		UserID: silent.ID, LegalTextID: aiText.ID, Kind: "ai_guidelines", Locale: "tr",
		TextVersion: aiText.Version, Accepted: true,
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := f.q.ListMarketingReachability(ctx, []int64{accepted.ID, optedOut.ID, withdrawn.ID, silent.ID})
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64]db.ListMarketingReachabilityRow{}
	for _, r := range rows {
		got[r.UserID] = r
	}
	want := map[int64][2]bool{
		accepted.ID: {true, false}, optedOut.ID: {true, true},
		withdrawn.ID: {false, false}, silent.ID: {false, false},
	}
	for id, w := range want {
		if r, ok := got[id]; !ok || r.MarketingAccepted != w[0] || r.MarketingOptedOut != w[1] {
			t.Fatalf("user %d = %+v, want accepted=%v opted_out=%v", id, r, w[0], w[1])
		}
	}

	f.expectConstraint(t, "unknown consent kind", "23514", "chk_consents_kind", func(sp pgx.Tx) error {
		_, err := db.New(sp).InsertConsent(ctx, db.InsertConsentParams{
			UserID: silent.ID, LegalTextID: texts["tr"].ID, Kind: "newsletter", Locale: "tr", TextVersion: 1,
		})
		return err
	})
}

func TestCampaignListQueries(t *testing.T) {
	f := newOrderFixture(t)
	ctx := f.ctx
	center, err := f.q.GetOrganizationByID(ctx, f.centerID)
	if err != nil {
		t.Fatal(err)
	}
	tag := fmt.Sprintf("T404-%d", time.Now().UnixNano())
	f.campaign(t, f.dealer, tag+" Alfa", "push")
	b := f.campaign(t, f.dealer, tag+" Bravo", "whatsapp", "email")
	c := f.campaign(t, f.dealer2, tag+" Charlie", "email")
	d := f.campaign(t, center, tag+" Delta", "push")
	if _, err := f.q.SetCampaignStatus(ctx, db.SetCampaignStatusParams{
		ID: b.ID, FromStatus: "draft", Status: "pending_approval",
		ApproverOrgID: pgtype.Int8{Int64: f.dist.ID, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.SetCampaignStatus(ctx, db.SetCampaignStatusParams{
		ID: c.ID, FromStatus: "draft", Status: "pending_approval",
		ApproverOrgID: pgtype.Int8{Int64: f.dist.ID, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	if _, err := f.q.SetCampaignStatus(ctx, db.SetCampaignStatusParams{
		ID: d.ID, FromStatus: "draft", Status: "scheduled", ScheduledAt: ts(at),
	}); err != nil {
		t.Fatal(err)
	}

	names := func(cs []db.Campaign) string {
		out := []string{}
		for _, c := range cs {
			out = append(out, c.Name[len(tag)+1:])
		}
		return fmt.Sprint(out)
	}
	list := func(arg db.ListCampaignsParams) string {
		t.Helper()
		arg.BrandID, arg.Q, arg.PageLimit = f.brandID, text(tag), 50
		if arg.SortKey == "" {
			arg.SortKey, arg.SortDesc = "created_at", true
		}
		cs, err := f.q.ListCampaigns(ctx, arg)
		if err != nil {
			t.Fatal(err)
		}
		n, err := f.q.CountCampaigns(ctx, db.CountCampaignsParams{
			BrandID: arg.BrandID, OrganizationIds: arg.OrganizationIds, Statuses: arg.Statuses,
			Channels: arg.Channels, ScheduledFrom: arg.ScheduledFrom, ScheduledTo: arg.ScheduledTo, Q: arg.Q,
		})
		if err != nil || int(n) != len(cs) {
			t.Fatalf("count = %d (%v), list = %d", n, err, len(cs))
		}
		return names(cs)
	}
	cases := []struct {
		name string
		arg  db.ListCampaignsParams
		want string
	}{
		{"default -created_at", db.ListCampaignsParams{}, "[Delta Charlie Bravo Alfa]"},
		{"name asc", db.ListCampaignsParams{SortKey: "name"}, "[Alfa Bravo Charlie Delta]"},
		{"status flow rank", db.ListCampaignsParams{SortKey: "status"}, "[Alfa Bravo Charlie Delta]"},
		{"scheduled_at nulls last", db.ListCampaignsParams{SortKey: "scheduled_at"}, "[Delta Alfa Bravo Charlie]"},
		{"dealer scope", db.ListCampaignsParams{OrganizationIds: []int64{f.dealer.ID}}, "[Bravo Alfa]"},
		{"statuses", db.ListCampaignsParams{Statuses: []string{"pending_approval", "scheduled"}}, "[Delta Charlie Bravo]"},
		{"channels overlap", db.ListCampaignsParams{Channels: []string{"email"}}, "[Charlie Bravo]"},
		{"scheduled range", db.ListCampaignsParams{
			ScheduledFrom: ts(at.Add(-time.Hour)), ScheduledTo: ts(at.Add(time.Hour)),
		}, "[Delta]"},
	}
	for _, tc := range cases {
		if got := list(tc.arg); got != tc.want {
			t.Fatalf("%s = %s, want %s", tc.name, got, tc.want)
		}
	}

	queue, err := f.q.ListCampaignApprovals(ctx, db.ListCampaignApprovalsParams{
		BrandID: f.brandID, ApproverOrgIds: []int64{f.dist.ID}, Q: text(tag),
		SortKey: "created_at", PageLimit: 50,
	})
	if err != nil || names(queue) != "[Bravo Charlie]" {
		t.Fatalf("dist queue = %s, %v", names(queue), err)
	}
	n, err := f.q.CountCampaignApprovals(ctx, db.CountCampaignApprovalsParams{
		BrandID: f.brandID, ApproverOrgIds: []int64{f.centerID}, Q: text(tag),
	})
	if err != nil || n != 0 {
		t.Fatalf("center queue = %d, %v", n, err)
	}

	// Recipient list filters.
	u1, u2 := f.campaignUser(t, phoneFor(7)), f.campaignUser(t, "")
	f.recipient(t, b, u1, "whatsapp")
	f.recipient(t, b, u1, "email")
	r3 := recipientParams(b, u2, "email")
	r3.Locale = "de"
	if _, err := f.q.InsertCampaignRecipient(ctx, r3); err != nil {
		t.Fatal(err)
	}
	rec := func(arg db.ListCampaignRecipientsParams) int {
		t.Helper()
		arg.CampaignID, arg.PageLimit = b.ID, 50
		if arg.SortKey == "" {
			arg.SortKey = "created_at"
		}
		rs, err := f.q.ListCampaignRecipients(ctx, arg)
		if err != nil {
			t.Fatal(err)
		}
		cnt, err := f.q.CountCampaignRecipients(ctx, db.CountCampaignRecipientsParams{
			CampaignID: arg.CampaignID, Statuses: arg.Statuses, Channels: arg.Channels, Locales: arg.Locales, Q: arg.Q,
		})
		if err != nil || int(cnt) != len(rs) {
			t.Fatalf("recipient count = %d (%v), list = %d", cnt, err, len(rs))
		}
		return len(rs)
	}
	if got := rec(db.ListCampaignRecipientsParams{}); got != 3 {
		t.Fatalf("all recipients = %d", got)
	}
	if got := rec(db.ListCampaignRecipientsParams{Channels: []string{"email"}}); got != 2 {
		t.Fatalf("email recipients = %d", got)
	}
	if got := rec(db.ListCampaignRecipientsParams{Locales: []string{"de"}, SortKey: "locale"}); got != 1 {
		t.Fatalf("de recipients = %d", got)
	}
	if got := rec(db.ListCampaignRecipientsParams{Statuses: []string{"sent"}}); got != 0 {
		t.Fatalf("sent recipients = %d", got)
	}
	if got := rec(db.ListCampaignRecipientsParams{Q: u1.PhoneE164}); got != 1 {
		t.Fatalf("q by phone = %d", got)
	}
}
