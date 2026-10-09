// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const http = vi.hoisted(() => ({ platformRequest: vi.fn() }));
const captured = vi.hoisted(() => ({
  push: vi.fn(),
  onDragEnd: null as null | ((e: unknown) => void),
  toastError: vi.fn(),
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
  useRouter: () => ({ push: captured.push }),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    dir: "ltr",
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      timeZone: "UTC",
      date: (v: string) => `date(${v})`,
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: captured.toastError },
}));
vi.mock("@/lib/api/platform-request", () => http);
// Drag and drop: the test drives DndContext's onDragEnd directly.
vi.mock("@dnd-kit/core", () => ({
  DndContext: ({
    children,
    onDragEnd,
  }: {
    children: ReactNode;
    onDragEnd: (e: unknown) => void;
  }) => {
    captured.onDragEnd = onDragEnd;
    return createElement("div", null, children);
  },
  PointerSensor: class {},
  KeyboardSensor: class {},
  useSensor: () => null,
  useSensors: () => [],
  useDraggable: () => ({
    attributes: {},
    listeners: {},
    setNodeRef: () => undefined,
    transform: null,
    isDragging: false,
  }),
  useDroppable: () => ({ setNodeRef: () => undefined, isOver: false }),
}));

import type {
  Appointment,
  AppointmentAvailabilityDay,
} from "@/features/appointments/services/appointments.service";
import { ApiError } from "@/lib/api/errors";
import { installRadixPolyfills, pickTime } from "@/test/form-controls";

import { AppointmentCalendar } from "./appointment-calendar";
import { AppointmentDialog } from "./appointment-dialog";
import { AppointmentSettingsPanel } from "./appointment-settings-panel";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
// Radix Switch measures itself; the time pickers are Radix popovers.
installRadixPolyfills();

const appt = (over: Partial<Appointment>): Appointment => ({
  uuid: "a-1",
  organization_id: 1,
  customer_user_id: 7,
  customer_uuid: "cust-1",
  customer_name: "Ayşe Yılmaz",
  vehicle_id: 9,
  vehicle_uuid: "veh-1",
  vehicle_plate: "34 ABC 12",
  starts_at: "2026-10-05T07:00:00Z",
  ends_at: "2026-10-05T08:00:00Z",
  estimated_minutes: 60,
  source: "panel",
  status: "scheduled",
  note: "keys at desk",
  ...over,
});

const day = (date: string, timezone: string): AppointmentAvailabilityDay => ({
  date,
  capacity: 3,
  occupied: 1,
  remaining_capacity: 2,
  closed: false,
  slots: [],
  timezone,
});

const WEEK = [
  "2026-10-05",
  "2026-10-06",
  "2026-10-07",
  "2026-10-08",
  "2026-10-09",
  "2026-10-10",
  "2026-10-11",
];

let container: HTMLDivElement;
let root: Root;

async function render(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  await flush();
}

async function flush() {
  for (let i = 0; i < 3; i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
  }
}

const cardDay = (uuid: string) =>
  document
    .querySelector(`[data-testid="appointment-${uuid}"]`)
    ?.closest("[data-day]")
    ?.getAttribute("data-day");

const cardTop = (uuid: string) =>
  (document.querySelector(`[data-testid="appointment-${uuid}"]`) as HTMLElement)
    ?.style.top;

function button(selector: string) {
  return document.querySelector(selector) as HTMLButtonElement | null;
}

/** Calendar API mock: list, availability, and a configurable PATCH. */
function mockCalendar(items: Appointment[], timezone: string) {
  http.platformRequest.mockImplementation(
    async (method: string, path: string) => {
      if (method === "GET" && path === "/v1/appointments/availability") {
        return WEEK.map((d) => day(d, timezone));
      }
      if (method === "GET" && path === "/v1/appointments") {
        return { items, total: items.length, limit: 100, offset: 0 };
      }
      throw new Error(`unexpected ${method} ${path}`);
    },
  );
}

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  captured.push.mockReset();
  captured.toastError.mockReset();
  captured.onDragEnd = null;
  http.platformRequest.mockReset();
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

describe("AppointmentCalendar week view (TEC-326)", () => {
  // 23:30 UTC on Monday is 01:30 on Tuesday in Berlin (CEST, UTC+2).
  const late = appt({
    uuid: "late",
    starts_at: "2026-10-05T23:30:00Z",
    ends_at: "2026-10-06T00:30:00Z",
  });

  it("places a booking on its Europe/Berlin day", async () => {
    mockCalendar([late], "Europe/Berlin");
    await render(
      createElement(AppointmentCalendar, {
        slug: "acme",
        canWrite: true,
        initialDate: "2026-10-07",
      }),
    );
    expect(cardDay("late")).toBe("2026-10-06");
    // The list asks the Berlin week: Monday 00:00 CEST = Sunday 22:00 UTC.
    expect(http.platformRequest).toHaveBeenCalledWith(
      "GET",
      "/v1/appointments",
      {
        query: {
          from: "2026-10-04T22:00:00Z",
          to: "2026-10-11T22:00:00Z",
          limit: 100,
          offset: 0,
        },
      },
    );
  });

  it("places the same booking on its UTC day", async () => {
    mockCalendar([late], "UTC");
    await render(
      createElement(AppointmentCalendar, {
        slug: "acme",
        canWrite: true,
        initialDate: "2026-10-07",
      }),
    );
    expect(cardDay("late")).toBe("2026-10-05");
  });
});

describe("AppointmentCalendar drag and drop (TEC-326)", () => {
  const a = appt({});

  async function renderAndDrop(patch: () => Promise<Appointment>) {
    let resolved: (() => void) | null = null;
    // After the drop, list refetches hang, so only the optimistic update
    // and its rollback can move the card.
    let frozen = false;
    http.platformRequest.mockImplementation(
      async (method: string, path: string, opts?: unknown) => {
        if (method === "GET" && path === "/v1/appointments/availability") {
          return WEEK.map((d) => day(d, "Europe/Berlin"));
        }
        if (method === "GET" && path === "/v1/appointments") {
          if (frozen) return new Promise(() => undefined);
          return { items: [a], total: 1, limit: 100, offset: 0 };
        }
        if (method === "PATCH") {
          void opts;
          await new Promise<void>((r) => (resolved = r));
          return patch();
        }
        throw new Error(`unexpected ${method} ${path}`);
      },
    );
    await render(
      createElement(AppointmentCalendar, {
        slug: "acme",
        canWrite: true,
        initialDate: "2026-10-05",
      }),
    );
    // 07:00Z Monday = 09:00 Berlin.
    expect(cardDay("a-1")).toBe("2026-10-05");
    const before = cardTop("a-1");
    frozen = true;
    await act(async () => {
      captured.onDragEnd?.({
        active: { id: "a-1" },
        over: { id: "slot|2026-10-06|630" },
      });
    });
    await flush();
    return {
      before,
      finish: async () => {
        await act(async () => {
          (resolved as (() => void) | null)?.();
        });
        await flush();
      },
    };
  }

  it("moves the card and sends the full PATCH body", async () => {
    const moved = {
      ...a,
      starts_at: "2026-10-06T08:30:00Z",
      ends_at: "2026-10-06T09:30:00Z",
    };
    const { finish } = await renderAndDrop(async () => moved);
    // Optimistic: already on Tuesday while the request runs.
    expect(cardDay("a-1")).toBe("2026-10-06");
    expect(http.platformRequest).toHaveBeenCalledWith(
      "PATCH",
      "/v1/appointments/a-1",
      {
        body: {
          customer_user_id: 7,
          vehicle_id: 9,
          // Tuesday 10:30 in Berlin (UTC+2).
          starts_at: "2026-10-06T08:30:00Z",
          estimated_minutes: 60,
          source: "panel",
          note: "keys at desk",
        },
      },
    );
    await finish();
    expect(cardDay("a-1")).toBe("2026-10-06");
    expect(captured.toastError).not.toHaveBeenCalled();
  });

  it("puts the card back and shows a toast on 422 capacity full", async () => {
    const { before, finish } = await renderAndDrop(async () => {
      throw new ApiError({
        status: 422,
        code: "APPOINTMENT_CAPACITY_FULL",
        message: "Appointment capacity is full",
      });
    });
    expect(cardDay("a-1")).toBe("2026-10-06");
    await finish();
    expect(cardDay("a-1")).toBe("2026-10-05");
    expect(cardTop("a-1")).toBe(before);
    expect(captured.toastError).toHaveBeenCalledWith(
      "appointments.errors.capacity_full",
    );
  });

  it("does not move arrived appointments", async () => {
    mockCalendar([appt({ status: "arrived" })], "UTC");
    await render(
      createElement(AppointmentCalendar, {
        slug: "acme",
        canWrite: true,
        initialDate: "2026-10-05",
      }),
    );
    await act(async () => {
      captured.onDragEnd?.({
        active: { id: "a-1" },
        over: { id: "slot|2026-10-06|600" },
      });
    });
    expect(http.platformRequest).not.toHaveBeenCalledWith(
      "PATCH",
      expect.anything(),
      expect.anything(),
    );
  });
});

describe("AppointmentDialog status and intake (TEC-326)", () => {
  it("enables vehicle intake after 'arrived' and opens the service wizard", async () => {
    const confirmed = appt({ status: "confirmed" });
    http.platformRequest.mockImplementation(
      async (method: string, path: string, opts?: { body?: unknown }) => {
        if (method === "GET" && path === "/v1/vehicles") {
          return { items: [], total: 0, limit: 50, offset: 0 };
        }
        if (method === "POST" && path === "/v1/appointments/a-1/status") {
          expect(opts?.body).toEqual({ status: "arrived" });
          return { ...confirmed, status: "arrived" };
        }
        if (method === "POST" && path === "/v1/appointments/a-1/start-intake") {
          return {
            ...confirmed,
            status: "arrived",
            service_id: 41,
            service_uuid: "svc-41",
          };
        }
        throw new Error(`unexpected ${method} ${path}`);
      },
    );
    await render(
      createElement(AppointmentDialog, {
        slug: "acme",
        timeZone: "Europe/Berlin",
        canWrite: true,
        appointment: confirmed,
        onClose: () => undefined,
      }),
    );
    const intake = () => button('[data-action="start-intake"]');
    expect(intake()).not.toBeNull();
    expect(intake()?.disabled).toBe(true);

    await act(async () => {
      button('[data-action="arrived"]')?.click();
    });
    await flush();
    expect(
      document.querySelector('[data-testid="dialog-status"]')?.textContent,
    ).toBe("appointments.status.arrived");
    expect(intake()?.disabled).toBe(false);

    await act(async () => {
      intake()?.click();
    });
    await flush();
    expect(http.platformRequest).toHaveBeenCalledWith(
      "POST",
      "/v1/appointments/a-1/start-intake",
    );
    expect(captured.push).toHaveBeenCalledWith(
      "/t/acme/services/svc-41/wizard",
    );
  });

  it("cancels only with a reason", async () => {
    const scheduled = appt({ status: "scheduled" });
    http.platformRequest.mockImplementation(
      async (method: string, path: string, opts?: { body?: unknown }) => {
        if (method === "GET" && path === "/v1/vehicles") {
          return { items: [], total: 0, limit: 50, offset: 0 };
        }
        if (path === "/v1/appointments/a-1/status") {
          expect(opts?.body).toEqual({
            status: "cancelled",
            cancel_reason: "Customer called",
          });
          return { ...scheduled, status: "cancelled" };
        }
        throw new Error(`unexpected ${method} ${path}`);
      },
    );
    await render(
      createElement(AppointmentDialog, {
        slug: "acme",
        timeZone: "UTC",
        canWrite: true,
        appointment: scheduled,
        onClose: () => undefined,
      }),
    );
    await act(async () => {
      button('[data-action="cancelled"]')?.click();
    });
    expect(button('[data-action="confirm-cancel"]')?.disabled).toBe(true);
    const reason = document.querySelector(
      "textarea[id$='-reason']",
    ) as HTMLTextAreaElement;
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(
        HTMLTextAreaElement.prototype,
        "value",
      )?.set;
      setter?.call(reason, "Customer called");
      reason.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      button('[data-action="confirm-cancel"]')?.click();
    });
    await flush();
    expect(
      document.querySelector('[data-testid="dialog-status"]')?.textContent,
    ).toBe("appointments.status.cancelled");
  });
});

describe("AppointmentSettingsPanel (TEC-326)", () => {
  function mockSettings(workingHours: Record<string, unknown>) {
    http.platformRequest.mockImplementation(
      async (method: string, path: string) => {
        if (method === "GET" && path === "/v1/appointment-settings") {
          return {
            uuid: "set-1",
            organization_id: 1,
            daily_vehicle_capacity: 5,
            default_estimated_minutes: 60,
            slot_interval_minutes: 30,
            working_hours: workingHours,
            portal_appointments_enabled: false,
          };
        }
        if (method === "GET" && path === "/v1/appointment-closures") return [];
        if (method === "PUT") return {};
        throw new Error(`unexpected ${method} ${path}`);
      },
    );
  }

  const input = (name: string) =>
    document.querySelector(`[name="${name}"]`) as HTMLButtonElement;

  async function submit() {
    const form = document.querySelector(
      '[data-testid="appointment-settings"] form',
    ) as HTMLFormElement;
    await act(async () => {
      form.requestSubmit();
    });
    await flush();
  }

  const putBody = (workingHours: Record<string, unknown>) => ({
    body: {
      daily_vehicle_capacity: 5,
      default_estimated_minutes: 60,
      slot_interval_minutes: 30,
      working_hours: workingHours,
      portal_appointments_enabled: false,
    },
  });

  const notSaved = () =>
    expect(http.platformRequest).not.toHaveBeenCalledWith(
      "PUT",
      expect.anything(),
      expect.anything(),
    );

  it("rejects a closing time before the opening time", async () => {
    mockSettings({ monday: [{ start: "09:00", end: "18:00" }] });
    await render(createElement(AppointmentSettingsPanel, { timeZone: "UTC" }));
    const end = input("monday-0-end");
    expect(end.value).toBe("18:00");
    await pickTime(end, "08:00");
    await submit();
    expect(document.querySelector('[data-error="monday-0"]')?.textContent).toBe(
      "appointments.settings.close_before_open",
    );
    notSaved();

    await pickTime(end, "17:00");
    await submit();
    expect(document.querySelector('[data-error="monday-0"]')).toBeNull();
    expect(http.platformRequest).toHaveBeenCalledWith(
      "PUT",
      "/v1/appointment-settings",
      putBody({ monday: [{ start: "09:00", end: "17:00" }] }),
    );
  });

  it("renders every saved window of a day and saves them back intact", async () => {
    const saved = {
      monday: [
        { start: "09:00", end: "12:00" },
        { start: "13:00", end: "18:00" },
      ],
      friday: [{ start: "10:00", end: "16:00" }],
    };
    mockSettings(saved);
    await render(createElement(AppointmentSettingsPanel, { timeZone: "UTC" }));
    expect(
      document.querySelectorAll('[data-weekday="monday"] [data-window]'),
    ).toHaveLength(2);
    expect(input("monday-0-start").value).toBe("09:00");
    expect(input("monday-0-end").value).toBe("12:00");
    expect(input("monday-1-start").value).toBe("13:00");
    expect(input("monday-1-end").value).toBe("18:00");
    await submit();
    expect(http.platformRequest).toHaveBeenCalledWith(
      "PUT",
      "/v1/appointment-settings",
      putBody(saved),
    );
  });

  it("adds and removes windows of a day", async () => {
    mockSettings({ tuesday: [{ start: "09:00", end: "12:00" }] });
    await render(createElement(AppointmentSettingsPanel, { timeZone: "UTC" }));
    expect(button('[data-action="remove-window-tuesday-0"]')).toBeNull();
    await act(async () => {
      button('[data-action="add-window-tuesday"]')?.click();
    });
    expect(input("tuesday-1-start").value).toBe("12:00");
    await pickTime(input("tuesday-1-start"), "14:00");
    await pickTime(input("tuesday-1-end"), "18:00");
    await act(async () => {
      button('[data-action="add-window-tuesday"]')?.click();
    });
    await act(async () => {
      button('[data-action="remove-window-tuesday-2"]')?.click();
    });
    await submit();
    expect(http.platformRequest).toHaveBeenCalledWith(
      "PUT",
      "/v1/appointment-settings",
      putBody({
        tuesday: [
          { start: "09:00", end: "12:00" },
          { start: "14:00", end: "18:00" },
        ],
      }),
    );
  });

  it("rejects overlapping windows of the same day", async () => {
    mockSettings({
      monday: [
        { start: "09:00", end: "12:00" },
        { start: "13:00", end: "18:00" },
      ],
    });
    await render(createElement(AppointmentSettingsPanel, { timeZone: "UTC" }));
    await pickTime(input("monday-1-start"), "11:00");
    await submit();
    expect(document.querySelector('[data-error="monday"]')?.textContent).toBe(
      "appointments.settings.windows_overlap",
    );
    expect(input("monday-1-start").getAttribute("aria-invalid")).toBe("true");
    notSaved();
  });
});
