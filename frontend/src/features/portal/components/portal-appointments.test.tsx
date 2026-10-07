// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const portal = vi.hoisted(() => ({
  me: vi.fn(),
  listAppointments: vi.fn(),
  createAppointment: vi.fn(),
  cancelAppointment: vi.fn(),
  dealerAvailability: vi.fn(),
  listVehicles: vi.fn(),
}));
const router = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }));
const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
const dealers = vi.hoisted(() => ({
  locateUser: vi.fn(),
  fetchNearbyDealers: vi.fn(),
}));

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
  usePathname: () => "/portal/appointments",
  useRouter: () => router,
}));
vi.mock("sonner", () => ({ toast: toasts }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    dir: "ltr",
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
      date: (v: string) => `date(${v})`,
      time: (v: string) => `time(${v})`,
      dateTime: (v: string) => `dt(${v})`,
      dateParts: (v: string) => `parts(${v})`,
    },
  }),
}));
vi.mock("@/features/portal/lib/portal-client", async (orig) => ({
  ...(await orig<object>()),
  portalApi: portal,
}));
vi.mock("@/features/dealers/lib/dealers", async (orig) => ({
  ...(await orig<object>()),
  locateUser: dealers.locateUser,
  fetchNearbyDealers: dealers.fetchNearbyDealers,
}));

import {
  addDaysIso,
  bookingPreset,
  dayState,
  portalAppointmentErrorKey,
  portalCancelState,
} from "@/features/portal/lib/portal-appointments";
import {
  PortalApiError,
  type AppointmentAvailabilityDay,
  type PortalAppointment,
} from "@/features/portal/lib/portal-client";

import { PortalAppointmentBooking } from "./portal-appointment-booking";
import { PortalAppointments } from "./portal-appointments";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const DEALER = {
  uuid: "11111111-1111-4111-8111-111111111111",
  name: "Olex Kadıköy",
};
const VEHICLE = "22222222-2222-4222-8222-222222222222";
const NOW = new Date("2026-10-07T09:00:00Z");

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  portal.me.mockResolvedValue({ user: {}, roles: ["customer"] });
  portal.listVehicles.mockResolvedValue({
    items: [
      {
        uuid: VEHICLE,
        car_brand: { uuid: "b", name: "BMW" },
        car_model: { uuid: "m", name: "320i" },
        model_year: 2021,
        plate: "34 ABC 123",
        plate_country: "TR",
        vin: null,
        service_count: 1,
        active_warranty_count: 0,
        last_service_at: null,
        created_at: "2026-01-01T00:00:00Z",
      },
    ],
    total: 1,
    limit: 100,
    offset: 0,
  });
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  vi.clearAllMocks();
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

async function click(el: Element | null | undefined) {
  if (!(el instanceof HTMLElement)) throw new Error("element not found");
  await act(async () => {
    el.click();
  });
  await flush();
}

function q(id: string, scope: ParentNode = document.body) {
  return scope.querySelector(`[data-testid="${id}"]`);
}

function appointment(over: Partial<PortalAppointment> = {}): PortalAppointment {
  return {
    uuid: "a-1",
    organization_id: 1,
    customer_user_id: 2,
    vehicle_id: 3,
    starts_at: "2026-10-08T07:00:00Z",
    ends_at: "2026-10-08T08:00:00Z",
    estimated_minutes: 60,
    source: "portal",
    status: "scheduled",
    note: "",
    dealer_uuid: DEALER.uuid,
    dealer_name: DEALER.name,
    vehicle_uuid: VEHICLE,
    vehicle_plate: "34 ABC 123",
    ...over,
  };
}

function day(
  date: string,
  over: Partial<AppointmentAvailabilityDay> = {},
): AppointmentAvailabilityDay {
  return {
    date,
    capacity: 3,
    occupied: 1,
    remaining_capacity: 2,
    closed: false,
    slots: [
      { start: `${date}T06:00:00Z`, end: `${date}T07:00:00Z` },
      { start: `${date}T07:00:00Z`, end: `${date}T08:00:00Z` },
    ],
    ...over,
  };
}

describe("portal appointment helpers", () => {
  it("allows cancelling only two hours or more before the start", () => {
    const at = (h: number) =>
      new Date(NOW.getTime() + h * 3_600_000).toISOString();
    expect(
      portalCancelState({ status: "scheduled", starts_at: at(3) }, NOW).kind,
    ).toBe("allowed");
    expect(
      portalCancelState({ status: "confirmed", starts_at: at(2) }, NOW).kind,
    ).toBe("allowed");
    expect(
      portalCancelState({ status: "scheduled", starts_at: at(1.99) }, NOW).kind,
    ).toBe("window_closed");
    expect(
      portalCancelState({ status: "cancelled", starts_at: at(5) }, NOW).kind,
    ).toBe("not_cancellable");
    expect(
      portalCancelState({ status: "arrived", starts_at: at(5) }, NOW).kind,
    ).toBe("not_cancellable");
  });

  it("classifies days and steps dates across months", () => {
    expect(dayState(day("2026-10-08"))).toBe("bookable");
    expect(dayState(day("2026-10-08", { remaining_capacity: 0 }))).toBe("full");
    expect(dayState(day("2026-10-08", { closed: true }))).toBe("closed");
    expect(dayState(day("2026-10-08", { slots: [] }))).toBe("none");
    expect(addDaysIso("2026-10-31", 1)).toBe("2026-11-01");
    expect(addDaysIso("2026-12-25", 13)).toBe("2027-01-07");
  });

  it("reads the booking preselection and drops malformed ids", () => {
    expect(
      bookingPreset({
        dealer: DEALER.uuid,
        dealer_name: DEALER.name,
        vehicle: VEHICLE,
      }),
    ).toEqual({ dealer: DEALER, vehicle: VEHICLE });
    expect(bookingPreset({ dealer: "x'; drop", vehicle: ["nope"] })).toEqual({
      dealer: null,
      vehicle: null,
    });
  });

  it("maps API error codes to messages", () => {
    expect(
      portalAppointmentErrorKey({ code: "APPOINTMENT_CAPACITY_FULL" }),
    ).toBe("portal.appointments.error.capacity_full");
    expect(
      portalAppointmentErrorKey({ code: "APPOINTMENT_CANCEL_WINDOW_CLOSED" }),
    ).toBe("portal.appointments.error.cancel_window");
    expect(portalAppointmentErrorKey({ status: 404, code: null })).toBe(
      "portal.appointments.error.not_found",
    );
    expect(portalAppointmentErrorKey({ status: 500, code: null })).toBe(
      "portal.appointments.error.generic",
    );
  });
});

describe("PortalAppointments (my appointments)", () => {
  it("lists upcoming appointments with dealer, vehicle and status", async () => {
    portal.listAppointments.mockResolvedValue({
      items: [appointment()],
      total: 1,
    });
    await render(createElement(PortalAppointments, { now: NOW }));
    expect(portal.listAppointments).toHaveBeenCalledWith("upcoming", 20, 0);
    const card = q("portal-appointment");
    expect(card?.textContent).toContain("Olex Kadıköy");
    expect(card?.textContent).toContain("34 ABC 123");
    expect(card?.textContent).toContain("time(2026-10-08T07:00:00Z)");
    expect(card?.textContent).toContain("portal.appointments.status.scheduled");
    expect(q("portal-appointment-book")?.getAttribute("href")).toBe(
      "/portal/appointments/new",
    );
  });

  it("disables cancel with an explanation when less than 2 hours are left", async () => {
    portal.listAppointments.mockResolvedValue({
      items: [
        appointment({ uuid: "soon", starts_at: "2026-10-07T10:30:00Z" }),
        appointment({ uuid: "later", starts_at: "2026-10-07T12:00:00Z" }),
      ],
      total: 2,
    });
    await render(createElement(PortalAppointments, { now: NOW }));
    const soon = container.querySelector('[data-uuid="soon"]')!;
    const later = container.querySelector('[data-uuid="later"]')!;
    const soonBtn = q("portal-appointment-cancel", soon) as HTMLButtonElement;
    const laterBtn = q("portal-appointment-cancel", later) as HTMLButtonElement;
    expect(soonBtn.disabled).toBe(true);
    expect(q("portal-appointment-cancel-hint", soon)?.textContent).toBe(
      "portal.appointments.cancel_window_hint",
    );
    expect(soonBtn.getAttribute("aria-describedby")).toBe("cancel-hint-soon");
    expect(laterBtn.disabled).toBe(false);
    expect(q("portal-appointment-cancel-hint", later)).toBeNull();
  });

  it("cancels after confirmation and reloads the list", async () => {
    portal.listAppointments.mockResolvedValue({
      items: [appointment({ starts_at: "2026-10-07T12:00:00Z" })],
      total: 1,
    });
    portal.cancelAppointment.mockResolvedValue(
      appointment({ status: "cancelled" }),
    );
    await render(createElement(PortalAppointments, { now: NOW }));
    await click(q("portal-appointment-cancel"));
    expect(q("portal-appointment-cancel-dialog")).not.toBeNull();
    expect(portal.cancelAppointment).not.toHaveBeenCalled();
    await click(q("portal-appointment-cancel-confirm"));
    expect(portal.cancelAppointment).toHaveBeenCalledWith("a-1");
    expect(toasts.success).toHaveBeenCalledWith(
      "portal.appointments.cancelled",
    );
    expect(portal.listAppointments).toHaveBeenCalledTimes(2);
  });

  it("hides booking and cancelling from a fleet session", async () => {
    portal.me.mockResolvedValue({ user: {}, roles: ["fleet"] });
    portal.listAppointments.mockResolvedValue({
      items: [appointment({ starts_at: "2026-10-07T12:00:00Z" })],
      total: 1,
    });
    await render(createElement(PortalAppointments, { now: NOW }));
    expect(q("portal-appointment")).not.toBeNull();
    expect(q("portal-appointment-book")).toBeNull();
    expect(q("portal-appointment-cancel")).toBeNull();
    expect(q("portal-read-only")?.textContent).toBe("portal.read_only.notice");
  });
});

describe("PortalAppointmentBooking", () => {
  async function renderBooking(
    props: Parameters<typeof PortalAppointmentBooking>[0] = {
      initialDealer: DEALER,
      initialVehicle: VEHICLE,
    },
  ) {
    await render(
      createElement(PortalAppointmentBooking, {
        today: new Date(2026, 9, 7, 12),
        ...props,
      }),
    );
  }

  function dayButton(date: string) {
    return document.querySelector(
      `[data-testid="booking-day"][data-date="${date}"]`,
    ) as HTMLButtonElement | null;
  }

  it("asks the dealer's availability for two weeks from today", async () => {
    portal.dealerAvailability.mockResolvedValue([day("2026-10-08")]);
    await renderBooking();
    expect(portal.dealerAvailability).toHaveBeenCalledWith(
      DEALER.uuid,
      "2026-10-07",
      "2026-10-20",
    );
    expect(q("booking-dealer-selected")?.textContent).toContain(DEALER.name);
    // The only vehicle is chosen without asking.
    expect(q("booking-vehicle")?.getAttribute("aria-checked")).toBe("true");
  });

  it("does not offer slots on a full day", async () => {
    portal.dealerAvailability.mockResolvedValue([
      day("2026-10-08", { occupied: 3, remaining_capacity: 0 }),
      day("2026-10-09"),
    ]);
    await renderBooking();
    const full = dayButton("2026-10-08")!;
    expect(full.disabled).toBe(true);
    expect(full.getAttribute("data-state")).toBe("full");
    expect(full.textContent).toContain("portal.appointments.day_full");
    await click(full);
    expect(full.getAttribute("aria-pressed")).toBe("false");
    expect(q("booking-slots")).toBeNull();
    expect(q("booking-slot")).toBeNull();
    expect((q("booking-submit") as HTMLButtonElement).disabled).toBe(true);

    await click(dayButton("2026-10-09"));
    expect(
      document.querySelectorAll('[data-testid="booking-slot"]'),
    ).toHaveLength(2);
  });

  it("books the chosen slot and goes to my appointments", async () => {
    portal.dealerAvailability.mockResolvedValue([day("2026-10-08")]);
    portal.createAppointment.mockResolvedValue(appointment());
    await renderBooking();
    await click(dayButton("2026-10-08"));
    await click(
      document.querySelector(
        '[data-testid="booking-slot"][data-start="2026-10-08T07:00:00Z"]',
      ),
    );
    const submit = q("booking-submit") as HTMLButtonElement;
    expect(submit.disabled).toBe(false);
    await click(submit);
    expect(portal.createAppointment).toHaveBeenCalledWith({
      dealer_uuid: DEALER.uuid,
      vehicle_uuid: VEHICLE,
      starts_at: "2026-10-08T07:00:00Z",
    });
    expect(toasts.success).toHaveBeenCalledWith("portal.appointments.booked");
    expect(router.push).toHaveBeenCalledWith("/portal/appointments");
  });

  it("stays on the page with a message when the day filled meanwhile", async () => {
    portal.dealerAvailability.mockResolvedValue([day("2026-10-08")]);
    portal.createAppointment.mockRejectedValue(
      new PortalApiError("full", 422, "APPOINTMENT_CAPACITY_FULL", []),
    );
    await renderBooking();
    await click(dayButton("2026-10-08"));
    await click(q("booking-slot"));
    await click(q("booking-submit"));
    expect(router.push).not.toHaveBeenCalled();
    expect(q("booking-error")?.textContent).toBe(
      "portal.appointments.error.capacity_full",
    );
    expect(portal.dealerAvailability).toHaveBeenCalledTimes(2);
  });

  it("lists only nearby dealers that take portal bookings", async () => {
    dealers.locateUser.mockResolvedValue({
      kind: "located",
      center: { lat: 41, lng: 29 },
    });
    dealers.fetchNearbyDealers.mockResolvedValue({
      kind: "ok",
      dealers: [
        {
          uuid: DEALER.uuid,
          slug: "kadikoy",
          name: DEALER.name,
          city: "İstanbul",
          district: "Kadıköy",
          latitude: 41,
          longitude: 29,
          distance_km: 2,
          accepts_appointments: true,
          whatsapp: null,
        },
        {
          uuid: "33333333-3333-4333-8333-333333333333",
          slug: "cankaya",
          name: "Olex Çankaya",
          city: "Ankara",
          district: "Çankaya",
          latitude: 39,
          longitude: 32,
          distance_km: 90,
          accepts_appointments: false,
          whatsapp: null,
        },
      ],
    });
    portal.dealerAvailability.mockResolvedValue([day("2026-10-08")]);
    await renderBooking({ initialVehicle: VEHICLE });
    const rows = document.querySelectorAll('[data-testid="booking-dealer"]');
    expect(rows).toHaveLength(1);
    expect(rows[0].textContent).toContain(DEALER.name);
    expect(portal.dealerAvailability).not.toHaveBeenCalled();
    await click(rows[0].querySelector("button"));
    expect(q("booking-dealer-selected")?.textContent).toContain(DEALER.name);
    expect(portal.dealerAvailability).toHaveBeenCalledWith(
      DEALER.uuid,
      "2026-10-07",
      "2026-10-20",
    );
  });

  it("does not let a fleet session book", async () => {
    portal.me.mockResolvedValue({ user: {}, roles: ["fleet"] });
    portal.dealerAvailability.mockResolvedValue([day("2026-10-08")]);
    await renderBooking();
    expect(q("portal-read-only")).not.toBeNull();
    expect(q("booking-submit")).toBeNull();
    expect(q("booking-days")).toBeNull();
  });
});
