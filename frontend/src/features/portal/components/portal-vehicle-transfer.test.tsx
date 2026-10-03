// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const portal = vi.hoisted(() => ({
  listVehicleTransfers: vi.fn(),
  startVehicleTransfer: vi.fn(),
  verifyVehicleTransfer: vi.fn(),
  cancelVehicleTransfer: vi.fn(),
}));
const router = vi.hoisted(() => ({ replace: vi.fn(), push: vi.fn() }));

vi.mock("next/navigation", () => ({ useRouter: () => router }));
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

import {
  PortalApiError,
  type PortalVehicleTransfer,
} from "@/features/portal/lib/portal-client";
import { portalTransferError } from "@/features/portal/lib/portal-vehicles";

import {
  PortalVehicleTransferDialog,
  PortalVehicleTransferFlow,
} from "./portal-vehicle-transfer";

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
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
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

async function type(selector: string, value: string) {
  const el = document.querySelector(selector);
  if (!(el instanceof HTMLInputElement)) throw new Error(`${selector} missing`);
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  await act(async () => {
    setter?.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

function transfer(
  over: Partial<PortalVehicleTransfer> = {},
): PortalVehicleTransfer {
  return {
    uuid: "t-1",
    vehicle_uuid: "v-1",
    status: "pending",
    to_phone_masked: "+90 *** ** 67",
    new_owner_known: false,
    from_verified: false,
    to_verified: false,
    attempts: 0,
    max_attempts: 5,
    expires_at: "2026-10-03T12:15:00Z",
    created_at: "2026-10-03T12:00:00Z",
    completed_at: null,
    cancelled_at: null,
    warranties_moved: 0,
    ...over,
  } as PortalVehicleTransfer;
}

describe("portalTransferError", () => {
  it("maps a wrong code to the attempts left", () => {
    expect(
      portalTransferError({
        status: 422,
        code: "VEHICLE_TRANSFER_INVALID_CODE",
        details: [{ field: "to_code", code: "3" }],
      }),
    ).toEqual({
      key: "portal.transfer.error.invalid_code",
      params: { count: 3 },
    });
  });

  it("maps the flow codes, validation and status fallbacks", () => {
    expect(
      portalTransferError({ status: 409, code: "VEHICLE_TRANSFER_LOCKED" }).key,
    ).toBe("portal.transfer.error.locked");
    expect(
      portalTransferError({
        status: 409,
        code: "VEHICLE_TRANSFER_NOT_PENDING",
      }).key,
    ).toBe("portal.transfer.error.not_pending");
    expect(
      portalTransferError({
        status: 400,
        code: "VALIDATION_ERROR",
        details: [{ field: "phone" }],
      }).key,
    ).toBe("portal.transfer.error.phone");
    expect(
      portalTransferError({
        status: 400,
        code: "VALIDATION_ERROR",
        details: [{ field: "new_owner_name" }],
      }).key,
    ).toBe("portal.transfer.error.name");
    expect(portalTransferError({ status: 429, code: null }).key).toBe(
      "portal.transfer.error.rate",
    );
    expect(portalTransferError({ status: 404, code: "NOT_FOUND" }).key).toBe(
      "portal.transfer.error.not_found",
    );
    expect(portalTransferError({ status: 500, code: "X" }).key).toBe(
      "portal.transfer.error.generic",
    );
  });
});

describe("PortalVehicleTransferFlow", () => {
  it("walks phone -> codes -> done", async () => {
    portal.listVehicleTransfers.mockResolvedValue({ items: [] });
    portal.startVehicleTransfer.mockResolvedValue(transfer());
    portal.verifyVehicleTransfer.mockResolvedValue(
      transfer({
        status: "completed",
        from_verified: true,
        to_verified: true,
        new_owner_known: true,
        warranties_moved: 2,
      }),
    );
    const onCompleted = vi.fn();
    await render(
      createElement(PortalVehicleTransferFlow, {
        vehicleUuid: "v-1",
        onCompleted,
      }),
    );
    expect(portal.listVehicleTransfers).toHaveBeenCalledWith("v-1");

    // 1. Phone step.
    expect(
      container.querySelector('[data-testid="portal-transfer-phone-step"]'),
    ).not.toBeNull();
    await type("#portal-transfer-phone", "+905551112233");
    await click(container.querySelector('button[type="submit"]'));
    expect(portal.startVehicleTransfer).toHaveBeenCalledWith(
      "v-1",
      "+905551112233",
    );

    // 2. Code step: both codes and the new owner's name.
    const step = container.querySelector(
      '[data-testid="portal-transfer-code-step"]',
    );
    expect(step?.textContent).toContain("portal.transfer.pending_info");
    const verify = container.querySelector(
      '[data-testid="portal-transfer-verify"]',
    ) as HTMLButtonElement;
    expect(verify.disabled).toBe(true);
    await type("#portal-transfer-from-code", "123456");
    await type("#portal-transfer-to-code", "654321");
    // The new owner has no account: the name is required with both codes.
    expect(verify.disabled).toBe(true);
    await type("#portal-transfer-name", "Ayşe");
    expect(verify.disabled).toBe(false);
    await click(verify);
    expect(portal.verifyVehicleTransfer).toHaveBeenCalledWith("t-1", {
      from_code: "123456",
      to_code: "654321",
      new_owner_name: "Ayşe",
    });

    // 3. Done.
    const done = container.querySelector(
      '[data-testid="portal-transfer-done"]',
    );
    expect(done?.textContent).toContain('portal.transfer.done {"count":2}');
    expect(onCompleted).toHaveBeenCalledTimes(1);
  });

  it("shows the wrong code message with the attempts left", async () => {
    portal.listVehicleTransfers.mockResolvedValue({
      items: [transfer({ new_owner_known: true, from_verified: true })],
    });
    portal.verifyVehicleTransfer.mockRejectedValue(
      new PortalApiError("wrong", 422, "VEHICLE_TRANSFER_INVALID_CODE", [
        { field: "to_code", message: "wrong code", code: "3" },
      ]),
    );
    await render(
      createElement(PortalVehicleTransferFlow, { vehicleUuid: "v-1" }),
    );
    // The owner's code is already verified: only the buyer's code is asked.
    expect(
      container.querySelector(
        '[data-testid="portal-transfer-from-code-verified"]',
      ),
    ).not.toBeNull();
    expect(container.querySelector("#portal-transfer-from-code")).toBeNull();
    await type("#portal-transfer-to-code", "000000");
    await click(
      container.querySelector('[data-testid="portal-transfer-verify"]'),
    );
    expect(portal.verifyVehicleTransfer).toHaveBeenCalledWith("t-1", {
      to_code: "000000",
    });
    const alert = container.querySelector('[role="alert"]');
    expect(alert?.textContent).toBe(
      'portal.transfer.error.invalid_code {"count":3}',
    );
    // Still on the code step; the list is refreshed for the attempt counter.
    expect(
      container.querySelector('[data-testid="portal-transfer-code-step"]'),
    ).not.toBeNull();
    expect(portal.listVehicleTransfers.mock.calls.length).toBeGreaterThan(1);
  });

  it("cancels an open transfer and returns to the phone step", async () => {
    portal.listVehicleTransfers
      .mockResolvedValueOnce({ items: [transfer()] })
      .mockResolvedValue({ items: [transfer({ status: "cancelled" })] });
    portal.cancelVehicleTransfer.mockResolvedValue(
      transfer({ status: "cancelled" }),
    );
    await render(
      createElement(PortalVehicleTransferFlow, { vehicleUuid: "v-1" }),
    );
    await click(
      container.querySelector('[data-testid="portal-transfer-cancel"]'),
    );
    expect(portal.cancelVehicleTransfer).toHaveBeenCalledWith("t-1");
    expect(
      container.querySelector('[data-testid="portal-transfer-phone-step"]'),
    ).not.toBeNull();
  });
});

describe("PortalVehicleTransferDialog", () => {
  it("opens the dialog with the flow", async () => {
    portal.listVehicleTransfers.mockResolvedValue({ items: [] });
    await render(
      createElement(PortalVehicleTransferDialog, { vehicleUuid: "v-1" }),
    );
    expect(document.querySelector('[role="dialog"]')).toBeNull();
    await click(
      container.querySelector('[data-testid="portal-transfer-open"]'),
    );
    const dialog = document.querySelector('[role="dialog"]');
    expect(dialog?.textContent).toContain("portal.transfer.title");
    expect(
      dialog?.querySelector('[data-testid="portal-transfer-phone-step"]'),
    ).not.toBeNull();
  });
});
