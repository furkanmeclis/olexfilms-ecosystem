// Command publish-guide publishes a Markdown user guide (docs/guides) to the
// document center of every brand (or one brand): Markdown → HTML (document
// template skeleton) → Gotenberg PDF → a new language version of the library
// item, access all_network (TEC-510). It is idempotent: a language whose
// rendered content is unchanged does not open a new version.
//
// Usage (from backend/):
//
//	go run ./cmd/publish-guide                      # eklentiler, all brands
//	go run ./cmd/publish-guide -brand olex -dir ../docs/guides -guide eklentiler
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/library/guide"
	libraryusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/library/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

func main() {
	slug := flag.String("guide", "eklentiler", "guide slug (<dir>/<slug>.<lang>.md)")
	dir := flag.String("dir", "../docs/guides", "directory of the guide Markdown files")
	brand := flag.String("brand", "", "brand slug (default: every brand)")
	flag.Parse()

	g, ok := guide.Guides[*slug]
	if !ok {
		fatal("unknown guide %q", *slug)
	}
	sources, err := guide.LoadSources(*dir, g.Slug)
	if err != nil {
		fatal("%v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		fatal("config: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fatal("database: %v", err)
	}
	defer pool.Close()
	store, err := storage.NewFromConfig(ctx, cfg.Storage)
	if err != nil {
		fatal("storage: %v", err)
	}
	pdf := pdfrender.New(cfg.Gotenberg.URL)
	if !pdf.Configured() {
		fatal("GOTENBERG_URL is not configured")
	}
	q := db.New(pool)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	pub := guide.New(q, libraryusecase.New(q, store), pdf, features.New(pool, q, nil, log), pdfrender.ParseFontMode(cfg.Gotenberg.Fonts), log)

	brands, err := q.ListBrands(ctx)
	if err != nil {
		fatal("brands: %v", err)
	}
	var results []guide.Result
	for _, b := range brands {
		if *brand != "" && b.Slug != *brand {
			continue
		}
		res, err := pub.Publish(ctx, b.ID, g, sources)
		if err != nil {
			fatal("brand %s: %v", b.Slug, err)
		}
		results = append(results, res)
	}
	if *brand != "" && len(results) == 0 {
		fatal("unknown brand %q", *brand)
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(out))
}

func fatal(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "publish-guide: "+format+"\n", args...)
	os.Exit(1)
}
