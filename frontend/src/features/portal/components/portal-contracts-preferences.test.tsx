// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const portal = vi.hoisted(() => ({
  listContracts: vi.fn(),
  getNotificationPreferences: vi.fn(),
  updateNotificationPreferences: vi.fn(),
}));
const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

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
vi.mock("next/navigation", () => ({
  usePathname: () => "/portal/contracts",
  useRouter: () => ({ replace: vi.fn(), push: vi.fn() }),
}));
vi.mock("sonner", () => ({ toast: toasts }));
vi.mock("@/hooks/use-mobile", () => ({
  useIsMobile: () => false,
  useIsXl: () => true,
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    dir: "ltr",
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
      date: (v: string) => `date(${v})`,
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/features/portal/lib/portal-client", async (orig) => ({
  ...(await orig<object>()),
  portalApi: portal,
}));

import type {
  PortalContract,
  PortalNotificationPreferences,
} from "@/features/portal/lib/portal-client";
import { isPortalReadOnly } from "@/features/portal/lib/portal-vehicles";

import { PortalContracts } from "./portal-contracts";
import { PortalNav } from "./portal-nav";
import { PortalPreferences } from "./portal-preferences";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
});

async function flush() {
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render(node: ReturnType<typeof createElement>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  await flush();
}

async function click(el: Element | null) {
  if (!(el instanceof HTMLElement)) throw new Error("element not found");
  await act(async () => {
    el.click();
  });
  await flush();
}

function contract(over: Partial<PortalContract> = {}): PortalContract {
  return {
    contract_uuid: "c-1",
    contract_no: 1001,
    service: { uuid: "s-1", service_no: "SRV-0001" },
    status: "completed",
    organization: { uuid: "o-1", name: "Bayi Kadıköy", type: "dealer" },
    vehicle_uuid: "v-1",
    car_brand_name: "BMW",
    car_model_name: "320i",
    model_year: 2021,
    plate: "34 ABC 123",
    plate_country: "TR",
    executed_at: "2026-05-01T11:00:00Z",
    pdf_ready: true,
    created_at: "2026-05-01T10:00:00Z",
    ...over,
  };
}

function page(items: PortalContract[]) {
  return { items, total: items.length, limit: 20, offset: 0 };
}

describe("PortalContracts", () => {
  it("shows the neutral empty state when the user has no contract", async () => {
    portal.listContracts.mockResolvedValue(page([]));
    await render(createElement(PortalContracts));
    expect(portal.listContracts).toHaveBeenCalledWith(20, 0);
    const empty = container.querySelector(
      '[data-testid="portal-contracts-empty"]',
    );
    expect(empty?.textContent).toContain("portal.contracts.empty_title");
    expect(empty?.textContent).toContain("portal.contracts.empty_hint");
    expect(empty?.textContent).not.toContain(
      "portal.contracts.empty_description",
    );
    expect(container.querySelector('[data-testid="portal-contracts"]')).toBe(
      null,
    );
  });

  it("lists a contract with its number, vehicle, dealer, date and PDF link", async () => {
    portal.listContracts.mockResolvedValue(page([contract()]));
    await render(createElement(PortalContracts));
    const table = container.querySelector('[data-testid="portal-contracts"]');
    expect(table).not.toBe(null);
    const no = table?.querySelectorAll('[data-testid="portal-contract-no"]');
    expect(no).toHaveLength(1);
    expect(no?.[0].textContent).toBe("#1001");
    const text = table?.textContent ?? "";
    expect(text).toContain("BMW 320i (2021)");
    expect(text).toContain("34 ABC 123");
    expect(text).toContain("Bayi Kadıköy");
    expect(text).toContain("date(2026-05-01T11:00:00Z)");
    expect(
      table
        ?.querySelector('[data-testid="portal-contract-service"]')
        ?.getAttribute("href"),
    ).toBe("/portal/services/s-1");
    const pdf = table?.querySelector('[data-testid="portal-contract-pdf"]');
    expect(pdf?.getAttribute("href")).toBe(
      "/api/portal/v1/portal/contracts/c-1/pdf",
    );
    expect(pdf?.textContent).toContain("portal.contracts.download_pdf");
    expect(
      table?.querySelector('[data-testid="portal-contract-pdf-pending"]'),
    ).toBe(null);
  });

  it("disables the PDF button while the PDF is being prepared", async () => {
    portal.listContracts.mockResolvedValue(
      page([contract({ pdf_ready: false, executed_at: null })]),
    );
    await render(createElement(PortalContracts));
    expect(container.querySelector('[data-testid="portal-contract-pdf"]')).toBe(
      null,
    );
    const pending = container.querySelector(
      '[data-testid="portal-contract-pdf-pending"]',
    );
    expect(pending).toBeInstanceOf(HTMLButtonElement);
    expect((pending as HTMLButtonElement).disabled).toBe(true);
    expect(pending?.textContent).toContain("portal.contracts.pdf_preparing");
    // No execution time yet: the record date stands in for the signing date.
    expect(container.textContent).toContain("date(2026-05-01T10:00:00Z)");
  });
});

describe("PortalPreferences", () => {
  const prefs: PortalNotificationPreferences = {
    email_enabled: true,
    inapp_enabled: true,
    realtime_enabled: true,
    push_enabled: false,
    rules: [{ event_code: null, channel: "email", enabled: true }],
  };

  it("toggles a channel and sends the whole preferences back", async () => {
    portal.getNotificationPreferences.mockResolvedValue(prefs);
    portal.updateNotificationPreferences.mockImplementation(
      async (body: PortalNotificationPreferences) => body,
    );
    await render(createElement(PortalPreferences));
    const email = container.querySelector(
      '[data-testid="portal-pref-email_enabled"]',
    );
    expect(email?.getAttribute("aria-checked")).toBe("true");

    await click(email);
    expect(portal.updateNotificationPreferences).toHaveBeenCalledWith({
      ...prefs,
      email_enabled: false,
    });
    expect(
      container
        .querySelector('[data-testid="portal-pref-email_enabled"]')
        ?.getAttribute("aria-checked"),
    ).toBe("false");
    expect(
      container
        .querySelector('[data-testid="portal-pref-inapp_enabled"]')
        ?.getAttribute("aria-checked"),
    ).toBe("true");
    expect(toasts.success).toHaveBeenCalledWith("portal.preferences.saved");
  });

  it("starts campaign marketing messages switched off when the API omits consent", async () => {
    portal.getNotificationPreferences.mockResolvedValue(prefs);
    portal.updateNotificationPreferences.mockImplementation(
      async (body: PortalNotificationPreferences) => body,
    );
    await render(createElement(PortalPreferences));

    const marketing = container.querySelector(
      '[data-testid="portal-pref-campaign_marketing_enabled"]',
    );
    expect(marketing?.getAttribute("aria-checked")).toBe("false");

    await click(marketing);
    expect(portal.updateNotificationPreferences).toHaveBeenCalledWith({
      ...prefs,
      campaign_marketing_enabled: true,
    });
  });

  it("keeps the saved value when the update fails", async () => {
    portal.getNotificationPreferences.mockResolvedValue(prefs);
    portal.updateNotificationPreferences.mockRejectedValue(new Error("boom"));
    await render(createElement(PortalPreferences));
    await click(
      container.querySelector('[data-testid="portal-pref-inapp_enabled"]'),
    );
    expect(toasts.error).toHaveBeenCalledWith("portal.preferences.failed");
    expect(
      container
        .querySelector('[data-testid="portal-pref-inapp_enabled"]')
        ?.getAttribute("aria-checked"),
    ).toBe("true");
  });
});

describe("portal read-only and nav (TEC-245)", () => {
  it("treats only a fleet-only session as read only", () => {
    expect(isPortalReadOnly(["fleet"])).toBe(true);
    expect(isPortalReadOnly(["customer"])).toBe(false);
    expect(isPortalReadOnly(["fleet", "customer"])).toBe(false);
    expect(isPortalReadOnly(undefined)).toBe(false);
  });

  it("links the contracts and preferences pages", async () => {
    await render(createElement(PortalNav));
    const hrefs = Array.from(container.querySelectorAll("a")).map((a) =>
      a.getAttribute("href"),
    );
    expect(hrefs).toContain("/portal/contracts");
    expect(hrefs).toContain("/portal/preferences");
    expect(
      container
        .querySelector('a[href="/portal/contracts"]')
        ?.getAttribute("aria-current"),
    ).toBe("page");
  });
});
