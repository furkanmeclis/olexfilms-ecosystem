package usecase

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/providers"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/mail"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type captureMail struct {
	mu   sync.Mutex
	msgs []mail.Message
}

func (c *captureMail) Send(_ context.Context, m mail.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, m)
	return nil
}

type captureSMS struct {
	mu   sync.Mutex
	sent []string // "to|body"
}

func (c *captureSMS) Name() string { return "test-sms" }

func (c *captureSMS) Send(_ context.Context, to, body string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, to+"|"+body)
	return fmt.Sprintf("sms-%d", len(c.sent)), nil
}

type centerTest struct {
	t      *testing.T
	ctx    context.Context
	pool   *pgxpool.Pool
	q      *db.Queries
	svc    *Service
	pub    *fakePublisher
	mail   *captureMail
	sms    *captureSMS
	suffix string
	event  string
}

func newCenterTest(t *testing.T) *centerTest {
	t.Helper()
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
	ct := &centerTest{
		t: t, ctx: ctx, pool: pool, q: db.New(pool),
		pub: &fakePublisher{}, mail: &captureMail{}, sms: &captureSMS{},
		suffix: fmt.Sprintf("%d", time.Now().UnixNano()),
	}
	ct.event = "test.tec87." + ct.suffix
	catalog.Register(catalog.Event{
		Code: ct.event, Module: "test",
		DefaultChannels: []string{catalog.ChannelInapp, catalog.ChannelEmail, catalog.ChannelSMS},
		Placeholders:    []msgtemplate.Placeholder{{Key: "item", SampleTR: "x", SampleEN: "x"}},
	})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ct.svc = New(ct.q, nil, []providers.Provider{
		providers.InappProvider{Pub: ct.pub},
		providers.EmailProvider{Mail: ct.mail},
		providers.SMSProvider{SMS: ct.sms},
	}, log)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM notification_templates WHERE code = $1", ct.event)
		ct.setSMS(false)
	})
	return ct
}

func (ct *centerTest) exec(sql string, args ...any) {
	ct.t.Helper()
	if _, err := ct.pool.Exec(ct.ctx, sql, args...); err != nil {
		ct.t.Fatalf("%s: %v", sql, err)
	}
}

func (ct *centerTest) setSMS(on bool) {
	_, _ = ct.pool.Exec(ct.ctx, "UPDATE notification_channel_settings SET enabled = $1 WHERE channel = 'sms'", on)
}

func (ct *centerTest) org(name, typ, locale string, parentID, brandID int64) db.Organization {
	ct.t.Helper()
	row, err := ct.q.CreateOrganization(ct.ctx, db.CreateOrganizationParams{
		Slug: "t87-" + name + "-" + ct.suffix, Name: name + " " + ct.suffix, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ, ParentID: pgtype.Int8{Int64: parentID, Valid: true},
		BrandID: brandID, Currency: "TRY", Locale: locale, Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		ct.t.Fatalf("create org %s: %v", name, err)
	}
	ct.t.Cleanup(func() {
		_, _ = ct.pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", row.ID)
	})
	return row
}

// user creates a user with an optional locale (NULL inherits), a phone and
// an optional membership.
func (ct *centerTest) user(name string, locale *string, phone string, orgID int64) db.User {
	ct.t.Helper()
	var id int64
	err := ct.pool.QueryRow(ct.ctx, `INSERT INTO users (email, password_hash, name, surname, status, locale, phone_e164)
		VALUES ($1, 'x', $2, 'T87', 'active', $3, $4) RETURNING id`,
		name+"-"+ct.suffix+"@t87.example.com", name, locale, phone).Scan(&id)
	if err != nil {
		ct.t.Fatalf("create user %s: %v", name, err)
	}
	ct.t.Cleanup(func() {
		_, _ = ct.pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id)
	})
	if orgID > 0 {
		ct.exec("INSERT INTO organization_members (organization_id, user_id, role) VALUES ($1, $2, 'owner')", orgID, id)
	}
	u, err := ct.q.GetUserByID(ct.ctx, id)
	if err != nil {
		ct.t.Fatal(err)
	}
	return u
}

func (ct *centerTest) template(role, channel, lang, subject, body string) {
	ct.t.Helper()
	ct.exec(`INSERT INTO notification_templates (code, role, channel, language, subject, body, format)
		VALUES ($1, $2, $3, $4, $5, $6, 'markdown')`, ct.event, role, channel, lang, subject, body)
}

type deliveryRow struct {
	userID                        int64
	channel, status, role, lang   string
	title, body, notificationLang string
}

func (ct *centerTest) deliveries(eventID uuid.UUID) map[string]deliveryRow {
	ct.t.Helper()
	rows, err := ct.pool.Query(ct.ctx, `
		SELECT d.user_id, d.channel, d.status, COALESCE(d.role, ''), COALESCE(d.language, ''),
		       COALESCE(n.title, ''), COALESCE(n.body, ''), COALESCE(n.language, '')
		FROM notification_deliveries d
		LEFT JOIN notifications n ON n.delivery_id = d.id
		WHERE d.event_id = $1`, eventID)
	if err != nil {
		ct.t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]deliveryRow{}
	for rows.Next() {
		var r deliveryRow
		if err := rows.Scan(&r.userID, &r.channel, &r.status, &r.role, &r.lang, &r.title, &r.body, &r.notificationLang); err != nil {
			ct.t.Fatal(err)
		}
		out[fmt.Sprintf("%d/%s", r.userID, r.channel)] = r
	}
	return out
}

func strp(s string) *string { return &s }

// Acceptance 1-5: one event in three languages to three users of three
// roles over the right channels; SMS switch; Centrifugo publish;
// idempotency; 90-day sweep.
func TestIntegrationNotificationCenter(t *testing.T) {
	ct := newCenterTest(t)
	ctx := ct.ctx
	ct.setSMS(false)

	brand, err := ct.q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatal(err)
	}
	center, err := ct.q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatal(err)
	}
	dist := ct.org("dist", "distributor", "ar", center.ID, brand.ID)
	dealer := ct.org("dealer", "dealer", "tr", dist.ID, brand.ID)

	customer := ct.user("customer", strp("tr"), "+905550000101", 0)      // customer, tr
	dealerU := ct.user("dealer", strp("en"), "+905550000102", dealer.ID) // dealer, en
	distU := ct.user("dist", nil, "+905550000103", dist.ID)              // distributor, inherits ar

	// Customer muted e-mail for every event.
	ct.exec("INSERT INTO notification_preferences (user_id, event_code, channel, enabled) VALUES ($1, NULL, 'email', FALSE)", customer.ID)

	ct.template("generic", "inapp", "tr", "TR genel", "Merhaba {{item}}")
	ct.template("generic", "inapp", "en", "EN generic", "Hello {{item}}")
	ct.template("dealer", "inapp", "en", "EN dealer", "Dealer hello {{item}}")
	ct.template("distributor", "inapp", "ar", "AR موزع", "مرحبا {{item}}")
	ct.template("generic", "email", "tr", "TR e-posta", "**{{item}}** hazır")
	ct.template("generic", "email", "en", "EN email", "**{{item}}** is ready")
	ct.template("generic", "email", "ar", "AR بريد", "**{{item}}** جاهز")
	ct.template("generic", "sms", "tr", "", "SMS TR {{item}}")
	ct.template("generic", "sms", "en", "", "SMS EN {{item}}")
	ct.template("generic", "sms", "ar", "", "SMS AR {{item}}")

	users := []int64{customer.ID, dealerU.ID, distU.ID}
	orgID := dealer.ID
	in := model.DispatchInput{
		EventID: uuid.New(), EventCode: ct.event, OrganizationID: &orgID,
		UserIDs: users, Vars: map[string]string{"item": "PPF"},
	}
	res, err := ct.svc.Dispatch(ctx, in)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if res.Queued != 5 || res.Skipped != 4 || res.Duplicates != 0 {
		t.Fatalf("result = %+v, want 5 queued (3 inapp + 2 email) / 4 skipped (muted email + 3 sms)", res)
	}

	got := ct.deliveries(in.EventID)
	if len(got) != 9 {
		t.Fatalf("deliveries = %d, want 9: %+v", len(got), got)
	}
	expect := []struct {
		user                        db.User
		channel, status, role, lang string
		title, bodyHas              string
	}{
		{customer, "inapp", "delivered", "customer", "tr", "TR genel", "Merhaba PPF"},
		{customer, "email", model.DeliverySkippedPreference, "customer", "", "", ""},
		{customer, "sms", model.DeliverySkippedDisabled, "customer", "", "", ""},
		{dealerU, "inapp", "delivered", "dealer", "en", "EN dealer", "Dealer hello PPF"},
		{dealerU, "email", "sent", "dealer", "en", "EN email", "**PPF** is ready"},
		{dealerU, "sms", model.DeliverySkippedDisabled, "dealer", "", "", ""},
		{distU, "inapp", "delivered", "distributor", "ar", "AR موزع", "مرحبا PPF"},
		{distU, "email", "sent", "distributor", "ar", "AR بريد", "**PPF** جاهز"},
		{distU, "sms", model.DeliverySkippedDisabled, "distributor", "", "", ""},
	}
	for _, e := range expect {
		r, ok := got[fmt.Sprintf("%d/%s", e.user.ID, e.channel)]
		if !ok {
			t.Fatalf("missing delivery %s/%s", e.user.Name, e.channel)
		}
		if r.status != e.status || r.role != e.role || r.lang != e.lang || r.title != e.title ||
			!strings.Contains(r.body, e.bodyHas) || (e.lang != "" && r.notificationLang != e.lang) {
			t.Errorf("%s/%s = %+v, want status=%s role=%s lang=%s title=%q", e.user.Name, e.channel, r, e.status, e.role, e.lang, e.title)
		}
	}

	// Acceptance 3: each in-app row went to Centrifugo user:{uuid}.
	wantCh := map[string]bool{}
	for _, u := range []db.User{customer, dealerU, distU} {
		wantCh["user:"+u.Uuid.String()] = true
	}
	ct.pub.mu.Lock()
	for _, ch := range ct.pub.channels {
		delete(wantCh, ch)
	}
	ct.pub.mu.Unlock()
	if len(wantCh) != 0 {
		t.Fatalf("missing realtime publishes: %v (got %v)", wantCh, ct.pub.channels)
	}
	// RTL e-mail for the Arabic recipient.
	var arHTML string
	for _, m := range ct.mail.msgs {
		if m.To[0] == distU.Email.String {
			arHTML = m.HTMLBody
		}
	}
	if !strings.Contains(arHTML, `dir="rtl"`) || !strings.Contains(arHTML, "<strong>PPF</strong>") {
		t.Fatalf("arabic e-mail html = %s", arHTML)
	}
	if len(ct.sms.sent) != 0 {
		t.Fatalf("sms sent while disabled: %v", ct.sms.sent)
	}

	// Acceptance 4: the same event id again changes nothing.
	again, err := ct.svc.Dispatch(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if again.Queued != 0 || again.Skipped != 0 || again.Duplicates != 9 || len(ct.deliveries(in.EventID)) != 9 {
		t.Fatalf("replay = %+v", again)
	}

	// Acceptance 2: SMS on -> sent in the recipient's language.
	ct.setSMS(true)
	in2 := in
	in2.EventID = uuid.New()
	in2.Channels = []string{catalog.ChannelSMS}
	res2, err := ct.svc.Dispatch(ctx, in2)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Queued != 3 {
		t.Fatalf("sms dispatch = %+v", res2)
	}
	got2 := ct.deliveries(in2.EventID)
	for u, lang := range map[int64]string{customer.ID: "tr", dealerU.ID: "en", distU.ID: "ar"} {
		r := got2[fmt.Sprintf("%d/sms", u)]
		if r.status != "sent" || r.lang != lang {
			t.Errorf("sms delivery user %d = %+v, want sent/%s", u, r, lang)
		}
	}
	if len(ct.sms.sent) != 3 || !strings.Contains(strings.Join(ct.sms.sent, ";"), "+905550000103|SMS AR PPF") {
		t.Fatalf("sms sent = %v", ct.sms.sent)
	}
	ct.setSMS(false)

	// Acceptance 5: the 90-day sweep removes old rows only.
	ct.exec("UPDATE notification_deliveries SET created_at = NOW() - INTERVAL '91 days' WHERE event_id = $1", in.EventID)
	ct.exec("UPDATE notifications SET created_at = NOW() - INTERVAL '91 days' WHERE event_id = $1", in.EventID)
	if _, err := ct.svc.PurgeExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if n := len(ct.deliveries(in.EventID)); n != 0 {
		t.Fatalf("old deliveries left: %d", n)
	}
	var oldNotifications int
	_ = ct.pool.QueryRow(ctx, "SELECT COUNT(*) FROM notifications WHERE event_id = $1", in.EventID).Scan(&oldNotifications)
	if oldNotifications != 0 {
		t.Fatalf("old notifications left: %d", oldNotifications)
	}
	if n := len(ct.deliveries(in2.EventID)); n != 3 {
		t.Fatalf("recent deliveries purged: %d left", n)
	}
}

// Unknown placeholders are rejected when a template is saved.
func TestIntegrationTemplateValidation(t *testing.T) {
	ct := newCenterTest(t)
	_, err := ct.svc.UpsertTemplate(ct.ctx, 0, model.TemplateInput{
		Code: ct.event, Role: "dealer", Channel: "email", Language: "ar", Subject: "x", Body: "{{item}} {{oops}}",
	})
	if err == nil || !strings.Contains(err.Error(), "oops") {
		t.Fatalf("want unknown placeholder error, got %v", err)
	}
	tpl, err := ct.svc.UpsertTemplate(ct.ctx, 0, model.TemplateInput{
		Code: ct.event, Role: "dealer", Channel: "email", Language: "ar", Subject: "x", Body: "{{item}}",
	})
	if err != nil || tpl.Role != "dealer" || tpl.Language != "ar" {
		t.Fatalf("upsert = %+v %v", tpl, err)
	}
	prev, err := ct.svc.PreviewTemplate(ct.ctx, model.TemplateInput{
		Code: ct.event, Channel: "email", Language: "ar", Subject: "{{item}}", Body: "**{{item}}**",
	})
	if err != nil || prev.Dir != "rtl" || !strings.Contains(prev.HTML, `dir="rtl"`) {
		t.Fatalf("preview = %+v %v", prev, err)
	}
	if err := SyncCatalog(ct.ctx, ct.q); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = ct.pool.QueryRow(ct.ctx, "SELECT COUNT(*) FROM notification_events WHERE code = $1", ct.event).Scan(&n)
	if n != 1 {
		t.Fatalf("catalog not synced: %d", n)
	}
	_, _ = ct.pool.Exec(ct.ctx, "DELETE FROM notification_events WHERE code = $1", ct.event)
}
