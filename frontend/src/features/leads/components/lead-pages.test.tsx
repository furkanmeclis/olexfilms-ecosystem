// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ComponentType, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  create: vi.fn(),
  patch: vi.fn(),
  setStatus: vi.fn(),
  addNote: vi.fn(),
  assign: vi.fn(),
  events: vi.fn(),
  followUpCount: vi.fn(),
  createTask: vi.fn(),
}));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  orgType: "center",
}));
const router = vi.hoisted(() => ({ push: vi.fn() }));

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
vi.mock("next/navigation", () => ({ useRouter: () => router }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
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
vi.mock("@/lib/auth/session-store", () => ({
  useAuthStore: (selector: (s: unknown) => unknown) =>
    selector({
      user: { organizations: [{ slug: "olex", type: state.orgType }] },
    }),
}));
vi.mock("@/features/leads/services/leads.service", async (orig) => ({
  ...(await orig<object>()),
  leadsService: api,
}));

import { NavBadges } from "@/features/nav-engine/components/nav-badges";
import type { NavAdornment } from "@/features/nav-engine/types";
import { permissions } from "@/config/permissions";
import type { Lead } from "@/features/leads/services/leads.service";

import { LeadDetailPage } from "./lead-detail-page";
import { LeadFormPage } from "./lead-form-page";
import { LeadsListPage } from "./leads-list-page";
import { leadsNavItem } from "../nav";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  api.list.mockResolvedValue({ items: [], total: 0, limit: 20, offset: 0 });
  api.events.mockResolvedValue({ items: [], total: 0 });
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
  state.grants = new Set();
  state.orgType = "center";
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

const q = <T extends Element>(sel: string) =>
  container.querySelector(sel) as T | null;

function BadgeProbe({
  Adornment,
}: {
  Adornment: ComponentType<{
    children: (adornment: NavAdornment) => ReactNode;
  }>;
}) {
  return (
    <Adornment>
      {(adornment) => <NavBadges badges={adornment.badges} />}
    </Adornment>
  );
}

async function choose(sel: string, value: string) {
  const el = q<HTMLSelectElement>(sel)!;
  await act(async () => {
    el.value = value;
    el.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await flush();
}

async function type(sel: string, value: string) {
  const el = q<HTMLInputElement | HTMLTextAreaElement>(sel)!;
  const proto =
    el instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, "value")!.set!;
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

function lead(patch: Partial<Lead> = {}): Lead {
  return {
    uuid: "l-1",
    organization_uuid: "o-1",
    target_type: "dealer_candidate",
    candidate_company_name: "Kuzey Oto",
    candidate_contact_name: "Ayşe",
    candidate_phone_e164: "+905551234567",
    candidate_email: "a@example.com",
    source: "incoming_call",
    temperature: "warm",
    status: "new",
    notes: "",
    created_at: "2026-10-01T10:00:00Z",
    updated_at: "2026-10-01T10:00:00Z",
    ...patch,
  };
}

describe("LeadsListPage", () => {
  it("calls the overdue tab with follow_up=overdue", async () => {
    state.grants = new Set([permissions.leads.read]);
    await render(createElement(LeadsListPage, { slug: "olex" }));

    await click("[data-testid=lead-tab-overdue]");
    expect(api.list).toHaveBeenLastCalledWith({
      follow_up: "overdue",
      limit: 20,
      offset: 0,
    });
  });
});

describe("lead nav badge", () => {
  it("shows the follow-up count", async () => {
    api.followUpCount.mockResolvedValue({ overdue: 2, today: 3 });
    const item = leadsNavItem("olex");
    const Adornment = item.Adornment!;
    await render(createElement(BadgeProbe, { Adornment }));
    expect(container.textContent).toContain("5");
  });
});

describe("LeadFormPage", () => {
  it("changes fields when target type changes", async () => {
    state.grants = new Set([permissions.leads.write]);
    await render(createElement(LeadFormPage, { slug: "olex" }));

    expect(q("[data-testid=lead-customer_user_id]")).not.toBeNull();
    expect(q("[data-testid=lead-candidate_company_name]")).toBeNull();

    await choose("[data-testid=lead-target-type]", "dealer_candidate");
    expect(q("[data-testid=lead-customer_user_id]")).toBeNull();
    expect(q("[data-testid=lead-candidate_company_name]")).not.toBeNull();
  });

  it("validates the customer phone before submitting", async () => {
    state.grants = new Set([permissions.leads.write]);
    await render(createElement(LeadFormPage, { slug: "olex" }));

    await type("[data-testid=lead-customer_user_id]", "42");
    await type("[data-testid=lead-candidate_phone_e164]", "0555 123 45 67");
    await click("[data-testid=lead-submit]");

    expect(api.create).not.toHaveBeenCalled();
    expect(container.textContent).toContain("leads.form.errors.phone_invalid");
  });
});

describe("LeadDetailPage", () => {
  it("requires a reason before marking lost", async () => {
    state.grants = new Set([permissions.leads.read, permissions.leads.write]);
    api.get.mockResolvedValue(lead());
    await render(createElement(LeadDetailPage, { slug: "olex", uuid: "l-1" }));

    await click('[data-status="lost"]');
    await click("[data-testid=lead-lost-confirm]");
    expect(api.setStatus).not.toHaveBeenCalled();
    expect(container.textContent).toContain(
      "leads.form.errors.lost_reason_required",
    );

    await type("[data-testid=lead-lost-reason]", "Bütçe yok");
    await click("[data-testid=lead-lost-confirm]");
    expect(api.setStatus).toHaveBeenCalledWith("l-1", {
      status: "lost",
      lost_reason: "Bütçe yok",
    });
  });

  it("hides task creation outside the center", async () => {
    state.orgType = "dealer";
    state.grants = new Set([permissions.leads.read, permissions.leads.write]);
    api.get.mockResolvedValue(lead());
    await render(createElement(LeadDetailPage, { slug: "olex", uuid: "l-1" }));
    expect(q("[data-testid=lead-task-card]")).toBeNull();
  });
});
