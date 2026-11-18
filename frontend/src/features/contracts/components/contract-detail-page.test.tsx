// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({ get: vi.fn(), downloadPdf: vi.fn() }));
const state = vi.hoisted(() => ({ grants: new Set<string>() }));

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: { href: string; children: unknown } & Record<string, unknown>) =>
    createElement("a", { href, ...rest }, children as never),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock("@/lib/toast", () => {
  const t = { success: vi.fn(), error: vi.fn() };
  return { toast: t, appToast: t };
});
vi.mock("@/features/organizations/providers/tenant-provider", () => ({
  useOptionalTenant: () => ({ slug: "olex" }),
}));
vi.mock(
  "@/features/contracts/services/contract-signing.service",
  async (orig) => ({
    ...(await orig<object>()),
    contractSigningService: api,
  }),
);
// Renders every cell of the rows (the real table is covered elsewhere).
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityTable: ({
    columns,
    data,
  }: {
    columns: {
      id?: string;
      accessorKey?: string;
      cell?: (ctx: unknown) => ReactNode;
    }[];
    data: { uuid: string }[];
  }) =>
    createElement(
      "div",
      { "data-testid": "table" },
      data.map((row) =>
        createElement(
          "div",
          { key: row.uuid, "data-row": row.uuid },
          columns.map((c, i) =>
            typeof c.cell === "function"
              ? createElement(
                  "span",
                  { key: c.id ?? c.accessorKey ?? i },
                  c.cell({ row: { original: row } }),
                )
              : null,
          ),
        ),
      ),
    ),
}));

import { permissions } from "@/config/permissions";
import type { Contract } from "@/features/contracts/services/contract-signing.service";

import { ContractDetailPage } from "./contract-detail-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

function contract(patch: Partial<Contract> = {}): Contract {
  return {
    uuid: "c-1",
    contract_no: 42,
    subject_type: "service",
    subject_id: 0,
    kind: "vehicle_intake",
    locale: "tr",
    template_version: 3,
    otp_required: true,
    signature_required: true,
    status: "executed",
    pdf_ready: true,
    executed_at: "2026-11-01T10:00:00Z",
    signers: [
      {
        uuid: "sg-1",
        role: "customer",
        name: "Ada Customer",
        signed_at: "2026-11-01T09:00:00Z",
        user: { kind: "customer", uuid: "cu-1", name: "Ada Customer" },
      },
      {
        uuid: "sg-2",
        role: "staff",
        name: "Sam Staff",
        signed_at: null,
        user: { kind: "user", uuid: "u-1", name: "Sam Staff" },
      },
    ],
    media: [],
    created_at: "2026-11-01T08:00:00Z",
    updated_at: "2026-11-01T10:00:00Z",
    subject: { kind: "service", uuid: "s-1", name: "SRV-0042" },
    ...patch,
  };
}

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  api.get.mockResolvedValue(contract());
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
  state.grants = new Set();
});

async function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(ContractDetailPage, { slug: "olex", uuid: "c-1" }),
      ),
    );
  });
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
  }
}

const q = (sel: string) => container.querySelector(sel);

describe("ContractDetailPage (TEC-1218)", () => {
  it("links the subject record, the customer and the signer accounts", async () => {
    state.grants = new Set([
      permissions.contracts.read,
      permissions.services.read,
      permissions.customers.read,
      permissions.members.read,
    ]);
    await render();
    expect(api.get).toHaveBeenCalledWith("c-1");
    const subject = q('[data-testid="contract-detail-subject"] a');
    expect(subject?.getAttribute("href")).toBe("/t/olex/services/s-1");
    expect(subject?.textContent).toBe("SRV-0042");
    expect(
      q('[data-testid="contract-detail-customer"] a')?.getAttribute("href"),
    ).toBe("/t/olex/customers/cu-1");
    expect(
      q('[data-row="sg-2"] a[data-entity-kind="user"]')?.getAttribute("href"),
    ).toBe("/t/olex/users/u-1");
    expect(q('[data-row="sg-2"]')?.textContent).toContain(
      "contract_templates.contract_detail.not_signed",
    );
    expect(q('[data-testid="contract-detail-pdf"]')).not.toBeNull();
  });

  it("links a subscription subject by its ref kind", async () => {
    state.grants = new Set([
      permissions.contracts.read,
      permissions.serviceSubscriptions.read,
    ]);
    api.get.mockResolvedValue(
      contract({
        subject_type: "service_subscription",
        subject: { kind: "subscription", uuid: "sub-1", name: "Plan" },
      }),
    );
    await render();
    const subject = q('[data-testid="contract-detail-subject"] a');
    expect(subject?.getAttribute("data-entity-kind")).toBe("subscription");
    expect(subject?.getAttribute("href")).toContain("sub-1");
  });

  it("shows the refs as plain text without their read permission", async () => {
    state.grants = new Set([permissions.contracts.read]);
    await render();
    expect(q('[data-testid="contract-detail-subject"] a')).toBeNull();
    expect(q('[data-testid="contract-detail-subject"]')?.textContent).toContain(
      "SRV-0042",
    );
    expect(q('[data-testid="contract-detail-customer"] a')).toBeNull();
    expect(q('[data-testid="contract-detail-customer"]')?.textContent).toBe(
      "Ada Customer",
    );
  });

  it("does not load the contract without contracts.read", async () => {
    await render();
    expect(api.get).not.toHaveBeenCalled();
    expect(container.textContent).toContain("services.contract.forbidden");
  });
});
