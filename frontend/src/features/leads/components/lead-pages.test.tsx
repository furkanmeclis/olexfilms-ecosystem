// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ComponentType, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  assignees: vi.fn(),
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
const geo = vi.hoisted(() => ({
  countries: vi.fn(),
  provinces: vi.fn(),
  districts: vi.fn(),
}));
vi.mock("@/features/geo/services/geo.service", () => ({ geoService: geo }));
const references = vi.hoisted(() => ({
  customers: vi.fn(),
  customer: vi.fn(),
  vehicles: vi.fn(),
  vehicle: vi.fn(),
}));
vi.mock("@/features/customers/services/customers.service", () => ({
  customersService: { list: references.customers, get: references.customer },
}));
vi.mock("@/features/vehicles/services/vehicles.service", () => ({
  vehiclesService: { list: references.vehicles, get: references.vehicle },
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
import { chooseValue, installRadixPolyfills } from "@/test/form-controls";

import { LeadDetailPage } from "./lead-detail-page";
import { LeadFormPage } from "./lead-form-page";
import { leadsNavItem } from "../nav";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
installRadixPolyfills();

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  geo.countries.mockResolvedValue({
    items: [
      { id: 1, iso2: "TR", name_en: "Turkey", name_tr: "Türkiye" },
      { id: 2, iso2: "DE", name_en: "Germany", name_tr: "Almanya" },
    ],
  });
  geo.provinces.mockImplementation((country: string) =>
    Promise.resolve({
      items:
        country === "TR"
          ? [{ id: 10, name: "Istanbul" }]
          : [{ id: 20, name: "Berlin" }],
    }),
  );
  geo.districts.mockResolvedValue({ items: [{ id: 100, name: "Kadikoy" }] });
  api.assignees.mockResolvedValue({
    items: [
      {
        uuid: "user-1",
        name: "Jane",
        surname: "Staff",
        email: "jane@example.com",
      },
      {
        uuid: "user-2",
        name: "Other",
        surname: "Staff",
        email: "other@example.com",
      },
    ],
  });
  references.customers.mockResolvedValue({
    items: [
      {
        uuid: "customer-1",
        name: "Ada",
        surname: "Customer",
        phone: "+905551234567",
      },
      { uuid: "customer-2", name: "Other", surname: "Customer" },
    ],
  });
  references.customer.mockResolvedValue({
    uuid: "customer-1",
    name: "Ada",
    surname: "Customer",
  });
  references.vehicles.mockResolvedValue({
    items: [
      { uuid: "vehicle-1", plate: "34 ABC 42", car_brand: { name: "Ford" } },
    ],
  });
  references.vehicle.mockResolvedValue({
    uuid: "vehicle-1",
    plate: "34 ABC 42",
    car_brand: { name: "Ford" },
  });
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
  await chooseValue(q(sel), value);
  await flush();
}

async function chooseRemote(sel: string, value: string) {
  await act(async () => {
    (q(sel) as HTMLElement).click();
  });
  await act(async () => {
    await new Promise((r) => setTimeout(r, 350));
  });
  const option = document.querySelector(
    `[role=option][data-value="${value}"]`,
  ) as HTMLElement;
  expect(option).not.toBeNull();
  await act(async () => option.click());
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

    expect(q("[data-testid=lead-customer_uuid]")).not.toBeNull();
    expect(q("[data-testid=lead-candidate_company_name]")).toBeNull();

    await choose("[data-testid=lead-target-type]", "dealer_candidate");
    expect(q("[data-testid=lead-customer_uuid]")).toBeNull();
    expect(q("[data-testid=lead-candidate_company_name]")).not.toBeNull();
  });

  it("selects named geographic resources, clears descendants, and submits numeric IDs", async () => {
    state.grants = new Set([permissions.leads.write]);
    api.create.mockResolvedValue(lead());
    await render(createElement(LeadFormPage, { slug: "olex" }));
    await choose("[data-testid=lead-target-type]", "dealer_candidate");
    expect(q<HTMLButtonElement>("#lead-province_id")!.disabled).toBe(true);
    await choose("#lead-country_id", "1");
    await choose("#lead-province_id", "10");
    await choose("#lead-district_id", "100");
    expect(q("#lead-country_id")!.textContent).toContain("Turkey");
    expect(q("#lead-province_id")!.textContent).toContain("Istanbul");
    expect(geo.provinces).toHaveBeenCalledWith("TR");
    expect(geo.districts).toHaveBeenCalledWith(10);
    await choose("#lead-country_id", "2");
    expect(q("#lead-province_id")!.textContent).not.toContain("Istanbul");
    expect(q("#lead-district_id")!.textContent).not.toContain("Kadikoy");
    expect(q<HTMLButtonElement>("#lead-district_id")!.disabled).toBe(true);
    await choose("#lead-country_id", "1");
    await choose("#lead-province_id", "10");
    await choose("#lead-district_id", "100");
    await type("[data-testid=lead-candidate_company_name]", "Dealer A");
    await type("[data-testid=lead-candidate_contact_name]", "Jane Doe");
    await type("[data-testid=lead-candidate_phone_e164]", "+905551234567");
    await click("[data-testid=lead-submit]");
    expect(api.create).toHaveBeenCalledWith(
      expect.objectContaining({
        country_id: 1,
        province_id: 10,
        district_id: 100,
      }),
    );
  });

  it("submits selected UUIDs and clears the vehicle when changing customer", async () => {
    state.grants = new Set([permissions.leads.write]);
    api.create.mockResolvedValue(lead());
    await render(createElement(LeadFormPage, { slug: "olex" }));
    await choose("#lead-assignee_user_uuid", "user-1");
    await chooseRemote("#lead-customer_uuid", "customer-1");
    await chooseRemote("#lead-vehicle_uuid", "vehicle-1");
    expect(references.vehicles).toHaveBeenLastCalledWith(
      expect.objectContaining({ customer_uuid: "customer-1" }),
    );
    expect(q("#lead-vehicle_uuid")!.textContent).toContain("34 ABC 42");
    await chooseRemote("#lead-customer_uuid", "customer-2");
    expect(q("#lead-vehicle_uuid")!.textContent).not.toContain("34 ABC 42");
    await click("[data-testid=lead-submit]");
    const body = api.create.mock.calls[0][0];
    expect(body).toMatchObject({
      assignee_user_uuid: "user-1",
      customer_uuid: "customer-2",
      vehicle_uuid: null,
    });
    expect(body).not.toHaveProperty("assignee_user_id");
    expect(body).not.toHaveProperty("customer_user_id");
    expect(body).not.toHaveProperty("vehicle_id");
  });

  it("validates the customer phone before submitting", async () => {
    state.grants = new Set([permissions.leads.write]);
    await render(createElement(LeadFormPage, { slug: "olex" }));

    await chooseRemote("[data-testid=lead-customer_uuid]", "customer-1");
    await type("[data-testid=lead-candidate_phone_e164]", "0555 123 45 67");
    await click("[data-testid=lead-submit]");

    expect(api.create).not.toHaveBeenCalled();
    expect(container.textContent).toContain("leads.form.errors.phone_invalid");
  });
});

describe("LeadDetailPage", () => {
  it("restores reference labels, assigns by UUID, and clears assignment", async () => {
    state.grants = new Set([permissions.leads.read, permissions.leads.write]);
    const current = lead({
      target_type: "customer",
      customer_uuid: "customer-1",
      vehicle_uuid: "vehicle-1",
      assignee_user_uuid: "user-1",
    });
    api.get.mockResolvedValue(current);
    api.assign.mockImplementation((_uuid, body) =>
      Promise.resolve({ ...current, ...body }),
    );
    await render(createElement(LeadDetailPage, { slug: "olex", uuid: "l-1" }));
    expect(q("#lead-customer_uuid")!.textContent).toContain("Ada Customer");
    expect(q("#lead-vehicle_uuid")!.textContent).toContain("34 ABC 42");
    expect(q("#lead-assign-input")!.textContent).toContain("Jane Staff");
    expect(api.assignees).toHaveBeenCalledWith("o-1");
    await choose("#lead-assign-input", "user-2");
    expect(api.assign).toHaveBeenCalledWith("l-1", {
      assignee_user_uuid: "user-2",
    });
    const clear = [...container.querySelectorAll("button")].find(
      (b) => b.textContent === "leads.form.unassigned",
    )!;
    await act(async () => clear.click());
    await flush();
    expect(api.assign).toHaveBeenLastCalledWith("l-1", {
      assignee_user_uuid: null,
    });
  });

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

  it("opens the conversion dialog instead of setting won directly", async () => {
    state.grants = new Set([permissions.leads.read, permissions.leads.write]);
    api.get.mockResolvedValue(lead());
    await render(createElement(LeadDetailPage, { slug: "olex", uuid: "l-1" }));

    await click("[data-testid=lead-convert-open]");
    expect(api.setStatus).not.toHaveBeenCalled();
    expect(
      document.querySelector("[data-testid=lead-convert-dialog]"),
    ).not.toBeNull();
  });

  it("disables the conversion for an already won lead", async () => {
    state.grants = new Set([permissions.leads.read, permissions.leads.write]);
    api.get.mockResolvedValue(lead({ status: "won" }));
    await render(createElement(LeadDetailPage, { slug: "olex", uuid: "l-1" }));
    expect(
      q<HTMLButtonElement>("[data-testid=lead-convert-open]")!.disabled,
    ).toBe(true);
  });

  it("hides task creation outside the center", async () => {
    state.orgType = "dealer";
    state.grants = new Set([permissions.leads.read, permissions.leads.write]);
    api.get.mockResolvedValue(lead());
    await render(createElement(LeadDetailPage, { slug: "olex", uuid: "l-1" }));
    expect(q("[data-testid=lead-task-card]")).toBeNull();
  });
});
