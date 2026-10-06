// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  listByLead: vi.fn(),
  create: vi.fn(),
  get: vi.fn(),
  patch: vi.fn(),
  replaceLines: vi.fn(),
  send: vi.fn(),
  remind: vi.fn(),
  accept: vi.fn(),
  reject: vi.fn(),
  requestPdf: vi.fn(),
  getRender: vi.fn(),
  downloadRender: vi.fn(),
  convert: vi.fn(),
}));
const leadsApi = vi.hoisted(() => ({ create: vi.fn() }));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  orgType: "center",
  superAdmin: false,
}));
const router = vi.hoisted(() => ({ push: vi.fn() }));
const tables = vi.hoisted(() => ({ props: [] as Record<string, unknown>[] }));

vi.mock("next/navigation", () => ({ useRouter: () => router }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    locale: "en",
    format: {
      number: (v: number) => String(v),
      date: (v: string) => v,
      dateTime: (v: string) => v,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock("@/lib/auth/session-store", () => ({
  useAuthStore: (selector: (s: unknown) => unknown) =>
    selector({
      user: {
        isSuperAdmin: state.superAdmin,
        organizations: [{ slug: "olex", type: state.orgType }],
      },
    }),
}));
vi.mock("@/features/leads/services/quotes.service", async (orig) => ({
  ...(await orig<object>()),
  quotesService: api,
}));
vi.mock("@/features/leads/services/leads.service", async (orig) => ({
  ...(await orig<object>()),
  leadsService: leadsApi,
}));
vi.mock("@/features/service-catalog/services/service-catalog.service", () => ({
  serviceCatalogKeys: { distributors: () => ["distributors"] },
  serviceCatalogService: {
    listVisible: vi.fn().mockResolvedValue({ items: [] }),
    listDistributors: vi
      .fn()
      .mockResolvedValue([{ uuid: "d-1", name: "Ege Dağıtım" }]),
  },
}));
vi.mock("@/features/exchange-rates/services/rates.service", () => ({
  ratesService: {
    currencies: vi.fn().mockResolvedValue({
      items: [{ code: "EUR", name: "Euro" }],
    }),
  },
}));
vi.mock("@/features/catalog/services/catalog.service", () => ({
  catalogService: { listProducts: vi.fn() },
}));
vi.mock("@/components/ui/async-combobox", () => ({
  AsyncCombobox: () => createElement("div", { "data-combobox": true }),
}));
vi.mock("@/components/ui/dialog", () => {
  const Pass = ({ children }: { children?: ReactNode }) =>
    createElement("div", null, children);
  return {
    Dialog: ({ open, children }: { open: boolean; children: ReactNode }) =>
      open ? createElement("div", null, children) : null,
    DialogContent: ({
      children,
      ...rest
    }: { children: ReactNode } & Record<string, unknown>) =>
      createElement("div", rest, children),
    DialogHeader: Pass,
    DialogFooter: Pass,
    DialogTitle: Pass,
    DialogDescription: Pass,
  };
});
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityTable: (props: Record<string, unknown>) => {
    tables.props.push(props);
    return null;
  },
}));

import { permissions } from "@/config/permissions";
import type { Lead } from "@/features/leads/services/leads.service";
import type { Quote } from "@/features/leads/services/quotes.service";
import { convertKinds } from "@/features/leads/lib/quotes";

import { LeadConvertDialog } from "./lead-convert-dialog";
import { LeadFormPage } from "./lead-form-page";
import { LeadQuotesTab } from "./lead-quotes-tab";

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
  tables.props = [];
  state.grants = new Set();
  state.orgType = "center";
  state.superAdmin = false;
});

async function flush() {
  for (let i = 0; i < 6; i++) {
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

const q = <T extends Element>(sel: string) =>
  container.querySelector(sel) as T | null;
const all = (sel: string) => Array.from(container.querySelectorAll(sel));

async function type(el: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )!.set!;
  await act(async () => {
    setter.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await flush();
}

async function click(sel: string) {
  await act(async () => {
    (q(sel) as HTMLElement).click();
  });
  await flush();
}

function quote(patch: Partial<Quote> = {}): Quote {
  return {
    uuid: "q-1",
    lead_uuid: "l-1",
    quote_no: 7,
    display_no: "Q-000007",
    currency: "TRY",
    // Deliberately not qty × price − discount: the UI must not recompute.
    subtotal: "200.00",
    discount_total: "10.00",
    tax_total: "0.00",
    grand_total: "175.00",
    valid_until: "2026-11-01T00:00:00Z",
    status: "draft",
    lines: [
      {
        line_type: "product",
        product_uuid: "p-1",
        description: "Seramik kaplama",
        quantity: "2",
        unit_price: "100.00",
        discount_amount: "10.00",
        line_total: "175.00",
        sort_order: 0,
      },
    ],
    created_at: "2026-10-01T10:00:00Z",
    updated_at: "2026-10-01T10:00:00Z",
    ...patch,
  };
}

function lead(patch: Partial<Lead> = {}): Lead {
  return {
    uuid: "l-1",
    organization_uuid: "o-1",
    target_type: "customer",
    customer_user_id: 42,
    vehicle_id: 9,
    candidate_company_name: "Kuzey Oto",
    candidate_contact_name: "Ayşe",
    candidate_phone_e164: "+905551234567",
    candidate_email: "a@example.com",
    source: "incoming_call",
    temperature: "warm",
    status: "quoted",
    notes: "",
    created_at: "2026-10-01T10:00:00Z",
    updated_at: "2026-10-01T10:00:00Z",
    ...patch,
  };
}

describe("LeadQuotesTab editor", () => {
  it("shows the line and grand totals from the backend response", async () => {
    state.grants = new Set([permissions.quotes.read, permissions.quotes.write]);
    api.listByLead.mockResolvedValueOnce({ items: [quote()], total: 1 });
    await render(createElement(LeadQuotesTab, { leadUuid: "l-1" }));

    expect(tables.props.at(-1)?.data).toHaveLength(1);
    expect(q("[data-testid=quote-line-total]")?.textContent).toBe("175.00 TRY");
    expect(q("[data-testid=quote-grand-total]")?.textContent).toBe(
      "175.00 TRY",
    );

    // Editing clears the stale line total until the backend recomputes it.
    await type(q("[data-testid=quote-line-quantity]")!, "3");
    expect(q("[data-testid=quote-line-total]")?.textContent).toBe(
      "leads.quotes.line.pending",
    );

    const saved = quote({
      grand_total: "260.00",
      updated_at: "2026-10-01T11:00:00Z",
      lines: [{ ...quote().lines[0]!, quantity: "3", line_total: "260.00" }],
    });
    api.replaceLines.mockResolvedValueOnce(saved);
    api.listByLead.mockResolvedValueOnce({ items: [saved], total: 1 });
    await click("[data-testid=quote-save]");

    expect(api.replaceLines).toHaveBeenCalledWith("q-1", [
      expect.objectContaining({
        line_type: "product",
        product_uuid: "p-1",
        quantity: "3",
        unit_price: null,
        discount_amount: "10.00",
      }),
    ]);
    expect(q("[data-testid=quote-line-total]")?.textContent).toBe("260.00 TRY");
    expect(q("[data-testid=quote-grand-total]")?.textContent).toBe(
      "260.00 TRY",
    );
  });

  it("keeps the unit price read-only without pricing.sale.write", async () => {
    state.grants = new Set([permissions.quotes.read, permissions.quotes.write]);
    api.listByLead.mockResolvedValue({ items: [quote()], total: 1 });
    await render(createElement(LeadQuotesTab, { leadUuid: "l-1" }));

    const price = q<HTMLInputElement>("[data-testid=quote-line-unit_price]")!;
    expect(price.readOnly).toBe(true);
    expect(q("[data-testid=quote-price-locked]")).not.toBeNull();
    expect(
      q<HTMLInputElement>("[data-testid=quote-line-quantity]")!.readOnly,
    ).toBe(false);
  });

  it("sends an overridden unit price with pricing.sale.write", async () => {
    state.grants = new Set([
      permissions.quotes.read,
      permissions.quotes.write,
      permissions.pricing.saleWrite,
    ]);
    api.listByLead.mockResolvedValue({ items: [quote()], total: 1 });
    api.replaceLines.mockResolvedValue(quote());
    await render(createElement(LeadQuotesTab, { leadUuid: "l-1" }));

    const price = q<HTMLInputElement>("[data-testid=quote-line-unit_price]")!;
    expect(price.readOnly).toBe(false);
    await type(price, "90.50");
    await click("[data-testid=quote-save]");
    expect(api.replaceLines).toHaveBeenCalledWith("q-1", [
      expect.objectContaining({ unit_price: "90.50" }),
    ]);
  });

  it("locks editing on a sent quote and offers remind / accept / reject", async () => {
    state.grants = new Set([permissions.quotes.read, permissions.quotes.write]);
    api.listByLead.mockResolvedValue({
      items: [quote({ status: "sent" })],
      total: 1,
    });
    await render(createElement(LeadQuotesTab, { leadUuid: "l-1" }));

    expect(q("[data-testid=quote-locked]")).not.toBeNull();
    expect(q("[data-testid=quote-save]")).toBeNull();
    expect(q("[data-testid=quote-send]")).toBeNull();
    expect(q("[data-testid=quote-add-product]")).toBeNull();
    expect(q("[data-testid=quote-line-remove]")).toBeNull();
    for (const input of all("[data-testid=quote-lines] input")) {
      expect((input as HTMLInputElement).disabled).toBe(true);
    }
    expect(
      q<HTMLButtonElement>("[data-testid=quote-valid-until] button")!.disabled,
    ).toBe(true);

    api.remind.mockResolvedValue(quote({ status: "sent" }));
    await click("[data-testid=quote-remind]");
    expect(api.remind).toHaveBeenCalledWith("q-1");

    api.accept.mockResolvedValue(quote({ status: "accepted" }));
    await click("[data-testid=quote-accept]");
    expect(api.accept).toHaveBeenCalledWith("q-1");
  });

  it("sends a draft over WhatsApp and shows the public link", async () => {
    state.grants = new Set([permissions.quotes.read, permissions.quotes.write]);
    api.listByLead.mockResolvedValue({ items: [quote()], total: 1 });
    api.send.mockResolvedValue({
      quote: quote({ status: "sent" }),
      public_url: "https://olex.test/q/abc",
    });
    await render(createElement(LeadQuotesTab, { leadUuid: "l-1" }));

    await click("[data-testid=quote-send]");
    expect(api.send).toHaveBeenCalledWith("q-1");
    expect(q("[data-testid=quote-public-url]")?.textContent).toContain(
      "https://olex.test/q/abc",
    );
  });

  it("hides write actions without quotes.write", async () => {
    state.grants = new Set([permissions.quotes.read]);
    api.listByLead.mockResolvedValue({ items: [quote()], total: 1 });
    await render(createElement(LeadQuotesTab, { leadUuid: "l-1" }));

    expect(q("[data-testid=quote-new]")).toBeNull();
    expect(q("[data-testid=quote-save]")).toBeNull();
    expect(q("[data-testid=quote-pdf]")).not.toBeNull();
  });
});

describe("LeadConvertDialog", () => {
  const open = (l: Lead, ctx: Parameters<typeof convertKinds>[0]) =>
    render(
      createElement(LeadConvertDialog, {
        lead: l,
        slug: "olex",
        ctx,
        open: true,
        onOpenChange: vi.fn(),
      }),
    );

  it("customer: draft service, then the wizard", async () => {
    api.convert.mockResolvedValue({
      lead: lead({ status: "won" }),
      service_draft: { uuid: "s-1" },
    });
    await open(lead(), {
      orgType: "center",
      isSuperAdmin: false,
      canConvertOrg: false,
    });

    expect(q("[data-testid=lead-convert-customer]")).not.toBeNull();
    expect(q("[data-testid=lead-convert-org]")).toBeNull();
    expect(q("[data-testid=lead-convert-distributor]")).toBeNull();
    expect(q("[data-testid=lead-convert-currency]")).toBeNull();

    await click("[data-testid=lead-convert-submit]");
    expect(api.convert).toHaveBeenCalledWith("l-1", { kind: "customer" });
    expect(router.push).toHaveBeenCalledWith("/t/olex/services/s-1/wizard");
  });

  it("dealer candidate at the center: distributor picker and org info", async () => {
    await open(lead({ target_type: "dealer_candidate" }), {
      orgType: "center",
      isSuperAdmin: false,
      canConvertOrg: true,
    });

    expect(q("[data-testid=lead-convert-org]")?.textContent).toContain(
      "Kuzey Oto",
    );
    const select = q<HTMLSelectElement>(
      "[data-testid=lead-convert-distributor]",
    )!;
    expect(select).not.toBeNull();
    expect(q("[data-testid=lead-convert-currency]")).toBeNull();

    // The center must pick a distributor before converting.
    await click("[data-testid=lead-convert-submit]");
    expect(api.convert).not.toHaveBeenCalled();
    expect(container.textContent).toContain(
      "leads.convert.errors.distributor_required",
    );

    await act(async () => {
      select.value = "d-1";
      select.dispatchEvent(new Event("change", { bubbles: true }));
    });
    api.convert.mockResolvedValue({ lead: lead({ status: "won" }) });
    await click("[data-testid=lead-convert-submit]");
    expect(api.convert).toHaveBeenCalledWith("l-1", {
      kind: "dealer_candidate",
      distributor_uuid: "d-1",
    });
  });

  it("dealer candidate at a distributor: no distributor picker", async () => {
    await open(lead({ target_type: "dealer_candidate" }), {
      orgType: "distributor",
      isSuperAdmin: false,
      canConvertOrg: true,
    });
    expect(q("[data-testid=lead-convert-org]")).not.toBeNull();
    expect(q("[data-testid=lead-convert-distributor]")).toBeNull();
  });

  it("distributor candidate for a center super_admin: currency and warehouse preset", async () => {
    await open(lead({ target_type: "distributor_candidate" }), {
      orgType: "center",
      isSuperAdmin: true,
      canConvertOrg: true,
    });
    expect(q("[data-testid=lead-convert-currency]")).not.toBeNull();
    expect(q("[data-testid=lead-convert-warehouse]")).not.toBeNull();
    expect(q("[data-testid=lead-convert-distributor]")).toBeNull();

    const currency = q<HTMLSelectElement>(
      "[data-testid=lead-convert-currency]",
    )!;
    await act(async () => {
      currency.value = "EUR";
      currency.dispatchEvent(new Event("change", { bubbles: true }));
    });
    api.convert.mockResolvedValue({ lead: lead({ status: "won" }) });
    await click("[data-testid=lead-convert-submit]");
    expect(api.convert).toHaveBeenCalledWith("l-1", {
      kind: "distributor_candidate",
      currency: "EUR",
      register_as_warehouse: true,
    });
  });

  it("a dealer user gets no distributor candidate conversion", async () => {
    expect(
      convertKinds({
        orgType: "dealer",
        isSuperAdmin: false,
        canConvertOrg: true,
      }),
    ).not.toContain("distributor_candidate");

    await open(lead({ target_type: "distributor_candidate" }), {
      orgType: "dealer",
      isSuperAdmin: false,
      canConvertOrg: true,
    });
    expect(q("[data-testid=lead-convert-forbidden]")).not.toBeNull();
    expect(q("[data-testid=lead-convert-currency]")).toBeNull();
    expect(q("[data-testid=lead-convert-warehouse]")).toBeNull();
    expect(q("[data-testid=lead-convert-submit]")).toBeNull();
  });

  it("a center user without super_admin cannot convert a distributor candidate", async () => {
    await open(lead({ target_type: "distributor_candidate" }), {
      orgType: "center",
      isSuperAdmin: false,
      canConvertOrg: true,
    });
    expect(q("[data-testid=lead-convert-forbidden]")).not.toBeNull();
    expect(q("[data-testid=lead-convert-currency]")).toBeNull();
  });
});

describe("lead target type options", () => {
  it("a dealer user has no distributor candidate option", async () => {
    state.orgType = "dealer";
    state.grants = new Set([permissions.leads.write]);
    await render(createElement(LeadFormPage, { slug: "olex" }));
    const options = all("[data-testid=lead-target-type] option").map(
      (o) => (o as HTMLOptionElement).value,
    );
    expect(options).toEqual(["customer", "dealer_candidate"]);
  });

  it("a center user keeps the distributor candidate option", async () => {
    state.grants = new Set([permissions.leads.write]);
    await render(createElement(LeadFormPage, { slug: "olex" }));
    const options = all("[data-testid=lead-target-type] option").map(
      (o) => (o as HTMLOptionElement).value,
    );
    expect(options).toContain("distributor_candidate");
  });
});
