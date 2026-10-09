// @vitest-environment jsdom
import { createElement, Fragment, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import {
  flush,
  mount,
  render,
  unmount,
  type Mounted,
} from "@/features/warehouse/components/test-helpers";

type AnyRow = Record<string, unknown>;

const s = vi.hoisted(() => ({
  tables: [] as Partial<DataTableProps<AnyRow>>[],
  grants: new Set<string>(),
  orgType: "center",
  service: {
    settings: vi.fn(),
    current: vi.fn(),
    versions: vi.fn(),
    publish: vi.fn(),
    discipline: vi.fn(),
    disciplineSummary: vi.fn(),
  },
  listProducts: vi.fn(),
  listDistributors: vi.fn(),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    dir: "ltr",
    format: {
      number: (v: number | null) => String(v),
      date: (v: string) => v,
      dateTime: (v: string) => v,
      currency: (v: number, c: string) => `${v.toFixed(2)} ${c}`,
      percent: (v: number) => `${(v * 100).toFixed(2)}%`,
    },
  }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() },
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => s.grants.has(p) }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({ uuid: "o1", slug: "acme", type: s.orgType }),
}));
vi.mock("@/features/step-up-engine", () => ({
  useStepUp: () => ({ ensure: () => Promise.resolve(false) }),
}));
vi.mock("@/features/geo/hooks/use-geo", () => ({
  useCountries: () => ({
    data: [
      {
        id: 1,
        iso2: "TR",
        iso3: "TUR",
        name_en: "Turkey",
        name_tr: "Türkiye",
        default_currency: "TRY",
      },
    ],
  }),
}));
vi.mock("@/features/pricing/services/recommended.service", async (orig) => ({
  ...(await orig<object>()),
  recommendedService: s.service,
}));
vi.mock("@/features/catalog/services/catalog.service", async (orig) => ({
  ...(await orig<object>()),
  catalogService: { listProducts: s.listProducts },
}));
vi.mock("@/features/catalog/services/pricing.service", async (orig) => ({
  ...(await orig<object>()),
  pricingService: { listDistributors: s.listDistributors },
}));
vi.mock("@/features/pricing/components/discipline-chart", () => ({
  DisciplineChart: () => createElement("div", { "data-testid": "chart" }),
}));
vi.mock("@/features/io/components/import-wizard", () => ({
  ImportWizard: () => null,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityPage: ({
    children,
    actions,
  }: {
    children: ReactNode;
    actions?: ReactNode;
  }) => createElement(Fragment, null, actions, children),
  EntityTable: (props: Partial<DataTableProps<AnyRow>>) => {
    s.tables.push(props);
    return null;
  },
}));

import {
  PRICE_DISCIPLINE_PERSIST_KEY,
  PriceDisciplinePage,
} from "./price-discipline-page";
import { RecommendedPricesPage } from "./recommended-prices-page";
import { RECOMMENDED_EDITOR_PERSIST_KEY } from "./recommended-editor";

let m: Mounted;
beforeEach(() => {
  m = mount();
  s.tables.length = 0;
  s.orgType = "center";
  s.grants = new Set(["pricing.recommended.read", "pricing.discipline.read"]);
  s.service.settings.mockResolvedValue({ deviation_warning_pct: 15 });
  s.service.current.mockResolvedValue({
    items: [
      {
        product_uuid: "p1",
        product_sku: "PPF-190",
        product_name: "Film",
        price: "1000.00",
        currency: "TRY",
        country_iso2: "",
        scope: "currency",
        effective_from: "2026-10-01",
        version_uuid: "v1",
        source: "publish",
        batch_id: null,
      },
    ],
    total: 1,
    limit: 100,
    offset: 0,
  });
  s.service.versions.mockResolvedValue({
    items: [],
    total: 0,
    limit: 100,
    offset: 0,
  });
  s.listProducts.mockResolvedValue({
    items: [{ uuid: "p1", sku: "PPF-190", name: "Film", active: true }],
    total: 1,
    limit: 20,
    offset: 0,
  });
  s.listDistributors.mockResolvedValue([{ uuid: "d1", name: "Dist" }]);
  s.service.disciplineSummary.mockResolvedValue({
    snapshot_date: "2026-10-09",
    threshold_pct: 20,
    countries: [
      {
        country_iso2: "TR",
        country_name_en: "Turkey",
        country_name_tr: "Türkiye",
        currency: "TRY",
        org_count: 3,
        row_count: 9,
        avg_deviation_pct: "12.50",
        median_deviation_pct: "25.00",
        over_threshold_org_count: 1,
      },
    ],
    products: [],
  });
  s.service.discipline.mockResolvedValue({
    items: [],
    total: 0,
    limit: 20,
    offset: 0,
  });
});
afterEach(() => {
  unmount(m);
  vi.clearAllMocks();
});

const ids = (t?: Partial<DataTableProps<AnyRow>>) =>
  (t?.columns ?? []).map(
    (c) => c.id ?? (c as { accessorKey?: string }).accessorKey,
  );

describe("RecommendedPricesPage (TEC-507)", () => {
  it("drafts an inline edit into the basket and merges the price in force", async () => {
    s.grants.add("pricing.recommended.write");
    await render(m, createElement(RecommendedPricesPage, { slug: "acme" }));
    await flush();
    const editor = s.tables.findLast(
      (t) => t.features?.persistKey === RECOMMENDED_EDITOR_PERSIST_KEY,
    );
    expect(editor?.features?.inlineEdit).toBe(true);
    expect(editor?.features?.rowSelection).toBe(true);
    expect(ids(editor)).toEqual(
      expect.arrayContaining([
        "name",
        "country",
        "currency",
        "current_price",
        "effective_from",
        "pending",
        "draft",
      ]),
    );
    expect(s.service.current).toHaveBeenCalledWith(
      expect.objectContaining({
        product: "p1",
        country: "none",
        currency: "TRY",
      }),
    );
    const row = editor?.data?.[0] as { current: { price: string } };
    expect(row.current.price).toBe("1000.00");

    const publish = () =>
      m.container.querySelector(
        '[data-testid="open-publish"]',
      ) as HTMLButtonElement | null;
    expect(publish()?.disabled).toBe(true);
    await render(m, createElement(RecommendedPricesPage, { slug: "acme" }));
    const props = s.tables.findLast(
      (t) => t.features?.persistKey === RECOMMENDED_EDITOR_PERSIST_KEY,
    ) as {
      onCellEdit: (e: {
        row: AnyRow;
        columnId: string;
        value: unknown;
      }) => void;
      data: AnyRow[];
    };
    props.onCellEdit({ row: props.data[0]!, columnId: "draft", value: "1100" });
    await flush();
    expect(publish()?.disabled).toBe(false);
  });

  it("is read-only without pricing.recommended.write", async () => {
    await render(m, createElement(RecommendedPricesPage, { slug: "acme" }));
    const editor = s.tables.findLast(
      (t) => t.features?.persistKey === RECOMMENDED_EDITOR_PERSIST_KEY,
    );
    expect(editor?.features?.inlineEdit).toBe(false);
    expect(
      m.container.querySelector('[data-testid="open-publish"]'),
    ).toBeNull();
  });

  it("is center only", async () => {
    s.orgType = "distributor";
    await render(m, createElement(RecommendedPricesPage, { slug: "acme" }));
    expect(m.container.textContent).toContain(
      "catalog.recommended.center_only",
    );
    expect(s.listProducts).not.toHaveBeenCalled();
  });
});

describe("PriceDisciplinePage (TEC-507)", () => {
  it("lists rows with the TEC-506 filters and country cards", async () => {
    await render(m, createElement(PriceDisciplinePage, { slug: "acme" }));
    await flush();
    const table = s.tables.findLast(
      (t) => t.features?.persistKey === PRICE_DISCIPLINE_PERSIST_KEY,
    );
    expect(ids(table)).toEqual(
      expect.arrayContaining([
        "org_name",
        "distributor",
        "product_name",
        "country_iso2",
        "deviation_pct",
        "over_threshold",
        "avg_sale_price",
      ]),
    );
    expect(s.service.discipline).toHaveBeenCalledWith(
      expect.objectContaining({ sort: "-deviation_pct" }),
    );
    const cards = m.container.querySelectorAll(
      '[data-testid="discipline-country-card"]',
    );
    expect(cards).toHaveLength(1);
    // Threshold from the summary (20): median 25 is over, average 12.5 not.
    const badges = cards[0]!.querySelectorAll(
      '[data-testid="deviation-badge"]',
    );
    expect(
      Array.from(badges).map((b) => b.getAttribute("data-over-threshold")),
    ).toEqual(["false", "true"]);
  });

  it("has no distributor filter for a distributor", async () => {
    s.orgType = "distributor";
    await render(m, createElement(PriceDisciplinePage, { slug: "acme" }));
    const table = s.tables.findLast(
      (t) => t.features?.persistKey === PRICE_DISCIPLINE_PERSIST_KEY,
    );
    expect(ids(table)).not.toContain("distributor");
    expect(s.listDistributors).not.toHaveBeenCalled();
  });
});
