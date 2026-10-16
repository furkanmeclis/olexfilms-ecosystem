import { beforeEach, describe, expect, it, vi } from "vitest";

const request = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/platform-request", () => ({ platformRequest: request }));

import {
  STOCK_FORECAST_EXPORT_PATH,
  STOCK_FORECAST_NETWORK_EXPORT_PATH,
  stockForecastService,
} from "./stock-forecast.service";

beforeEach(() => {
  request.mockReset();
  request.mockResolvedValue({ items: [], total: 7, limit: 1, offset: 0 });
});

// TEC-755: the service calls the routes of backend stockforecast/routes.go.
describe("stockForecastService", () => {
  it("lists, reads meta and the product detail", async () => {
    await stockForecastService.list({
      status: "critical",
      limit: 20,
      offset: 0,
    });
    await stockForecastService.meta();
    await stockForecastService.detail("p 1");
    expect(request.mock.calls).toEqual([
      [
        "GET",
        "/v1/stock-forecasts",
        { query: { status: "critical", limit: 20, offset: 0 } },
      ],
      ["GET", "/v1/stock-forecasts/meta"],
      ["GET", "/v1/stock-forecasts/p%201"],
    ]);
  });

  it("saves thresholds and order drafts with the backend bodies", async () => {
    const thresholds = {
      warning_days: 14,
      cover_days: 30,
      products: [{ product_uuid: "p1", warning_days: 10, cover_days: 45 }],
    };
    await stockForecastService.putThresholds(thresholds);
    await stockForecastService.createOrderDraft({
      items: [{ product_uuid: "p1", meters: "12.5" }],
    });
    expect(request.mock.calls).toEqual([
      ["PUT", "/v1/stock-forecasts/thresholds", { body: thresholds }],
      [
        "POST",
        "/v1/stock-forecasts/order-draft",
        { body: { items: [{ product_uuid: "p1", meters: "12.5" }] } },
      ],
    ]);
  });

  it("reads network demand and the distributor subtree", async () => {
    await stockForecastService.network({ q: "film", limit: 20, offset: 0 });
    await stockForecastService.subtree({ limit: 20, offset: 0 });
    expect(request.mock.calls).toEqual([
      [
        "GET",
        "/v1/stock-forecasts/network",
        { query: { q: "film", limit: 20, offset: 0 } },
      ],
      [
        "GET",
        "/v1/stock-forecasts/subtree",
        { query: { limit: 20, offset: 0 } },
      ],
    ]);
  });

  it("counts critical products for the widget through the list", async () => {
    await expect(stockForecastService.criticalCount()).resolves.toBe(7);
    expect(request).toHaveBeenCalledWith("GET", "/v1/stock-forecasts", {
      query: { status: "critical", limit: 1, offset: 0 },
    });
  });

  it("exports through the backend export routes", () => {
    expect(STOCK_FORECAST_EXPORT_PATH).toBe("/v1/stock-forecasts/export");
    expect(STOCK_FORECAST_NETWORK_EXPORT_PATH).toBe(
      "/v1/stock-forecasts/network/export",
    );
  });
});
