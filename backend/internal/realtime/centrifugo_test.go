package realtime

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
)

func TestCentrifugoPing(t *testing.T) {
	var gotAuth, gotBody string
	answer := `{"result":{"nodes":[]}}`
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = r.URL.Path + " " + string(b)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, answer)
	}))
	defer srv.Close()

	c := NewCentrifugoClient(config.CentrifugoConfig{Enabled: true, APIURL: srv.URL, APIKey: "k1"})
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if gotAuth != "apikey k1" || !strings.HasPrefix(gotBody, `/api {"method":"info"`) {
		t.Fatalf("request auth=%q body=%q", gotAuth, gotBody)
	}

	// A real node's info (metrics included) is larger than 4 KiB.
	answer = `{"result":{"nodes":[{"name":"n1","metrics":{"items":{"x":"` + strings.Repeat("m", 8192) + `"}}}]}}`
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping with a large info response: %v", err)
	}

	answer = `{"error":{"code":101,"message":"unauthorized"}}`
	if err := c.Ping(context.Background()); err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("Ping with API error = %v", err)
	}
	answer, status = "nope", http.StatusUnauthorized
	if err := c.Ping(context.Background()); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("Ping on 401 = %v", err)
	}
	var disabled *CentrifugoClient
	if err := disabled.Ping(context.Background()); err == nil {
		t.Fatal("disabled client Ping succeeded")
	}
}
