package glorian

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-268 (F2-02c): catalog and dealer pull. Every 15 minutes worker-core
// pulls the categories, products and dealers changed on the Glorian hub
// since the last successful run (minus an overlap) into the connection's
// brand: categories and products into the catalog (brand=glorian, remote
// fields locked), dealers into integration_external_parties. Each kind is
// one integration_sync_runs row with its counts and watermark.
//
// TEC-269 (F2-02d): the same pass then pulls stock_items into the
// external_status mirror of the local units (see pull_stock.go).

// Sync run kinds of the pull (chk_integration_sync_runs_kind).
const (
	KindPullCategories = "pull_categories"
	KindPullProducts   = "pull_products"
	KindPullDealers    = "pull_dealers"
	KindPullStock      = "pull_stock"
)

// Sync run statuses.
const (
	RunRunning   = "running"
	RunSucceeded = "succeeded"
	RunFailed    = "failed"
)

// PullOverlap is subtracted from the last watermark so rows written on the
// hub while the previous run was paging are fetched again; the upserts are
// idempotent, so the overlap never duplicates a row.
const PullOverlap = 90 * time.Second

// pullPerPage is the page size of every pull request (contract maximum).
const pullPerPage = MaxPerPage

// maxPullPages stops a run that would page forever (a hub that keeps
// returning a next page).
const maxPullPages = 10000

// Column limits of the target tables.
const (
	maxCategoryName = 200
	maxProductSKU   = 64
	maxProductName  = 200
	maxPartyName    = 200
	maxRemoteID     = 64
	maxExternalID   = 128
	maxWarranty     = 600
)

// PullCounts are the per-run totals stored in integration_sync_runs.counts.
type PullCounts struct {
	Fetched   int `json:"fetched"`
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Skipped   int `json:"skipped"`
	Pages     int `json:"pages"`
	// Unmatched counts remote stock items without a local unit of the same
	// barcode (pull_stock only); they are reconcile input (TEC-269).
	Unmatched int `json:"unmatched,omitempty"`
}

// PullIndexer refreshes the product search index (searchengine.Indexer).
type PullIndexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
}

// productSearchSpec is the catalog's Meilisearch spec id (catalog
// usecase.SearchSpec; not imported to keep this package light).
const productSearchSpec = "products"

// Puller runs the catalog and dealer pull for every glorian connection.
type Puller struct {
	q        db.Querier
	store    *Store
	resolver *ClientResolver
	indexer  PullIndexer
	log      *slog.Logger
}

// NewPuller wires a puller. factory builds the client of a connection
// (HTTPClientFactory in production); indexer and log may be nil.
func NewPuller(q db.Querier, box SecretBox, factory ClientFactory, indexer PullIndexer, log *slog.Logger) *Puller {
	if log == nil {
		log = slog.Default()
	}
	store := NewStore(q, box)
	return &Puller{q: q, store: store, resolver: NewClientResolver(store, factory), indexer: indexer, log: log}
}

// Task is the worker-core entry point (glorian:pull_catalog).
func (p *Puller) Task(ctx context.Context) error {
	return p.Run(ctx)
}

// Run pulls every active glorian connection. An inactive connection is
// skipped without a request or a sync run. Errors of one connection do not
// stop the others; they are joined into the result.
func (p *Puller) Run(ctx context.Context) error {
	conns, err := p.q.ListIntegrationConnectionsByKey(ctx, ConnectionKey)
	if err != nil {
		return fmt.Errorf("glorian pull: list connections: %w", err)
	}
	var errs []error
	for _, conn := range conns {
		if !conn.Active {
			p.log.Info("glorian_pull_skipped_inactive", "connection", conn.Uuid)
			continue
		}
		if err := p.PullConnection(ctx, conn); err != nil {
			errs = append(errs, fmt.Errorf("connection %s: %w", conn.Uuid, err))
		}
	}
	return errors.Join(errs...)
}

// PullConnection runs the four pulls of one connection: categories first
// (products point at them), then products, then dealers, then the stock
// item status mirror.
func (p *Puller) PullConnection(ctx context.Context, conn db.IntegrationConnection) error {
	if !conn.Active {
		return fmt.Errorf("%w: connection %q", ErrInactiveConnection, conn.Key)
	}
	c, err := p.store.Decrypt(conn)
	if err != nil {
		return err
	}
	client, err := p.resolver.ForConnection(c)
	if err != nil {
		return err
	}
	run := &connectionPull{p: p, conn: conn, client: client}
	return errors.Join(
		p.runKind(ctx, conn, KindPullCategories, run.categories),
		p.runKind(ctx, conn, KindPullProducts, run.products),
		p.runKind(ctx, conn, KindPullDealers, run.dealers),
		p.runKind(ctx, conn, KindPullStock, run.stockItems),
	)
}

// pullFunc pulls one kind from since (zero: everything), updating counts,
// and returns the newest remote updated_at it saw (zero when none).
type pullFunc func(ctx context.Context, since time.Time, counts *PullCounts) (time.Time, error)

// runKind wraps one pull in an integration_sync_runs row.
func (p *Puller) runKind(ctx context.Context, conn db.IntegrationConnection, kind string, fn pullFunc) error {
	var prev time.Time
	last, err := p.q.LastSucceededIntegrationSyncRun(ctx, db.LastSucceededIntegrationSyncRunParams{ConnectionID: conn.ID, Kind: kind})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return fmt.Errorf("%s: last run: %w", kind, err)
	case last.Watermark.Valid:
		prev = last.Watermark.Time
	}
	var since time.Time
	if !prev.IsZero() {
		since = prev.Add(-PullOverlap)
	}
	run, err := p.q.StartIntegrationSyncRun(ctx, db.StartIntegrationSyncRunParams{
		OrganizationID: conn.OrganizationID, BrandID: conn.BrandID, ConnectionID: conn.ID, Kind: kind,
	})
	if err != nil {
		return fmt.Errorf("%s: start run: %w", kind, err)
	}
	var counts PullCounts
	newest, pullErr := fn(ctx, since, &counts)
	countsJSON, _ := json.Marshal(counts)
	finish := db.FinishIntegrationSyncRunParams{ID: run.ID, Status: RunSucceeded, Counts: countsJSON}
	if pullErr != nil {
		finish.Status = RunFailed
		finish.Error = pgtype.Text{String: truncateRunes(pullErr.Error(), 2000), Valid: true}
	} else {
		// The watermark only moves forward; a run that saw nothing keeps
		// the previous one.
		wm := prev
		if newest.After(wm) {
			wm = newest
		}
		if !wm.IsZero() {
			finish.Watermark = pgtype.Timestamptz{Time: wm, Valid: true}
		}
	}
	// Record the outcome even when the task context was cancelled mid-run.
	if _, err := p.q.FinishIntegrationSyncRun(context.WithoutCancel(ctx), finish); err != nil {
		return errors.Join(pullErr, fmt.Errorf("%s: finish run: %w", kind, err))
	}
	p.log.Info("glorian_pull_finished", "connection", conn.Uuid, "kind", kind, "status", finish.Status,
		"fetched", counts.Fetched, "created", counts.Created, "updated", counts.Updated, "skipped", counts.Skipped)
	if pullErr != nil {
		return fmt.Errorf("%s: %w", kind, pullErr)
	}
	return nil
}

// connectionPull is the state of one connection's pull.
type connectionPull struct {
	p      *Puller
	conn   db.IntegrationConnection
	client InventoryClient
	// remoteCategoryNames maps every remote category id to its name; loaded
	// once (full list) when the product pull needs it.
	remoteCategoryNames map[string]string
}

// pages walks every page of a list endpoint from since.
func pages[T any](ctx context.Context, since time.Time, counts *PullCounts,
	list func(context.Context, ListParams) (Page[T], error), each func(T) error,
) error {
	params := ListParams{UpdatedSince: since, PerPage: pullPerPage}
	for range maxPullPages {
		page, err := list(ctx, params)
		if err != nil {
			return err
		}
		counts.Pages++
		for _, item := range page.Items {
			counts.Fetched++
			if err := each(item); err != nil {
				return err
			}
		}
		if page.NextCursor == "" {
			return nil
		}
		params.Cursor = page.NextCursor
	}
	return fmt.Errorf("glorian pull: more than %d pages", maxPullPages)
}

// --- Categories -------------------------------------------------------------

func (r *connectionPull) categories(ctx context.Context, since time.Time, counts *PullCounts) (time.Time, error) {
	var newest time.Time
	err := pages(ctx, since, counts, r.client.ListCategories, func(c Category) error {
		newest = later(newest, c.UpdatedAt)
		return r.upsertCategory(ctx, c, counts)
	})
	return newest, err
}

func (r *connectionPull) upsertCategory(ctx context.Context, c Category, counts *PullCounts) error {
	name := truncateRunes(strings.TrimSpace(c.Name), maxCategoryName)
	if name == "" || strings.TrimSpace(c.ID) == "" {
		counts.Skipped++
		return nil
	}
	parts := cleanParts(c.AvailableParts)
	active := c.IsActive && c.DeletedAt == nil

	cur, err := r.p.q.GetProductCategoryByName(ctx, db.GetProductCategoryByNameParams{BrandID: r.conn.BrandID, Name: name})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// The name is new: either a new category or a rename on the hub.
		renamed, ok, err := r.renamedCategory(ctx, c.ID)
		if err != nil {
			return err
		}
		if !ok {
			partsJSON, _ := json.Marshal(parts)
			if _, err := r.p.q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
				OrganizationID: r.conn.OrganizationID, BrandID: r.conn.BrandID, Name: name,
				AvailableParts: partsJSON, Active: active,
			}); err != nil {
				return fmt.Errorf("create category %s: %w", c.ID, err)
			}
			counts.Created++
			return nil
		}
		cur = renamed
	case err != nil:
		return fmt.Errorf("category %s: %w", c.ID, err)
	}
	if cur.Name == name && cur.Active == active && slices.Equal(decodeStrings(cur.AvailableParts), parts) {
		counts.Unchanged++
		return nil
	}
	partsJSON, _ := json.Marshal(parts)
	if _, err := r.p.q.UpdateProductCategory(ctx, db.UpdateProductCategoryParams{
		ID: cur.ID, BrandID: r.conn.BrandID, Name: name, AvailableParts: partsJSON, Sort: cur.Sort, Active: active,
	}); err != nil {
		return fmt.Errorf("update category %s: %w", c.ID, err)
	}
	counts.Updated++
	return nil
}

// renamedCategory finds the local category of a remote category whose name
// changed on the hub: the category of a synced product of that remote
// category. Categories carry no remote id (000039), so a product is the
// only link; a renamed category without products is created anew.
func (r *connectionPull) renamedCategory(ctx context.Context, remoteID string) (db.ProductCategory, bool, error) {
	page, err := r.client.ListProducts(ctx, ListParams{PerPage: 1, Filters: map[string]string{"category_id": remoteID}})
	if err != nil {
		return db.ProductCategory{}, false, fmt.Errorf("products of category %s: %w", remoteID, err)
	}
	for _, rp := range page.Items {
		prod, err := r.p.q.GetProductByConnectionExternalID(ctx, db.GetProductByConnectionExternalIDParams{
			ConnectionID: pgtype.Int8{Int64: r.conn.ID, Valid: true},
			ExternalID:   pgtype.Text{String: rp.ID, Valid: true},
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return db.ProductCategory{}, false, err
		}
		cat, err := r.p.q.GetProductCategory(ctx, db.GetProductCategoryParams{ID: prod.CategoryID, BrandID: r.conn.BrandID})
		if err != nil {
			return db.ProductCategory{}, false, err
		}
		return cat, true, nil
	}
	return db.ProductCategory{}, false, nil
}

// --- Products ---------------------------------------------------------------

func (r *connectionPull) products(ctx context.Context, since time.Time, counts *PullCounts) (time.Time, error) {
	var newest time.Time
	err := pages(ctx, since, counts, r.client.ListProducts, func(rp Product) error {
		newest = later(newest, rp.UpdatedAt)
		return r.upsertProduct(ctx, rp, counts)
	})
	return newest, err
}

// categoryFor resolves the local category of a remote category id.
func (r *connectionPull) categoryFor(ctx context.Context, remoteID string) (db.ProductCategory, bool, error) {
	if r.remoteCategoryNames == nil {
		names := map[string]string{}
		var ignored PullCounts
		if err := pages(ctx, time.Time{}, &ignored, r.client.ListCategories, func(c Category) error {
			names[c.ID] = truncateRunes(strings.TrimSpace(c.Name), maxCategoryName)
			return nil
		}); err != nil {
			return db.ProductCategory{}, false, fmt.Errorf("category map: %w", err)
		}
		r.remoteCategoryNames = names
	}
	name, ok := r.remoteCategoryNames[remoteID]
	if !ok || name == "" {
		return db.ProductCategory{}, false, nil
	}
	cat, err := r.p.q.GetProductCategoryByName(ctx, db.GetProductCategoryByNameParams{BrandID: r.conn.BrandID, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ProductCategory{}, false, nil
	}
	if err != nil {
		return db.ProductCategory{}, false, err
	}
	return cat, true, nil
}

func (r *connectionPull) upsertProduct(ctx context.Context, rp Product, counts *PullCounts) error {
	remoteID := strings.TrimSpace(rp.ID)
	sku := strings.TrimSpace(rp.SKU)
	name := truncateRunes(strings.TrimSpace(rp.Name), maxProductName)
	if remoteID == "" || utf8.RuneCountInString(remoteID) > maxExternalID ||
		sku == "" || utf8.RuneCountInString(sku) > maxProductSKU || name == "" {
		r.p.log.Warn("glorian_pull_product_skipped", "connection", r.conn.Uuid, "remote_id", remoteID, "reason", "invalid")
		counts.Skipped++
		return nil
	}
	cat, ok, err := r.categoryFor(ctx, rp.CategoryID)
	if err != nil {
		return err
	}
	if !ok {
		r.p.log.Warn("glorian_pull_product_skipped", "connection", r.conn.Uuid, "remote_id", remoteID, "reason", "category")
		counts.Skipped++
		return nil
	}
	want := db.UpdateSyncedProductParams{
		CategoryID: cat.ID, Sku: sku, Name: name, DescriptionMd: deref(rp.Description),
		WarrantyDurationMonths: warrantyArg(rp.WarrantyDuration), MicronThickness: micronArg(rp.MicronThickness),
		Active:       rp.IsActive && rp.DeletedAt == nil,
		ExternalID:   pgtype.Text{String: remoteID, Valid: true},
		ConnectionID: pgtype.Int8{Int64: r.conn.ID, Valid: true},
		LockedFields: model.SyncedProductLockedFields,
		BrandID:      r.conn.BrandID,
	}

	cur, err := r.p.q.GetProductByConnectionExternalID(ctx, db.GetProductByConnectionExternalIDParams{
		ConnectionID: want.ConnectionID, ExternalID: want.ExternalID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Adopt a local product with the same SKU that no sync owns yet
		// (e.g. entered before the connection existed).
		cur, err = r.p.q.GetProductBySKU(ctx, db.GetProductBySKUParams{BrandID: r.conn.BrandID, Sku: sku})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return r.createProduct(ctx, want, counts)
		case err != nil:
			return err
		case cur.ConnectionID.Valid:
			r.p.log.Warn("glorian_pull_product_skipped", "connection", r.conn.Uuid, "remote_id", remoteID, "reason", "sku_taken")
			counts.Skipped++
			return nil
		}
	} else if err != nil {
		return err
	}
	if sameProduct(cur, want) {
		counts.Unchanged++
		return nil
	}
	want.ID = cur.ID
	row, err := r.p.q.UpdateSyncedProduct(ctx, want)
	if err != nil {
		if isUniqueViolation(err) {
			r.p.log.Warn("glorian_pull_product_skipped", "connection", r.conn.Uuid, "remote_id", remoteID, "reason", "sku_taken")
			counts.Skipped++
			return nil
		}
		return fmt.Errorf("update product %s: %w", remoteID, err)
	}
	counts.Updated++
	r.reindex(ctx, row.Uuid)
	return nil
}

func (r *connectionPull) createProduct(ctx context.Context, want db.UpdateSyncedProductParams, counts *PullCounts) error {
	row, err := r.p.q.CreateProduct(ctx, db.CreateProductParams{
		OrganizationID: r.conn.OrganizationID, BrandID: r.conn.BrandID, CategoryID: want.CategoryID,
		Sku: want.Sku, Name: want.Name, DescriptionMd: want.DescriptionMd,
		WarrantyDurationMonths: want.WarrantyDurationMonths, MicronThickness: want.MicronThickness,
		Images: []byte("[]"), UnitType: model.UnitPiece, Active: want.Active,
		ExternalID: want.ExternalID, ConnectionID: want.ConnectionID, LockedFields: want.LockedFields,
	})
	if err != nil {
		if isUniqueViolation(err) {
			counts.Skipped++
			return nil
		}
		return fmt.Errorf("create product %s: %w", want.ExternalID.String, err)
	}
	counts.Created++
	r.reindex(ctx, row.Uuid)
	return nil
}

func (r *connectionPull) reindex(ctx context.Context, id uuid.UUID) {
	if r.p.indexer != nil {
		r.p.indexer.EnqueueUpsert(ctx, productSearchSpec, id.String())
	}
}

// sameProduct reports whether cur already holds every synced value.
func sameProduct(cur db.Product, want db.UpdateSyncedProductParams) bool {
	return cur.CategoryID == want.CategoryID && cur.Sku == want.Sku && cur.Name == want.Name &&
		cur.DescriptionMd == want.DescriptionMd && cur.Active == want.Active &&
		cur.WarrantyDurationMonths == want.WarrantyDurationMonths &&
		numericEqual(cur.MicronThickness, want.MicronThickness) &&
		cur.ExternalID == want.ExternalID && cur.ConnectionID == want.ConnectionID &&
		slices.Equal(cur.LockedFields, want.LockedFields)
}

func warrantyArg(v *int) pgtype.Int4 {
	if v == nil || *v < 0 || *v > maxWarranty {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*v), Valid: true}
}

func micronArg(v *int) pgtype.Numeric {
	if v == nil || *v <= 0 || *v > 999999 {
		return pgtype.Numeric{}
	}
	return pgtype.Numeric{Int: big.NewInt(int64(*v)), Exp: 0, Valid: true}
}

func numericEqual(a, b pgtype.Numeric) bool {
	if !a.Valid || !b.Valid {
		return a.Valid == b.Valid
	}
	fa, errA := a.Float64Value()
	fb, errB := b.Float64Value()
	return errA == nil && errB == nil && fa.Float64 == fb.Float64
}

// --- Dealers ----------------------------------------------------------------

func (r *connectionPull) dealers(ctx context.Context, since time.Time, counts *PullCounts) (time.Time, error) {
	var newest time.Time
	err := pages(ctx, since, counts, r.client.ListDealers, func(d Dealer) error {
		newest = later(newest, d.UpdatedAt)
		return r.upsertDealer(ctx, d, counts)
	})
	return newest, err
}

func (r *connectionPull) upsertDealer(ctx context.Context, d Dealer, counts *PullCounts) error {
	remoteID := strings.TrimSpace(d.ID)
	name := truncateRunes(strings.TrimSpace(d.Name), maxPartyName)
	if remoteID == "" || utf8.RuneCountInString(remoteID) > maxRemoteID || name == "" {
		counts.Skipped++
		return nil
	}
	phoneArg := dealerPhone(d)
	cur, err := r.p.q.GetIntegrationExternalPartyByRemoteID(ctx, db.GetIntegrationExternalPartyByRemoteIDParams{
		ConnectionID: r.conn.ID, RemoteID: remoteID,
	})
	exists := err == nil
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return fmt.Errorf("dealer %s: %w", remoteID, err)
	}
	if exists && cur.Name == name && cur.PhoneE164 == phoneArg && cur.Active == d.IsActive {
		counts.Unchanged++
		return nil
	}
	if _, err := r.p.q.UpsertIntegrationExternalParty(ctx, db.UpsertIntegrationExternalPartyParams{
		OrganizationID: r.conn.OrganizationID, BrandID: r.conn.BrandID, ConnectionID: r.conn.ID,
		RemoteID: remoteID, Name: name, PhoneE164: phoneArg, Active: d.IsActive,
	}); err != nil {
		return fmt.Errorf("upsert dealer %s: %w", remoteID, err)
	}
	if exists {
		counts.Updated++
	} else {
		counts.Created++
	}
	return nil
}

// dealerPhone is the dealer phone in E.164 (dealer country as the default
// region), or NULL when it does not parse.
func dealerPhone(d Dealer) pgtype.Text {
	raw := strings.TrimSpace(deref(d.Phone))
	if raw == "" {
		return pgtype.Text{}
	}
	e164, err := phone.NormalizeE164(raw, phone.Region(strings.TrimSpace(deref(d.Country))))
	if err != nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: e164, Valid: true}
}

// --- Helpers ----------------------------------------------------------------

// later returns the newer of cur and the RFC 3339 timestamp ts.
func later(cur time.Time, ts string) time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(ts))
	if err != nil || !t.After(cur) {
		return cur
	}
	return t
}

func cleanParts(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || slices.Contains(out, p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func decodeStrings(raw []byte) []string {
	out := []string{}
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = []string{}
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
