package db_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
)

// TEC-312: database-level guards of the lead and quote schema (migration
// 000087). Reuses the service catalog fixture (center > distributor >
// dealer; rolled-back transaction, savepoint per failure).

func (f *serviceCatalogFixture) lead(t *testing.T) int64 {
	t.Helper()
	var id int64
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO leads
		(organization_id, brand_id, target_type, source, temperature, status, notes)
		VALUES ($1, $2, 'customer', 'walk_in', 'warm', 'new', '') RETURNING id`,
		f.dealer.ID, f.dealer.BrandID).Scan(&id); err != nil {
		t.Fatalf("lead: %v", err)
	}
	return id
}

func (f *serviceCatalogFixture) quote(t *testing.T, leadID int64, no int64) int64 {
	t.Helper()
	var id int64
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO quotes
		(organization_id, brand_id, lead_id, quote_no, currency, subtotal, discount_total, tax_total, grand_total)
		VALUES ($1, $2, $3, $4, 'TRY', 100, 0, 0, 100) RETURNING id`,
		f.dealer.ID, f.dealer.BrandID, leadID, no).Scan(&id); err != nil {
		t.Fatalf("quote: %v", err)
	}
	return id
}

func (f *serviceCatalogFixture) product(t *testing.T) int64 {
	t.Helper()
	suffix := time.Now().UnixNano()
	var catID, productID int64
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO product_categories
		(organization_id, brand_id, name) VALUES ($1, $2, $3) RETURNING id`,
		f.olex.ID, f.olex.BrandID, fmt.Sprintf("T312 Cat %d", suffix)).Scan(&catID); err != nil {
		t.Fatalf("product category: %v", err)
	}
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO products
		(organization_id, brand_id, category_id, sku, name)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		f.olex.ID, f.olex.BrandID, catID, fmt.Sprintf("T312-%d", suffix), "T312 Film").Scan(&productID); err != nil {
		t.Fatalf("product: %v", err)
	}
	return productID
}

func TestLeadsQuotesSchemaConstraints(t *testing.T) {
	f := newServiceCatalogFixture(t)
	ctx := f.ctx
	leadID := f.lead(t)
	quoteID := f.quote(t, leadID, 1)

	t.Run("lead events are append-only", func(t *testing.T) {
		var eventID int64
		if err := f.tx.QueryRow(ctx, `SELECT id FROM lead_events WHERE lead_id = $1 ORDER BY id LIMIT 1`, leadID).Scan(&eventID); err != nil {
			t.Fatalf("lead event: %v", err)
		}
		f.expectCode(t, "update event", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE lead_events SET payload = '{}'::jsonb WHERE id = $1`, eventID)
			return err
		}, "23001")
		f.expectCode(t, "delete event", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `DELETE FROM lead_events WHERE id = $1`, eventID)
			return err
		}, "23001")
	})

	t.Run("quote line has exactly one reference", func(t *testing.T) {
		productID := f.product(t)
		serviceItem := f.item(t, "setup")
		f.expectCode(t, "two line references", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `INSERT INTO quote_lines
				(quote_id, organization_id, brand_id, line_type, product_id, service_catalog_item_id,
				 description_snapshot, quantity, unit_price, discount_amount, line_total)
				VALUES ($1, $2, $3, 'product', $4, $5, 'T312 line', 1, 100, 0, 100)`,
				quoteID, f.dealer.ID, f.dealer.BrandID, productID, serviceItem.ID)
			return err
		}, "23514")
	})

	t.Run("candidate phone is E164", func(t *testing.T) {
		f.expectCode(t, "bad phone", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `INSERT INTO leads
				(organization_id, brand_id, target_type, candidate_phone_e164, source, temperature, status, notes)
				VALUES ($1, $2, 'dealer_candidate', '05551234567', 'application_form', 'hot', 'new', '')`,
				f.dealer.ID, f.dealer.BrandID)
			return err
		}, "23514")
	})

	t.Run("quote number is unique per organization", func(t *testing.T) {
		f.expectCode(t, "duplicate quote no", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `INSERT INTO quotes
				(organization_id, brand_id, lead_id, quote_no, currency, subtotal, discount_total, tax_total, grand_total)
				VALUES ($1, $2, $3, 1, 'TRY', 100, 0, 0, 100)`,
				f.dealer.ID, f.dealer.BrandID, leadID)
			return err
		}, "23505")
		otherDealer := f.org(t, "dealer-qno", "dealer", f.dist.ID)
		otherLead, err := db.New(f.tx).CreateLead(ctx, db.CreateLeadParams{
			OrganizationID: otherDealer.ID, BrandID: otherDealer.BrandID,
			TargetType: "customer", Source: "walk_in", Temperature: "warm", Status: "new", Notes: "",
		})
		if err != nil {
			t.Fatalf("other lead: %v", err)
		}
		if _, err := f.tx.Exec(ctx, `INSERT INTO quotes
			(organization_id, brand_id, lead_id, quote_no, currency, subtotal, discount_total, tax_total, grand_total)
			VALUES ($1, $2, $3, 1, 'TRY', 100, 0, 0, 100)`,
			otherDealer.ID, otherDealer.BrandID, otherLead.ID); err != nil {
			t.Fatalf("same quote no in another org: %v", err)
		}
	})
}
