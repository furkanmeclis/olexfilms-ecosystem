package usecase

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// Pure unit tests (no DB, no network) for the SmartPTG curve proxy (TEC-934).

type fakeUpstream struct {
	mu    sync.Mutex
	calls []string
	// by URL: status + body, or err.
	resp map[string]fakeResp
}

type fakeResp struct {
	status int
	body   string
	err    error
	wait   chan struct{}
}

func (f *fakeUpstream) Do(req *http.Request) (*http.Response, error) {
	url := req.URL.String()
	f.mu.Lock()
	f.calls = append(f.calls, url)
	r, ok := f.resp[url]
	f.mu.Unlock()
	if r.wait != nil {
		<-r.wait
	}
	if !ok {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("nf"))}, nil
	}
	if r.err != nil {
		return nil, r.err
	}
	return &http.Response{StatusCode: r.status, Body: io.NopCloser(strings.NewReader(r.body))}, nil
}

func (f *fakeUpstream) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func realCurve(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/smartptg_32475.dat")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var testSources = []string{"http://a.test/char/%s.dat", "https://b.test/char/%s.dat"}

func newTestCurves(store CurveStore, up *fakeUpstream) (*Curves, *time.Time) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	c := NewCurves(store, up, CurveConfig{Sources: testSources, Timeout: time.Second}, nil)
	c.now = func() time.Time { return now }
	return c, &now
}

func TestNormalizeCurveSerial(t *testing.T) {
	for in, want := range map[string]string{"32475": "32475", " 0032475 ": "32475", "1": "1", "999999999": "999999999"} {
		got, err := NormalizeCurveSerial(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "0", "abc", "12a", "-1", "1234567890", "../1", "1.0", "１２"} {
		if _, err := NormalizeCurveSerial(in); !errors.Is(err, ErrCurveSerialInvalid) {
			t.Errorf("%q: want invalid, got %v", in, err)
		}
	}
}

func TestValidCurveFile(t *testing.T) {
	if err := ValidCurveFile([]byte(realCurve(t))); err != nil {
		t.Fatalf("real factory file rejected: %v", err)
	}
	bad := []string{
		"", "<html><body>404 Not Found</body></html>",
		"short:AAAA",
		"0123456789abcdef:not-base64!!!!!!!!!!!!!!!!!!!",
		"0123456789abcdef:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==", // 28 bytes, not whole blocks
		strings.Repeat("A", CurveMaxBytes+1),
	}
	for _, b := range bad {
		if ValidCurveFile([]byte(b)) == nil {
			t.Errorf("accepted %.40q", b)
		}
	}
}

func TestCurvesFetchesStoresAndServesFromCache(t *testing.T) {
	body := realCurve(t)
	up := &fakeUpstream{resp: map[string]fakeResp{
		"http://a.test/char/32475.dat": {status: http.StatusOK, body: body},
	}}
	store := storage.NewMemory()
	c, _ := newTestCurves(store, up)

	got, err := c.Get(context.Background(), "32475")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Data) != body || got.Serial != "32475" || !strings.HasPrefix(got.ETag, `"`) {
		t.Fatalf("unexpected curve %+v", got)
	}
	if ok, _ := store.Exists(context.Background(), "smartptg/curves/32475.dat"); !ok {
		t.Fatal("curve not persisted")
	}

	// A fresh process (no memory) with the vendor down still serves it.
	down := &fakeUpstream{resp: map[string]fakeResp{
		"http://a.test/char/32475.dat":  {err: errors.New("dial tcp: refused")},
		"https://b.test/char/32475.dat": {err: errors.New("dial tcp: refused")},
	}}
	c2, _ := newTestCurves(store, down)
	again, err := c2.Get(context.Background(), "032475")
	if err != nil || string(again.Data) != body || again.ETag != got.ETag {
		t.Fatalf("cache miss: %v", err)
	}
	if down.count() != 0 {
		t.Fatalf("upstream called %d times despite cache", down.count())
	}
}

func TestCurvesFallsThroughSourcesInOrder(t *testing.T) {
	body := realCurve(t)
	up := &fakeUpstream{resp: map[string]fakeResp{
		"http://a.test/char/32475.dat":  {status: http.StatusOK, body: "<html>no such serial</html>"},
		"https://b.test/char/32475.dat": {status: http.StatusOK, body: body},
	}}
	c, _ := newTestCurves(nil, up)
	got, err := c.Get(context.Background(), "32475")
	if err != nil || string(got.Data) != body {
		t.Fatalf("got %v", err)
	}
	if up.count() != 2 || up.calls[0] != "http://a.test/char/32475.dat" {
		t.Fatalf("calls %v", up.calls)
	}
}

func TestCurvesNegativeCacheForAnHour(t *testing.T) {
	up := &fakeUpstream{resp: map[string]fakeResp{}}
	c, now := newTestCurves(storage.NewMemory(), up)

	if _, err := c.Get(context.Background(), "1355"); !errors.Is(err, ErrCurveNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
	if up.count() != 2 {
		t.Fatalf("calls %d", up.count())
	}
	*now = now.Add(59 * time.Minute)
	if _, err := c.Get(context.Background(), "1355"); !errors.Is(err, ErrCurveNotFound) {
		t.Fatal(err)
	}
	if up.count() != 2 {
		t.Fatalf("miss not remembered: %d calls", up.count())
	}
	*now = now.Add(2 * time.Minute)
	_, _ = c.Get(context.Background(), "1355")
	if up.count() != 4 {
		t.Fatalf("miss not expired: %d calls", up.count())
	}
}

func TestCurvesUpstreamOutageIsNotANotFound(t *testing.T) {
	up := &fakeUpstream{resp: map[string]fakeResp{
		"http://a.test/char/7.dat":  {err: errors.New("timeout")},
		"https://b.test/char/7.dat": {status: http.StatusBadGateway},
	}}
	c, now := newTestCurves(nil, up)
	if _, err := c.Get(context.Background(), "7"); !errors.Is(err, ErrCurveUpstreamUnavailable) {
		t.Fatalf("want unavailable, got %v", err)
	}
	// Short memory (1 min default) for outages.
	_, _ = c.Get(context.Background(), "7")
	if up.count() != 2 {
		t.Fatalf("outage not remembered: %d", up.count())
	}
	*now = now.Add(61 * time.Second)
	_, _ = c.Get(context.Background(), "7")
	if up.count() != 4 {
		t.Fatalf("outage memory too long: %d", up.count())
	}
}

func TestCurvesRejectsOversizedUpstream(t *testing.T) {
	up := &fakeUpstream{resp: map[string]fakeResp{
		"http://a.test/char/9.dat": {status: http.StatusOK, body: "0123456789abcdef:" + strings.Repeat("A", CurveMaxBytes)},
	}}
	c, _ := newTestCurves(nil, up)
	if _, err := c.Get(context.Background(), "9"); !errors.Is(err, ErrCurveNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestCurvesConcurrentRequestsShareOneFetch(t *testing.T) {
	gate := make(chan struct{})
	up := &fakeUpstream{resp: map[string]fakeResp{
		"http://a.test/char/32475.dat": {status: http.StatusOK, body: realCurve(t), wait: gate},
	}}
	c, _ := newTestCurves(nil, up)
	var wg sync.WaitGroup
	var ok atomic.Int32
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Get(context.Background(), "32475"); err == nil {
				ok.Add(1)
			}
		}()
	}
	for up.count() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(gate)
	wg.Wait()
	if ok.Load() != 8 || up.count() != 1 {
		t.Fatalf("ok=%d calls=%d", ok.Load(), up.count())
	}
}

func TestCurvesInvalidSerialNeverHitsUpstream(t *testing.T) {
	up := &fakeUpstream{}
	c, _ := newTestCurves(nil, up)
	if _, err := c.Get(context.Background(), "../etc"); !errors.Is(err, ErrCurveSerialInvalid) {
		t.Fatal(err)
	}
	if up.count() != 0 {
		t.Fatal("upstream called")
	}
}
