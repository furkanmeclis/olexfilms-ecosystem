import type { MockApi, MockCall } from "./mock-api";

/**
 * TEC-1212: mocked report envelopes (TEC-1210 contract v2: period,
 * series totals with the previous window, kpis, typed columns, row links)
 * for the role overviews and the reports center. Values are seeded so the
 * screenshots are stable.
 */

type Json = Record<string, unknown>;

const DAYS = 30;
const START = Date.UTC(2026, 9, 16);

function seeded(seed: number) {
  let s = seed;
  return () => {
    s = (s * 9301 + 49297) % 233280;
    return s / 233280;
  };
}

function buckets(seed: number, base: number, spread: number) {
  const rnd = seeded(seed);
  return Array.from({ length: DAYS }, (_, i) => {
    const day = new Date(START + i * 86_400_000).toISOString().slice(0, 10);
    const value = Math.round(base + rnd() * spread + i * (spread / 60));
    const previous = Math.round(base * 0.85 + rnd() * spread);
    return { key: day, label: day, value, previous };
  });
}

function series(
  key: string,
  label: string,
  seed: number,
  base: number,
  spread: number,
) {
  const points = buckets(seed, base, spread);
  const total = points.reduce((a, p) => a + p.value, 0);
  const previous_total = points.reduce((a, p) => a + p.previous, 0);
  return {
    key,
    label,
    unit: "count",
    points,
    total,
    previous_total,
    delta_pct:
      Math.round(((total - previous_total) / previous_total) * 1000) / 10,
  };
}

function kpi(
  key: string,
  label: string,
  value: number,
  previous: number,
  opts: {
    unit?: string;
    good?: "up" | "down";
    seed?: number;
    link?: Json;
  } = {},
) {
  const rnd = seeded(opts.seed ?? value);
  return {
    key,
    label,
    unit: opts.unit ?? "count",
    value,
    previous,
    delta_pct:
      previous === 0
        ? null
        : Math.round(((value - previous) / previous) * 1000) / 10,
    good: opts.good ?? "up",
    spark: Array.from({ length: 14 }, () =>
      Math.round(value / 14 + rnd() * (value / 20)),
    ),
    ...(opts.link ? { link: opts.link } : {}),
  };
}

const DEALERS = [
  "Anka Oto Kaplama",
  "Boğaz Detailing",
  "Çınar Car Care",
  "Delta Film",
  "Ege Koruma",
  "Fırat Oto",
  "Gökkuşağı Detay",
  "Hisar PPF",
];

function ranking(key: string, label: string, seed: number, max: number) {
  const rnd = seeded(seed);
  const points = DEALERS.map((name, i) => ({
    key: `0b9c4c1e-0000-4000-8000-00000000f0${i}`,
    label: name,
    value: Math.round(max * (1 - i * 0.11) * (0.9 + rnd() * 0.1)),
  }));
  return {
    key,
    label,
    unit: "count",
    points,
    total: null,
    previous_total: null,
    delta_pct: null,
  };
}

const PERIOD = {
  from: "2026-10-16",
  to: "2026-11-14",
  granularity: "day",
  compare: "previous",
  previous_from: "2026-09-16",
  previous_to: "2026-10-15",
};

export function reportEnvelope(
  key: string,
  title: string,
  kind: string,
  extra: Json,
): Json {
  return {
    report: key,
    kind,
    title,
    locale: "en",
    timezone: "Europe/Istanbul",
    scope: "brand",
    period: "30d",
    period_info: PERIOD,
    range_from: PERIOD.from,
    range_to: PERIOD.to,
    granularity: "day",
    generated_at: "2026-11-15T09:00:00Z",
    series: [],
    kpis: [],
    columns: [],
    rows: [],
    ...extra,
  };
}

const dealerRows = DEALERS.map((name, i) => ({
  organization: name,
  services: 120 - i * 11,
  orders: 40 - i * 4,
  warranties: 90 - i * 9,
  revenue: (52000 - i * 5100).toFixed(2),
  currency: "TRY",
  // percent columns are ratios (0..1) in the v2 contract.
  share: Math.round(18 - i * 1.6) / 100,
  last_order_at: new Date(START + (29 - i * 3) * 86_400_000).toISOString(),
  _link: {
    kind: "organization",
    uuid: `0b9c4c1e-0000-4000-8000-00000000f0${i}`,
  },
}));

const dealerColumns = [
  {
    key: "organization",
    label: "Dealer",
    type: "org",
    align: "left",
    aggregate: "none",
  },
  {
    key: "services",
    label: "Services",
    type: "number",
    align: "right",
    aggregate: "sum",
  },
  {
    key: "orders",
    label: "Orders",
    type: "number",
    align: "right",
    aggregate: "sum",
  },
  {
    key: "warranties",
    label: "Warranties",
    type: "number",
    align: "right",
    aggregate: "sum",
  },
  {
    key: "revenue",
    label: "Revenue",
    type: "money",
    align: "right",
    aggregate: "sum",
    currency: "TRY",
  },
  {
    key: "share",
    label: "Share",
    type: "percent",
    align: "right",
    aggregate: "none",
  },
  {
    key: "last_order_at",
    label: "Last order",
    type: "date",
    align: "left",
    aggregate: "none",
  },
];

/** Envelopes by report key. */
export const REPORTS: Record<string, () => Json> = {
  overview: () =>
    reportEnvelope("overview", "Overview", "summary", {
      kpis: [
        kpi("services_created", "Services", 412, 368, { seed: 1 }),
        kpi("orders_created", "Orders", 138, 151, { seed: 2 }),
        kpi("warranties_started", "Warranties", 377, 340, { seed: 3 }),
        kpi("dealers_active", "Active dealers", 64, 61, { seed: 4 }),
        kpi("customers_created", "New customers", 295, 260, { seed: 5 }),
        kpi("measurements_created", "Measurements", 88, 92, { seed: 6 }),
      ],
    }),
  "services.trend": () =>
    reportEnvelope("services.trend", "Services trend", "timeseries", {
      series: [
        series("created", "Opened", 11, 8, 9),
        series("completed", "Completed", 12, 6, 8),
      ],
    }),
  "orders.trend": () =>
    reportEnvelope("orders.trend", "Orders trend", "timeseries", {
      series: [series("created", "Orders", 21, 3, 5)],
    }),
  "customers.trend": () =>
    reportEnvelope("customers.trend", "Customers trend", "timeseries", {
      series: [series("created", "New customers", 31, 6, 7)],
    }),
  "services.status_distribution": () =>
    reportEnvelope(
      "services.status_distribution",
      "Services by status",
      "distribution",
      {
        series: [
          {
            key: "status",
            label: "Services",
            unit: "count",
            points: [
              { key: "completed", label: "Completed", value: 280 },
              { key: "pending", label: "Pending", value: 84 },
              { key: "draft", label: "Draft", value: 31 },
              { key: "cancelled", label: "Cancelled", value: 17 },
            ],
            total: 412,
            previous_total: 368,
            delta_pct: 12,
          },
        ],
      },
    ),
  "services.top_products": () =>
    reportEnvelope("services.top_products", "Most used products", "ranking", {
      series: [
        {
          key: "products",
          label: "Uses",
          unit: "count",
          points: [
            "PPF Gloss 7.5",
            "PPF Matte 8",
            "Window Film 35",
            "Ceramic Pro",
            "PPF Black",
            "Window Film 50",
          ].map((label, i) => ({ key: `p${i}`, label, value: 160 - i * 22 })),
          total: null,
          previous_total: null,
          delta_pct: null,
        },
      ],
    }),
  "orders.by_buyer": () =>
    reportEnvelope("orders.by_buyer", "Top ordering dealers", "ranking", {
      series: [ranking("orders", "Orders", 41, 48)],
    }),
  "services.by_org": () =>
    reportEnvelope("services.by_org", "Top service dealers", "ranking", {
      series: [ranking("services", "Services", 51, 130)],
    }),
  "orders.inactive_buyers": () =>
    reportEnvelope(
      "orders.inactive_buyers",
      "Dealers without orders",
      "table",
      {
        columns: [
          {
            key: "organization",
            label: "Dealer",
            type: "org",
            align: "left",
            aggregate: "none",
          },
          {
            key: "last_order_at",
            label: "Last order",
            type: "date",
            align: "left",
            aggregate: "none",
          },
        ],
        rows: ["Ilgaz Oto", "Jale Detay", "Kuzey PPF"].map((name, i) => ({
          organization: name,
          last_order_at: i === 2 ? null : `2026-0${7 + i}-12T10:00:00Z`,
          _link: {
            kind: "organization",
            uuid: `0b9c4c1e-0000-4000-8000-00000000e0${i}`,
          },
        })),
      },
    ),
  "network.summary": () =>
    reportEnvelope("network.summary", "Network summary", "table", {
      kpis: [
        kpi("services", "Network services", 286, 250, { seed: 7 }),
        kpi("orders", "Network orders", 74, 69, { seed: 8 }),
        kpi("warranties", "Network warranties", 240, 210, { seed: 9 }),
        kpi("active_dealers", "Active dealers", 18, 19, { seed: 10 }),
      ],
      series: [
        series("services", "Services", 61, 6, 8),
        series("orders", "Orders", 62, 1, 4),
      ],
      columns: dealerColumns,
      rows: dealerRows,
    }),
  "stock.summary": () =>
    reportEnvelope("stock.summary", "Stock summary", "summary", {
      kpis: [
        kpi("units_in_stock", "Units in stock", 1840, 1920, { seed: 11 }),
        kpi("meters_in_stock", "Meters in stock", 9320, 8800, {
          seed: 12,
          unit: "meter",
        }),
        kpi("low_stock", "Low stock products", 7, 4, {
          seed: 13,
          good: "down",
        }),
      ],
    }),
  "stock.movement": () =>
    reportEnvelope("stock.movement", "Stock movement", "timeseries", {
      series: [
        series("in", "In", 71, 20, 30),
        series("out", "Out", 72, 18, 26),
      ],
      kpis: [
        kpi("in", "Stock in", 1410, 1250, { seed: 14 }),
        kpi("out", "Stock out", 1290, 1310, { seed: 15 }),
      ],
    }),
  "orders.fulfillment": () =>
    reportEnvelope("orders.fulfillment", "Order fulfillment", "timeseries", {
      kpis: [
        kpi("prep_hours", "Avg. preparation", 14.5, 18.2, {
          seed: 16,
          unit: "hours",
          good: "down",
        }),
        kpi("open", "Open orders", 23, 19, { seed: 17, good: "down" }),
        kpi("late", "Late orders", 4, 6, { seed: 18, good: "down" }),
      ],
      series: [
        series("shipped", "Shipped", 81, 3, 6),
        series("received", "Received", 82, 2, 6),
      ],
      columns: [
        {
          key: "warehouse",
          label: "Warehouse",
          type: "text",
          align: "left",
          aggregate: "none",
        },
        {
          key: "shipped",
          label: "Shipped",
          type: "number",
          align: "right",
          aggregate: "sum",
        },
        {
          key: "received",
          label: "Received",
          type: "number",
          align: "right",
          aggregate: "sum",
        },
        {
          key: "prep_hours",
          label: "Avg. preparation",
          type: "duration",
          align: "right",
          aggregate: "avg",
        },
      ],
      rows: [
        {
          warehouse: "Merkez Depo",
          shipped: 118,
          received: 102,
          prep_hours: 12.4,
        },
        {
          warehouse: "Ankara Depo",
          shipped: 46,
          received: 41,
          prep_hours: 19.8,
        },
      ],
    }),
  "warranties.summary": () =>
    reportEnvelope("warranties.summary", "Warranties", "summary", {
      kpis: [
        kpi("started", "Warranties started", 377, 340, { seed: 19 }),
        kpi("active", "Active warranties", 5120, 4980, { seed: 20 }),
      ],
      series: [series("started", "Started", 91, 9, 8)],
    }),
  "measurements.summary": () =>
    reportEnvelope("measurements.summary", "Measurements", "summary", {
      kpis: [kpi("created", "Measurements", 88, 92, { seed: 21 })],
      series: [series("created", "Measurements", 92, 1, 5)],
    }),
  "activities.recent": () =>
    reportEnvelope("activities.recent", "Recent activity", "table", {
      columns: [
        {
          key: "service_no",
          label: "Service",
          type: "text",
          align: "left",
          aggregate: "none",
        },
        {
          key: "organization",
          label: "Dealer",
          type: "org",
          align: "left",
          aggregate: "none",
        },
        {
          key: "status",
          label: "Status",
          type: "status",
          align: "left",
          aggregate: "none",
        },
        {
          key: "created_at",
          label: "Created",
          type: "datetime",
          align: "left",
          aggregate: "none",
        },
      ],
      rows: DEALERS.slice(0, 6).map((name, i) => ({
        service_no: `SE2026${String(410 - i).padStart(5, "0")}`,
        organization: name,
        status: i % 3 === 0 ? "pending" : "completed",
        created_at: new Date(
          START + (29 - i) * 86_400_000 + 36e5 * 9,
        ).toISOString(),
        _link: {
          kind: "service",
          uuid: `0b9c4c1e-0000-4000-8000-00000000d0${i}`,
        },
        _status: {
          status:
            i % 3 === 0
              ? { label: "Pending", tone: "progress" }
              : { label: "Completed", tone: "done" },
        },
      })),
    }),
  "dealers.performance": () =>
    reportEnvelope("dealers.performance", "Dealer performance", "table", {
      columns: dealerColumns,
      rows: dealerRows,
    }),
};

export const CATALOG_ITEMS = Object.entries(REPORTS).map(([key, build]) => {
  const env = build();
  return {
    key,
    kind: env.kind,
    title: env.title,
    module: null,
    periodic: true,
    periods: ["today", "7d", "30d", "90d", "12m"],
    default_period: "30d",
    granularities: ["day", "week", "month"],
    limited: false,
  };
});

/** Adds the reports routes to a MockApi (catalog, envelopes, export). */
export function mockReports(api: MockApi, only?: readonly string[]) {
  const allowed = (key: string) => !only || only.includes(key);
  api.extra.push(async (call: MockCall) => {
    if (call.method !== "GET") return false;
    if (call.path === "/v1/reports/catalog") {
      await call.ok({ items: CATALOG_ITEMS.filter((i) => allowed(i.key)) });
      return true;
    }
    const m = call.path.match(/^\/v1\/reports\/([^/]+)(\/export)?$/);
    if (!m) return false;
    const key = decodeURIComponent(m[1] ?? "");
    const build = REPORTS[key];
    if (!build || !allowed(key)) {
      await call.route.fulfill({
        status: 403,
        json: { error: { code: "FORBIDDEN", message: "No access" } },
      });
      return true;
    }
    if (m[2]) {
      await call.route.fulfill({
        status: 200,
        headers: {
          "Content-Type": "text/csv",
          "Content-Disposition": `attachment; filename="${key}.csv"`,
        },
        body: "a,b\n1,2\n",
      });
      return true;
    }
    await call.ok(build());
    return true;
  });
}

/** Accounting P&L (GET /v1/accounting/reports/pnl) by month. */
export function pnlReport(
  from: string | null,
  to: string | null,
  factor = 1,
): Json {
  const months = [
    ["2026-09", "September 2026", 412000, 296000],
    ["2026-10", "October 2026", 468000, 318000],
    ["2026-11", "November 2026", 251000, 174000],
  ] as const;
  const lines = months.map(([key, label, income, expense]) => ({
    key,
    label,
    income: (income * factor).toFixed(2),
    expense: (expense * factor).toFixed(2),
    net: ((income - expense) * factor).toFixed(2),
    entry_count: Math.round(120 * factor),
  }));
  const sum = (k: "income" | "expense" | "net") =>
    lines.reduce((a, l) => a + Number(l[k]), 0).toFixed(2);
  return {
    organization: {
      uuid: "0b9c4c1e-0000-4000-8000-000000000001",
      name: "Acme",
    },
    currency: "TRY",
    from,
    to,
    group: "month",
    lines,
    totals: { income: sum("income"), expense: sum("expense"), net: sum("net") },
    generated_at: "2026-11-15T09:00:00Z",
  };
}

/**
 * Every other GET of the overview widgets (announcements, queues, order
 * lists, stock) answers an empty page so no "not found" toast covers the
 * screen. Added last.
 */
export function mockEmptyLists(api: MockApi) {
  api.extra.push(async (call: MockCall) => {
    if (call.method !== "GET") return false;
    if (call.path === "/v1/accounting/reports/pnl") {
      const from = call.url.searchParams.get("from");
      const to = call.url.searchParams.get("to");
      // the compared window is the older one: smaller figures
      await call.ok(
        pnlReport(from, to, from && from < "2026-10-01" ? 0.86 : 1),
      );
      return true;
    }
    if (call.path === "/v1/accounting/queue/summary") {
      await call.ok({
        orders_to_invoice: 6,
        orders_in_transit: 3,
        returns_pending: 1,
      });
      return true;
    }
    await call.ok({ items: [], total: 0, limit: 20, offset: 0 });
    return true;
  });
}
