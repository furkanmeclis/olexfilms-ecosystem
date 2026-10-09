package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	searchadapters "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine/adapters"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const internalPkg = "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal"

// TestAdaptersCoverEverySearchAdapter scans the backend for every type
// with a `Spec() searchengine.Spec` method (a search index adapter) and
// requires it in Adapters: an adapter the server indexes but the worker
// registry lacks fails here (TEC-524, search_reindex_unknown_spec).
func TestAdaptersCoverEverySearchAdapter(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "Spec" || fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
				continue
			}
			sel, ok := fn.Type.Results.List[0].Type.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Spec" || fmt.Sprint(sel.X) != "searchengine" {
				continue
			}
			recv := fn.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}
			rel, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			declared[internalPkg+"/"+filepath.ToSlash(rel)+"."+fmt.Sprint(recv)] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(declared) == 0 {
		t.Fatal("scan found no search adapters")
	}

	registered := map[string]bool{}
	for _, a := range Adapters(nil) {
		typ := reflect.TypeOf(a)
		if typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		registered[typ.PkgPath()+"."+typ.Name()] = true
	}
	for name := range declared {
		if !registered[name] {
			t.Errorf("search adapter %s is not in registry.Adapters; the worker would log search_reindex_unknown_spec", name)
		}
	}
	for name := range registered {
		if !declared[name] {
			t.Errorf("registry.Adapters has %s, which the scan did not find", name)
		}
	}
}

// TestSpecIDs requires unique spec ids (NewRegistry keeps the last one of
// a duplicate) and every known spec, leads included.
func TestSpecIDs(t *testing.T) {
	adapters := Adapters(nil)
	reg := New(nil)
	if got := len(reg.SpecIDs()); got != len(adapters) {
		t.Fatalf("registry has %d specs for %d adapters; duplicate spec id", got, len(adapters))
	}
	for _, id := range []string{
		searchadapters.SpecUsers, searchadapters.SpecRoles,
		searchengine.SpecServices, searchengine.SpecWarranties, searchengine.SpecVehicles,
		searchengine.SpecOrganizations, searchengine.SpecOrders, searchengine.SpecStockUnits,
		searchengine.SpecLeads,
	} {
		if !slices.Contains(reg.SpecIDs(), id) {
			t.Errorf("spec %q missing from the registry", id)
		}
	}
}

// fakeMeili accepts every Meilisearch write as a succeeded task and keeps
// the documents posted per index.
type fakeMeili struct {
	mu    sync.Mutex
	task  atomic.Int64
	docs  map[string][]searchengine.Document
	index []string
}

func (f *fakeMeili) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/tasks/") {
		_, _ = fmt.Fprintf(w, `{"uid":%s,"status":"succeeded"}`, strings.TrimPrefix(r.URL.Path, "/tasks/"))
		return
	}
	body, _ := io.ReadAll(r.Body)
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	f.mu.Lock()
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/indexes":
		var cfg struct {
			UID string `json:"uid"`
		}
		_ = json.Unmarshal(body, &cfg)
		f.index = append(f.index, cfg.UID)
	case r.Method == http.MethodPost && len(parts) == 3 && parts[2] == "documents":
		var docs []searchengine.Document
		_ = json.Unmarshal(body, &docs)
		f.docs[parts[1]] = append(f.docs[parts[1]], docs...)
	}
	f.mu.Unlock()
	w.WriteHeader(http.StatusAccepted)
	_, _ = fmt.Fprintf(w, `{"taskUid":%d,"status":"enqueued"}`, f.task.Add(1))
}

// TestLeadsReindexTask runs the worker's reindex handler for the leads spec
// against the shared registry: it succeeds and indexes the lead.
func TestLeadsReindexTask(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	q := db.New(tx)

	var orgID, brandID int64
	if err := tx.QueryRow(ctx, `SELECT o.id, o.brand_id FROM organizations o JOIN brands b ON b.id = o.brand_id
		WHERE b.slug = 'olex' AND o.deleted_at IS NULL ORDER BY o.id LIMIT 1`).Scan(&orgID, &brandID); err != nil {
		t.Fatalf("org: %v", err)
	}
	lead, err := q.CreateLead(ctx, db.CreateLeadParams{
		OrganizationID: orgID, BrandID: brandID, TargetType: "customer", Source: "walk_in",
		Temperature: "warm", Status: "new", Notes: "",
		CandidateCompanyName: pgtype.Text{String: "TEC-524 Reindex Ltd", Valid: true},
	})
	if err != nil {
		t.Fatalf("lead: %v", err)
	}

	meili := &fakeMeili{docs: map[string][]searchengine.Document{}}
	srv := httptest.NewServer(meili)
	t.Cleanup(srv.Close)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := searchengine.NewClient(config.SearchConfig{
		Enabled: true, Driver: "meilisearch", MeiliHost: srv.URL, MeiliKey: "test", IndexPrefix: "t524",
	}, log)
	indexer := searchengine.NewIndexer(client, New(q), nil, log)

	if err := indexer.ProcessReindex(ctx, searchengine.SpecLeads); err != nil {
		t.Fatalf("leads reindex: %v", err)
	}
	if !slices.Contains(meili.index, "t524_leads") {
		t.Fatalf("indexes created = %v, want t524_leads", meili.index)
	}
	found := false
	for _, d := range meili.docs["t524_leads"] {
		found = found || d.ID == lead.Uuid.String()
	}
	if !found {
		t.Fatalf("lead %s not indexed; got %d leads docs", lead.Uuid, len(meili.docs["t524_leads"]))
	}

	// An unknown spec is an error the task does not retry.
	err = indexer.ProcessReindex(ctx, "no_such_spec")
	if err == nil || !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("unknown spec err = %v, want SkipRetry", err)
	}
}
