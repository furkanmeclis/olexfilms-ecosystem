package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealershowcase/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/places"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
)

// TEC-469 acceptance (F5-01d): the Places worker against a fake HTTP
// Places API and the manual rating entry.

// fakePlaces is a Places API (New) stand-in: per place id a JSON body or a
// status code; it counts the calls per place id.
type fakePlaces struct {
	mu     sync.Mutex
	srv    *httptest.Server
	bodies map[string]string
	codes  map[string]int
	calls  map[string]int
}

func newFakePlaces(t *testing.T) *fakePlaces {
	t.Helper()
	fp := &fakePlaces{bodies: map[string]string{}, codes: map[string]int{}, calls: map[string]int{}}
	fp.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/places/")
		fp.mu.Lock()
		defer fp.mu.Unlock()
		fp.calls[id]++
		if r.Header.Get("X-Goog-FieldMask") != places.FieldMask || r.Header.Get("X-Goog-Api-Key") != "test-key" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if code, ok := fp.codes[id]; ok {
			w.WriteHeader(code)
			return
		}
		body, ok := fp.bodies[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound) // rows of other tests
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(fp.srv.Close)
	return fp
}

func (fp *fakePlaces) client(key string) *places.Client {
	return places.New(key, places.WithBaseURL(fp.srv.URL))
}

func (fp *fakePlaces) callsOf(id string) int {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	return fp.calls[id]
}

func (f *fixture) placeID(name string) string {
	return "ChIJ" + strings.ReplaceAll(f.prefix, "-", "_") + "_" + name
}

// withPlaceID saves the dealer showcase with a place id.
func (f *fixture) withPlaceID(t *testing.T, id string) db.DealerShowcase {
	t.Helper()
	in := content("Ankara PPF")
	in.GooglePlaceID = &id
	if _, err := f.svc.Save(f.ctx, f.ownerCaller(), nil, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	return f.showcase(t)
}

func (f *fixture) showcase(t *testing.T) db.DealerShowcase {
	t.Helper()
	row, err := f.q.GetDealerShowcaseByOrg(f.ctx, f.dealer.ID)
	if err != nil {
		t.Fatalf("showcase: %v", err)
	}
	return row
}

func (f *fixture) refresher(p PlacesClient, failures FailureStore) *RatingRefresher {
	return NewRatingRefresher(f.q, p, f.feat, failures, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func ratingString(row db.DealerShowcase) string {
	r := numericPtr(row.GoogleRating)
	if r == nil {
		return "nil"
	}
	return fmt.Sprintf("%.1f/%d/%s", *r, row.GoogleReviewCount.Int32, row.GoogleRatingSource.String)
}

// Two runs the same day: one Places call, one update, the same result.
func TestRatingRefresherRunTwiceSameResult(t *testing.T) {
	f := newFixture(t)
	fp := newFakePlaces(t)
	id := f.placeID("twice")
	fp.bodies[id] = `{"rating":4.64,"userRatingCount":87}`
	f.withPlaceID(t, id)
	r := f.refresher(fp.client("test-key"), nil)

	if _, err := r.Run(f.ctx); err != nil {
		t.Fatal(err)
	}
	first := f.showcase(t)
	if got := ratingString(first); got != "4.6/87/places" || !first.GoogleRatingUpdatedAt.Valid {
		t.Fatalf("after first run: %s", got)
	}
	if _, err := r.Run(f.ctx); err != nil {
		t.Fatal(err)
	}
	second := f.showcase(t)
	if fp.callsOf(id) != 1 {
		t.Fatalf("places calls = %d, want 1", fp.callsOf(id))
	}
	if ratingString(second) != ratingString(first) || !second.GoogleRatingUpdatedAt.Time.Equal(first.GoogleRatingUpdatedAt.Time) ||
		!second.UpdatedAt.Time.Equal(first.UpdatedAt.Time) {
		t.Fatalf("second run changed the row: %s %v → %s %v", ratingString(first), first.GoogleRatingUpdatedAt.Time,
			ratingString(second), second.GoogleRatingUpdatedAt.Time)
	}
}

// No API key: the job is a no-op (no call, no write, no error).
func TestRatingRefresherNoKeyNoop(t *testing.T) {
	f := newFixture(t)
	fp := newFakePlaces(t)
	id := f.placeID("nokey")
	fp.bodies[id] = `{"rating":4.9,"userRatingCount":3}`
	f.withPlaceID(t, id)
	for _, p := range []PlacesClient{fp.client(""), nil} {
		res, err := f.refresher(p, nil).Run(f.ctx)
		if err != nil || res != (RefreshResult{}) {
			t.Fatalf("run without key: %+v %v", res, err)
		}
	}
	if fp.callsOf(id) != 0 || ratingString(f.showcase(t)) != "nil" {
		t.Fatalf("no-op touched Places or the row: %d %s", fp.callsOf(id), ratingString(f.showcase(t)))
	}
}

// A 5xx keeps the old value (no fallback to manual), backs the
// organization off and the third failure in a row writes the activity log
// once.
func TestRatingRefresher5xxKeepsOldValue(t *testing.T) {
	f := newFixture(t)
	fp := newFakePlaces(t)
	id := f.placeID("fail")
	row := f.withPlaceID(t, id)
	if _, err := f.tx.Exec(f.ctx, `UPDATE dealer_showcases SET google_rating = 4.2, google_review_count = 50,
		google_rating_source = 'places', google_rating_updated_at = NOW() - interval '2 days' WHERE id = $1`, row.ID); err != nil {
		t.Fatal(err)
	}
	fp.codes[id] = http.StatusServiceUnavailable
	failures := NewMemoryFailures()
	r := f.refresher(fp.client("test-key"), failures)
	now := time.Now()
	r.now = func() time.Time { return now }

	if _, err := r.Run(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got := ratingString(f.showcase(t)); got != "4.2/50/places" {
		t.Fatalf("after 503: %s", got)
	}
	// Backing off: an immediate rerun does not call Places.
	if _, err := r.Run(f.ctx); err != nil || fp.callsOf(id) != 1 {
		t.Fatalf("backoff rerun: calls %d, %v", fp.callsOf(id), err)
	}
	for i := 2; i <= 4; i++ {
		now = now.Add(RatingBackoff(i-1) + time.Minute)
		if _, err := r.Run(f.ctx); err != nil {
			t.Fatal(err)
		}
		if fp.callsOf(id) != i {
			t.Fatalf("run %d: calls %d", i, fp.callsOf(id))
		}
	}
	if got := ratingString(f.showcase(t)); got != "4.2/50/places" {
		t.Fatalf("after four failures: %s", got)
	}
	var logs int
	if err := f.tx.QueryRow(f.ctx, `SELECT COUNT(*) FROM activity_events WHERE action = $1 AND resource_uuid = $2`,
		ActionGoogleRatingRefreshFailed, row.Uuid).Scan(&logs); err != nil {
		t.Fatal(err)
	}
	if logs != 1 {
		t.Fatalf("activity entries = %d, want 1", logs)
	}

	// Recovery resets the backoff and writes the new value.
	delete(fp.codes, id)
	fp.bodies[id] = `{"rating":4.4,"userRatingCount":52}`
	now = now.Add(RatingBackoff(4) + time.Minute)
	if _, err := r.Run(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got := ratingString(f.showcase(t)); got != "4.4/52/places" {
		t.Fatalf("after recovery: %s", got)
	}
	if next, _ := failures.NextAttempt(f.ctx, f.dealer.ID); !next.IsZero() {
		t.Fatalf("backoff not reset: %v", next)
	}
}

// An organization with the module off is skipped.
func TestRatingRefresherSkipsModuleOff(t *testing.T) {
	f := newFixture(t)
	fp := newFakePlaces(t)
	id := f.placeID("off")
	fp.bodies[id] = `{"rating":4.1,"userRatingCount":9}`
	f.withPlaceID(t, id)
	f.feat.off[f.dealer.ID] = true
	if _, err := f.refresher(fp.client("test-key"), nil).Run(f.ctx); err != nil {
		t.Fatal(err)
	}
	if fp.callsOf(id) != 0 || ratingString(f.showcase(t)) != "nil" {
		t.Fatalf("module off refreshed: calls %d, %s", fp.callsOf(id), ratingString(f.showcase(t)))
	}
}

// A quota answer stops the run; the organization is backed off.
func TestRatingRefresherQuotaStops(t *testing.T) {
	f := newFixture(t)
	fp := newFakePlaces(t)
	id := f.placeID("quota")
	f.withPlaceID(t, id)
	fp.codes[id] = http.StatusTooManyRequests
	failures := NewMemoryFailures()
	res, err := f.refresher(fp.client("test-key"), failures).Run(f.ctx)
	if err != nil || res.Failed < 1 {
		t.Fatalf("quota run: %+v %v", res, err)
	}
	if next, _ := failures.NextAttempt(f.ctx, f.dealer.ID); next.IsZero() {
		t.Fatal("quota did not back off")
	}
}

func TestRatingBackoffAndRedisFailures(t *testing.T) {
	for n, want := range map[int]time.Duration{1: 20 * time.Hour, 2: 40 * time.Hour, 3: 80 * time.Hour, 5: 7 * 24 * time.Hour, 40: 7 * 24 * time.Hour} {
		if got := RatingBackoff(n); got != want {
			t.Errorf("backoff(%d) = %v, want %v", n, got, want)
		}
	}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = rdb.Close() }()
	s := NewRedisFailures(rdb, "test")
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0)
	for i := 1; i <= 2; i++ {
		if n, err := s.Fail(ctx, 7, now); err != nil || n != i {
			t.Fatalf("fail %d: %d %v", i, n, err)
		}
	}
	if next, err := s.NextAttempt(ctx, 7); err != nil || !next.Equal(now.Add(40*time.Hour)) {
		t.Fatalf("next = %v %v", next, err)
	}
	if err := s.Reset(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if next, err := s.NextAttempt(ctx, 7); err != nil || !next.IsZero() {
		t.Fatalf("after reset: %v %v", next, err)
	}
}

type stubPlaces struct{ configured bool }

func (s stubPlaces) Configured() bool { return s.configured }
func (s stubPlaces) Rating(context.Context, string) (places.Rating, error) {
	return places.Rating{}, errors.New("not called")
}

func ptrF(v float64) *float64 { return &v }
func ptrI(v int64) *int64     { return &v }

// Manual entry: refused while Places owns the rating, range checked,
// allowed without a key or without a place id.
func TestManualGoogleRating(t *testing.T) {
	f := newFixture(t)
	f.svc.SetPlaces(stubPlaces{configured: true})
	f.withPlaceID(t, f.placeID("manual"))
	if _, err := f.svc.SetGoogleRating(f.ctx, f.ownerCaller(), nil, RatingInput{Rating: ptrF(4.5), ReviewCount: ptrI(10)}); !errors.Is(err, ErrRatingManagedByPlaces) {
		t.Fatalf("places configured + place id: %v", err)
	}
	v, err := f.svc.Get(f.ctx, f.ownerCaller(), nil)
	if err != nil || !v.PlacesConfigured || v.ManualRatingAllowed {
		t.Fatalf("view flags: %+v %v", v, err)
	}

	var re *RatingRangeError
	for _, in := range []RatingInput{
		{Rating: ptrF(0.9), ReviewCount: ptrI(1)}, {Rating: ptrF(5.01), ReviewCount: ptrI(1)}, {Rating: ptrF(4), ReviewCount: ptrI(-1)},
	} {
		if _, err := f.svc.SetGoogleRating(f.ctx, f.ownerCaller(), nil, in); !errors.As(err, &re) {
			t.Fatalf("out of range %v/%v: %v", *in.Rating, *in.ReviewCount, err)
		}
	}
	var ve *ValidationError
	if _, err := f.svc.SetGoogleRating(f.ctx, f.ownerCaller(), nil, RatingInput{Rating: ptrF(4)}); !errors.As(err, &ve) {
		t.Fatalf("missing count: %v", err)
	}

	// No key: manual entry works even with a place id.
	f.svc.SetPlaces(stubPlaces{})
	got, err := f.svc.SetGoogleRating(f.ctx, f.ownerCaller(), nil, RatingInput{Rating: ptrF(4.25), ReviewCount: ptrI(0)})
	if err != nil || got.GoogleRating == nil || *got.GoogleRating != 4.3 || got.GoogleRatingSource == nil ||
		*got.GoogleRatingSource != model.RatingSourceManual || !got.ManualRatingAllowed {
		t.Fatalf("manual without key: %+v %v", got, err)
	}

	// Key set but no place id: allowed too; null/null clears.
	f.svc.SetPlaces(stubPlaces{configured: true})
	in := content("Ankara PPF")
	if _, err := f.svc.Save(f.ctx, f.ownerCaller(), nil, in); err != nil {
		t.Fatal(err)
	}
	if got, err := f.svc.SetGoogleRating(f.ctx, f.ownerCaller(), nil, RatingInput{}); err != nil || got.GoogleRating != nil ||
		got.GoogleRatingSource != nil {
		t.Fatalf("clear: %+v %v", got, err)
	}
}

// With approval on, a manual rating reaches the public page only through
// the approved snapshot; with approval off it is live at once. A Places
// rating is always live.
func TestManualRatingFollowsApproval(t *testing.T) {
	f := newFixture(t)
	publicRatingOf := func() string {
		t.Helper()
		b, err := f.svc.PublicBlock(f.ctx, f.brandID, f.dealer.Slug, "tr")
		if err != nil || b == nil {
			t.Fatalf("public: %v %v", b, err)
		}
		p := b.(PublicShowcase)
		if p.GoogleRating == nil {
			return "nil"
		}
		return fmt.Sprintf("%.1f/%s", *p.GoogleRating, *p.GoogleRatingSource)
	}
	badge := func() string {
		t.Helper()
		m, err := f.svc.Badges(f.ctx, f.brandID, []uuid.UUID{f.dealer.Uuid})
		if err != nil {
			t.Fatal(err)
		}
		if m[f.dealer.Uuid] == nil {
			return "nil"
		}
		return fmt.Sprintf("%.1f", *m[f.dealer.Uuid])
	}
	if _, err := f.svc.Save(f.ctx, f.ownerCaller(), nil, content("Ankara PPF")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Submit(f.ctx, f.ownerCaller(), nil); err != nil {
		t.Fatal(err)
	}
	// Approval off: live at once.
	if _, err := f.svc.SetGoogleRating(f.ctx, f.ownerCaller(), nil, RatingInput{Rating: ptrF(4.0), ReviewCount: ptrI(5)}); err != nil {
		t.Fatal(err)
	}
	if got := publicRatingOf(); got != "4.0/manual" || badge() != "4.0" {
		t.Fatalf("approval off: %s %s", got, badge())
	}

	// Approval on: the snapshot (no rating yet) wins until the center approves.
	f.set.approval = true
	if got := publicRatingOf(); got != "nil" || badge() != "nil" {
		t.Fatalf("approval on before review: %s %s", got, badge())
	}
	if _, err := f.svc.Submit(f.ctx, f.ownerCaller(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Review(f.ctx, f.reviewerCaller(), f.dealer.Uuid, ReviewInput{Decision: DecisionApprove}); err != nil {
		t.Fatal(err)
	}
	if got := publicRatingOf(); got != "4.0/manual" {
		t.Fatalf("approved: %s", got)
	}
	if _, err := f.svc.SetGoogleRating(f.ctx, f.ownerCaller(), nil, RatingInput{Rating: ptrF(2.0), ReviewCount: ptrI(6)}); err != nil {
		t.Fatal(err)
	}
	if got := publicRatingOf(); got != "4.0/manual" || badge() != "4.0" {
		t.Fatalf("unapproved edit leaked: %s %s", got, badge())
	}

	// A Places rating is live without review.
	row := f.showcase(t)
	n, _ := ratingNumeric(4.8)
	if _, err := f.q.SetDealerShowcaseGoogleRating(f.ctx, db.SetDealerShowcaseGoogleRatingParams{
		ID: row.ID, GoogleRating: n, GoogleReviewCount: pgtype.Int4{Int32: 99, Valid: true},
		GoogleRatingSource: pgtype.Text{String: model.RatingSourcePlaces, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if got := publicRatingOf(); got != "4.8/places" || badge() != "4.8" {
		t.Fatalf("places: %s %s", got, badge())
	}
}

// The editor suggests the place id of the organization's Google Business
// link while google_place_id is empty; a cid-only link suggests nothing.
func TestPlaceIDSuggestion(t *testing.T) {
	f := newFixture(t)
	set := func(u string) {
		t.Helper()
		if _, err := f.tx.Exec(f.ctx, `UPDATE organizations SET google_business_url = $1 WHERE id = $2`, u, f.dealer.ID); err != nil {
			t.Fatal(err)
		}
	}
	set("https://search.google.com/local/writereview?placeid=ChIJsuggest_1")
	v, err := f.svc.Get(f.ctx, f.ownerCaller(), nil)
	if err != nil || v.GooglePlaceIDSuggestion == nil || *v.GooglePlaceIDSuggestion != "ChIJsuggest_1" {
		t.Fatalf("suggestion: %+v %v", v.GooglePlaceIDSuggestion, err)
	}
	set("https://maps.google.com/?cid=123456789")
	if v, err = f.svc.Get(f.ctx, f.ownerCaller(), nil); err != nil || v.GooglePlaceIDSuggestion != nil {
		t.Fatalf("cid suggestion: %+v %v", v.GooglePlaceIDSuggestion, err)
	}
	set("https://search.google.com/local/writereview?placeid=ChIJsuggest_1")
	f.withPlaceID(t, "ChIJown")
	if v, err = f.svc.Get(f.ctx, f.ownerCaller(), nil); err != nil || v.GooglePlaceIDSuggestion != nil {
		t.Fatalf("suggestion with a place id: %+v %v", v.GooglePlaceIDSuggestion, err)
	}
}
