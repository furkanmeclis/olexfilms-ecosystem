// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, Fragment, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const http = vi.hoisted(() => ({ platformRequest: vi.fn() }));
const state = vi.hoisted(() => ({ grants: new Set<string>() }));
const captured = vi.hoisted(() => ({
  tables: [] as Record<string, unknown>[],
  push: vi.fn(),
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
      number: (v: number) => String(v),
      date: (v: string) => `date(${v})`,
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/lib/api/platform-request", () => http);
// The table renders each row's cells; row actions are plain buttons.
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityPage: ({ children }: { children: ReactNode }) =>
    createElement("div", null, children),
  EntityRowActions: ({
    actions,
  }: {
    actions: { id: string; onSelect: () => void }[];
  }) =>
    createElement(
      "span",
      null,
      ...actions.map((a) =>
        createElement(
          "button",
          {
            key: a.id,
            type: "button",
            "data-action": a.id,
            onClick: a.onSelect,
          },
          a.id,
        ),
      ),
    ),
  EntityTable: (props: {
    columns: ColumnDef<unknown, unknown>[];
    data: unknown[];
  }) => {
    captured.tables.push(props as Record<string, unknown>);
    const cells = props.columns.filter((c) => typeof c.cell === "function");
    return createElement(
      "div",
      null,
      ...props.data.map((row, i) =>
        createElement(
          "div",
          { key: i, "data-testid": "table-row" },
          ...cells.map((c, j) =>
            createElement(
              Fragment,
              { key: j },
              (c.cell as (ctx: unknown) => ReactNode)({
                row: { original: row },
                getValue: () => "",
              }),
            ),
          ),
        ),
      ),
    );
  },
}));

import { Permission } from "@/config/permissions";
import type {
  MeasurementDetail,
  MeasurementDevice,
  MeasurementSummary,
} from "@/features/measurements/services/measurements.service";

import { MeasurementDetailPage } from "./measurement-detail-page";
import { MeasurementDevicesPage } from "./measurement-devices-page";
import { MeasurementsListPage } from "./measurements-list-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const device: MeasurementDevice = {
  uuid: "dev-1",
  serial: "NX-100",
  type: "NexPTG",
  label: "Front desk",
  model: "Professional",
  is_active: true,
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-01T10:00:00Z",
};

const summary = (over: Partial<MeasurementSummary>): MeasurementSummary => ({
  uuid: "m-1",
  organization: { uuid: "org-1", name: "Acme" },
  vin: "WVWZZZ1KZ6W000001",
  plate: "34 ABC 12",
  status: "accepted",
  source: "mobile",
  device_serial: "NX-100",
  device,
  service: null,
  measured_at: "2026-10-01T09:00:00Z",
  created_at: "2026-10-01T09:05:00Z",
  ...over,
});

const pendingDetail: MeasurementDetail = {
  ...summary({ uuid: "m-2", vin: null, plate: null, status: "vin_pending" }),
  body_type: "SEDAN",
  pdf_key: null,
  raw: {},
  values: [
    {
      place_id: "top",
      part_type: "HOOD",
      is_inside: false,
      position: 1,
      value_um: "112.5",
      interpretation: 3,
      substrate_type: "Fe",
      measured_at: null,
    },
  ],
  tires: [],
};

const partMap = {
  id: "MUvi2UUvtDeNoPrWss2qpL",
  name: "Sedan",
  bodywork: "SEDAN",
  point_radius: 22,
  places: ["left", "right", "top", "back"],
  assets: [
    {
      part: "TOP_SIDE",
      place: "top",
      kind: "main",
      points: [],
      svg: '<svg xmlns="http://www.w3.org/2000/svg" width="100" height="50"><g id="TOP_SIDE"/></svg>',
    },
    {
      part: "HOOD",
      place: "top",
      kind: "element",
      points: [{ count: 1, x: 10, y: 20 }],
      svg: null,
    },
  ],
};

let container: HTMLDivElement;
let root: Root;

async function render(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, node));
  });
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
}

async function flush() {
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
}

function setInput(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  state.grants = new Set();
  captured.tables = [];
  captured.push.mockReset();
  http.platformRequest.mockReset();
  http.platformRequest.mockImplementation(
    async (method: string, path: string) => {
      if (path === "/v1/measurements") {
        return {
          items: [
            summary({}),
            summary({ uuid: "m-2", vin: null, status: "vin_pending" }),
          ],
          total: 2,
          limit: 20,
          offset: 0,
        };
      }
      if (path === "/v1/measurements/m-2") return pendingDetail;
      if (path.startsWith("/v1/measurement-part-maps/")) return partMap;
      if (path === "/v1/measurement-devices") return [device];
      if (path === "/v1/measurements/m-2/vin") {
        return {
          ...pendingDetail,
          vin: "WVWZZZ1KZ6W000001",
          status: "accepted",
        };
      }
      if (method === "PATCH") return { ...device, is_active: false };
      throw new Error(`unexpected ${method} ${path}`);
    },
  );
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe("measurements list (TEC-299)", () => {
  it("asks the server list with the default sort and shows the pending badge", async () => {
    state.grants = new Set([Permission.MeasurementsRead]);
    await render(createElement(MeasurementsListPage, { slug: "acme" }));

    const call = http.platformRequest.mock.calls.find(
      ([, path]) => path === "/v1/measurements",
    );
    expect(call?.[2]).toEqual({
      query: expect.objectContaining({ sort: "-measured_at", offset: 0 }),
    });
    const rows = container.querySelectorAll('[data-testid="table-row"]');
    expect(rows).toHaveLength(2);
    expect(
      rows[0].querySelector('[data-testid="vin-pending-badge"]'),
    ).toBeNull();
    expect(rows[0].textContent).toContain("WVWZZZ1KZ6W000001");
    expect(
      rows[1].querySelector('[data-testid="vin-pending-badge"]')?.textContent,
    ).toBe("measurements.vin.pending_badge");
    // PDF only for an accepted measurement.
    expect(rows[0].querySelector('[data-action="pdf"]')).not.toBeNull();
    expect(rows[1].querySelector('[data-action="pdf"]')).toBeNull();
  });

  it("maps the filters to the list contract params", async () => {
    state.grants = new Set([
      Permission.MeasurementsRead,
      Permission.MeasurementDevicesManage,
    ]);
    await render(createElement(MeasurementsListPage, { slug: "acme" }));
    const columns = captured.tables.at(-1)?.columns as {
      id?: string;
      accessorKey?: string;
      meta?: { param?: string; filterVariant?: string };
    }[];
    const params = Object.fromEntries(
      columns
        .filter((c) => c.meta?.param)
        .map((c) => [
          c.id ?? c.accessorKey,
          `${c.meta?.filterVariant}:${c.meta?.param}`,
        ]),
    );
    expect(params).toEqual({
      measured_at: "date-range:measured",
      device: "faceted:device_uuid",
      status: "faceted:status",
      service: "boolean:linked",
    });
  });
});

describe("measurement detail (TEC-299)", () => {
  it("shows the badge and the VIN form, and rejects an invalid VIN", async () => {
    state.grants = new Set([
      Permission.MeasurementsRead,
      Permission.MeasurementsLink,
    ]);
    await render(
      createElement(MeasurementDetailPage, { slug: "acme", uuid: "m-2" }),
    );

    expect(
      container.querySelector('[data-testid="vin-pending-badge"]'),
    ).not.toBeNull();
    const form = container.querySelector(
      '[data-testid="measurement-vin-form"]',
    );
    expect(form).not.toBeNull();
    const pdfButton = container.querySelector<HTMLButtonElement>(
      '[data-testid="measurement-pdf"]',
    );
    expect(pdfButton?.disabled).toBe(true);

    const input = form!.querySelector<HTMLInputElement>('input[name="vin"]')!;
    await act(async () => setInput(input, "WVWZZZ1KZ6W00000O"));
    await act(async () => {
      form!
        .querySelector("form")!
        .dispatchEvent(
          new Event("submit", { bubbles: true, cancelable: true }),
        );
    });
    expect(
      container.querySelector('[data-testid="vin-error"]')?.textContent,
    ).toBe("services.vin.errors.forbidden_letters");
    expect(
      http.platformRequest.mock.calls.some(([method]) => method === "PATCH"),
    ).toBe(false);

    await act(async () => setInput(input, "wvw-zzz1kz6w000001"));
    await act(async () => {
      form!
        .querySelector("form")!
        .dispatchEvent(
          new Event("submit", { bubbles: true, cancelable: true }),
        );
    });
    await flush();
    expect(http.platformRequest).toHaveBeenCalledWith(
      "PATCH",
      "/v1/measurements/m-2/vin",
      { body: { vin: "WVWZZZ1KZ6W000001" } },
    );
  });

  it("hides the VIN form without measurements.link", async () => {
    state.grants = new Set([Permission.MeasurementsRead]);
    await render(
      createElement(MeasurementDetailPage, { slug: "acme", uuid: "m-2" }),
    );
    expect(
      container.querySelector('[data-testid="measurement-vin-form"]'),
    ).toBeNull();
    expect(container.textContent).toContain("measurements.vin.no_permission");
  });

  it("draws the part map with the threshold colors, not mirrored", async () => {
    state.grants = new Set([Permission.MeasurementsRead]);
    await render(
      createElement(MeasurementDetailPage, { slug: "acme", uuid: "m-2" }),
    );
    expect(http.platformRequest).toHaveBeenCalledWith(
      "GET",
      "/v1/measurement-part-maps/SEDAN",
    );
    await flush();
    const map = container.querySelector('[data-testid="part-map"]');
    expect(map).not.toBeNull();
    expect(map!.querySelector('[dir="ltr"]')).not.toBeNull();
    const circle = map!.querySelector('[data-place="top"] circle');
    // HOOD point 1 is interpretation 3 (thin putty).
    expect(circle?.getAttribute("fill")).toBe("#ec4a08");
    expect(
      container.querySelector('[data-testid="interpretation-legend"]'),
    ).not.toBeNull();
  });
});

describe("measurement devices (TEC-299)", () => {
  it("deactivates a device with PATCH is_active=false", async () => {
    state.grants = new Set([Permission.MeasurementDevicesManage]);
    await render(createElement(MeasurementDevicesPage, { slug: "acme" }));

    const button = container.querySelector<HTMLButtonElement>(
      '[data-action="deactivate"]',
    );
    expect(button).not.toBeNull();
    await act(async () => button!.click());
    await flush();
    expect(http.platformRequest).toHaveBeenCalledWith(
      "PATCH",
      "/v1/measurement-devices/dev-1",
      { body: { is_active: false } },
    );
  });

  it("shows no device table without measurement_devices.manage", async () => {
    state.grants = new Set([Permission.MeasurementsRead]);
    await render(createElement(MeasurementDevicesPage, { slug: "acme" }));
    expect(container.querySelector('[data-action="deactivate"]')).toBeNull();
    expect(
      http.platformRequest.mock.calls.some(
        ([, path]) => path === "/v1/measurement-devices",
      ),
    ).toBe(false);
  });
});
