package usecase

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testDB struct {
	t      *testing.T
	ctx    context.Context
	pool   *pgxpool.Pool
	q      *db.Queries
	svc    *Service
	caller Caller
	prefix string
}

func newTestDB(t *testing.T) *testDB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping contracts database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
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
	prefix := "TEC286 " + uuid.NewString()
	var oldDefaultID int64
	_ = pool.QueryRow(ctx, `SELECT id FROM contract_templates WHERE brand_id = $1 AND kind = 'vehicle_intake' AND is_default LIMIT 1`, brand.ID).Scan(&oldDefaultID)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE contract_templates SET is_default = false WHERE brand_id = $1 AND kind = 'vehicle_intake' AND name LIKE $2`, brand.ID, prefix+"%")
		if oldDefaultID > 0 {
			_, _ = pool.Exec(ctx, `UPDATE contract_templates SET is_default = true, is_active = true WHERE id = $1`, oldDefaultID)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM contract_templates WHERE name LIKE $1`, prefix+"%")
	})
	return &testDB{
		t: t, ctx: ctx, pool: pool, q: q, svc: New(repository.New(pool, q)),
		caller: Caller{UserID: 0, OrganizationID: center.ID, BrandID: brand.ID}, prefix: prefix,
	}
}

func (d *testDB) create(t *testing.T, kind string, def bool) model.Template {
	t.Helper()
	item, err := d.svc.Create(d.ctx, d.caller, Input{
		Name: d.prefix + " " + kind + " " + time.Now().Format("150405.000000"),
		Kind: kind, IsDefault: &def,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return item
}

func TestSetDefaultClearsPreviousDefault(t *testing.T) {
	d := newTestDB(t)
	first := d.create(t, model.KindVehicleIntake, true)
	second := d.create(t, model.KindVehicleIntake, false)

	if _, err := d.svc.SetDefault(d.ctx, d.caller, second.UUID); err != nil {
		t.Fatalf("set default: %v", err)
	}
	gotFirst, err := d.svc.Get(d.ctx, d.caller, first.UUID)
	if err != nil {
		t.Fatal(err)
	}
	gotSecond, err := d.svc.Get(d.ctx, d.caller, second.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if gotFirst.IsDefault || !gotSecond.IsDefault {
		t.Fatalf("defaults = first %v, second %v", gotFirst.IsDefault, gotSecond.IsDefault)
	}
}

func TestPrepareHTMLValidationAndSanitize(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		wantErr bool
	}{
		{name: "known variable", html: `<p>{{ customer_name }}</p>`},
		{name: "unknown variable", html: `<p>{{ nope }}</p>`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PrepareHTML(tt.html)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "nope") {
					t.Fatalf("err = %v, want unknown variable", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("PrepareHTML: %v", err)
			}
			if got == "" {
				t.Fatal("empty sanitized html")
			}
		})
	}

	out, err := PrepareHTML(`<p>ok</p><script>alert(1)</script>`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(out), "script") {
		t.Fatalf("script survived sanitize: %s", out)
	}
}

func TestRenderFallsBackToTurkishAndEscapesValues(t *testing.T) {
	d := newTestDB(t)
	tpl := d.create(t, model.KindVehicleIntake, false)
	if _, err := d.svc.PutLocale(d.ctx, d.caller, tpl.UUID, LocaleInput{
		Locale: "tr", HTML: `<p>Merhaba {{customer_name}}</p>`,
	}); err != nil {
		t.Fatalf("put tr: %v", err)
	}
	rendered, err := d.svc.Render(d.ctx, d.caller, model.RenderInput{
		TemplateUUID: tpl.UUID, Locale: "fr", Values: map[string]string{"customer_name": `<Ada & Co>`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rendered.Locale != "tr" {
		t.Fatalf("locale = %s, want tr", rendered.Locale)
	}
	if !strings.Contains(rendered.HTML, "&lt;Ada &amp; Co&gt;") {
		t.Fatalf("value was not escaped: %s", rendered.HTML)
	}
}
