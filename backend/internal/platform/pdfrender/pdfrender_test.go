package pdfrender

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const fakePDF = "%PDF-1.7\n%fake\n"

// fakeGotenberg records request bodies and answers with the given handler.
type fakeGotenberg struct {
	calls  atomic.Int32
	mu     sync.Mutex
	bodies []string
	srv    *httptest.Server
}

func newFake(t *testing.T, h func(n int32, w http.ResponseWriter, r *http.Request)) *fakeGotenberg {
	t.Helper()
	f := &fakeGotenberg{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := f.calls.Add(1)
		if err := r.ParseMultipartForm(32 << 20); err == nil {
			if fh := r.MultipartForm.File["files"]; len(fh) > 0 {
				file, _ := fh[0].Open()
				b, _ := io.ReadAll(file)
				_ = file.Close()
				f.mu.Lock()
				f.bodies = append(f.bodies, string(b))
				f.mu.Unlock()
			}
		}
		h(n, w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func okPDF(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = io.WriteString(w, fakePDF)
}

func fastOpts() Options {
	return Options{Timeout: 2 * time.Second, Backoff: 5 * time.Millisecond}
}

func TestConvertRetriesOn503(t *testing.T) {
	f := newFake(t, func(n int32, w http.ResponseWriter, _ *http.Request) {
		if n == 1 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		okPDF(w)
	})
	c := NewWithOptions(f.srv.URL, fastOpts())
	data, err := c.HTMLToPDF(context.Background(), "<p>x</p>")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != fakePDF || f.calls.Load() != 2 {
		t.Fatalf("calls=%d data=%q", f.calls.Load(), data)
	}
}

func TestConvertNoRetryOn400(t *testing.T) {
	f := newFake(t, func(_ int32, w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad form", http.StatusBadRequest)
	})
	c := NewWithOptions(f.srv.URL, fastOpts())
	_, err := c.HTMLToPDF(context.Background(), "<p>x</p>")
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusBadRequest {
		t.Fatalf("err = %v", err)
	}
	if f.calls.Load() != 1 {
		t.Fatalf("400 must not retry, calls=%d", f.calls.Load())
	}
}

func TestConvertGivesUpAfterMaxRetries(t *testing.T) {
	f := newFake(t, func(_ int32, w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	})
	c := NewWithOptions(f.srv.URL, fastOpts())
	if _, err := c.HTMLToPDF(context.Background(), "x"); err == nil {
		t.Fatal("want error")
	}
	if f.calls.Load() != 3 {
		t.Fatalf("1 attempt + 2 retries expected, calls=%d", f.calls.Load())
	}
}

func TestConvertAttemptTimeout(t *testing.T) {
	f := newFake(t, func(_ int32, w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
			okPDF(w)
		case <-r.Context().Done():
		}
	})
	c := NewWithOptions(f.srv.URL, Options{Timeout: 100 * time.Millisecond, MaxRetries: -1})
	start := time.Now()
	_, err := c.HTMLToPDF(context.Background(), "x")
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline error, got %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("timeout not enforced: %v", time.Since(start))
	}
}

func TestConvertCallerCancelStopsRetries(t *testing.T) {
	f := newFake(t, func(_ int32, w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "busy", http.StatusServiceUnavailable)
	})
	c := NewWithOptions(f.srv.URL, Options{Timeout: time.Second, Backoff: 200 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.HTMLToPDF(ctx, "x"); err == nil {
		t.Fatal("want error")
	}
	if f.calls.Load() != 1 {
		t.Fatalf("cancelled caller must stop retrying, calls=%d", f.calls.Load())
	}
}

func TestConvertRejectsNonPDF(t *testing.T) {
	f := newFake(t, func(_ int32, w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html>oops</html>")
	})
	c := NewWithOptions(f.srv.URL, fastOpts())
	if _, err := c.HTMLToPDF(context.Background(), "x"); !errors.Is(err, ErrNotPDF) {
		t.Fatalf("err = %v", err)
	}
	if f.calls.Load() != 1 {
		t.Fatalf("non-PDF must not retry, calls=%d", f.calls.Load())
	}
}

func TestNotConfigured(t *testing.T) {
	if _, err := New("").HTMLToPDF(context.Background(), "x"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
}

// 20 concurrent conversions against a 300 ms server finish in about one
// server latency: the client neither serializes nor starves the pool.
func TestConvertConcurrent(t *testing.T) {
	var inflight, peak atomic.Int32
	f := newFake(t, func(_ int32, w http.ResponseWriter, _ *http.Request) {
		cur := inflight.Add(1)
		for {
			p := peak.Load()
			if cur <= p || peak.CompareAndSwap(p, cur) {
				break
			}
		}
		time.Sleep(300 * time.Millisecond)
		inflight.Add(-1)
		okPDF(w)
	})
	c := NewWithOptions(f.srv.URL, Options{Timeout: 5 * time.Second})
	start := time.Now()
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.HTMLToPDF(context.Background(), Document{Lang: "tr", Body: "<p>Hizmet</p>"}.HTML())
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("20 parallel conversions took %v (serialized?)", elapsed)
	}
	if peak.Load() < 10 {
		t.Fatalf("peak concurrency %d, want ~20", peak.Load())
	}
	t.Logf("20 concurrent: %v, peak in-flight %d", elapsed, peak.Load())
}

func TestFillEscapesValues(t *testing.T) {
	tpl := `<p>{{customer_name}}</p><div>{{items_table}}</div><p>{{notes}}</p>`
	out := Fill(tpl, map[string]string{
		"customer_name": `<script>alert(1)</script><img src=http://evil/x>`,
		"items_table":   Table([]Column{{Label: "Ad"}, {Label: "Tutar", Numeric: true}}, [][]string{{"<b>x</b>", "10"}}),
		"notes":         "a\nb",
	}, map[string]bool{"items_table": true})
	if strings.Contains(out, "<script>") || strings.Contains(out, "<img") || strings.Contains(out, "<b>") {
		t.Fatalf("unescaped value in %s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") || !strings.Contains(out, `<td class="num">10</td>`) || !strings.Contains(out, "a<br>b") {
		t.Fatalf("unexpected fill output %s", out)
	}
}

func TestSanitizeHTML(t *testing.T) {
	in := `<html><head><meta http-equiv="refresh" content="0;url=http://x"><link rel="stylesheet" href="http://x/a.css"><base href="http://x/"></head>` +
		`<body><p onclick="x()" style="color:red">ok {{name}}</p><script>alert(1)</script><iframe src="http://x"></iframe>` +
		`<img src="http://evil/p.png"><img src="data:image/png;base64,AAAA"><a href="javascript:alert(1)">j</a><a href="https://olexfilms.app">l</a>` +
		`<style>@import url(http://x/y.css); .a{background:url(http://x/b.png)} .b{color:blue}</style>` +
		`<div style="background:url(http://x)">s</div><object data="x"><param name="a"></object></body></html>`
	out := SanitizeHTML(in)
	for _, bad := range []string{"<script", "alert(1)</", "<iframe", "<meta", "<link", "<base", "onclick", "http://evil", "javascript:", "@import", "url(http://x", "<object", "<html", "<body"} {
		if strings.Contains(strings.ToLower(out), strings.ToLower(bad)) {
			t.Errorf("sanitized output still contains %q:\n%s", bad, out)
		}
	}
	for _, good := range []string{`<p style="color:red">ok {{name}}</p>`, `src="data:image/png;base64,AAAA"`, `href="https://olexfilms.app"`, ".b{color:blue}", "<div>s</div>"} {
		if !strings.Contains(out, good) {
			t.Errorf("sanitized output lost %q:\n%s", good, out)
		}
	}
}

func TestDocumentCSPAndDirection(t *testing.T) {
	tr := Document{Lang: "tr", Title: "Hizmet", Body: "<p>Şükrü</p>"}.HTML()
	if !strings.Contains(tr, `dir="ltr"`) || !strings.Contains(tr, `content="`+CSP+`"`) {
		t.Fatalf("missing ltr/csp:\n%s", tr[:400])
	}
	if !strings.Contains(tr, "data:font/woff2;base64,") || strings.Contains(tr, `"Noto Sans Arabic";font-style`) {
		t.Fatal("tr document must embed latin fonts only")
	}

	ar := Document{Lang: "ar", Title: "خدمة", Body: "<p>مرحبا</p>"}.HTML()
	if !strings.Contains(ar, `<html lang="ar" dir="rtl">`) {
		t.Fatal("ar document must be rtl")
	}
	if !strings.Contains(ar, `font-family:"Noto Sans Arabic";font-style:normal;font-weight:400`) {
		t.Fatal("ar document must embed Noto Sans Arabic")
	}

	// Arabic text in an LTR document still pulls the Arabic faces.
	mixed := Document{Lang: "en", Body: "<p>Customer: محمد</p>"}.HTML()
	if !strings.Contains(mixed, `"Noto Sans Arabic";font-style`) {
		t.Fatal("arabic text must embed Noto Sans Arabic")
	}

	sys := Document{Lang: "ar", Body: "x", Fonts: FontsSystem}.HTML()
	if strings.Contains(sys, "data:font/woff2") {
		t.Fatal("system font mode must not embed fonts")
	}

	if got := (Document{PrimaryColor: "red;}body{display:none"}).HTML(); !strings.Contains(got, "--primary:#0F172A") {
		t.Fatal("invalid primary color must fall back")
	}
}

func TestIsRTL(t *testing.T) {
	for loc, want := range map[string]bool{"ar": true, "ar-SA": true, "AR_eg": true, "tr": false, "en": false, "zh_CN": false, "": false} {
		if IsRTL(loc) != want {
			t.Errorf("IsRTL(%q) = %v", loc, !want)
		}
	}
}

func TestPingHealth(t *testing.T) {
	var path string
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.Method + " " + r.URL.Path
		w.WriteHeader(status)
	}))
	defer srv.Close()
	c := NewWithOptions(srv.URL+"/", fastOpts())
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if path != "GET /health" {
		t.Fatalf("request = %q", path)
	}
	status = http.StatusServiceUnavailable
	var se *StatusError
	if err := c.Ping(context.Background()); !errors.As(err, &se) || se.Code != http.StatusServiceUnavailable {
		t.Fatalf("Ping on 503 = %v", err)
	}
	if err := New("").Ping(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured Ping = %v", err)
	}
}
