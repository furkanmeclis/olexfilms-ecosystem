package usecase

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/google/uuid"
)

// TEC-1214: the organization currency is its ledger currency (K7). It may
// change while the organization has no finance account, cari or finance
// entry; afterwards Patch answers ErrCurrencyLocked (409 CURRENCY_LOCKED)
// and the stored currency stays (database test, rolled back).
func TestPatchCurrencyLockedByLedger(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	svc := New(pool, q)

	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatal(err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx = brandctx.WithBrand(ctx, brandctx.Brand{ID: brand.ID, Slug: brand.Slug, Status: "active"})
	var orgID int64
	var orgUUID uuid.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO organizations (slug, name, status, access_starts_at, type, parent_id, brand_id, currency, locale, timezone, settings)
		VALUES ($1, 'Currency lock test', 'active', NOW() - interval '1 hour', 'dealer', $2, $3, 'TRY', 'tr', 'Europe/Istanbul', '{}')
		RETURNING id, uuid`, fmt.Sprintf("t1214-lock-%d", time.Now().UnixNano()), center.ID, brand.ID).Scan(&orgID, &orgUUID); err != nil {
		t.Fatal(err)
	}

	// No ledger yet: the currency changes.
	usd := "USD"
	got, err := svc.Patch(ctx, orgUUID, PatchInput{Currency: &usd})
	if err != nil {
		t.Fatalf("Patch without ledger: %v", err)
	}
	if got.Currency != "USD" {
		t.Fatalf("currency = %q, want USD", got.Currency)
	}

	// A cari where the organization is the counterparty locks it too.
	if _, err := tx.Exec(ctx, `INSERT INTO cari_accounts (organization_id, brand_id, counterparty_type, counterparty_org_id, currency)
		SELECT o.id, o.brand_id, 'organization', $1, o.currency FROM organizations o WHERE o.id = $2`, orgID, center.ID); err != nil {
		t.Fatal(err)
	}
	try := "TRY"
	if _, err := svc.Patch(ctx, orgUUID, PatchInput{Currency: &try}); !errors.Is(err, ErrCurrencyLocked) {
		t.Fatalf("Patch with counterparty cari err = %v, want ErrCurrencyLocked", err)
	}
	// Same currency (any case) and other fields still pass.
	same := "usd"
	name := "Currency lock test 2"
	got, err = svc.Patch(ctx, orgUUID, PatchInput{Currency: &same, Name: &name})
	if err != nil {
		t.Fatalf("Patch same currency: %v", err)
	}
	if got.Currency != "USD" || got.Name != name {
		t.Fatalf("got %q/%q", got.Currency, got.Name)
	}

	// An own finance account locks as well.
	if _, err := tx.Exec(ctx, `DELETE FROM cari_accounts WHERE counterparty_org_id = $1`, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO finance_accounts (organization_id, brand_id, type, name, currency)
		VALUES ($1, $2, 'cash', 'Kasa', 'USD')`, orgID, brand.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Patch(ctx, orgUUID, PatchInput{Currency: &try}); !errors.Is(err, ErrCurrencyLocked) {
		t.Fatalf("Patch with finance account err = %v, want ErrCurrencyLocked", err)
	}
}
