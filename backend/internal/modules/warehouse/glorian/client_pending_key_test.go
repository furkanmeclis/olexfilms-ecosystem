package glorian_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
)

// TEC-1265: after Glorian activates a new connection package the old key
// gets 401; the client switches to the pending key once and promotes it.
func pendingKeyHub(t *testing.T, accepted string) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get(glorian.HeaderAPIKey)
		seen = append(seen, key)
		w.Header().Set("Content-Type", "application/json")
		if key != accepted {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":"UNAUTHORIZED","message":"Invalid or missing inventory API key."},"meta":{}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[],"meta":{"pagination":{"has_more":false}}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func TestPendingKeyIsUsedAfter401AndPromoted(t *testing.T) {
	srv, seen := pendingKeyHub(t, "yeni-sahte-anahtar")
	var promoted atomic.Int32
	c, err := glorian.NewHTTPClient(glorian.Options{
		BaseURL: srv.URL, APIKey: "eski-sahte-anahtar", PendingAPIKey: "yeni-sahte-anahtar",
		PendingAccepted: func(context.Context) error { promoted.Add(1); return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := c.ListCategories(ctx, glorian.ListParams{PerPage: 1}); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := c.ListCategories(ctx, glorian.ListParams{PerPage: 1}); err != nil {
		t.Fatalf("second call: %v", err)
	}
	want := []string{"eski-sahte-anahtar", "yeni-sahte-anahtar", "yeni-sahte-anahtar"}
	if len(*seen) != len(want) {
		t.Fatalf("keys sent = %v", *seen)
	}
	for i := range want {
		if (*seen)[i] != want[i] {
			t.Fatalf("keys sent = %v", *seen)
		}
	}
	if promoted.Load() != 1 {
		t.Fatalf("promoted %d times", promoted.Load())
	}
}

func TestOldKeyStaysWhenHubHasNotActivatedPackage(t *testing.T) {
	srv, seen := pendingKeyHub(t, "eski-sahte-anahtar")
	c, _ := glorian.NewHTTPClient(glorian.Options{
		BaseURL: srv.URL, APIKey: "eski-sahte-anahtar", PendingAPIKey: "yeni-sahte-anahtar",
		PendingAccepted: func(context.Context) error { t.Fatal("must not promote"); return nil },
	})
	if _, err := c.ListCategories(context.Background(), glorian.ListParams{PerPage: 1}); err != nil {
		t.Fatal(err)
	}
	if len(*seen) != 1 {
		t.Fatalf("keys sent = %v", *seen)
	}
}

func TestBothKeysRefusedKeeps401AndOldKey(t *testing.T) {
	srv, seen := pendingKeyHub(t, "baska-anahtar")
	c, _ := glorian.NewHTTPClient(glorian.Options{
		BaseURL: srv.URL, APIKey: "eski-sahte-anahtar", PendingAPIKey: "yeni-sahte-anahtar",
		PendingAccepted: func(context.Context) error { t.Fatal("must not promote"); return nil },
	})
	_, err := c.ListCategories(context.Background(), glorian.ListParams{PerPage: 1})
	var apiErr *glorian.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
		t.Fatalf("err = %v", err)
	}
	_, _ = c.ListCategories(context.Background(), glorian.ListParams{PerPage: 1})
	want := []string{"eski-sahte-anahtar", "yeni-sahte-anahtar", "eski-sahte-anahtar", "yeni-sahte-anahtar"}
	if len(*seen) != 4 || (*seen)[2] != want[2] || (*seen)[3] != want[3] {
		t.Fatalf("keys sent = %v", *seen)
	}
}
