package glorian

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// TEC-1265, kullanıcı kararı 5 (TEC-1253): the ecosystem owns prices. The
// USD list price of every product synced from a connection (the center's
// sale_to_distributor_price and meter_price, rounded to 2 decimals) is sent
// to the hub with PUT /products/prices. Each run sends the full set in
// pages of MaxBulkItems: the hub stores absolute values and answers which
// products changed, so a repeated run is harmless and a price changed on
// the hub's side is put back. A product without a USD list price is not
// sent; the hub keeps its price.

// KindPushPrices is the sync run kind of the price push.
const KindPushPrices = "push_prices"

// PricePushCounts are the per-run totals of a push_prices run.
type PricePushCounts struct {
	Products  int `json:"products"`
	Calls     int `json:"calls"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	NotFound  int `json:"not_found"`
	Held      int `json:"held"`
}

// PricePusher runs the price push.
type PricePusher struct {
	outbound
}

// NewPricePusher wires a price pusher; factory builds the client of a
// connection (HTTPClientFactory in production), log may be nil.
func NewPricePusher(q db.Querier, box SecretBox, factory ClientFactory, log *slog.Logger) *PricePusher {
	return &PricePusher{outbound: newOutbound(q, box, factory, log)}
}

// PushPricesTask is the glorian:push_prices handler. connectionID 0 (the
// cron run) pushes every active glorian connection; an inactive one is
// skipped there without a run. A failure is returned for an Asynq retry.
func (p *PricePusher) PushPricesTask(ctx context.Context, connectionID int64) error {
	if connectionID != 0 {
		conn, err := p.connection(ctx, connectionID)
		if errors.Is(err, ErrConnectionNotFound) {
			p.log.Warn("glorian_price_push_connection_missing", "connection_id", connectionID)
			return nil
		}
		if err != nil {
			return err
		}
		return TaskError(p.PushPrices(ctx, conn))
	}
	conns, err := p.q.ListIntegrationConnectionsByKey(ctx, ConnectionKey)
	if err != nil {
		return fmt.Errorf("glorian price push: list connections: %w", err)
	}
	var errs []error
	for _, conn := range conns {
		if !conn.Active {
			continue
		}
		if err := p.PushPrices(ctx, conn); err != nil {
			errs = append(errs, fmt.Errorf("connection %s: %w", conn.Uuid, err))
		}
	}
	return TaskError(errors.Join(errs...))
}

// PushPrices sends the USD list prices of the connection's synced
// products. No priced product means no request and no run. On an inactive
// connection it records a held run (counts.held) and returns the
// HeldError.
func (p *PricePusher) PushPrices(ctx context.Context, conn db.IntegrationConnection) error {
	var after int64
	batch, err := p.priceBatch(ctx, conn, &after)
	if err != nil {
		return err
	}
	if len(batch) == 0 {
		return nil
	}
	run, err := p.startRun(ctx, conn, KindPushPrices)
	if err != nil {
		return err
	}
	var counts PricePushCounts
	client, err := p.client(conn)
	if err != nil {
		if _, held := IsHeld(err); held {
			counts.Held = len(batch)
			p.log.Warn("glorian_price_push_held", "connection", conn.Uuid, "reason", HeldInactiveConnection, "products", len(batch))
		}
		return run.finish(ctx, counts, time.Time{}, err)
	}
	for page := 0; len(batch) > 0; page++ {
		if page >= maxPushPages {
			return run.finish(ctx, counts, time.Time{}, fmt.Errorf("glorian price push: more than %d pages", maxPushPages))
		}
		counts.Products += len(batch)
		counts.Calls++
		res, err := client.UpdateProductPrices(ctx, batch)
		if err != nil {
			return run.finish(ctx, counts, time.Time{}, fmt.Errorf("price update: %w", err))
		}
		counts.Updated += len(res.Updated)
		counts.Unchanged += len(res.Unchanged)
		counts.NotFound += len(res.NotFound)
		for _, id := range res.NotFound {
			p.log.Warn("glorian_price_push_not_found", "connection", conn.Uuid, "remote_id", id)
		}
		if batch, err = p.priceBatch(ctx, conn, &after); err != nil {
			return run.finish(ctx, counts, time.Time{}, err)
		}
	}
	p.log.Info("glorian_price_push_done", "connection", conn.Uuid, "products", counts.Products, "calls", counts.Calls,
		"updated", counts.Updated, "unchanged", counts.Unchanged, "not_found", counts.NotFound)
	return run.finish(ctx, counts, time.Time{}, nil)
}

// priceBatch reads the next page of priced products after *after and
// moves the keyset.
func (p *PricePusher) priceBatch(ctx context.Context, conn db.IntegrationConnection, after *int64) ([]ProductPrice, error) {
	rows, err := p.q.ListGlorianPriceRows(ctx, db.ListGlorianPriceRowsParams{
		BrandID: conn.BrandID, ConnectionID: conn.ID, AfterID: *after, RowLimit: MaxBulkItems,
	})
	if err != nil {
		return nil, fmt.Errorf("glorian price push: list prices: %w", err)
	}
	out := make([]ProductPrice, 0, len(rows))
	for _, r := range rows {
		*after = r.ID
		item := ProductPrice{ProductID: r.ProductExternalID, Price: r.Price}
		if r.MeterPrice != "" {
			mp := r.MeterPrice
			item.MeterPrice = &mp
		}
		out = append(out, item)
	}
	return out, nil
}
