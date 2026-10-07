package usecase_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm/fake"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// TEC-394 (F4-02b) acceptance. Every test runs in one rolled-back
// transaction.

type idFixture struct {
	t      *testing.T
	ctx    context.Context
	tx     pgx.Tx
	q      *db.Queries
	brand  db.Brand
	center db.Organization
	llm    *fake.Provider
	models llm.Models
	res    *usecase.IdentityResolver
}

var idSeq atomic.Int64

func newIDFixture(t *testing.T, cache usecase.IdentityCache) *idFixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	f := &idFixture{t: t, ctx: ctx, tx: tx, q: db.New(tx), llm: fake.New()}
	if f.brand, err = f.q.GetBrandBySlug(ctx, "olex"); err != nil {
		t.Fatal(err)
	}
	if f.center, err = f.q.GetBrandCenter(ctx, f.brand.ID); err != nil {
		t.Fatal(err)
	}
	f.models = llm.Models{Default: "claude-sonnet-5-5", Fast: "claude-haiku-4-5"}
	f.res = usecase.NewIdentityResolver(f.q, cache, f.llm, f.models, nil)
	return f
}

func (f *idFixture) uniq() string {
	return fmt.Sprintf("%d%03d", time.Now().UnixNano()%1_000_000, idSeq.Add(1)%1000)
}

func (f *idFixture) phone() string { return "+90555" + f.uniq()[:7] }

func (f *idFixture) user(phone, status string) db.User {
	f.t.Helper()
	u, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "TEC394", Surname: f.uniq(), Status: status,
		Email:     pgtype.Text{String: "tec394-" + f.uniq() + "@example.test", Valid: true},
		PhoneE164: pgtype.Text{String: phone, Valid: true},
	})
	if err != nil {
		f.t.Fatalf("user: %v", err)
	}
	return u
}

func (f *idFixture) customer(u db.User) {
	f.t.Helper()
	f.exec(`INSERT INTO customer_profiles (user_id) VALUES ($1)`, u.ID)
}

func (f *idFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.tx.Exec(f.ctx, sql, args...); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
}

func (f *idFixture) org(name, status string) db.Organization {
	f.t.Helper()
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: "tec394-" + f.uniq(), Name: name, Status: status,
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "dealer", ParentID: pgtype.Int8{Int64: f.center.ID, Valid: true},
		BrandID: f.brand.ID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		f.t.Fatalf("org: %v", err)
	}
	return o
}

func (f *idFixture) member(o db.Organization, u db.User) {
	f.t.Helper()
	if _, err := f.q.CreateOrganizationMember(f.ctx, db.CreateOrganizationMemberParams{
		OrganizationID: o.ID, UserID: u.ID, Role: "staff",
	}); err != nil {
		f.t.Fatalf("member: %v", err)
	}
}

func (f *idFixture) conv(phone string) db.Conversation {
	f.t.Helper()
	c, err := f.q.UpsertConversation(f.ctx, db.UpsertConversationParams{Channel: "whatsapp", ContactE164: phone})
	if err != nil {
		f.t.Fatalf("conversation: %v", err)
	}
	return c
}

func (f *idFixture) resolve(c db.Conversation, text string) usecase.Identity {
	f.t.Helper()
	id, err := f.res.Resolve(f.ctx, c, text)
	if err != nil {
		f.t.Fatalf("resolve %q: %v", text, err)
	}
	return id
}

func (f *idFixture) stored(c db.Conversation) db.Conversation {
	f.t.Helper()
	got, err := f.q.GetConversationByID(f.ctx, c.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	return got
}

// Acceptance: a known customer number resolves to customer, a staff number to
// panel_user, an unknown number to visitor; the outcome is stored on the
// conversation.
func TestResolveIdentityKinds(t *testing.T) {
	f := newIDFixture(t, nil)

	custPhone := f.phone()
	cust := f.user(custPhone, "active")
	f.customer(cust)
	f.exec(`UPDATE users SET locale = 'de' WHERE id = $1`, cust.ID)
	id := f.resolve(f.conv(custPhone), "Hallo")
	if id.Kind != model.IdentityCustomer || id.UserID != cust.ID || id.OrgID != 0 || id.Locale != "de" || id.Reply != "" {
		t.Fatalf("customer: %+v", id)
	}
	if st := f.stored(id.Conversation); st.IdentityKind != model.IdentityCustomer ||
		st.IdentityUserID.Int64 != cust.ID || st.IdentityOrgID.Valid || !st.IdentityResolvedAt.Valid || st.Locale.String != "de" {
		t.Fatalf("customer stored: %+v", st)
	}

	staffPhone := f.phone()
	staff := f.user(staffPhone, "active")
	dealer := f.org("TEC394 Bayi Kadıköy", "active")
	f.member(dealer, staff)
	id = f.resolve(f.conv(staffPhone), "merhaba")
	if id.Kind != model.IdentityPanelUser || id.UserID != staff.ID || id.OrgID != dealer.ID ||
		id.OrgName != dealer.Name || id.WritesDisabled || id.Locale != "tr" {
		t.Fatalf("staff: %+v", id)
	}
	if st := f.stored(id.Conversation); st.IdentityKind != model.IdentityPanelUser || st.IdentityOrgID.Int64 != dealer.ID {
		t.Fatalf("staff stored: %+v", st)
	}

	f.llm.Push(fake.Text("tr", llm.Usage{InputTokens: 10, OutputTokens: 1}))
	unknown := f.phone()
	id = f.resolve(f.conv(unknown), "Merhaba, cam filmi fiyatı nedir?")
	if id.Kind != model.IdentityVisitor || id.UserID != 0 || id.OfferPortalOTP || id.Locale != "tr" {
		t.Fatalf("visitor: %+v", id)
	}
	if st := f.stored(id.Conversation); st.IdentityKind != model.IdentityVisitor || st.IdentityUserID.Valid || st.Locale.String != "tr" {
		t.Fatalf("visitor stored: %+v", st)
	}
	// The visitor's language is detected once: a second message keeps it.
	id = f.resolve(id.Conversation, "another message")
	if id.Locale != "tr" || f.llm.Remaining() != 0 || len(f.llm.Requests()) != 1 {
		t.Fatalf("visitor second message: %+v requests=%d", id, len(f.llm.Requests()))
	}
}

// Acceptance: a member of two organizations gets the numbered menu, "2"
// picks the second organization (stored in identity_org_id), the choice
// sticks, and "kimlik değiştir" resets it.
func TestResolveIdentityMenuChoice(t *testing.T) {
	f := newIDFixture(t, nil)
	phone := f.phone()
	u := f.user(phone, "active")
	a := f.org("TEC394 A Bayi", "active")
	b := f.org("TEC394 B Bayi", "active")
	f.member(a, u)
	f.member(b, u)
	f.exec(`UPDATE users SET locale = 'tr' WHERE id = $1`, u.ID)
	c := f.conv(phone)

	id := f.resolve(c, "merhaba")
	if !id.AwaitingChoice || id.Kind != model.IdentityUnknown || len(id.Options) != 2 {
		t.Fatalf("menu: %+v", id)
	}
	if !strings.Contains(id.Reply, "1) TEC394 A Bayi\n2) TEC394 B Bayi") || !strings.Contains(id.Reply, "kimlik değiştir") {
		t.Fatalf("menu text: %q", id.Reply)
	}
	if st := f.stored(c); st.IdentityKind != model.IdentityUnknown {
		t.Fatalf("menu stored: %+v", st)
	}

	// An answer outside the menu repeats it.
	if id = f.resolve(id.Conversation, "5"); !id.AwaitingChoice {
		t.Fatalf("out of range: %+v", id)
	}

	id = f.resolve(id.Conversation, "2")
	if id.Kind != model.IdentityPanelUser || id.OrgID != b.ID || id.AwaitingChoice || id.Reply != "" {
		t.Fatalf("choice 2: %+v", id)
	}
	if st := f.stored(c); st.IdentityKind != model.IdentityPanelUser || st.IdentityOrgID.Int64 != b.ID || st.IdentityUserID.Int64 != u.ID {
		t.Fatalf("choice stored: %+v", st)
	}

	// The choice sticks for later messages ("1" is now plain text).
	if id = f.resolve(f.stored(c), "1"); id.Kind != model.IdentityPanelUser || id.OrgID != b.ID {
		t.Fatalf("sticky: %+v", id)
	}

	id = f.resolve(f.stored(c), "Kimlik Değiştir")
	if !id.Reset || !id.AwaitingChoice {
		t.Fatalf("reset: %+v", id)
	}
	if st := f.stored(c); st.IdentityKind != model.IdentityUnknown || st.IdentityOrgID.Valid {
		t.Fatalf("reset stored: %+v", st)
	}
	if id = f.resolve(f.stored(c), "1"); id.OrgID != a.ID {
		t.Fatalf("choice 1 after reset: %+v", id)
	}

	// Staff and customer at once: the customer identity is the last entry.
	f.customer(u)
	_ = f.resolve(f.stored(c), "kimlik degistir")
	id = f.resolve(f.stored(c), "3)")
	if id.Kind != model.IdentityCustomer || id.OrgID != 0 {
		t.Fatalf("customer choice: %+v", id)
	}
	if st := f.stored(c); st.IdentityKind != model.IdentityCustomer || st.IdentityOrgID.Valid {
		t.Fatalf("customer stored: %+v", st)
	}
}

// Acceptance: a K26 unverified record is not claimed over WhatsApp: visitor
// with the portal OTP hint.
func TestResolveUnverifiedRecordIsVisitor(t *testing.T) {
	f := newIDFixture(t, nil)
	phone := f.phone()
	u := f.user(phone, "active")
	f.customer(u)
	f.exec(`UPDATE users SET legacy_unverified = TRUE, locale = 'tr' WHERE id = $1`, u.ID)
	f.llm.Push(fake.Text("tr", llm.Usage{}))
	id := f.resolve(f.conv(phone), "selam")
	if id.Kind != model.IdentityVisitor || id.UserID != 0 || !id.OfferPortalOTP {
		t.Fatalf("unverified: %+v", id)
	}
	if st := f.stored(id.Conversation); st.IdentityKind != model.IdentityVisitor || st.IdentityUserID.Valid {
		t.Fatalf("unverified stored: %+v", st)
	}
	if txt := usecase.PortalClaimText("tr", "https://portal.example/giris"); !strings.Contains(txt, "https://portal.example/giris") {
		t.Fatalf("portal text: %q", txt)
	}
}

// A disabled (suspended) user gets the fixed short answer; a read_only
// organization (contract ended) and a pending user turn the write tools off;
// suspended / expired organizations are not offered.
func TestResolveSuspendedAndReadOnly(t *testing.T) {
	f := newIDFixture(t, nil)

	phone := f.phone()
	u := f.user(phone, "disabled")
	f.member(f.org("TEC394 Askı", "active"), u)
	f.exec(`UPDATE users SET locale = 'en' WHERE id = $1`, u.ID)
	id := f.resolve(f.conv(phone), "hi")
	if !id.Suspended || !id.WritesDisabled || id.Reply == "" || !strings.Contains(id.Reply, "suspended") {
		t.Fatalf("suspended: %+v", id)
	}

	phone = f.phone()
	u = f.user(phone, "active")
	ro := f.org("TEC394 Salt Okunur", "read_only")
	f.member(ro, u)
	f.member(f.org("TEC394 Askıda Org", "suspended"), u)
	f.member(f.org("TEC394 Bitmiş Org", "expired"), u)
	id = f.resolve(f.conv(phone), "merhaba")
	if id.Kind != model.IdentityPanelUser || id.OrgID != ro.ID || !id.WritesDisabled || id.Suspended {
		t.Fatalf("read_only: %+v", id)
	}

	phone = f.phone()
	u = f.user(phone, "pending")
	f.member(f.org("TEC394 Pending", "active"), u)
	if id = f.resolve(f.conv(phone), "merhaba"); id.Kind != model.IdentityPanelUser || !id.WritesDisabled {
		t.Fatalf("pending user: %+v", id)
	}
}

// Acceptance: the identity cache serves a number until a membership change
// event drops it.
func TestIdentityCacheClearedOnMembershipChange(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := usecase.NewRedisIdentityCache(rdb, "test", nil)
	bus := events.NewBus(nil)
	usecase.RegisterIdentityInvalidation(bus, cache)
	f := newIDFixture(t, cache)

	phone := f.phone()
	u := f.user(phone, "active")
	f.customer(u)
	c := f.conv(phone)
	if id := f.resolve(c, "hello"); id.Kind != model.IdentityCustomer {
		t.Fatalf("before: %+v", id)
	}
	if snap, ok := cache.Get(f.ctx, phone); !ok || snap.UserID != u.ID || !snap.Customer || len(snap.Members) != 0 {
		t.Fatalf("cached: %+v %v", snap, ok)
	}
	if ttl := mr.TTL("test:waid:v0:" + phone); ttl != usecase.IdentityCacheTTL {
		t.Fatalf("ttl = %v", ttl)
	}

	dealer := f.org("TEC394 Yeni Bayi", "active")
	f.member(dealer, u)
	// Still the cached snapshot: no menu yet.
	if id := f.resolve(f.stored(c), "hello"); id.Kind != model.IdentityCustomer || id.AwaitingChoice {
		t.Fatalf("cached resolve: %+v", id)
	}

	if err := bus.Publish(f.ctx, events.New(events.TenantMemberAdded).WithTenant(dealer.ID).
		WithPayload(map[string]any{"organization_id": dealer.ID, "user_id": u.ID})); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.Get(f.ctx, phone); ok {
		t.Fatal("cache survived tenant.member_added")
	}
	// The stored customer choice is still valid, so it is kept; a reset
	// offers the new organization.
	id := f.resolve(f.stored(c), "change identity")
	if !id.AwaitingChoice || len(id.Options) != 2 || id.Options[0].OrgID != dealer.ID {
		t.Fatalf("after invalidation: %+v", id)
	}
	for _, name := range []string{events.CustomerMerged, events.CustomerCreated, events.OrganizationUpdated} {
		if bus.HandlerCount(name) == 0 {
			t.Fatalf("no invalidation handler for %s", name)
		}
	}
}

// Acceptance: Arabic text is detected as ar with the fast model (fake llm);
// languages outside the 13 locales fall back to en; Chinese is zh_CN.
func TestDetectLocale(t *testing.T) {
	f := newIDFixture(t, nil)
	settings, err := f.q.GetAISettings(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.llm.Push(
		fake.Text("ar", llm.Usage{InputTokens: 20, OutputTokens: 1}),
		fake.Text("pt", llm.Usage{}),
		fake.Text("zh", llm.Usage{}),
	)
	id := f.resolve(f.conv(f.phone()), "مرحبا، أريد معرفة سعر فيلم حماية الطلاء")
	if id.Kind != model.IdentityVisitor || id.Locale != "ar" {
		t.Fatalf("arabic: %+v", id)
	}
	if st := f.stored(id.Conversation); st.Locale.String != "ar" {
		t.Fatalf("arabic stored: %+v", st.Locale)
	}
	req := f.llm.Requests()[0]
	if want := f.models.ResolveFast(settings.FastModel); req.Model != want || req.MaxTokens <= 0 || req.MaxTokens > 16 {
		t.Fatalf("model = %q (want %q), max_tokens = %d", req.Model, want, req.MaxTokens)
	}
	if got := req.Messages[0].Text(); !strings.Contains(got, "مرحبا") {
		t.Fatalf("detected text: %q", got)
	}

	if id = f.resolve(f.conv(f.phone()), "Olá, quanto custa?"); id.Locale != "en" {
		t.Fatalf("portuguese: %+v", id)
	}
	if id = f.resolve(f.conv(f.phone()), "你好"); id.Locale != "zh_CN" {
		t.Fatalf("chinese: %+v", id)
	}
	if st := f.stored(id.Conversation); st.Locale.String != "zh_CN" {
		t.Fatalf("chinese stored: %+v", st.Locale)
	}

	// Detection failure: en for this message, nothing stored.
	f.llm.Push(fake.Turn{Err: llm.ErrUnavailable})
	id = f.resolve(f.conv(f.phone()), "hello")
	if id.Locale != "en" || f.stored(id.Conversation).Locale.Valid {
		t.Fatalf("failure: %+v", id)
	}
}

func TestIsIdentityResetCommand(t *testing.T) {
	for _, s := range []string{"kimlik değiştir", "KİMLİK DEĞİŞTİR", " Kimlik  degistir. ", "KIMLIK DEGISTIR", "Change identity", "switch identity!"} {
		if !usecase.IsIdentityResetCommand(s) {
			t.Errorf("%q not a reset command", s)
		}
	}
	for _, s := range []string{"kimlik", "kimliğimi değiştirmek istiyorum", "2", ""} {
		if usecase.IsIdentityResetCommand(s) {
			t.Errorf("%q is a reset command", s)
		}
	}
	got := usecase.MenuText("tr", []usecase.IdentityOption{
		{Kind: model.IdentityPanelUser, OrgID: 1, OrgName: "Olex Bayi Kadıköy"},
		{Kind: model.IdentityCustomer},
	})
	if !strings.Contains(got, "1) Olex Bayi Kadıköy\n2) Müşteri") {
		t.Fatalf("menu: %q", got)
	}
	if got := usecase.MenuText("xx", nil); !strings.Contains(got, "change identity") {
		t.Fatalf("fallback menu: %q", got)
	}
}
