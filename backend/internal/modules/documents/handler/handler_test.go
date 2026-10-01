package handler_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/documentstest"
	dochandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	docusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Acceptance: the download endpoint streams application/pdf whose sha256
// matches the render; a user of another organization gets 404.
func TestRenderAndDownloadHTTP(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping documents HTTP test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatal(err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatal(err)
	}
	mkOrg := func(name string) db.Organization {
		row, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
			Slug: fmt.Sprintf("t88h-%s-%d", name, time.Now().UnixNano()), Name: "Bayi " + name, Status: "active",
			AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
			Type:           "dealer", ParentID: pgtype.Int8{Int64: center.ID, Valid: true},
			BrandID: brand.ID, Currency: "TRY", Locale: "tr-TR", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), "DELETE FROM document_renders WHERE organization_id = $1", row.ID)
			_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", row.ID)
		})
		return row
	}
	orgA, orgB := mkOrg("a"), mkOrg("b")

	gotb := documentstest.NewGotenberg(t, 0)
	svc := docusecase.New(pool, q, storage.NewMemory(), pdfrender.New(gotb.URL), nil, pdfrender.FontsEmbedded, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.RegisterLoader(model.KindService, documentstest.NewLoader(orgA.ID, brand.ID)); err != nil {
		t.Fatal(err)
	}
	h := dochandler.New(svc, nil)

	withScope := func(r *http.Request, org db.Organization) *http.Request {
		c := authctx.WithPrincipal(r.Context(), authctx.Principal{})
		c = orgctx.WithScope(c, orgctx.Scope{InternalID: org.ID, UUID: org.Uuid, BrandID: org.BrandID})
		return r.WithContext(c)
	}

	body, _ := json.Marshal(map[string]string{"kind": "service", "source_id": "7001", "locale": "tr"})
	rec := httptest.NewRecorder()
	h.RequestRender(rec, withScope(httptest.NewRequest(http.MethodPost, "/v1/tenant/documents/render", bytes.NewReader(body)), orgA))
	if rec.Code != http.StatusOK {
		t.Fatalf("render status %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data model.RenderView `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Data.SHA256 == nil {
		t.Fatalf("render body: %s", rec.Body.String())
	}

	download := func(org db.Organization) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/tenant/documents/"+env.Data.UUID.String()+"/download", nil)
		req.SetPathValue("uuid", env.Data.UUID.String())
		rec := httptest.NewRecorder()
		h.Download(rec, withScope(req, org))
		return rec
	}
	ok := download(orgA)
	sum := sha256.Sum256(ok.Body.Bytes())
	if ok.Code != http.StatusOK || ok.Header().Get("Content-Type") != "application/pdf" || hex.EncodeToString(sum[:]) != *env.Data.SHA256 {
		t.Fatalf("download: status=%d ct=%q", ok.Code, ok.Header().Get("Content-Type"))
	}
	if other := download(orgB); other.Code != http.StatusNotFound {
		t.Fatalf("other organization download status = %d, want 404", other.Code)
	}

	// the other organization cannot render orgA's source either
	rec = httptest.NewRecorder()
	h.RequestRender(rec, withScope(httptest.NewRequest(http.MethodPost, "/v1/tenant/documents/render", bytes.NewReader(body)), orgB))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign render status = %d, want 404", rec.Code)
	}
}
