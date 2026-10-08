// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { act, createElement, Fragment, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

type Row = { uuid: string };
type TableProps = {
  columns: ColumnDef<Row, unknown>[];
  data: Row[];
  toolbarExtra?: ReactNode;
  state?: { onRowSelectionChange?: (v: Record<string, boolean>) => void };
};

const captured = vi.hoisted(() => ({ table: null as unknown }));
const api = vi.hoisted(() => ({
  getPlan: vi.fn(),
  card: vi.fn(),
  startIntake: vi.fn(),
  cancelPlan: vi.fn(),
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
    format: { dateTime: (v: string) => v },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/providers/toast-provider", () => ({ appToast: toast }));
vi.mock("next/link", () => ({
  default: ({ href, children }: { href: string; children: ReactNode }) =>
    createElement("a", { href }, children),
}));
vi.mock("@/components/layout/page-header", () => ({
  PageHeader: ({ title }: { title: string }) =>
    createElement("h1", null, title as ReactNode),
}));
// Renders the toolbar and the intake result cell of every row.
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityTable: (props: TableProps) => {
    captured.table = props;
    const result = props.columns.find((c) => c.id === "result");
    return createElement(
      "div",
      null,
      props.toolbarExtra,
      props.data.map((row) =>
        createElement(
          Fragment,
          { key: row.uuid },
          (result!.cell as (ctx: unknown) => ReactNode)({
            row: { original: row },
          }),
        ),
      ),
    );
  },
}));
vi.mock("@/features/fleets/services/fleets.service", async (orig) => ({
  ...(await orig<object>()),
  fleetsService: api,
}));

import { FleetPlanDetailPage } from "./fleet-plan-detail-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const appointment = (uuid: string, plate: string) => ({
  uuid,
  organization_id: 1,
  customer_user_id: 1,
  starts_at: "2026-10-12T07:00:00Z",
  ends_at: "2026-10-12T08:00:00Z",
  estimated_minutes: 60,
  source: "fleet_plan",
  status: "scheduled",
  note: "",
  vehicle_uuid: `veh-${uuid}`,
  vehicle_plate: plate,
  service_uuid: null,
});

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  for (const fn of [...Object.values(api), ...Object.values(toast)]) {
    fn.mockReset();
  }
  api.card.mockResolvedValue({ uuid: "fleet-1", name: "Acme Filo" });
  api.getPlan.mockResolvedValue({
    uuid: "plan-1",
    fleet_uuid: "fleet-1",
    dealer_uuid: "dealer-1",
    status: "scheduled",
    service_type: "PPF",
    note: "",
    appointments: [appointment("a-1", "34 A 1"), appointment("a-2", "34 A 2")],
    created_at: "2026-10-08T00:00:00Z",
  });
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

const $ = (id: string) =>
  container.querySelector<HTMLElement>(`[data-testid="${id}"]`);

describe("fleet plan detail: row-wise intake (TEC-477)", () => {
  it("takes the selected appointments into intake and shows each row's result", async () => {
    api.startIntake.mockResolvedValue({
      plan_uuid: "plan-1",
      results: [
        { appointment_uuid: "a-1", ok: true, service_uuid: "svc-1" },
        {
          appointment_uuid: "a-2",
          ok: false,
          code: "APPOINTMENT_INVALID_TRANSITION",
          message: "appointment status transition is not allowed",
        },
      ],
    });
    const qc = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    act(() =>
      root.render(
        createElement(
          QueryClientProvider,
          { client: qc },
          createElement(FleetPlanDetailPage, {
            slug: "acme",
            fleetUuid: "fleet-1",
            planUuid: "plan-1",
          }),
        ),
      ),
    );
    await flush();

    const button = $("intake-selected") as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    act(() =>
      (captured.table as TableProps).state!.onRowSelectionChange!({
        "a-1": true,
        "a-2": true,
      }),
    );
    expect(($("intake-selected") as HTMLButtonElement).disabled).toBe(false);

    act(() => $("intake-selected")!.click());
    await flush();

    expect(api.startIntake).toHaveBeenCalledWith("fleet-1", "plan-1", [
      "a-1",
      "a-2",
    ]);
    const ok = $("intake-result-ok")!;
    const failed = $("intake-result-error")!;
    expect(ok.dataset.appointment).toBe("a-1");
    expect(failed.dataset.appointment).toBe("a-2");
    expect(failed.textContent).toBe(
      "fleets.intake.codes.APPOINTMENT_INVALID_TRANSITION",
    );
    expect($("intake-summary")!.textContent).toContain('"ok":1,"failed":1');
    expect(toast.warning).toHaveBeenCalledWith(
      'fleets.intake.partial {"ok":1,"failed":1}',
    );
  });
});
