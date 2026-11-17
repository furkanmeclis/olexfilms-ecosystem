// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({ get: vi.fn() }));
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
    t: (key: string) => key,
    format: { dateTime: (v: string) => `dt(${v})` },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock(
  "@/features/appointments/services/appointments.service",
  async (orig) => ({
    ...(await orig<object>()),
    appointmentsService: api,
  }),
);

import { permissions } from "@/config/permissions";
import type { Appointment } from "@/features/appointments/services/appointments.service";

import { AppointmentDetailPage } from "./appointment-detail-page";

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
        createElement(AppointmentDetailPage, { slug: "olex", uuid: "ap-1" }),
      ),
    );
  });
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

const href = (text: string) =>
  [...container.querySelectorAll("a")]
    .find((a) => a.textContent === text)
    ?.getAttribute("href");

function appointment(): Appointment {
  return {
    uuid: "ap-1",
    organization_id: 1,
    customer_user_id: 2,
    starts_at: "2026-11-20T09:00:00Z",
    ends_at: "2026-11-20T10:00:00Z",
    estimated_minutes: 60,
    source: "panel",
    status: "cancelled",
    cancel_reason: "Customer called",
    note: "Bring the key",
    organization: { kind: "organization", uuid: "org-1", name: "Olex Center" },
    customer: { kind: "customer", uuid: "cu-1", name: "Ada Customer" },
    vehicle: { kind: "vehicle", uuid: "ve-1", name: "34 ABC 42" },
    service: { kind: "service", uuid: "se-1", name: "S-100" },
    lead: { kind: "lead", uuid: "le-1", name: "Lead Ada" },
    created_by: { kind: "user", uuid: "us-1", name: "Jane Staff" },
  };
}

describe("AppointmentDetailPage (TEC-1218)", () => {
  it("links every ref the user may open and keeps the rest as text", async () => {
    state.grants = new Set([
      permissions.appointments.read,
      permissions.customers.read,
      permissions.services.read,
      permissions.leads.read,
      permissions.members.read,
    ]);
    api.get.mockResolvedValue(appointment());
    await render();

    expect(api.get).toHaveBeenCalledWith("ap-1");
    expect(href("Ada Customer")).toBe("/t/olex/customers/cu-1");
    expect(href("S-100")).toBe("/t/olex/services/se-1");
    expect(href("Lead Ada")).toBe("/t/olex/leads/le-1");
    expect(href("Jane Staff")).toBe("/t/olex/users/us-1");
    // No vehicles.read / organizations read: plain text.
    expect(href("34 ABC 42")).toBeUndefined();
    expect(href("Olex Center")).toBeUndefined();
    expect(container.textContent).toContain("34 ABC 42");
    expect(container.textContent).toContain("Olex Center");
    expect(
      container.querySelector('[data-testid="appointment-status"]')
        ?.textContent,
    ).toBe("appointments.status.cancelled");
    expect(container.textContent).toContain("Customer called");
    expect(container.textContent).toContain("Bring the key");
    expect(container.textContent).toContain("dt(2026-11-20T09:00:00Z)");
  });

  it("shows the forbidden state without appointments.read", async () => {
    api.get.mockResolvedValue(appointment());
    await render();
    expect(api.get).not.toHaveBeenCalled();
    expect(container.textContent).toContain("appointments.forbidden");
  });
});
