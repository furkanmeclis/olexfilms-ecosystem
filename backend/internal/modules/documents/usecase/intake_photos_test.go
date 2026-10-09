package usecase

import (
	"context"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-499: the startup sync adds {{intake_photos_html}} to the system seed of
// the default contract template and never touches an admin-published one.
func TestEnsureContractIntakePhotos(t *testing.T) {
	e := newEnv(t, nil, 0)
	ctx := context.Background()
	// Languages without a contract seed (000030 seeds tr and en only), so
	// the test owns their rows.
	const seedLang, adminLang = "de", "it"
	cleanup := func() {
		_, _ = e.pool.Exec(context.Background(), `DELETE FROM document_templates WHERE kind = 'contract' AND brand_id IS NULL AND language = ANY($1)`,
			[]string{seedLang, adminLang})
	}
	cleanup()
	t.Cleanup(cleanup)

	const seedHTML = `<h1>{{contract_title}}</h1>{{contract_body_html}}<p class="doc-footer">{{footer_text}}</p>`
	insert := func(lang string, createdBy pgtype.Int8) db.DocumentTemplate {
		t.Helper()
		spec, _ := model.Spec(model.KindContract)
		html, vars, hash, err := prepareHTML(spec, seedHTML)
		if err != nil {
			t.Fatal(err)
		}
		row, err := e.q.CreateDocumentTemplate(ctx, db.CreateDocumentTemplateParams{
			Kind: model.KindContract, Language: lang, Name: "TEC499 " + uuid.NewString()[:8], Version: 1,
			Html: html, Variables: vars, ContentHash: hash, CreatedBy: createdBy,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.q.PublishDocumentTemplate(ctx, row.ID); err != nil {
			t.Fatal(err)
		}
		return row
	}
	seed := insert(seedLang, pgtype.Int8{})
	admin, err := e.q.CreateUser(ctx, db.CreateUserParams{
		Email: pgtype.Text{String: "tec499-" + uuid.NewString() + "@example.test", Valid: true}, PasswordHash: "x",
		Name: "TEC499", Surname: "Admin", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, admin.ID) })
	edited := insert(adminLang, pgtype.Int8{Int64: admin.ID, Valid: true})

	if _, err := e.svc.EnsureContractIntakePhotos(ctx); err != nil {
		t.Fatal(err)
	}
	active := func(lang string) db.DocumentTemplate {
		t.Helper()
		row, err := e.q.GetActiveDocumentTemplate(ctx, db.GetActiveDocumentTemplateParams{Kind: model.KindContract, Language: lang})
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	got := active(seedLang)
	if got.ID == seed.ID || got.Version != 2 || got.CreatedBy.Valid {
		t.Fatalf("seed not republished: id=%d version=%d", got.ID, got.Version)
	}
	if !strings.Contains(got.Html, IntakePhotosPlaceholder+"\n"+`<p class="doc-footer">`) {
		t.Fatalf("placeholder not before the footer:\n%s", got.Html)
	}
	if !strings.Contains(string(got.Variables), "intake_photos_html") {
		t.Fatalf("variables = %s", got.Variables)
	}
	if row := active(adminLang); row.ID != edited.ID || strings.Contains(row.Html, "intake_photos_html") {
		t.Fatalf("admin-published template was changed: %+v", row)
	}

	// Idempotent: a second run adds no version.
	if _, err := e.svc.EnsureContractIntakePhotos(ctx); err != nil {
		t.Fatal(err)
	}
	if row := active(seedLang); row.ID != got.ID {
		t.Fatalf("second run republished: %d != %d", row.ID, got.ID)
	}

	// The real system seed (tr / en) carries the placeholder too.
	for _, lang := range []string{"tr", "en"} {
		row, err := e.q.GetActiveDocumentTemplate(ctx, db.GetActiveDocumentTemplateParams{Kind: model.KindContract, Language: lang})
		if err != nil || row.CreatedBy.Valid {
			continue
		}
		if !strings.Contains(row.Html, "intake_photos_html") {
			t.Fatalf("%s default contract template lacks the placeholder", lang)
		}
	}
}
