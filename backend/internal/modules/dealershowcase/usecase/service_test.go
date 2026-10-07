package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealershowcase/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealershowcase/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-467 acceptance (use case level). Every test runs in one rolled-back
// transaction; the service opens savepoints on it.

type fakeFeatures struct{ off map[int64]bool }

func (f *fakeFeatures) Enabled(_ context.Context, orgID int64, key string) (bool, error) {
	if key != features.ModuleDealerShowcase {
		return true, nil
	}
	return !f.off[orgID], nil
}

type fakeSettings struct {
	approval bool
	max      int64
}

func (s *fakeSettings) Bool(_ context.Context, key string) bool {
	return key == sysconfig.KeyShowcaseApprovalRequired && s.approval
}

func (s *fakeSettings) Int(_ context.Context, key string) int64 {
	if key == sysconfig.KeyShowcaseMaxPhotos {
		return s.max
	}
	return 0
}

type fakeOutbox struct{ evs []events.Event }

func (o *fakeOutbox) Enqueue(_ context.Context, _ pgx.Tx, ev events.Event) error {
	o.evs = append(o.evs, ev)
	return nil
}

func (o *fakeOutbox) names() []string {
	out := []string{}
	for _, e := range o.evs {
		out = append(out, e.Name)
	}
	return out
}

func (o *fakeOutbox) last(t *testing.T, name string) events.Event {
	t.Helper()
	for i := len(o.evs) - 1; i >= 0; i-- {
		if o.evs[i].Name == name {
			return o.evs[i]
		}
	}
	t.Fatalf("no %s event in %v", name, o.names())
	return events.Event{}
}

type fakeStorage struct{ deleted []string }

func (s *fakeStorage) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}

type fixture struct {
	ctx      context.Context
	tx       pgx.Tx
	q        *db.Queries
	svc      *Service
	feat     *fakeFeatures
	set      *fakeSettings
	out      *fakeOutbox
	store    *fakeStorage
	prefix   string
	seq      int
	brandID  int64
	center   db.Organization
	dist     db.Organization
	dealer   db.Organization
	other    db.Organization // dealer of another distributor
	owner    db.User         // dealer owner
	reviewer db.User         // center member holding platform.showcase.review
}

func newFixture(t *testing.T) *fixture {
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
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	f := &fixture{
		ctx: ctx, tx: tx, q: db.New(tx), prefix: fmt.Sprintf("t467-%d", time.Now().UnixNano()),
		feat: &fakeFeatures{off: map[int64]bool{}}, set: &fakeSettings{max: 12}, out: &fakeOutbox{}, store: &fakeStorage{},
	}
	f.svc = New(tx, f.q, f.feat, f.set, f.out)
	f.svc.SetStorage(f.store)
	brand, err := f.q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("olex brand: %v", err)
	}
	f.brandID = brand.ID
	if f.center, err = f.q.GetBrandCenter(ctx, brand.ID); err != nil {
		t.Fatalf("center: %v", err)
	}
	f.dist = f.org(t, "dist", "distributor", f.center.ID)
	f.dealer = f.org(t, "dealer", "dealer", f.dist.ID)
	otherDist := f.org(t, "dist2", "distributor", f.center.ID)
	f.other = f.org(t, "other", "dealer", otherDist.ID)
	f.owner = f.user(t, "owner")
	f.member(t, f.dealer, f.owner, "owner", rbac.RoleDealerOwner)
	f.reviewer = f.user(t, "reviewer")
	f.member(t, f.center, f.reviewer, "staff", rbac.RoleCenterStaff)
	return f
}

func (f *fixture) org(t *testing.T, name, typ string, parent int64) db.Organization {
	t.Helper()
	f.seq++
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("%s-%s-%d", f.prefix, name, f.seq), Name: f.prefix + " " + name, City: "Ankara",
		Status:         "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ, ParentID: pgtype.Int8{Int64: parent, Valid: true},
		BrandID: f.brandID, Currency: "TRY", Locale: "de", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
		Phone: "+905321112233",
	})
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	return o
}

func (f *fixture) user(t *testing.T, name string) db.User {
	t.Helper()
	u, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		PasswordHash: "x", Name: name, Surname: "T467", Status: "active",
		Email: pgtype.Text{String: fmt.Sprintf("%s-%s@example.test", f.prefix, name), Valid: true},
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	return u
}

func (f *fixture) member(t *testing.T, o db.Organization, u db.User, role, roleSlug string) {
	t.Helper()
	m, err := f.q.CreateOrganizationMember(f.ctx, db.CreateOrganizationMemberParams{OrganizationID: o.ID, UserID: u.ID, Role: role})
	if err != nil {
		t.Fatalf("member: %v", err)
	}
	if err := f.q.AssignMemberRoleBySlug(f.ctx, db.AssignMemberRoleBySlugParams{MemberID: m.ID, Slug: roleSlug}); err != nil {
		t.Fatalf("member role: %v", err)
	}
}

// ownerCaller is the dealer owner in the dealer (showcase.write managed).
func (f *fixture) ownerCaller() Caller {
	return Caller{UserID: f.owner.ID, OrganizationID: f.dealer.ID, BrandID: f.brandID,
		Filter: scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{f.dealer.ID}}}
}

// distCaller is the distributor owner (showcase.write subtree).
func (f *fixture) distCaller() Caller {
	return Caller{UserID: f.owner.ID, OrganizationID: f.dist.ID, BrandID: f.brandID,
		Filter: scopefilter.Filter{Scope: rbac.ScopeSubtree, OrgIDs: []int64{f.dist.ID, f.dealer.ID}}}
}

// reviewerCaller is the center reviewer (platform.showcase.review brand).
func (f *fixture) reviewerCaller() Caller {
	return Caller{UserID: f.reviewer.ID, OrganizationID: f.center.ID, BrandID: f.brandID,
		Filter: scopefilter.Filter{Scope: rbac.ScopeBrand, BrandID: f.brandID}}
}

func content(headline string) Input {
	return Input{
		Content:      map[string]LocaleContent{"tr": {Headline: headline, About: "Hakkımızda"}, "en": {Headline: headline + " EN"}},
		WorkingHours: json.RawMessage(`{"monday":[{"start":"09:00","end":"18:00"}]}`),
		SocialLinks:  map[string]string{"instagram": "https://instagram.com/olex"},
		SeoKeywords:  []string{"ankara ppf", " ", "ankara ppf"},
	}
}

func publishedHeadline(t *testing.T, raw json.RawMessage, locale string) string {
	t.Helper()
	var snap struct {
		Content map[string]map[string]string `json:"content"`
	}
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("published_content: %s", raw)
	}
	return snap.Content[locale]["headline"]
}

func ids(ev events.Event) []int64 {
	raw, _ := json.Marshal(ev.Payload["notify_user_ids"])
	var out []int64
	_ = json.Unmarshal(raw, &out)
	return out
}

// Approval off: submit publishes directly and writes published_content.
func TestSubmitApprovalOffPublishes(t *testing.T) {
	f := newFixture(t)
	c := f.ownerCaller()
	saved, err := f.svc.Save(f.ctx, c, nil, content("PPF uzmanı"))
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != model.StatusDraft || string(saved.PublishedContent) != "null" ||
		!slices.Equal(saved.SeoKeywords, []string{"ankara ppf"}) {
		t.Fatalf("draft view: %+v", saved)
	}
	out, err := f.svc.Submit(f.ctx, c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != model.StatusPublished || out.PublishedAt == nil || publishedHeadline(t, out.PublishedContent, "tr") != "PPF uzmanı" {
		t.Fatalf("published view: status %s content %s", out.Status, out.PublishedContent)
	}
	ev := f.out.last(t, events.ShowcasePublished)
	if len(ids(ev)) != 0 || ev.Payload["organization_uuid"] != f.dealer.Uuid.String() {
		t.Fatalf("direct publish event: %+v", ev.Payload)
	}
	if slices.Contains(f.out.names(), events.ShowcaseReviewRequested) {
		t.Fatal("direct publish must not ask for a review")
	}
	// Submitting again republishes (published → published).
	if out, err = f.svc.Submit(f.ctx, c, nil); err != nil || out.Status != model.StatusPublished {
		t.Fatalf("republish: %v %s", err, out.Status)
	}
}

// Approval on: submit waits in pending_review and notifies the center's
// reviewers; a second submit is a conflict.
func TestSubmitApprovalOnPendingReviewNotifiesCenter(t *testing.T) {
	f := newFixture(t)
	f.set.approval = true
	c := f.ownerCaller()
	if _, err := f.svc.Save(f.ctx, c, nil, content("Onaylı")); err != nil {
		t.Fatal(err)
	}
	out, err := f.svc.Submit(f.ctx, c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != model.StatusPendingReview || out.SubmittedAt == nil || string(out.PublishedContent) != "null" {
		t.Fatalf("pending view: %+v", out)
	}
	ev := f.out.last(t, events.ShowcaseReviewRequested)
	if !slices.Contains(ids(ev), f.reviewer.ID) || slices.Contains(ids(ev), f.owner.ID) {
		t.Fatalf("review_requested recipients = %v, want reviewer %d", ids(ev), f.reviewer.ID)
	}
	if slices.Contains(f.out.names(), events.ShowcasePublished) {
		t.Fatal("pending showcase must not publish")
	}
	if _, err := f.svc.Submit(f.ctx, c, nil); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("second submit: %v", err)
	}
}

// Center review: a rejection needs a note (422 in the handler); reject keeps
// the previous snapshot; approve publishes; owners hear both decisions.
func TestReviewDecisions(t *testing.T) {
	f := newFixture(t)
	c := f.ownerCaller()
	rc := f.reviewerCaller()
	if _, err := f.svc.Save(f.ctx, c, nil, content("İlk")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Submit(f.ctx, c, nil); err != nil { // approval off: live "İlk"
		t.Fatal(err)
	}
	f.set.approval = true
	if _, err := f.svc.Save(f.ctx, c, nil, content("İkinci")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Submit(f.ctx, c, nil); err != nil {
		t.Fatal(err)
	}
	for _, note := range []string{"", "   "} {
		if _, err := f.svc.Review(f.ctx, rc, f.dealer.Uuid, ReviewInput{Decision: DecisionReject, Note: note}); !errors.Is(err, ErrReviewNoteRequired) {
			t.Fatalf("reject without note %q: %v", note, err)
		}
	}
	var ve *ValidationError
	if _, err := f.svc.Review(f.ctx, rc, f.dealer.Uuid, ReviewInput{Decision: "maybe"}); !errors.As(err, &ve) {
		t.Fatalf("unknown decision: %v", err)
	}
	out, err := f.svc.Review(f.ctx, rc, f.dealer.Uuid, ReviewInput{Decision: DecisionReject, Note: "Fotoğraf bulanık"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != model.StatusRejected || out.ReviewNote == nil || *out.ReviewNote != "Fotoğraf bulanık" ||
		publishedHeadline(t, out.PublishedContent, "tr") != "İlk" {
		t.Fatalf("rejected view: %s %v %s", out.Status, out.ReviewNote, out.PublishedContent)
	}
	ev := f.out.last(t, events.ShowcaseRejected)
	if !slices.Equal(ids(ev), []int64{f.owner.ID}) || ev.Payload["reason"] != "Fotoğraf bulanık" {
		t.Fatalf("rejected event: %+v", ev.Payload)
	}
	// Deciding a non-pending showcase is a conflict.
	if _, err := f.svc.Review(f.ctx, rc, f.dealer.Uuid, ReviewInput{Decision: DecisionApprove}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("approve rejected: %v", err)
	}
	if _, err := f.svc.Submit(f.ctx, c, nil); err != nil {
		t.Fatal(err)
	}
	out, err = f.svc.Review(f.ctx, rc, f.dealer.Uuid, ReviewInput{Decision: DecisionApprove})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != model.StatusPublished || publishedHeadline(t, out.PublishedContent, "tr") != "İkinci" || out.ReviewedAt == nil {
		t.Fatalf("approved view: %s %s", out.Status, out.PublishedContent)
	}
	if ev := f.out.last(t, events.ShowcasePublished); !slices.Equal(ids(ev), []int64{f.owner.ID}) {
		t.Fatalf("approved event recipients: %v", ids(ev))
	}
	// Another brand's or unknown organization: 404.
	if _, err := f.svc.Review(f.ctx, rc, uuid.New(), ReviewInput{Decision: DecisionApprove}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown org: %v", err)
	}
}

// A draft edit after publishing never changes published_content or the
// public page; services and photos are frozen in the snapshot too.
func TestDraftEditKeepsPublishedContent(t *testing.T) {
	f := newFixture(t)
	c := f.ownerCaller()
	if _, err := f.svc.Save(f.ctx, c, nil, content("Canlı")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateService(f.ctx, c, nil, ServiceInput{Kind: model.ServiceKindCustom, Title: map[string]string{"tr": "Seramik"}}); err != nil {
		t.Fatal(err)
	}
	pub, err := f.svc.Submit(f.ctx, c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Save(f.ctx, c, nil, content("Taslak")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateService(f.ctx, c, nil, ServiceInput{Kind: model.ServiceKindCustom, Title: map[string]string{"tr": "PPF"}}); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.Get(f.ctx, c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.PublishedContent) != string(pub.PublishedContent) || got.Status != model.StatusPublished {
		t.Fatalf("published_content changed by a draft edit:\n%s\n%s", pub.PublishedContent, got.PublishedContent)
	}
	if !strings.Contains(string(got.Content), "Taslak") || len(got.Services) != 2 {
		t.Fatalf("draft not saved: %s, %d services", got.Content, len(got.Services))
	}
	block, err := f.svc.PublicBlock(f.ctx, f.brandID, f.dealer.Slug, "tr")
	if err != nil || block == nil {
		t.Fatalf("public block: %v %v", block, err)
	}
	pb := block.(PublicShowcase)
	if pb.Headline != "Canlı" || len(pb.Services) != 1 || pb.Services[0].Title != "Seramik" {
		t.Fatalf("public block follows the draft: %+v", pb)
	}
}

// The gallery holds showcase.max_photos photos; the 13th is refused before
// the upload (PreparePhoto) and at insert (AddPhoto).
func TestThirteenthPhotoRefused(t *testing.T) {
	f := newFixture(t)
	c := f.ownerCaller()
	for i := range 12 {
		slot, err := f.svc.PreparePhoto(f.ctx, c, nil, "jpg")
		if err != nil {
			t.Fatalf("prepare %d: %v", i, err)
		}
		if !strings.HasPrefix(slot.ObjectKey, "showcases/"+f.dealer.Uuid.String()+"/") {
			t.Fatalf("key = %s", slot.ObjectKey)
		}
		if _, err := f.svc.AddPhoto(f.ctx, c, nil, PhotoInput{
			ObjectKey: slot.ObjectKey, Mime: "image/jpeg", SizeBytes: 100, SHA256: fmt.Sprintf("%064x", i+1),
		}); err != nil {
			t.Fatalf("photo %d: %v", i, err)
		}
	}
	var le *repository.PhotoLimitError
	if _, err := f.svc.PreparePhoto(f.ctx, c, nil, "jpg"); !errors.As(err, &le) || le.Count != 12 || le.Max != 12 {
		t.Fatalf("13th prepare: %v", err)
	}
	if _, err := f.svc.AddPhoto(f.ctx, c, nil, PhotoInput{
		ObjectKey: "showcases/x/13.jpg", Mime: "image/jpeg", SizeBytes: 100, SHA256: fmt.Sprintf("%064x", 13),
	}); !errors.As(err, &le) {
		t.Fatalf("13th add: %v", err)
	}
	// Same bytes twice: conflict.
	got, _ := f.svc.Get(f.ctx, c, nil)
	if err := f.svc.DeletePhoto(f.ctx, c, nil, got.Photos[0].UUID); err != nil {
		t.Fatal(err)
	}
	if len(f.store.deleted) != 1 {
		t.Fatalf("unpublished photo object not removed: %v", f.store.deleted)
	}
	if _, err := f.svc.AddPhoto(f.ctx, c, nil, PhotoInput{
		ObjectKey: "showcases/x/dup.jpg", Mime: "image/jpeg", SizeBytes: 100, SHA256: fmt.Sprintf("%064x", 2),
	}); !errors.Is(err, ErrDuplicatePhoto) {
		t.Fatalf("duplicate: %v", err)
	}
}

// A published photo removed from the draft keeps serving the live page
// until the next publish, which then drops its object.
func TestDeletedPublishedPhotoServesUntilRepublish(t *testing.T) {
	f := newFixture(t)
	c := f.ownerCaller()
	p, err := f.svc.AddPhoto(f.ctx, c, nil, PhotoInput{ObjectKey: "showcases/live.jpg", Mime: "image/jpeg", SizeBytes: 10, SHA256: fmt.Sprintf("%064x", 7)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Submit(f.ctx, c, nil); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeletePhoto(f.ctx, c, nil, p.UUID); err != nil {
		t.Fatal(err)
	}
	if len(f.store.deleted) != 0 {
		t.Fatalf("live photo object removed: %v", f.store.deleted)
	}
	if key, mime, err := f.svc.PublicPhoto(f.ctx, f.brandID, f.dealer.Slug, p.UUID); err != nil || key != "showcases/live.jpg" || mime != "image/jpeg" {
		t.Fatalf("public photo: %s %s %v", key, mime, err)
	}
	if _, err := f.svc.Submit(f.ctx, c, nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.store.deleted, []string{"showcases/live.jpg"}) {
		t.Fatalf("republish did not drop the old object: %v", f.store.deleted)
	}
	if _, _, err := f.svc.PublicPhoto(f.ctx, f.brandID, f.dealer.Slug, p.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("dropped photo still public: %v", err)
	}
}

// A distributor edits the showcase of a dealer in its subtree with ?org=;
// a dealer outside the subtree, the center and unknown uuids are 404; a
// target without the module is FEATURE_DISABLED.
func TestDistributorScope(t *testing.T) {
	f := newFixture(t)
	dc := f.distCaller()
	dealer := f.dealer.Uuid
	out, err := f.svc.Save(f.ctx, dc, &dealer, content("Dağıtıcıdan"))
	if err != nil {
		t.Fatal(err)
	}
	if out.Organization.UUID != f.dealer.Uuid {
		t.Fatalf("target = %s", out.Organization.UUID)
	}
	// Its own showcase without ?org=.
	if out, err := f.svc.Get(f.ctx, dc, nil); err != nil || out.Organization.UUID != f.dist.Uuid || out.UUID != nil {
		t.Fatalf("own showcase: %+v %v", out, err)
	}
	for name, id := range map[string]uuid.UUID{"other subtree": f.other.Uuid, "center": f.center.Uuid, "unknown": uuid.New()} {
		if _, err := f.svc.Save(f.ctx, dc, &id, content("x")); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := f.svc.Get(f.ctx, dc, &id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s get: %v", name, err)
		}
	}
	// The dealer owner cannot reach its parent either.
	dist := f.dist.Uuid
	if _, err := f.svc.Get(f.ctx, f.ownerCaller(), &dist); !errors.Is(err, ErrNotFound) {
		t.Fatalf("dealer → distributor: %v", err)
	}
	f.feat.off[f.dealer.ID] = true
	if _, err := f.svc.Submit(f.ctx, dc, &dealer); !errors.Is(err, ErrFeatureDisabled) {
		t.Fatalf("module off target: %v", err)
	}
}

// With the module off the public block is absent (the endpoint answers
// the F2 skeleton); the panel routes are closed by RequireFeature
// (TestIntegrationDealerShowcaseAPI).
func TestPublicBlockModuleOffOrUnpublished(t *testing.T) {
	f := newFixture(t)
	c := f.ownerCaller()
	if _, err := f.svc.Save(f.ctx, c, nil, content("Görünmez")); err != nil {
		t.Fatal(err)
	}
	if b, err := f.svc.PublicBlock(f.ctx, f.brandID, f.dealer.Slug, "tr"); err != nil || b != nil {
		t.Fatalf("unpublished: %v %v", b, err)
	}
	if _, err := f.svc.Submit(f.ctx, c, nil); err != nil {
		t.Fatal(err)
	}
	f.feat.off[f.dealer.ID] = true
	if b, err := f.svc.PublicBlock(f.ctx, f.brandID, f.dealer.Slug, "tr"); err != nil || b != nil {
		t.Fatalf("module off: %v %v", b, err)
	}
	if badges, err := f.svc.Badges(f.ctx, f.brandID, []uuid.UUID{f.dealer.Uuid}); err != nil || len(badges) != 0 {
		t.Fatalf("module off badges: %v %v", badges, err)
	}
	if dates, err := f.svc.PublishedDates(f.ctx, f.brandID); err != nil || !dates[f.dealer.Slug].IsZero() {
		t.Fatalf("module off dates: %v %v", dates, err)
	}
	delete(f.feat.off, f.dealer.ID)
	badges, err := f.svc.Badges(f.ctx, f.brandID, []uuid.UUID{f.dealer.Uuid, f.other.Uuid})
	if _, ok := badges[f.dealer.Uuid]; err != nil || !ok || len(badges) != 1 {
		t.Fatalf("badges: %v %v", badges, err)
	}
}

// Public block: requested locale → organization locale (de) → tr per field,
// the weekly hours Monday first, open now in the organization's zone, no
// price anywhere.
func TestPublicBlockLocaleAndOpenNow(t *testing.T) {
	f := newFixture(t)
	c := f.ownerCaller()
	in := Input{
		Content: map[string]LocaleContent{
			"tr": {Headline: "Başlık", About: "Türkçe hakkında"},
			"de": {Headline: "Überschrift"},
			"en": {About: "English about"},
		},
		WorkingHours: json.RawMessage(`{"mon":[{"start":"09:00","end":"18:00"}],"6":[{"start":"10:00","end":"14:00"}]}`),
	}
	if _, err := f.svc.Save(f.ctx, c, nil, in); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateService(f.ctx, c, nil, ServiceInput{Kind: model.ServiceKindCustom, Title: map[string]string{"tr": "Cam filmi", "en": "Window film"}}); err != nil {
		t.Fatal(err)
	}
	hidden := false
	if _, err := f.svc.CreateService(f.ctx, c, nil, ServiceInput{Kind: model.ServiceKindCustom, Title: map[string]string{"tr": "Gizli"}, Visible: &hidden}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Submit(f.ctx, c, nil); err != nil {
		t.Fatal(err)
	}
	ist, _ := time.LoadLocation("Europe/Istanbul")
	f.svc.now = func() time.Time { return time.Date(2026, 10, 5, 10, 0, 0, 0, ist) } // Monday 10:00
	b, err := f.svc.PublicBlock(f.ctx, f.brandID, f.dealer.Slug, "en")
	if err != nil || b == nil {
		t.Fatal(b, err)
	}
	pb := b.(PublicShowcase)
	if pb.Locale != "en" || pb.Headline != "Überschrift" || pb.About != "English about" {
		t.Fatalf("en fallback: %+v", pb)
	}
	if pb.OpenNow == nil || !*pb.OpenNow || pb.Timezone != "Europe/Istanbul" || len(pb.WorkingHours) != 7 ||
		pb.WorkingHours[0].Day != "monday" || len(pb.WorkingHours[5].Windows) != 1 || len(pb.WorkingHours[6].Windows) != 0 {
		t.Fatalf("hours: %+v open %v", pb.WorkingHours, pb.OpenNow)
	}
	if len(pb.Services) != 1 || pb.Services[0].Title != "Window film" {
		t.Fatalf("services: %+v", pb.Services)
	}
	if pb.WhatsAppChatURL == nil || *pb.WhatsAppChatURL != "https://wa.me/905321112233" || !pb.LeadFormEnabled {
		t.Fatalf("contact: %+v", pb)
	}
	raw, _ := json.Marshal(pb)
	if strings.Contains(strings.ToLower(string(raw)), "price") || strings.Contains(string(raw), "storage_key") {
		t.Fatalf("public block leaks: %s", raw)
	}
	f.svc.now = func() time.Time { return time.Date(2026, 10, 5, 18, 30, 0, 0, ist) }
	b, _ = f.svc.PublicBlock(f.ctx, f.brandID, f.dealer.Slug, "")
	if pb = b.(PublicShowcase); pb.OpenNow == nil || *pb.OpenNow || pb.Locale != "de" || pb.About != "Türkçe hakkında" {
		t.Fatalf("closed / de fallback: %+v", pb)
	}
}

// Input validation (400): unknown locale, bad hours, non-https link,
// custom service without title, unknown category.
func TestInputValidation(t *testing.T) {
	f := newFixture(t)
	c := f.ownerCaller()
	var ve *ValidationError
	for name, in := range map[string]Input{
		"locale": {Content: map[string]LocaleContent{"xx": {Headline: "a"}}},
		"hours":  {WorkingHours: json.RawMessage(`{"monday":[{"start":"18:00","end":"09:00"}]}`)},
		"link":   {SocialLinks: map[string]string{"instagram": "http://insecure.example"}},
		"net":    {SocialLinks: map[string]string{"myspace": "https://myspace.example"}},
	} {
		if _, err := f.svc.Save(f.ctx, c, nil, in); !errors.As(err, &ve) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := f.svc.CreateService(f.ctx, c, nil, ServiceInput{Kind: model.ServiceKindCustom}); !errors.As(err, &ve) {
		t.Fatalf("custom without title: %v", err)
	}
	unknown := uuid.New()
	if _, err := f.svc.CreateService(f.ctx, c, nil, ServiceInput{Kind: model.ServiceKindProductCategory, CategoryUUID: &unknown}); !errors.As(err, &ve) {
		t.Fatalf("unknown category: %v", err)
	}
	var catUUID uuid.UUID
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO product_categories (organization_id, brand_id, name) VALUES ($1, $2, $3) RETURNING uuid`,
		f.center.ID, f.brandID, f.prefix+" PPF").Scan(&catUUID); err != nil {
		t.Fatal(err)
	}
	sv, err := f.svc.CreateService(f.ctx, c, nil, ServiceInput{Kind: model.ServiceKindProductCategory, CategoryUUID: &catUUID})
	if err != nil || sv.Category == nil || sv.Category.UUID != catUUID {
		t.Fatalf("category service: %+v %v", sv, err)
	}
	if _, err := f.svc.CreateService(f.ctx, c, nil, ServiceInput{Kind: model.ServiceKindProductCategory, CategoryUUID: &catUUID}); !errors.Is(err, ErrDuplicateService) {
		t.Fatalf("duplicate category: %v", err)
	}
	second, _ := f.svc.CreateService(f.ctx, c, nil, ServiceInput{Kind: model.ServiceKindCustom, Title: map[string]string{"tr": "B"}})
	items, err := f.svc.ReorderServices(f.ctx, c, nil, []uuid.UUID{second.UUID, sv.UUID})
	if err != nil || items[0].UUID != second.UUID {
		t.Fatalf("reorder: %v %v", items, err)
	}
	if _, err := f.svc.ReorderServices(f.ctx, c, nil, []uuid.UUID{sv.UUID}); !errors.As(err, &ve) {
		t.Fatalf("partial reorder: %v", err)
	}
}

// Review queue list contract: sort asc/desc, unknown sort 400, status=a,b.
func TestReviewQueueListContract(t *testing.T) {
	f := newFixture(t)
	c := f.ownerCaller()
	// dealer: published; other: pending_review (approval on); dist: draft.
	if _, err := f.svc.Save(f.ctx, c, nil, content("A")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Submit(f.ctx, c, nil); err != nil {
		t.Fatal(err)
	}
	f.set.approval = true
	oc := Caller{UserID: f.owner.ID, OrganizationID: f.other.ID, BrandID: f.brandID,
		Filter: scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{f.other.ID}}}
	if _, err := f.svc.Submit(f.ctx, oc, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Save(f.ctx, f.distCaller(), nil, content("D")); err != nil {
		t.Fatal(err)
	}
	rc := f.reviewerCaller()
	list := func(query string) []string {
		t.Helper()
		vals, _ := url.ParseQuery(query + "&q=" + f.prefix + "&limit=100")
		flt, err := ParseReviewFilter(vals)
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		page, err := f.svc.ListReviews(f.ctx, rc, flt)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, it := range page.Items {
			out = append(out, it.Status)
		}
		if int(page.Total) != len(out) {
			t.Fatalf("%s: total %d, items %d", query, page.Total, len(out))
		}
		return out
	}
	if got := list("sort=status"); !slices.Equal(got, []string{"draft", "pending_review", "published"}) {
		t.Fatalf("sort=status: %v", got)
	}
	if got := list("sort=-status"); !slices.Equal(got, []string{"published", "pending_review", "draft"}) {
		t.Fatalf("sort=-status: %v", got)
	}
	if got := list("status=pending_review,published&sort=status"); !slices.Equal(got, []string{"pending_review", "published"}) {
		t.Fatalf("status=a,b: %v", got)
	}
	var qe *apiquery.ValidationError
	for _, bad := range []string{"sort=organization_id", "status=archived"} {
		vals, _ := url.ParseQuery(bad)
		if _, err := ParseReviewFilter(vals); !errors.As(err, &qe) {
			t.Fatalf("%s: %v", bad, err)
		}
	}
}
