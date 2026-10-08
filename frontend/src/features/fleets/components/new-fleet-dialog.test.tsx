// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, Fragment, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  lookup: vi.fn(),
  requestLink: vi.fn(),
  open: vi.fn(),
  inviteUser: vi.fn(),
}));
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
  }),
}));
vi.mock("@/providers/toast-provider", () => ({ appToast: toast }));
vi.mock("@/components/ui/dialog", () => {
  const Pass = ({ children }: { children?: ReactNode }) =>
    createElement(Fragment, null, children);
  return {
    Dialog: ({ open, children }: { open: boolean; children: ReactNode }) =>
      open ? createElement("div", { role: "dialog" }, children) : null,
    DialogContent: Pass,
    DialogDescription: Pass,
    DialogFooter: Pass,
    DialogHeader: Pass,
    DialogTitle: Pass,
  };
});
vi.mock("@/components/ui/select", () => {
  const Pass = ({ children }: { children?: ReactNode }) =>
    createElement(Fragment, null, children);
  return {
    Select: Pass,
    SelectContent: () => null,
    SelectItem: () => null,
    SelectTrigger: Pass,
    SelectValue: () => null,
  };
});
vi.mock("@/features/fleets/services/fleets.service", async (orig) => ({
  ...(await orig<object>()),
  fleetsService: api,
}));

import { ApiError } from "@/lib/api/errors";

import { NewFleetDialog } from "./new-fleet-dialog";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;
const onOpened = vi.fn();

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  for (const fn of [...Object.values(api), ...Object.values(toast), onOpened]) {
    fn.mockReset();
  }
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

function render() {
  const qc = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  act(() =>
    root.render(
      createElement(
        QueryClientProvider,
        { client: qc },
        createElement(NewFleetDialog, {
          open: true,
          onOpenChange: () => {},
          onOpened,
        }),
      ),
    ),
  );
}

const $ = (id: string) =>
  container.querySelector<HTMLElement>(`[data-testid="${id}"]`);

function type(id: string, value: string) {
  const el = $(id) as HTMLInputElement;
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )!.set!;
  act(() => {
    setter.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function click(id: string) {
  act(() => $(id)!.click());
  await flush();
}

const MATCH = {
  fleet_uuid: "11111111-1111-4111-8111-111111111111",
  name: "Acme Filo",
  legal_name: "Acme Lojistik A.Ş.",
  link_status: "",
};

describe("new fleet dialog (TEC-477)", () => {
  it("offers a link request instead of the form when the VKN is found", async () => {
    api.lookup.mockResolvedValue(MATCH);
    api.requestLink.mockResolvedValue({ uuid: MATCH.fleet_uuid });
    render();

    type("fleet-tax-number", " 1234567890 ");
    await click("fleet-lookup");

    expect(api.lookup).toHaveBeenCalledWith("1234567890");
    expect($("fleet-found")).not.toBeNull();
    expect($("fleet-found")!.textContent).toContain("Acme Lojistik A.Ş.");
    expect($("fleet-open-form")).toBeNull();

    await click("fleet-request-link");
    expect(api.requestLink).toHaveBeenCalledWith(MATCH.fleet_uuid);
    expect(api.open).not.toHaveBeenCalled();
    expect(toast.success).toHaveBeenCalledWith("fleets.new.link_requested");
  });

  it("shows the pending link instead of a second request", async () => {
    api.lookup.mockResolvedValue({ ...MATCH, link_status: "pending" });
    render();
    type("fleet-tax-number", "1234567890");
    await click("fleet-lookup");

    expect($("fleet-found-status")!.textContent).toBe(
      "fleets.new.found_link_pending",
    );
    expect(($("fleet-request-link") as HTMLButtonElement).disabled).toBe(true);
  });

  it("shows the opening form when the VKN is not registered and invites the primary user", async () => {
    api.lookup.mockResolvedValue(null);
    api.open.mockResolvedValue({ uuid: "fleet-new" });
    api.inviteUser.mockResolvedValue({ uuid: "fu-1" });
    render();
    type("fleet-tax-number", "12345678901");
    await click("fleet-lookup");

    expect($("fleet-found")).toBeNull();
    expect($("fleet-open-form")).not.toBeNull();

    type("fleet-form-legal_name", "Yeni Filo A.Ş.");
    type("fleet-form-user_email", "filo@example.test");
    type("fleet-form-user_name", "Ayşe");
    await click("fleet-open-submit");

    expect(api.open).toHaveBeenCalledWith(
      expect.objectContaining({
        legal_name: "Yeni Filo A.Ş.",
        tax_number: "12345678901",
        report_frequency: "monthly",
      }),
    );
    expect(api.inviteUser).toHaveBeenCalledWith("fleet-new", {
      email: "filo@example.test",
      name: "Ayşe",
      surname: undefined,
    });
    expect(onOpened).toHaveBeenCalledWith("fleet-new");
  });

  it("switches to the link request when the fleet was opened meanwhile (409)", async () => {
    api.lookup.mockResolvedValue(null);
    api.open.mockRejectedValue(
      new ApiError({
        status: 409,
        code: "FLEET_ALREADY_EXISTS",
        message: "exists",
        body: {
          success: false,
          error: { code: "FLEET_ALREADY_EXISTS", message: "exists" },
          data: MATCH,
        } as never,
      }),
    );
    render();
    type("fleet-tax-number", "1234567890");
    await click("fleet-lookup");
    type("fleet-form-legal_name", "Acme");
    await click("fleet-open-submit");

    expect($("fleet-open-form")).toBeNull();
    expect($("fleet-found")).not.toBeNull();
    expect($("fleet-request-link")).not.toBeNull();
  });

  it("rejects a tax number that is not 10 or 11 digits without calling the API", async () => {
    render();
    type("fleet-tax-number", "12345");
    await click("fleet-lookup");
    expect(api.lookup).not.toHaveBeenCalled();
    expect($("fleet-new-error")!.textContent).toContain(
      "fleets.new.tax_invalid",
    );
  });
});
