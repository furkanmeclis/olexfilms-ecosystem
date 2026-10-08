// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  card: vi.fn(),
  listVehicles: vi.fn(),
  previewPlan: vi.fn(),
  createPlan: vi.fn(),
}));
const push = vi.hoisted(() => vi.fn());
const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  info: vi.fn(),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      timeZone: "UTC",
      date: (v: string) => v,
      time: (v: string) => v.slice(11, 16),
      number: (v: number) => String(v),
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/providers/toast-provider", () => ({ appToast: toast }));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, replace: vi.fn() }),
}));
vi.mock("@/components/layout/page-header", () => ({
  PageHeader: ({ title }: { title: string }) =>
    createElement("h1", null, title as ReactNode),
}));
vi.mock("@/components/ui/date-picker", () => ({
  DatePicker: ({
    id,
    value,
    onChange,
  }: {
    id: string;
    value: string;
    onChange: (v: string) => void;
  }) =>
    createElement("input", {
      id,
      value,
      onChange: (e: { target: { value: string } }) => onChange(e.target.value),
    }),
}));
vi.mock("@/features/fleets/services/fleets.service", async (orig) => ({
  ...(await orig<object>()),
  fleetsService: api,
}));

import { ApiError } from "@/lib/api/errors";

import { FleetPlanWizard } from "./fleet-plan-wizard";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const V = ["v-1", "v-2", "v-3"];
const vehicle = (uuid: string, i: number) => ({
  uuid,
  plate: `34 FLT ${i + 1}`,
  plate_country: "TR",
  vin: null,
  model_year: null,
  car_brand: null,
  car_model: null,
  last_service_at: null,
  active_warranty_count: 0,
  created_at: "2026-10-01T00:00:00Z",
});
// The server put all three on one day (an edited preview could do the
// same); the first one also has another appointment that day.
const PREVIEW = {
  fleet_uuid: "fleet-1",
  dealer_uuid: "dealer-1",
  service_type: "PPF",
  note: "",
  appointments: V.map((uuid, i) => ({
    vehicle_uuid: uuid,
    starts_at: `2026-10-12T0${7 + i}:00:00.000Z`,
  })),
  warnings: [
    {
      vehicle_uuid: "v-1",
      date: "2026-10-12",
      code: "FLEET_PLAN_VEHICLE_DAY_CONFLICT",
      message: "vehicle has another active appointment on this day",
    },
  ],
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  for (const fn of [...Object.values(api), ...Object.values(toast), push]) {
    fn.mockReset();
  }
  api.card.mockResolvedValue({ uuid: "fleet-1", name: "Acme Filo" });
  api.listVehicles.mockResolvedValue({
    items: V.map(vehicle),
    total: 3,
    limit: 100,
    offset: 0,
  });
  api.previewPlan.mockResolvedValue(PREVIEW);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

async function flush() {
  for (let i = 0; i < 4; i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
  }
}

async function render() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  act(() =>
    root.render(
      createElement(
        QueryClientProvider,
        { client: qc },
        createElement(FleetPlanWizard, {
          slug: "acme",
          fleetUuid: "fleet-1",
          initialVehicleUuids: V,
        }),
      ),
    ),
  );
  await flush();
}

const $ = (id: string) =>
  container.querySelector<HTMLElement>(`[data-testid="${id}"]`);
const byId = (id: string) =>
  container.querySelector<HTMLInputElement>(`[id="${id}"]`)!;
const $$ = (id: string) =>
  Array.from(container.querySelectorAll<HTMLElement>(`[data-testid="${id}"]`));

function type(el: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )!.set!;
  act(() => {
    setter.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function click(el: HTMLElement | null) {
  act(() => el!.click());
  await flush();
}

async function previewWithDailyMax(max: string) {
  await render();
  expect($("plan-step-settings")).not.toBeNull();
  type($("plan-service-type") as HTMLInputElement, "PPF");
  type(byId("plan-start-date"), "2026-10-12");
  type($("plan-daily-max") as HTMLInputElement, max);
  type($("plan-times") as HTMLInputElement, "09:00, 14:00");
  await click($("plan-preview"));
}

describe("fleet plan wizard (TEC-477)", () => {
  it("shows capacity and conflict warnings in the preview", async () => {
    await previewWithDailyMax("2");

    expect(api.previewPlan).toHaveBeenCalledWith("fleet-1", {
      vehicle_uuids: V,
      service_type: "PPF",
      start_date: "2026-10-12",
      daily_max_vehicles: 2,
      preferred_times: ["09:00", "14:00"],
    });
    expect($("plan-step-preview")).not.toBeNull();
    expect($("plan-warnings")).not.toBeNull();
    // Three vehicles on a day with a limit of two: every row is flagged.
    expect($$("plan-row-warning-capacity")).toHaveLength(3);
    expect($$("plan-row-warning-capacity")[0].textContent).toContain(
      '"count":3,"limit":2',
    );
    expect($$("plan-row-warning-server")).toHaveLength(1);
    expect($$("plan-row-warning-server")[0].textContent).toBe(
      "fleets.plan.warning_codes.FLEET_PLAN_VEHICLE_DAY_CONFLICT",
    );
  });

  it("moves a row to another day and sends the edited appointments", async () => {
    api.createPlan.mockResolvedValue({ uuid: "plan-1" });
    await previewWithDailyMax("2");

    type(byId("plan-row-date-v-3"), "2026-10-13");
    expect($$("plan-row-warning-capacity")).toHaveLength(0);
    expect($$("plan-day")).toHaveLength(2);

    await click($("plan-confirm"));
    const [, body, key] = api.createPlan.mock.calls[0];
    expect(body.appointments).toContainEqual({
      vehicle_uuid: "v-3",
      starts_at: "2026-10-13T09:00:00.000Z",
    });
    expect(typeof key).toBe("string");
    expect(push).toHaveBeenCalledWith("/t/acme/fleets/fleet-1/plans/plan-1");
  });

  it("offers to preview again after 409 FLEET_SERVICE_PLAN_STALE", async () => {
    api.createPlan.mockRejectedValue(
      new ApiError({
        status: 409,
        code: "FLEET_SERVICE_PLAN_STALE",
        message: "fleet service plan capacity changed; preview again",
      }),
    );
    await previewWithDailyMax("3");
    expect($("plan-stale")).toBeNull();

    await click($("plan-confirm"));
    expect(api.createPlan).toHaveBeenCalledTimes(1);
    expect($("plan-stale")).not.toBeNull();
    expect(($("plan-confirm") as HTMLButtonElement).disabled).toBe(true);
    expect(push).not.toHaveBeenCalled();

    await click($("plan-repreview"));
    expect(api.previewPlan).toHaveBeenCalledTimes(2);
    expect($("plan-stale")).toBeNull();
    expect(($("plan-confirm") as HTMLButtonElement).disabled).toBe(false);
  });

  it("keeps the settings step on invalid preferred hours", async () => {
    await render();
    type($("plan-service-type") as HTMLInputElement, "PPF");
    type($("plan-times") as HTMLInputElement, "9am");
    await click($("plan-preview"));
    expect(api.previewPlan).not.toHaveBeenCalled();
    expect($("plan-step-settings")!.textContent).toContain(
      "fleets.plan.times_invalid",
    );
  });
});
