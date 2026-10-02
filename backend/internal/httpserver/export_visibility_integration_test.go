package httpserver

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	catalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/usecase"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	pricingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

type ioJobRow struct {
	UUID           string  `json:"uuid"`
	Resource       string  `json:"resource"`
	Status         string  `json:"status"`
	Filename       string  `json:"filename"`
	SourceFilename string  `json:"source_filename"`
	DownloadURL    *string `json:"download_url"`
	Actor          *struct {
		UUID string `json:"uuid"`
		Name string `json:"name"`
	} `json:"actor"`
}

type ioJobPage struct {
	Items []ioJobRow `json:"items"`
	Total int64      `json:"total"`
}

// TEC-211 acceptance: the catalog export of a dealer owner (pricing.purchase
// .read) carries the purchase price column, the same export requested by
// dealer staff (no pricing grant) has no price column at all; the grants are
// stored on the job and applied by the worker. The organization export and
// import lists return only the active organization's jobs with who, file
// and status.
func TestIntegrationExportColumnVisibilityAndOrgLists(t *testing.T) {
	store := storage.NewMemory()
	mr := miniredis.RunT(t)
	qc := queue.NewClient(config.RedisConfig{Addr: mr.Addr()})
	t.Cleanup(func() { _ = qc.Close() })
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = store; d.Queue = qc })
	ctx := context.Background()

	center := it.brandCenter("olex")
	dist := it.org("t211-dist", "distributor", center)
	dealer := it.org("t211-dealer", "dealer", dist)
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM export_jobs WHERE organization_id = ANY($1)`, []int64{dealer.ID, dist.ID})
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM import_jobs WHERE organization_id = ANY($1)`, []int64{dealer.ID, dist.ID})
	})

	owner, ownerPW := it.user("t211-owner")
	it.member(dealer, owner, "owner")
	staff, staffPW := it.user("t211-staff")
	it.member(dealer, staff, "staff")
	distOwner, distPW := it.user("t211-dist-owner")
	it.member(dist, distOwner, "owner")

	p := it.product(center, "T211")
	if _, err := it.q.UpsertDistributorDealerPrice(ctx, db.UpsertDistributorDealerPriceParams{
		ProductID: p.ID, BrandID: center.BrandID, DistributorOrgID: dist.ID, Currency: "TRY", Price: "70.5",
	}); err != nil {
		t.Fatalf("dealer price: %v", err)
	}

	ownerTok := it.loginOrg(owner, ownerPW, dealer)
	staffTok := it.loginOrg(staff, staffPW, dealer)
	distTok := it.loginOrg(distOwner, distPW, dist)

	// 1. Both dealer users queue the product export.
	req := map[string]any{"format": "csv", "locale": "en"}
	ownerJob := decodeData[ioJobRow](t, it.custDo("POST", "/v1/catalog/products/export", ownerTok, req, http.StatusAccepted))
	staffJob := decodeData[ioJobRow](t, it.custDo("POST", "/v1/catalog/products/export", staffTok, req, http.StatusAccepted))
	if ownerJob.Resource != catalogusecase.IOResource || ownerJob.Filename == "" {
		t.Fatalf("owner job = %+v", ownerJob)
	}

	// The grants are stored on the job: the owner's holds the purchase
	// permission, the staff's holds none.
	storedGrants := func(jobUUID string) string {
		var raw pgtype.Text
		if err := it.pool.QueryRow(ctx, `SELECT query_json->>$2 FROM export_jobs WHERE uuid = $1`,
			jobUUID, ioengine.QueryGrantedPermissions).Scan(&raw); err != nil {
			t.Fatalf("stored grants: %v", err)
		}
		return raw.String
	}
	if g := storedGrants(ownerJob.UUID); !strings.Contains(g, rbac.PermPricingPurchaseRead) {
		t.Fatalf("owner job grants = %q, want %s", g, rbac.PermPricingPurchaseRead)
	}
	if g := storedGrants(staffJob.UUID); g != "" {
		t.Fatalf("staff job grants = %q, want none", g)
	}

	// 2. The worker builds both files.
	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = insp.Close() })
	tasks, err := insp.ListPendingTasks(queue.QueueExports)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("pending export tasks = %d (%v)", len(tasks), err)
	}
	adapter := catalogusecase.NewIOAdapter(catalogusecase.New(it.q, nil), it.q).WithPrices(pricingusecase.New(it.q))
	worker := exportusecase.New(it.q, store, ioengine.NewRegistry(adapter), nil, nil, nil, nil)
	for _, task := range tasks {
		pl, err := queue.ParseExportProcessPayload(task.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := worker.ProcessExport(ctx, pl.ExportJobID); err != nil {
			t.Fatalf("process export: %v", err)
		}
	}
	fileOf := func(jobUUID string) string {
		var key string
		if err := it.pool.QueryRow(ctx, `SELECT file_key FROM export_jobs WHERE uuid = $1 AND status = 'completed'`, jobUUID).Scan(&key); err != nil {
			t.Fatalf("file key of %s: %v", jobUUID, err)
		}
		rc, _, err := store.Download(ctx, key)
		if err != nil {
			t.Fatalf("download %s: %v", key, err)
		}
		defer func() { _ = rc.Close() }()
		b, _ := io.ReadAll(rc)
		return string(b)
	}
	ownerCSV := fileOf(ownerJob.UUID)
	if !strings.Contains(ownerCSV, "Purchase price") || !strings.Contains(ownerCSV, "TRY 70.5") {
		t.Fatalf("owner export must hold the purchase price column:\n%s", ownerCSV)
	}
	staffCSV := fileOf(staffJob.UUID)
	if strings.Contains(staffCSV, "Purchase price") || strings.Contains(staffCSV, "Sale price") ||
		strings.Contains(staffCSV, "Recommended sale price") || strings.Contains(staffCSV, "70.5") {
		t.Fatalf("staff export must hold no price column:\n%s", staffCSV)
	}
	if !strings.Contains(staffCSV, p.Sku) {
		t.Fatalf("staff export misses the product:\n%s", staffCSV)
	}

	// The HTTP download of the owner is the same file.
	rec := it.raw("GET", "/v1/tenant/exports/"+ownerJob.UUID+"/download", ownerTok, "", nil, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != ownerCSV {
		t.Fatalf("owner download = %d", rec.Code)
	}

	// 3. Organization export list: both dealer jobs with who, file and
	// status; the distributor list has none of them.
	page := decodeData[ioJobPage](t, it.custDo("GET", "/v1/tenant/exports", ownerTok, nil, http.StatusOK))
	found := map[string]ioJobRow{}
	for _, row := range page.Items {
		found[row.UUID] = row
	}
	for _, want := range []struct {
		job  ioJobRow
		user db.User
	}{{ownerJob, owner}, {staffJob, staff}} {
		row, ok := found[want.job.UUID]
		if !ok {
			t.Fatalf("dealer export list misses %s: %+v", want.job.UUID, page.Items)
		}
		if row.Status != "completed" || row.Filename == "" || row.Actor == nil ||
			row.Actor.UUID != want.user.Uuid.String() || !strings.Contains(row.Actor.Name, want.user.Name) {
			t.Fatalf("dealer export row = %+v", row)
		}
	}
	distPage := decodeData[ioJobPage](t, it.custDo("GET", "/v1/tenant/exports", distTok, nil, http.StatusOK))
	for _, row := range distPage.Items {
		if row.UUID == ownerJob.UUID || row.UUID == staffJob.UUID {
			t.Fatalf("distributor export list leaks a dealer job: %+v", row)
		}
	}
	it.custDo("GET", "/v1/tenant/exports/"+ownerJob.UUID, distTok, nil, http.StatusNotFound)

	// 4. Organization import list: who and the uploaded file name.
	imp, err := it.q.CreateImportJob(ctx, db.CreateImportJobParams{
		Resource: catalogusecase.IOResource, ActorID: owner.ID, Format: "csv", Locale: "en",
		OrganizationID: pgtype.Int8{Int64: dealer.ID, Valid: true}, SourceFilename: "urunler-ekim.csv",
	})
	if err != nil {
		t.Fatalf("import job: %v", err)
	}
	ipage := decodeData[ioJobPage](t, it.custDo("GET", "/v1/tenant/imports", ownerTok, nil, http.StatusOK))
	var irow *ioJobRow
	for i := range ipage.Items {
		if ipage.Items[i].UUID == imp.Uuid.String() {
			irow = &ipage.Items[i]
		}
	}
	if irow == nil || irow.Status != "uploaded" || irow.SourceFilename != "urunler-ekim.csv" ||
		irow.Actor == nil || irow.Actor.UUID != owner.Uuid.String() {
		t.Fatalf("dealer import list row = %+v (items %+v)", irow, ipage.Items)
	}
	dipage := decodeData[ioJobPage](t, it.custDo("GET", "/v1/tenant/imports", distTok, nil, http.StatusOK))
	for _, row := range dipage.Items {
		if row.UUID == imp.Uuid.String() {
			t.Fatalf("distributor import list leaks a dealer job: %+v", row)
		}
	}
}
