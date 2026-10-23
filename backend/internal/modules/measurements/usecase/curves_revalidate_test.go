package usecase

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// TEC-934 review round: re-validation, miss cache cap, redirect policy.

// otherCurve is a valid curve file different from the real one.
func otherCurve() string {
	return "0123456789abcdef:" + base64.StdEncoding.EncodeToString(make([]byte, 32))
}

type countingStore struct {
	*storage.Memory
	downloads atomic.Int32
}

func (s *countingStore) Download(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	s.downloads.Add(1)
	return s.Memory.Download(ctx, path)
}

func storedBody(t *testing.T, store CurveStore, serial string) string {
	t.Helper()
	rc, _, err := store.Download(context.Background(), SmartPTGCurveObjectKey(serial))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	b, _ := io.ReadAll(rc)
	return string(b)
}

func (f *fakeUpstream) set(url string, r fakeResp) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resp[url] = r
}

func TestCurvesRevalidatesStoredCopy(t *testing.T) {
	up := &fakeUpstream{resp: map[string]fakeResp{
		"http://a.test/char/32475.dat": {status: http.StatusOK, body: realCurve(t)},
	}}
	store := storage.NewMemory()
	c, now := newTestCurves(store, up)
	first, err := c.Get(context.Background(), "32475")
	if err != nil {
		t.Fatal(err)
	}

	// Within RevalidateAfter (24 h default): served from the store only.
	*now = now.Add(23 * time.Hour)
	if _, err := c.Get(context.Background(), "32475"); err != nil || up.count() != 1 {
		t.Fatalf("err=%v calls=%d", err, up.count())
	}

	// Due: the vendor now has another file, which replaces the stored copy.
	*now = now.Add(2 * time.Hour)
	up.set("http://a.test/char/32475.dat", fakeResp{status: http.StatusOK, body: otherCurve()})
	got, err := c.Get(context.Background(), "32475")
	if err != nil || string(got.Data) != otherCurve() || got.ETag == first.ETag {
		t.Fatalf("not replaced: %v", err)
	}
	if storedBody(t, store, "32475") != otherCurve() || up.count() != 2 {
		t.Fatalf("store not overwritten, calls=%d", up.count())
	}

	// Due again with the vendor down: keep serving the stored copy and
	// retry only after the not-found TTL (1 h).
	*now = now.Add(25 * time.Hour)
	up.set("http://a.test/char/32475.dat", fakeResp{err: errors.New("down")})
	up.set("https://b.test/char/32475.dat", fakeResp{err: errors.New("down")})
	got, err = c.Get(context.Background(), "32475")
	if err != nil || string(got.Data) != otherCurve() || up.count() != 4 {
		t.Fatalf("outage: err=%v calls=%d", err, up.count())
	}
	*now = now.Add(30 * time.Minute)
	if _, err := c.Get(context.Background(), "32475"); err != nil || up.count() != 4 {
		t.Fatalf("retried too early: calls=%d", up.count())
	}
	*now = now.Add(31 * time.Minute)
	_, _ = c.Get(context.Background(), "32475")
	if up.count() != 6 {
		t.Fatalf("no retry after backoff: calls=%d", up.count())
	}
}

func TestCurvesStoredCopyWithoutSidecarRevalidates(t *testing.T) {
	store := storage.NewMemory()
	if err := store.Upload(context.Background(), storage.File{Body: strings.NewReader(realCurve(t))},
		SmartPTGCurveObjectKey("32475")); err != nil {
		t.Fatal(err)
	}
	up := &fakeUpstream{resp: map[string]fakeResp{
		"http://a.test/char/32475.dat": {status: http.StatusOK, body: realCurve(t)},
	}}
	c, _ := newTestCurves(store, up)
	if _, err := c.Get(context.Background(), "32475"); err != nil || up.count() != 1 {
		t.Fatalf("err=%v calls=%d", err, up.count())
	}
	if _, err := c.Get(context.Background(), "32475"); err != nil || up.count() != 1 {
		t.Fatalf("sidecar not written: calls=%d", up.count())
	}
}

func TestCurvesNegativeCacheBeforeStore(t *testing.T) {
	store := &countingStore{Memory: storage.NewMemory()}
	c, _ := newTestCurves(store, &fakeUpstream{})
	_, _ = c.Get(context.Background(), "1355")
	before := store.downloads.Load()
	if _, err := c.Get(context.Background(), "1355"); !errors.Is(err, ErrCurveNotFound) {
		t.Fatal(err)
	}
	if store.downloads.Load() != before {
		t.Fatal("remembered miss still read the store")
	}
}

func TestCurvesMissCacheIsCapped(t *testing.T) {
	c, now := newTestCurves(nil, &fakeUpstream{})
	c.mu.Lock()
	defer c.mu.Unlock()
	c.remember("expired", ErrCurveNotFound, time.Second)
	*now = now.Add(time.Minute)
	for i := range curveMaxMisses - 1 {
		c.remember(strconv.Itoa(i+1), ErrCurveNotFound, time.Hour+time.Duration(i)*time.Millisecond)
	}
	if len(c.misses) != curveMaxMisses {
		t.Fatalf("len %d", len(c.misses))
	}
	// Full: the expired entry goes first.
	c.remember("a", ErrCurveNotFound, 2*time.Hour)
	if _, ok := c.misses["expired"]; ok || len(c.misses) != curveMaxMisses {
		t.Fatalf("expired kept, len %d", len(c.misses))
	}
	// Still full, nothing expired: the soonest to expire goes.
	c.remember("b", ErrCurveNotFound, 2*time.Hour)
	if _, ok := c.misses["1"]; ok || len(c.misses) != curveMaxMisses {
		t.Fatalf("oldest kept, len %d", len(c.misses))
	}
}

func TestCurveHTTPClientFollowsOnlySameHostRedirects(t *testing.T) {
	body := realCurve(t)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, otherCurve())
	}))
	defer other.Close()
	var src *httptest.Server
	src = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/same/32475.dat":
			http.Redirect(w, r, src.URL+"/files/32475.dat", http.StatusFound)
		case "/files/32475.dat":
			_, _ = io.WriteString(w, body)
		case "/away/32475.dat":
			http.Redirect(w, r, other.URL+"/x.dat", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer src.Close()

	same := NewCurves(nil, nil, CurveConfig{Sources: []string{src.URL + "/same/%s.dat"}}, nil)
	got, err := same.Get(context.Background(), "32475")
	if err != nil || string(got.Data) != body {
		t.Fatalf("same-host redirect: %v", err)
	}
	away := NewCurves(nil, nil, CurveConfig{Sources: []string{src.URL + "/away/%s.dat"}}, nil)
	if _, err := away.Get(context.Background(), "32475"); !errors.Is(err, ErrCurveNotFound) {
		t.Fatalf("cross-host redirect followed: %v", err)
	}
}
