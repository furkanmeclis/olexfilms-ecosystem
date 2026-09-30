package httpserver

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestServeDocsIndex_ContainsScalarBootstrap(t *testing.T) {
	t.Parallel()
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/docs/", nil)
	s.serveDocsIndex(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Scalar.createApiReference",
		"/docs/openapi.yaml",
		"cdn.jsdelivr.net/npm/@scalar/api-reference@",
		"App API Docs",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("docs HTML missing %q", want)
		}
	}
}

func TestDocsOpenAPI_ServesSpec(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	s := &Server{log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))}
	s.mountDocs(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/docs/openapi.yaml", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"openapi: 3.1",
		"/v1/auth/login",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("openapi.yaml missing %q", want)
		}
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/yaml") {
		t.Fatalf("Content-Type = %q", ct)
	}
}
