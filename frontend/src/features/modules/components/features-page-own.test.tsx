// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, Fragment, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { FeatureListItem } from "@/features/modules/types";

const captured = vi.hoisted(() => ({
  items: [] as unknown[],
  request: vi.fn(),
  cancelRequest: vi.fn(),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params?.price ? `${key}=${params.price}` : key,
    locale: "en",
    format: {
      currency: (v: number, code: string) => `${v.toFixed(2)} ${code}`,
      dateTime: (v: string) => v,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({ uuid: "o", slug: "acme", type: "dealer" }),
}));
vi.mock("@/features/modules/hooks/use-features", async (orig) => ({
  ...(await orig<object>()),
  useFeatures: () => ({
    data: { organization_type: "dealer", items: captured.items, enabled: [] },
    isLoading: false,
    isError: false,
    isFetching: false,
    refetch: vi.fn(),
  }),
}));
vi.mock("@/features/modules/services/modules.service", () => ({
  modulesService: {
    request: captured.request,
    cancelRequest: captured.cancelRequest,
  },
}));
// A plain table: one row per item, every column's cell rendered.
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityTable: ({
    columns,
    data,
  }: {
    columns: ColumnDef<FeatureListItem, unknown>[];
    data: FeatureListItem[];
  }) =>
    createElement(
      "div",
      null,
      data.map((row) =>
        createElement(
          "div",
          { key: row.key, "data-row": row.key },
          columns.map((col) =>
            createElement(
              Fragment,
              {
                key:
                  col.id ??
                  String((col as { accessorKey?: string }).accessorKey),
              },
              typeof col.cell === "function"
                ? (col.cell as (ctx: unknown) => ReactNode)({
                    row: { original: row },
                    getValue: () => undefined,
                  })
                : null,
            ),
          ),
        ),
      ),
    ),
}));

import { OwnModules } from "./features-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

function item(
  key: string,
  level: FeatureListItem["level"],
  extra: Partial<FeatureListItem> = {},
): FeatureListItem {
  return {
    key,
    level,
    enabled: false,
    visible: true,
    paid: level === "addon",
    default_enabled: level !== "addon",
    source: "default",
    upstream_enabled: true,
    admin_override: false,
    description: `${key} description`,
    free_default: level !== "addon",
    price: null,
    contact_for_price: false,
    request: null,
    ...extra,
  };
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  captured.request.mockReset().mockResolvedValue({});
  captured.cancelRequest.mockReset().mockResolvedValue({});
  captured.items = [
    item("organizations", "core", { enabled: true, source: "core" }),
    item("tasks", "core", { enabled: true, source: "core" }),
    item("leads", "standard", { enabled: true }),
    item("fleet", "addon", {
      price: {
        amount: "250",
        currency: "TRY",
        recurrence: "monthly",
        item_uuid: "i",
        item_name: "Filo paketi",
      },
    }),
    item("efficiency", "addon", {
      upstream_enabled: false,
      contact_for_price: true,
    }),
    item("performance", "addon", {
      contact_for_price: true,
      request: {
        uuid: "r1",
        status: "pending",
        note: "please",
        decision_note: "",
        created_at: "2026-10-01T00:00:00Z",
        decided_at: null,
      },
    }),
    item("certificates", "addon", {
      enabled: true,
      source: "service",
      contact_for_price: true,
    }),
  ];
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

async function renderPage() {
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client: new QueryClient() },
        createElement(OwnModules, { slug: "acme" }),
      ),
    );
  });
}

const q = (selector: string) => document.querySelector(selector);
const row = (key: string) => q(`[data-row="${key}"]`) as HTMLElement;

describe("Özellikler page (TEC-509)", () => {
  it("groups modules by level with counts and shows descriptions", async () => {
    await renderPage();
    const core = q('[data-testid="module-level-core"]')!;
    expect(core.querySelector("h2")?.textContent).toBe("core2");
    expect(q('[data-testid="module-level-standard"] h2')?.textContent).toBe(
      "standard1",
    );
    expect(q('[data-testid="module-level-addon"] h2')?.textContent).toBe(
      "addon4",
    );
    expect(row("fleet").textContent).toContain("fleet description");
  });

  it("core modules have neither request nor close", async () => {
    await renderPage();
    for (const key of ["organizations", "tasks"]) {
      expect(row(key).querySelector("button")).toBeNull();
      expect(row(key).querySelector('[role="switch"]')).toBeNull();
    }
  });

  it("a module off at the level above says so and has no request button", async () => {
    await renderPage();
    const off = row("efficiency");
    expect(
      off.querySelector('[data-testid="module-upstream-closed"]')?.textContent,
    ).toBe("modules.upstream_closed");
    expect(off.querySelector("button")).toBeNull();
    expect(q('[data-testid="module-request-fleet"]')).not.toBeNull();
  });

  it("shows the three price lines", async () => {
    await renderPage();
    const price = (key: string) =>
      q(`[data-testid="module-price-${key}"]`)?.textContent;
    expect(price("leads")).toBe("modules.price.free_default");
    expect(price("fleet")).toBe("modules.price.paid_monthly=250.00 TRY");
    expect(price("efficiency")).toBe("modules.price.contact");
  });

  // Labels fall back to the raw key with the identity `t` mock.
  it("source badges: subscription (TEC-311) and default", async () => {
    await renderPage();
    expect(
      row("leads").querySelector('[data-testid="module-via-subscription"]'),
    ).toBeNull();
    expect(
      row("certificates").querySelector(
        '[data-testid="module-via-subscription"]',
      ),
    ).not.toBeNull();
    expect(
      row("leads").querySelector('[data-testid="module-source"]')?.textContent,
    ).toBe("default");
  });

  it("pending request: badge and withdraw", async () => {
    await renderPage();
    expect(
      row("performance").querySelector(
        '[data-testid="module-request-pending"]',
      ),
    ).not.toBeNull();
    expect(q('[data-testid="module-request-performance"]')).toBeNull();
    await act(async () => {
      (
        q('[data-testid="module-request-cancel-performance"]') as HTMLElement
      ).click();
    });
    expect(captured.cancelRequest).toHaveBeenCalledWith("performance");
  });

  it("request dialog: a note over 1000 characters errors and is not sent", async () => {
    await renderPage();
    await act(async () => {
      (q('[data-testid="module-request-fleet"]') as HTMLElement).click();
    });
    const note = q(
      '[data-testid="module-request-note"]',
    ) as HTMLTextAreaElement;
    expect(note).not.toBeNull();
    const type = async (value: string) =>
      act(async () => {
        Object.getOwnPropertyDescriptor(
          HTMLTextAreaElement.prototype,
          "value",
        )?.set?.call(note, value);
        note.dispatchEvent(new Event("input", { bubbles: true }));
      });
    await type("x".repeat(1001));
    expect(q('[data-testid="module-request-note-error"]')?.textContent).toBe(
      "modules.requests.note_too_long",
    );
    const submit = q(
      '[data-testid="module-request-submit"]',
    ) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    await act(async () => submit.click());
    expect(captured.request).not.toHaveBeenCalled();

    await type("Filo modülü gerekli");
    expect(q('[data-testid="module-request-note-error"]')?.textContent).toBe(
      "",
    );
    await act(async () => submit.click());
    expect(captured.request).toHaveBeenCalledWith(
      "fleet",
      "Filo modülü gerekli",
    );
  });
});
