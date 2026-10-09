// @vitest-environment jsdom
import { act, createElement, isValidElement, type ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { DataTableProps } from "@/components/tables";
import {
  click,
  fill,
  flush,
  mount,
  render,
  submit,
  unmount,
  type Mounted,
} from "@/features/warehouse/components/test-helpers";

type AnyRow = Record<string, unknown>;

const s = vi.hoisted(() => ({
  tables: [] as Partial<DataTableProps<AnyRow>>[],
  grants: new Set<string>(),
  orgType: "center",
  features: ["e_invoice", "accounting"] as string[],
  ensure: vi.fn(),
  push: vi.fn(),
  service: {
    list: vi.fn(),
    billable: vi.fn(),
    get: vi.fn(),
    html: vi.fn(),
    archive: vi.fn(),
    void: vi.fn(),
    retryPdf: vi.fn(),
    download: vi.fn(),
    createDraft: vi.fn(),
    getSettings: vi.fn(),
    putSettings: vi.fn(),
    uploadXslt: vi.fn(),
    resetXslt: vi.fn(),
    samplePreview: vi.fn(),
    getProfile: vi.fn(),
    putProfile: vi.fn(),
  },
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: s.push }),
  usePathname: () => "/t/acme/einvoices",
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
    },
  }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() },
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => s.grants.has(x)),
  }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({ uuid: "c1", slug: "acme", type: s.orgType }),
}));
vi.mock("@/features/modules/hooks/use-features", () => ({
  useFeature: (_slug: string, key: string) => ({
    enabled: s.features.includes(key),
    isLoading: false,
    isError: false,
  }),
}));
vi.mock("@/features/step-up-engine", () => ({
  useStepUp: () => ({ ensure: s.ensure }),
}));
vi.mock("@/features/einvoice/services/einvoice.service", async (orig) => ({
  ...(await orig<object>()),
  einvoiceService: s.service,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityTable: (props: Partial<DataTableProps<AnyRow>>) => {
    s.tables.push(props);
    return null;
  },
}));

import { tenantNav } from "@/config/nav";
import { ApiError } from "@/lib/api";
import { visibleNavItems } from "@/features/nav-engine/lib/access";
import { BILLABLE_PERSIST_KEY, BillablePage } from "./billable-page";
import { InvoiceDetailPage } from "./invoice-detail-page";
import { InvoiceProfileCard } from "./invoice-profile-card";
import { INVOICES_PERSIST_KEY, InvoicesPage } from "./invoices-page";
import { EinvoiceSettingsPage } from "./settings-page";
import { VoidDialog } from "./void-dialog";

const READ = "einvoice.read";
const MANAGE = "einvoice.manage";
const SETTINGS = "einvoice.settings";

function invoice(over: AnyRow = {}) {
  return {
    uuid: "11111111-1111-4111-8111-111111111111",
    number: null,
    profile: "EARSIVFATURA",
    invoice_type: "SATIS",
    status: "draft",
    validation_status: "valid",
    validation_messages: [],
    source_type: "order",
    source_uuid: "22222222-2222-4222-8222-222222222222",
    buyer_organization: { uuid: "o2", name: "Ege Dağıtım" },
    buyer: {
      name: "Ege Dağıtım",
      vkn: "1234567890",
      einvoice_registered: false,
    },
    seller: { name: "Merkez", einvoice_registered: true },
    lines: [],
    tax_breakdown: [],
    currency: "TRY",
    line_extension: "300.00",
    tax_exclusive: "300.00",
    tax_total: "60.00",
    payable: "360.00",
    issue_date: "2026-10-01",
    xml_sha256: null,
    has_xml: false,
    has_pdf: false,
    error: null,
    voided_at: null,
    void_reason: null,
    created_at: "2026-10-01T08:00:00Z",
    updated_at: "2026-10-01T08:00:00Z",
    ...over,
  };
}

function settings(over: AnyRow = {}) {
  return {
    configured: true,
    vkn: "9000068418",
    tax_office: "Beşiktaş",
    legal_name: "Olexfilms Merkez A.Ş.",
    address: "Papatya Cad. No:21",
    city: "İstanbul",
    district: "Beşiktaş",
    country: "TR",
    iban: null,
    email: null,
    phone: null,
    website: null,
    trade_registry_no: null,
    mersis_no: null,
    default_note: null,
    earchive_series: "EAR",
    efatura_series: "EFN",
    pdf_enabled: true,
    custom_xslt: false,
    xslt_sha1: null,
    updated_at: "2026-10-01T08:00:00Z",
    counters: [{ series: "EAR", year: 2026, last_no: 42, updated_at: null }],
    ...over,
  };
}

// The Radix switch measures itself.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

const q = (sel: string) => document.body.querySelector(sel);

async function typeText(el: Element | null, value: string) {
  if (!el) throw new Error("element not found");
  await act(async () => {
    Object.getOwnPropertyDescriptor(
      HTMLTextAreaElement.prototype,
      "value",
    )?.set?.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

function buttonByText(text: string) {
  return Array.from(document.body.querySelectorAll("button")).find(
    (b) => b.textContent?.trim() === text,
  );
}

let m: Mounted;
beforeEach(() => {
  m = mount();
  s.tables.length = 0;
  s.orgType = "center";
  s.features = ["e_invoice", "accounting"];
  s.grants = new Set([READ, MANAGE, SETTINGS]);
  s.ensure.mockReset().mockResolvedValue(true);
  for (const fn of Object.values(s.service)) fn.mockReset();
  s.service.list.mockResolvedValue({
    items: [],
    total: 0,
    limit: 20,
    offset: 0,
  });
  s.service.billable.mockResolvedValue({
    items: [],
    total: 0,
    limit: 20,
    offset: 0,
  });
  s.service.html.mockResolvedValue("<html><body>invoice</body></html>");
  s.service.samplePreview.mockResolvedValue("<html><body>sample</body></html>");
  s.service.getSettings.mockResolvedValue(settings());
});
afterEach(() => unmount(m));

describe("e-invoice menu (TEC-504)", () => {
  const ids = (perms: string[], features: string[], type = "center") =>
    tenantNav("acme").groups.flatMap((group) =>
      visibleNavItems(group, {
        can: (p) =>
          (Array.isArray(p) ? p : [p]).every((x) => perms.includes(x)),
        canAny: (ps) => ps.some((x) => perms.includes(x)),
        org: { type, role: "owner", features },
      }).map((item) => item.id),
    );

  it("sits under Muhasebe for the center with einvoice.read and the module", () => {
    const group = tenantNav("acme").groups.find((g) => g.id === "accounting");
    expect(group?.items.map((i) => i.id)).toContain("accounting-einvoices");
    expect(
      ids(["accounting.read", READ], ["accounting", "e_invoice"]),
    ).toContain("accounting-einvoices");
  });

  it("is hidden while the e_invoice module is off", () => {
    expect(ids(["accounting.read", READ], ["accounting"])).not.toContain(
      "accounting-einvoices",
    );
  });

  it("is hidden without einvoice.read and outside the center", () => {
    expect(ids(["accounting.read"], ["accounting", "e_invoice"])).not.toContain(
      "accounting-einvoices",
    );
    expect(
      ids(
        ["accounting.read", READ],
        ["accounting", "e_invoice"],
        "distributor",
      ),
    ).not.toContain("accounting-einvoices");
  });
});

describe("e-invoice screens gate (TEC-504)", () => {
  it("shows the module-off state and loads nothing while the add-on is off", async () => {
    s.features = ["accounting"];
    await render(m, createElement(InvoicesPage, { slug: "acme" }));
    expect(m.container.textContent).toContain("einvoice.module_off_title");
    expect(s.tables).toHaveLength(0);
    expect(s.service.list).not.toHaveBeenCalled();
  });

  it("is forbidden for a distributor", async () => {
    s.orgType = "distributor";
    await render(m, createElement(InvoicesPage, { slug: "acme" }));
    expect(m.container.textContent).toContain("einvoice.forbidden");
  });
});

describe("invoice list (TEC-504)", () => {
  it("is a server DataTable with facets, ranges, persistKey and export", async () => {
    await render(m, createElement(InvoicesPage, { slug: "acme" }));
    const props = s.tables.at(-1)!;
    expect(props.features?.persistKey).toBe(INVOICES_PERSIST_KEY);
    const meta = (props.columns ?? []).map((c) => ({
      id: (c as { id?: string }).id,
      meta: (c as { meta?: { filterVariant?: string; param?: string } }).meta,
    }));
    const byId = Object.fromEntries(meta.map((c) => [c.id, c.meta]));
    expect(byId.status?.filterVariant).toBe("faceted");
    expect(byId.profile?.filterVariant).toBe("faceted");
    expect(byId.issue_date?.filterVariant).toBe("date-range");
    expect(byId.payable?.filterVariant).toBe("number-range");
    expect(s.service.list).toHaveBeenCalledWith(
      expect.objectContaining({ sort: "-issue_date", limit: 20, offset: 0 }),
    );
  });
});

describe("billable records (TEC-504)", () => {
  it("selects rows for the bulk create draft", async () => {
    await render(m, createElement(BillablePage, { slug: "acme" }));
    const props = s.tables.at(-1)!;
    expect(props.features?.persistKey).toBe(BILLABLE_PERSIST_KEY);
    expect(props.features?.rowSelection).toBe(true);
    const extra = (props as { toolbarExtra?: unknown })
      .toolbarExtra as ReactElement<{ children: unknown[] }>;
    const menu = (extra.props.children as unknown[]).find(
      (c) =>
        isValidElement(c) &&
        (c.props as { resource?: string }).resource === "einvoices.billable",
    ) as ReactElement<{ actions: { id: string }[] }> | undefined;
    expect(menu?.props.actions.map((a) => a.id)).toEqual(["create_draft"]);
  });

  it("has no selection without einvoice.manage", async () => {
    s.grants = new Set([READ]);
    await render(m, createElement(BillablePage, { slug: "acme" }));
    expect(s.tables.at(-1)!.features?.rowSelection).toBe(false);
  });
});

describe("settings form (TEC-504)", () => {
  it("shows an error for a series outside 3 characters and does not save", async () => {
    await render(m, createElement(EinvoiceSettingsPage, { slug: "acme" }));
    await fill(
      q('[data-testid="einvoice-earchive-series"]') as HTMLInputElement,
      "AB",
    );
    await submit(q("form"));
    expect(m.container.textContent).toContain(
      "einvoice.settings.validation.series_length",
    );
    expect(s.service.putSettings).not.toHaveBeenCalled();

    s.service.putSettings.mockResolvedValue(
      settings({ earchive_series: "OEA" }),
    );
    await fill(
      q('[data-testid="einvoice-earchive-series"]') as HTMLInputElement,
      "oea",
    );
    await submit(q("form"));
    expect(s.service.putSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        earchive_series: "OEA",
        efatura_series: "EFN",
      }),
    );
  });

  it("lists the counters read-only and renders the sample preview", async () => {
    await render(m, createElement(EinvoiceSettingsPage, { slug: "acme" }));
    expect(q('[data-testid="einvoice-counters"]')?.textContent).toContain(
      "EAR2026000000042",
    );
    const frame = q(
      '[data-testid="einvoice-xslt-sample"]',
    ) as HTMLIFrameElement;
    expect(frame.getAttribute("sandbox")).toBe("");
    expect(frame.getAttribute("srcdoc")).toContain("sample");
  });

  it("is read only without einvoice.settings", async () => {
    s.grants = new Set([READ]);
    await render(m, createElement(EinvoiceSettingsPage, { slug: "acme" }));
    expect(q('[data-testid="einvoice-settings-save"]')).toBeNull();
    expect(
      (q('[data-testid="einvoice-earchive-series"]') as HTMLInputElement)
        .disabled,
    ).toBe(true);
  });
});

describe("void (TEC-504)", () => {
  it("cannot be sent without a reason, then voids after step-up", async () => {
    s.service.void.mockResolvedValue(invoice({ status: "voided" }));
    await render(
      m,
      createElement(VoidDialog, {
        invoice: { uuid: "inv-1", number: "EAR2026000000001" },
        open: true,
        onOpenChange: () => undefined,
      }),
    );
    const confirm = q(
      '[data-testid="einvoice-void-confirm"]',
    ) as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);
    await typeText(q('[data-testid="einvoice-void-reason"]'), "   ");
    expect(confirm.disabled).toBe(true);
    await click(confirm);
    expect(s.service.void).not.toHaveBeenCalled();

    await typeText(q('[data-testid="einvoice-void-reason"]'), " Yanlış tutar ");
    expect(confirm.disabled).toBe(false);
    await click(confirm);
    expect(s.ensure).toHaveBeenCalled();
    expect(s.service.void).toHaveBeenCalledWith("inv-1", "Yanlış tutar");
  });

  it("does not void when the step-up is cancelled", async () => {
    s.ensure.mockResolvedValue(false);
    await render(
      m,
      createElement(VoidDialog, {
        invoice: { uuid: "inv-1", number: null },
        open: true,
        onOpenChange: () => undefined,
      }),
    );
    await typeText(q('[data-testid="einvoice-void-reason"]'), "Yanlış tutar");
    await click(q('[data-testid="einvoice-void-confirm"]'));
    expect(s.service.void).not.toHaveBeenCalled();
  });
});

describe("invoice detail (TEC-504)", () => {
  it("disables Arşivle for an invalid invoice and lists the validation messages", async () => {
    s.service.get.mockResolvedValue(
      invoice({
        status: "failed",
        validation_status: "invalid",
        validation_messages: ["[BR-TR-01] Alıcı VKN zorunludur"],
        error: "schematron: 1 error",
      }),
    );
    await render(
      m,
      createElement(InvoiceDetailPage, { slug: "acme", uuid: "inv-1" }),
    );
    const archive = q('[data-testid="einvoice-archive"]') as HTMLButtonElement;
    expect(archive.disabled).toBe(true);
    expect(q('[data-testid="einvoice-invalid-hint"]')).not.toBeNull();
    expect(
      q('[data-testid="einvoice-validation-messages"]')?.textContent,
    ).toContain("BR-TR-01");
    await click(archive);
    expect(s.service.archive).not.toHaveBeenCalled();
  });

  it("archives a valid draft after the 'nothing is sent' confirmation and step-up", async () => {
    s.service.get.mockResolvedValue(invoice());
    s.service.archive.mockResolvedValue(
      invoice({ status: "archived", number: "EAR2026000000001" }),
    );
    await render(
      m,
      createElement(InvoiceDetailPage, { slug: "acme", uuid: "inv-1" }),
    );
    const frame = q('[data-testid="einvoice-preview"]') as HTMLIFrameElement;
    expect(frame.getAttribute("sandbox")).toBe("");
    expect(
      q('[data-testid="einvoice-source-link"]')?.getAttribute("href"),
    ).toBe("/t/acme/orders/22222222-2222-4222-8222-222222222222");

    await click(q('[data-testid="einvoice-archive"]'));
    expect(document.body.textContent).toContain(
      "einvoice.archive.confirm_description",
    );
    expect(s.service.archive).not.toHaveBeenCalled();
    const dialogs = document.body.querySelectorAll('[role="dialog"]');
    const confirm = Array.from(
      dialogs[dialogs.length - 1].querySelectorAll("button"),
    ).find((b) => b.textContent?.trim() === "einvoice.actions.archive");
    await click(confirm ?? null);
    await flush();
    expect(s.ensure).toHaveBeenCalled();
    expect(s.service.archive).toHaveBeenCalledWith(
      "11111111-1111-4111-8111-111111111111",
    );
  });

  it("offers XML / PDF for an archived invoice and no Arşivle", async () => {
    s.service.get.mockResolvedValue(
      invoice({
        status: "archived",
        number: "EAR2026000000001",
        has_xml: true,
        has_pdf: false,
      }),
    );
    await render(
      m,
      createElement(InvoiceDetailPage, { slug: "acme", uuid: "inv-1" }),
    );
    expect(q('[data-testid="einvoice-archive"]')).toBeNull();
    expect(q('[data-testid="einvoice-download-xml"]')).not.toBeNull();
    expect(
      (q('[data-testid="einvoice-download-pdf"]') as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    expect(buttonByText("einvoice.actions.void")).toBeDefined();
  });
});

describe("organization invoice profile card (TEC-504)", () => {
  it("shows the profile with its missing fields", async () => {
    s.service.getProfile.mockResolvedValue({
      organization_uuid: "o2",
      organization_name: "Ege Dağıtım",
      organization_type: "distributor",
      invoice_vkn: "1234567890",
      invoice_tckn: null,
      invoice_tax_office: null,
      invoice_legal_name: null,
      einvoice_registered: false,
      einvoice_alias: null,
      invoice_email: null,
      missing_fields: ["invoice_tax_office"],
    });
    await render(m, createElement(InvoiceProfileCard, { orgUuid: "o2" }));
    expect(s.service.getProfile).toHaveBeenCalledWith("o2");
    expect(q('[data-testid="invoice-profile-card"]')).not.toBeNull();
    expect(m.container.textContent).toContain("einvoice.profile.incomplete");
    expect((q("#invoice_vkn") as HTMLInputElement).value).toBe("1234567890");
  });

  it("stays hidden when the center has the add-on off (403)", async () => {
    s.service.getProfile.mockRejectedValue(
      new ApiError({ status: 403, code: "FEATURE_DISABLED", message: "off" }),
    );
    await render(m, createElement(InvoiceProfileCard, { orgUuid: "o2" }));
    expect(m.container.innerHTML).toBe("");
  });

  it("is not rendered without einvoice.read", async () => {
    s.grants = new Set();
    await render(m, createElement(InvoiceProfileCard, { orgUuid: "o2" }));
    expect(m.container.innerHTML).toBe("");
    expect(s.service.getProfile).not.toHaveBeenCalled();
  });
});
