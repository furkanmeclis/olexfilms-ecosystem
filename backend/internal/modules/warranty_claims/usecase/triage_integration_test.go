package usecase

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	aimodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	airepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm/fake"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-392 (F4-01j): AI first triage of warranty claims, on the test
// database with a scripted model.

type triageFeatures struct{ off bool }

func (f triageFeatures) Enabled(context.Context, int64, string) (bool, error) { return !f.off, nil }

type triagePhotoStore map[string][]byte

func (s triagePhotoStore) Download(_ context.Context, key string) (io.ReadCloser, int64, error) {
	b, ok := s[key]
	if !ok {
		return nil, 0, errors.New("missing object")
	}
	return io.NopCloser(bytes.NewReader(b)), int64(len(b)), nil
}

// syncTriageQueue runs the task inline, like the worker would.
type syncTriageQueue struct {
	t   *Triage
	err error
}

func (q *syncTriageQueue) EnqueueClaimTriage(ctx context.Context, claim uuid.UUID) error {
	if err := q.t.Auto(ctx, claim); err != nil {
		q.err = err
	}
	return nil
}

type triageFixture struct {
	*reapplyFixture
	llm    *fake.Provider
	triage *Triage
	queue  *syncTriageQueue
	photos triagePhotoStore
}

func newTriageFixture(t *testing.T, feats triageFeatures, turns ...fake.Turn) *triageFixture {
	t.Helper()
	base := newReapplyFixture(t)
	provider := fake.New(turns...)
	photos := triagePhotoStore{}
	tr := NewTriage(TriageDeps{
		Conn: base.tx, Provider: provider, Features: feats, Storage: photos, Log: slog.Default(),
		Models: llm.Models{Default: "claude-sonnet-5-5", Fast: "claude-haiku-4-5"},
	})
	q := &syncTriageQueue{t: tr}
	RegisterTriageHandlers(base.bus, q, slog.Default())
	return &triageFixture{reapplyFixture: base, llm: provider, triage: tr, queue: q, photos: photos}
}

// submitted moves a new claim to center_review (forwarded to the center)
// with one photo.
func (f *triageFixture) submitted(t *testing.T) db.WarrantyClaim {
	t.Helper()
	claim := f.openClaim(t)
	key := "warranty-claims/" + claim.Uuid.String() + "/photos/1.png"
	f.photos[key] = testPNG(t)
	if _, err := f.q.AddWarrantyClaimPhoto(f.ctx, db.AddWarrantyClaimPhotoParams{
		ClaimID: claim.ID, OrganizationID: claim.OrganizationID, BrandID: claim.BrandID,
		StorageKey: key, MimeType: "image/png", SizeBytes: int64(len(f.photos[key])),
		Sha256: strings.Repeat("a", 64),
	}); err != nil {
		t.Fatalf("photo: %v", err)
	}
	row, err := f.q.SetWarrantyClaimStatus(f.ctx, db.SetWarrantyClaimStatusParams{
		ID: claim.ID, BrandID: claim.BrandID, FromStatus: StatusOpen, Status: StatusCenterReview,
	})
	if err != nil {
		t.Fatalf("center review: %v", err)
	}
	return row
}

func (f *triageFixture) publishStatus(t *testing.T, claim db.WarrantyClaim, to string) {
	t.Helper()
	ev := events.New(events.WarrantyClaimStatusChanged).
		WithTenant(claim.OrganizationID).
		WithEntity("warranty_claim", &claim.ID, &claim.Uuid).
		WithPayload(map[string]any{"brand_id": claim.BrandID, "claim_uuid": claim.Uuid.String(), "from": StatusOpen, "to": to})
	if err := f.bus.Publish(f.ctx, ev); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if f.queue.err != nil {
		t.Fatalf("triage task: %v", f.queue.err)
	}
}

func (f *triageFixture) reload(t *testing.T, claim db.WarrantyClaim) db.WarrantyClaim {
	t.Helper()
	row, err := f.q.GetWarrantyClaimByID(f.ctx, db.GetWarrantyClaimByIDParams{ID: claim.ID, BrandID: claim.BrandID})
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	return row
}

func (f *triageFixture) triageEvents(t *testing.T, claim db.WarrantyClaim) []db.WarrantyClaimEvent {
	t.Helper()
	evs, err := f.q.ListWarrantyClaimEvents(f.ctx, claim.ID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	var out []db.WarrantyClaimEvent
	for _, e := range evs {
		if e.EventType == "ai_triaged" {
			out = append(out, e)
		}
	}
	return out
}

func (f *triageFixture) systemTokens(t *testing.T) int64 {
	t.Helper()
	n, err := airepo.New(f.tx).MonthlyTokens(f.ctx, f.center.ID, aimodel.PoolSystem, f.triage.now())
	if err != nil {
		t.Fatalf("monthly tokens: %v", err)
	}
	return n
}

func triageTurn(damage string, confidence any) fake.Turn {
	return fake.ToolCall("tu_1", "record_triage", map[string]any{
		"damage_type": damage, "summary": "Kaputta filmin kenarlarında kalkma görülüyor.", "confidence": confidence,
	}, llm.Usage{InputTokens: 900, OutputTokens: 60})
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func confidence(t *testing.T, n pgtype.Numeric) float64 {
	t.Helper()
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		t.Fatalf("confidence %+v: %v", n, err)
	}
	return f.Float64
}

func TestTriageSubmittedEventFillsFields(t *testing.T) {
	f := newTriageFixture(t, triageFeatures{}, triageTurn("peeling", 0.82))
	claim := f.submitted(t)
	before := f.systemTokens(t)

	f.publishStatus(t, claim, StatusCenterReview)

	got := f.reload(t, claim)
	if got.AiDamageType.String != DamagePeeling || !got.AiTriagedAt.Valid ||
		!strings.Contains(got.AiSummary.String, "kalkma") || confidence(t, got.AiConfidence) != 0.82 {
		t.Fatalf("ai fields = %q %q %+v %v", got.AiDamageType.String, got.AiSummary.String, got.AiConfidence, got.AiTriagedAt)
	}
	if got.Status != StatusCenterReview {
		t.Fatalf("status changed to %q", got.Status)
	}
	if evs := f.triageEvents(t, claim); len(evs) != 1 || evs[0].ToStatus.Valid || evs[0].ActorUserID.Valid {
		t.Fatalf("ai_triaged events = %+v", evs)
	}
	reqs := f.llm.Requests()
	if len(reqs) != 1 {
		t.Fatalf("model calls = %d", len(reqs))
	}
	req := reqs[0]
	if req.Model != "claude-haiku-4-5" || len(req.Tools) != 1 || req.Tools[0].Name != "record_triage" {
		t.Fatalf("request model %q tools %+v", req.Model, req.Tools)
	}
	var images int
	var text string
	for _, b := range req.Messages[0].Content {
		switch b.Type {
		case llm.BlockImage:
			images++
		case llm.BlockText:
			text += b.Text
		}
	}
	if images != 1 || !strings.Contains(text, "body_kaput") || !strings.Contains(text, "Film kalkti") {
		t.Fatalf("images %d text %q", images, text)
	}
	if used := f.systemTokens(t) - before; used != 960 {
		t.Fatalf("system pool tokens = %d, want 960", used)
	}
	var channel, purpose string
	if err := f.tx.QueryRow(f.ctx, `SELECT channel, purpose FROM ai_usage
		WHERE organization_id = $1 AND pool = 'system' ORDER BY id DESC LIMIT 1`, f.center.ID).Scan(&channel, &purpose); err != nil {
		t.Fatal(err)
	}
	if channel != aimodel.UsageChannelTriage || purpose != aimodel.PurposeTriage {
		t.Fatalf("usage channel %q purpose %q", channel, purpose)
	}
}

func TestTriageDuplicateEventCallsModelOnce(t *testing.T) {
	f := newTriageFixture(t, triageFeatures{}, triageTurn("bubbling", 0.7))
	claim := f.submitted(t)

	f.publishStatus(t, claim, StatusCenterReview)
	f.publishStatus(t, claim, StatusCenterReview)

	if n := len(f.llm.Requests()); n != 1 {
		t.Fatalf("model calls = %d, want 1", n)
	}
	if evs := f.triageEvents(t, claim); len(evs) != 1 {
		t.Fatalf("ai_triaged events = %d, want 1", len(evs))
	}
}

func TestTriageOtherStatusesDoNotTrigger(t *testing.T) {
	f := newTriageFixture(t, triageFeatures{})
	claim := f.submitted(t)
	f.publishStatus(t, claim, StatusApproved)
	if n := len(f.llm.Requests()); n != 0 {
		t.Fatalf("model calls = %d", n)
	}
}

func TestTriageOutOfEnumBecomesOtherLowConfidence(t *testing.T) {
	f := newTriageFixture(t, triageFeatures{}, triageTurn("delamination", 0.95))
	claim := f.submitted(t)

	f.publishStatus(t, claim, StatusCenterReview)

	got := f.reload(t, claim)
	if got.AiDamageType.String != DamageOther || confidence(t, got.AiConfidence) > TriageUnknownConfidence {
		t.Fatalf("ai fields = %q %+v", got.AiDamageType.String, got.AiConfidence)
	}
}

func TestTriagePoolFullSkipsModel(t *testing.T) {
	f := newTriageFixture(t, triageFeatures{}, triageTurn("peeling", 0.8))
	claim := f.submitted(t)
	settings, err := f.q.GetAISettings(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	limit := settings.SystemPoolMonthlyQuota
	if limit == 0 {
		limit = 1000
		if _, err := f.tx.Exec(f.ctx, `UPDATE ai_settings SET system_pool_monthly_quota = $1 WHERE id = 1`, limit); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := airepo.New(f.tx).RecordUsage(f.ctx, airepo.Usage{
		OrganizationID: f.center.ID, BrandID: f.brand.ID, Pool: aimodel.PoolSystem,
		Channel: aimodel.UsageChannelWhatsApp, Purpose: aimodel.PurposeChat, Model: "fake", InputTokens: limit,
	}); err != nil {
		t.Fatal(err)
	}

	f.publishStatus(t, claim, StatusCenterReview)

	if n := len(f.llm.Requests()); n != 0 {
		t.Fatalf("model calls = %d with a full pool", n)
	}
	if got := f.reload(t, claim); got.AiTriagedAt.Valid || got.Status != StatusCenterReview {
		t.Fatalf("claim = %+v", got)
	}
	if _, _, err := f.triage.Run(f.ctx, TriageRun{ClaimUUID: claim.Uuid, Force: true}); !errors.Is(err, ErrTriageQuota) {
		t.Fatalf("manual run err = %v", err)
	}
}

func TestTriageModuleOffOnCenterSkips(t *testing.T) {
	f := newTriageFixture(t, triageFeatures{off: true}, triageTurn("peeling", 0.8))
	claim := f.submitted(t)

	f.publishStatus(t, claim, StatusCenterReview)

	if n := len(f.llm.Requests()); n != 0 {
		t.Fatalf("model calls = %d with ai_assistant off", n)
	}
	if evs := f.triageEvents(t, claim); len(evs) != 0 {
		t.Fatalf("ai_triaged events = %d", len(evs))
	}
}

func TestTriageRetriggerOverwrites(t *testing.T) {
	f := newTriageFixture(t, triageFeatures{}, triageTurn("peeling", 0.8), triageTurn("install_error", 0.6))
	claim := f.submitted(t)
	f.publishStatus(t, claim, StatusCenterReview)

	center := Caller{
		UserID: f.user.ID, OrganizationID: f.center.ID, BrandID: f.brand.ID, OrgType: "center",
		Filter: scopefilter.Filter{Scope: rbac.ScopeAll},
		Permissions: map[string]rbac.Scope{
			rbac.PermWarrantyClaimsRead: rbac.ScopeAll, rbac.PermWarrantyClaimsDecide: rbac.ScopeAll,
		},
	}
	view, err := f.svc.Retrigger(f.ctx, f.triage, center, claim.Uuid)
	if err != nil {
		t.Fatalf("retrigger: %v", err)
	}
	if view.AIDamageType == nil || *view.AIDamageType != DamageInstallError || view.Status != StatusCenterReview {
		t.Fatalf("view = %+v", view)
	}
	evs := f.triageEvents(t, claim)
	if len(evs) != 2 || evs[1].ActorUserID.Int64 != f.user.ID {
		t.Fatalf("ai_triaged events = %+v", evs)
	}

	dealer := f.apiCaller(claim)
	if _, err := f.svc.Retrigger(f.ctx, f.triage, dealer, claim.Uuid); !errors.Is(err, ErrForbidden) {
		t.Fatalf("dealer retrigger err = %v", err)
	}
}

func TestParseTriage(t *testing.T) {
	tool := func(input string) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.Block{{
			Type: llm.BlockToolUse, ID: "t", Name: "record_triage", Input: []byte(input),
		}}}
	}
	cases := []struct {
		name   string
		msg    llm.Message
		damage string
		conf   float64
	}{
		{"tool", tool(`{"damage_type":"Scratch","summary":"Çizik","confidence":0.71}`), DamageScratch, 0.71},
		{"percent", tool(`{"damage_type":"stain","summary":"Leke","confidence":85}`), DamageStain, 0.85},
		{"string confidence", tool(`{"damage_type":"cracking","summary":"x","confidence":"0.4"}`), DamageCracking, 0.4},
		{"out of enum", tool(`{"damage_type":"rust","summary":"x","confidence":0.9}`), DamageOther, TriageUnknownConfidence},
		{"text json", llm.Message{Content: []llm.Block{llm.TextBlock(`Sonuç: {"damage_type":"yellowing","summary":"Sararma","confidence":0.5}`)}}, DamageYellowing, 0.5},
		{"no answer", llm.Message{Content: []llm.Block{llm.TextBlock("Fotoğraf net değil.")}}, DamageOther, TriageUnknownConfidence},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseTriage(c.msg)
			if got.DamageType != c.damage || got.Confidence != c.conf {
				t.Fatalf("got %+v, want %s %v", got, c.damage, c.conf)
			}
		})
	}
}
