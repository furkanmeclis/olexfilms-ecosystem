// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const exportsApi = vi.hoisted(() => ({ request: vi.fn() }));
const http = vi.hoisted(() => ({ platformRequest: vi.fn() }));
const state = vi.hoisted(() => ({ grants: new Set<string>() }));

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string;
    children: unknown;
  } & Record<string, unknown>) =>
    createElement("a", { href, ...rest }, children as never),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    dir: "ltr",
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
      percent: (v: number) => `${v * 100}%`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock("@/lib/api/platform-request", () => http);
vi.mock("@/components/ui/date-picker", async () => {
  const { createElement: h } = await import("react");
  return {
    DatePicker: ({
      id,
      value,
      onChange,
    }: {
      id?: string;
      value?: string;
      onChange?: (v: string) => void;
    }) =>
      h("input", {
        id,
        value: value ?? "",
        onChange: (e: { target: { value: string } }) =>
          onChange?.(e.target.value),
      }),
  };
});
vi.mock("@/hooks/use-mobile", () => ({
  useIsMobile: () => false,
  useIsXl: () => true,
}));
vi.mock("@/features/io/services/exports.service", () => ({
  exportsService: exportsApi,
}));

import { Permission } from "@/config/permissions";
import type {
  ByDealerRow,
  FailureRateRow,
} from "@/features/warranty-claims/services/claim-reports.service";

import { chooseValue, installRadixPolyfills } from "@/test/form-controls";
import { WarrantyClaimReportsPage } from "./warranty-claim-reports-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
installRadixPolyfills();

Element.prototype.scrollIntoView ??= () => {};

let container: HTMLDivElement;
let root: Root;

const failureRows: FailureRateRow[] = [
  {
    group: "product",
    product_uuid: "p-1",
    product_sku: "PPF-1",
    product_name: "Film PPF",
    lot_uuid: null,
    lot_code: null,
    warranty_count: 8,
    claim_count: 2,
    approved_claim_count: 1,
    claim_rate: 0.25,
    approved_rate: 0.125,
  },
];

// The by-dealer report as the API scopes it for a distributor: itself and
// the dealers of its subtree, nothing else.
const subtreeRows: ByDealerRow[] = [
  {
    organization_uuid: "dist",
    organization_name: "Distributor X",
    organization_type: "distributor",
    claim_count: 1,
    approved_claim_count: 1,
    rejected_claim_count: 0,
    approval_rate: 1,
  },
  {
    organization_uuid: "d-1",
    organization_name: "Dealer A",
    organization_type: "dealer",
    claim_count: 3,
    approved_claim_count: 2,
    rejected_claim_count: 1,
    approval_rate: 2 / 3,
  },
];

function respond(path: string, query?: Record<string, string>) {
  if (path.endsWith("/failure-rate")) {
    const group = query?.group ?? "product";
    return Promise.resolve({
      group,
      items: failureRows.map((r) => ({ ...r, group })),
    });
  }
  if (path.endsWith("/by-dealer")) {
    return Promise.resolve({ items: subtreeRows });
  }
  if (path.endsWith("/parts")) {
    return Promise.resolve({
      items: [
        {
          part_key: "body_kaput",
          product_uuid: "p-1",
          product_sku: "PPF-1",
          product_name: "Film PPF",
          part_count: 2,
          claim_count: 2,
          approved_claim_count: 1,
        },
      ],
    });
  }
  return Promise.reject(new Error(`unexpected ${path}`));
}

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  http.platformRequest.mockImplementation(
    (
      _method: string,
      path: string,
      opts?: { query?: Record<string, string> },
    ) => respond(path, opts?.query),
  );
  exportsApi.request.mockResolvedValue({ uuid: "job-1", status: "queued" });
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
  state.grants = new Set();
});

async function flush() {
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(WarrantyClaimReportsPage, { slug: "dist-x" }),
      ),
    );
  });
  await flush();
}

const $ = (sel: string) => container.querySelector(sel);
const $$ = (sel: string) => [...container.querySelectorAll(sel)];

async function click(el: Element | null) {
  if (!(el instanceof HTMLElement)) throw new Error("element not found");
  await act(async () => {
    el.click();
  });
  await flush();
}

async function select(el: Element | null, value: string) {
  await chooseValue(el, value);
  await flush();
}

async function typeDate(el: Element | null, value: string) {
  if (!(el instanceof HTMLInputElement)) throw new Error("input not found");
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  await act(async () => {
    setter?.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await flush();
}

describe("WarrantyClaimReportsPage", () => {
  it("is forbidden without warranty_claims.read", async () => {
    await render();
    expect(http.platformRequest).not.toHaveBeenCalled();
    expect(container.textContent).toContain("warranty.claim_reports.forbidden");
  });

  it("shows the product failure rates formatted with Intl", async () => {
    state.grants = new Set([Permission.WarrantyClaimsRead]);
    await render();
    expect(http.platformRequest).toHaveBeenCalledWith(
      "GET",
      "/v1/warranty-claims/reports/failure-rate",
      { query: { group: "product" } },
    );
    expect($$("[data-testid=claim-report-failure-row]")).toHaveLength(1);
    expect($("[data-testid=claim-rate]")?.textContent).toBe("25%");
    expect($("[data-testid=approved-rate]")?.textContent).toBe("12.5%");
  });

  it("limits the dealer filter to the scoped (subtree) rows", async () => {
    state.grants = new Set([Permission.WarrantyClaimsRead]);
    await render();
    await click($("[data-testid=claim-report-tab-dealer]"));

    const paths = http.platformRequest.mock.calls.map((c) => c[1]);
    expect(paths).toContain("/v1/warranty-claims/reports/by-dealer");
    // No unscoped organization list is ever loaded for the filter.
    expect(paths.every((p) => p.startsWith("/v1/warranty-claims/"))).toBe(true);

    expect($$("[data-testid=claim-report-dealer-row]")).toHaveLength(2);

    // TEC-378: the dealer filter is the organization facet of the table;
    // its options come only from the scoped rows (center rows excluded).
    const facet = $$("button").find((b) =>
      b.textContent?.includes("warranty.claim_reports.columns.organization"),
    );
    await click(facet ?? null);
    const items = [...document.body.querySelectorAll("[cmdk-item]")];
    expect(items.map((i) => i.textContent?.trim())).toEqual([
      "Dealer A",
      "Distributor X",
    ]);
    await click(items[0] ?? null);
    const rows = $$("[data-testid=claim-report-dealer-row]");
    expect(rows.map((r) => r.getAttribute("data-uuid"))).toEqual(["d-1"]);
    expect(rows[0]?.closest("tr")?.textContent).toContain("66.7%");
  });

  it("renders each report as a sortable client-side table (TEC-378)", async () => {
    state.grants = new Set([Permission.WarrantyClaimsRead]);
    await render();
    const sortable = (table: string) =>
      $$(`[data-testid=${table}] thead th`)
        .filter((th) => th.querySelector("button"))
        .map((th) => th.textContent?.trim());
    expect(sortable("claim-report-failure-table")).toEqual([
      "warranty.claim_reports.columns.product",
      "warranty.claim_reports.columns.warranties",
      "warranty.claim_reports.columns.claims",
      "warranty.claim_reports.columns.approved",
      "warranty.claim_reports.columns.claim_rate",
      "warranty.claim_reports.columns.approved_rate",
    ]);
    await click($("[data-testid=claim-report-tab-lot]"));
    expect(sortable("claim-report-failure-table")).toContain(
      "warranty.claim_reports.columns.lot",
    );
    await click($("[data-testid=claim-report-tab-parts]"));
    expect(sortable("claim-report-parts-table")).toEqual([
      "warranty.claim_reports.columns.part",
      "warranty.claim_reports.columns.product",
      "warranty.claim_reports.columns.part_count",
      "warranty.claim_reports.columns.claims",
      "warranty.claim_reports.columns.approved",
    ]);
  });

  it("exports the active tab through its own endpoint", async () => {
    state.grants = new Set([Permission.WarrantyClaimsRead]);
    await render();

    await typeDate($("#claim-report-from"), "2026-09-01");
    await click($("[data-testid=claim-report-tab-lot]"));
    await click($("[data-testid=claim-report-export]"));
    expect(exportsApi.request).toHaveBeenLastCalledWith(
      "/v1/warranty-claims/reports/failure-rate/export",
      {
        format: "xlsx",
        locale: "en",
        query: { from: "2026-09-01", group: "lot" },
      },
    );
    expect(
      $("[data-testid=claim-report-export-link]")?.getAttribute("href"),
    ).toBe("/t/dist-x/exports");

    await click($("[data-testid=claim-report-tab-parts]"));
    await select($("[data-testid=claim-report-export-format]"), "csv");
    await click($("[data-testid=claim-report-export]"));
    expect(exportsApi.request).toHaveBeenLastCalledWith(
      "/v1/warranty-claims/reports/parts/export",
      { format: "csv", locale: "en", query: { from: "2026-09-01" } },
    );
    expect($$("[data-testid=claim-report-parts-row]")).toHaveLength(1);

    await click($("[data-testid=claim-report-tab-dealer]"));
    await click($("[data-testid=claim-report-export]"));
    expect(exportsApi.request).toHaveBeenLastCalledWith(
      "/v1/warranty-claims/reports/by-dealer/export",
      { format: "csv", locale: "en", query: { from: "2026-09-01" } },
    );
  });

  it("blocks reads and export while the period is inverted", async () => {
    state.grants = new Set([Permission.WarrantyClaimsRead]);
    await render();
    http.platformRequest.mockClear();
    await typeDate($("#claim-report-from"), "2026-10-01");
    await typeDate($("#claim-report-to"), "2026-09-01");
    expect($("[data-testid=claim-report-period-error]")).not.toBeNull();
    expect(
      ($("[data-testid=claim-report-export]") as HTMLButtonElement).disabled,
    ).toBe(true);
    const calls = http.platformRequest.mock.calls.filter(
      (c) => c[2]?.query?.to === "2026-09-01",
    );
    expect(calls).toHaveLength(0);
  });
});
