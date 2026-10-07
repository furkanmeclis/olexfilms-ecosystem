// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const http = vi.hoisted(() => ({ platformRequest: vi.fn() }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));
vi.mock("next/link", async () => {
  const { createElement: h } = await import("react");
  return {
    default: ({ href, children }: { href: string; children: unknown }) =>
      h("a", { href }, children as never),
  };
});
vi.mock("@/providers/toast-provider", () => ({ appToast: toast }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    dir: "ltr",
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      number: (v: number) => String(v),
      percent: (v: number) => `${v * 100}%`,
      date: (v: string) => v,
      dateTime: (v: string) => v,
      dateParts: (v: Date) =>
        `${v.getFullYear()}-${String(v.getMonth() + 1).padStart(2, "0")}`,
      currency: (v: number, c: string) => `${v} ${c}`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/lib/api/platform-request", () => http);
vi.mock("@/lib/api/platform-form-request", () => ({
  platformDownloadFile: vi.fn(),
  triggerBrowserDownload: vi.fn(),
}));
vi.mock("@/hooks/use-mobile", () => ({
  useIsMobile: () => false,
  useIsXl: () => true,
}));
// recharts needs layout; the charts are covered by their row helpers.
vi.mock("@/components/charts", () => ({ AppChart: () => null }));
vi.mock("@/components/ui/date-picker", async () => {
  const { createElement: h } = await import("react");
  return {
    DatePicker: ({
      id,
      value,
      onChange,
    }: {
      id?: string;
      value?: string;
      onChange?: (v: string) => void;
    }) =>
      h("input", {
        id,
        value: value ?? "",
        onChange: (e: { target: { value: string } }) =>
          onChange?.(e.target.value),
      }),
  };
});
vi.mock("@/features/staff-reports/hooks/use-staff-access", () => ({
  useStaffAccess: () => ({
    orgUuid: "dealer-1",
    loaded: true,
    canManage: true,
    canPay: true,
    canReadReports: true,
    canSeeCost: true,
  }),
}));

import { ApiError } from "@/lib/api";
import type {
  CariAgingReport,
  StaffProfile,
} from "@/features/staff-reports/services/staff-reports.service";

import { PayrollDialog } from "./payroll-dialog";
import { ReportsPage } from "./reports-page";
import { StaffPage } from "./staff-page";
import { StaffPaymentForm } from "./staff-payment-dialog";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
Element.prototype.scrollIntoView ??= () => {};
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

let container: HTMLDivElement;
let root: Root;

function card(over: Partial<StaffProfile>): StaffProfile {
  return {
    uuid: "s-1",
    user_uuid: null,
    name: "Ali Usta",
    title: null,
    hired_on: null,
    monthly_salary: "30000.00",
    currency: "TRY",
    active: true,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  };
}

const STAFF: StaffProfile[] = [
  card({ uuid: "ali", name: "Ali Usta" }),
  card({ uuid: "veli", name: "Veli Kalfa", monthly_salary: "20000.00" }),
  card({ uuid: "eski", name: "Eski Personel", active: false }),
  card({ uuid: "yeni", name: "Yeni Çırak", monthly_salary: null }),
];

const zero = {
  days_0_30: "0.00",
  days_31_60: "0.00",
  days_61_90: "0.00",
  days_90_plus: "0.00",
};
const AGING: CariAgingReport = {
  organization: { uuid: "dealer-1", name: "Dealer" } as never,
  currency: "TRY",
  as_of: "2026-10-07",
  lines: [
    {
      cari_uuid: "c-1",
      counterparty: { type: "user", uuid: "u-1", name: "Müşteri A" },
      side: "receivable",
      balance: "700.00",
      buckets: { ...zero, days_0_30: "500.00", days_90_plus: "200.00" },
    },
  ],
  totals: {
    receivable: {
      balance: "700.00",
      buckets: { ...zero, days_0_30: "500.00", days_90_plus: "200.00" },
    },
    payable: { balance: "0.00", buckets: zero },
  },
  generated_at: "2026-10-07T00:00:00Z",
};

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
  document.body.innerHTML = "";
});

async function flush() {
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render(el: ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  await act(async () => {
    root.render(createElement(QueryClientProvider, { client }, el));
  });
  await flush();
}

// Dialogs portal into document.body.
const $ = (sel: string) => document.body.querySelector(sel);
const $$ = (sel: string) => [...document.body.querySelectorAll(sel)];

async function click(el: Element | null) {
  if (!(el instanceof HTMLElement)) throw new Error("element not found");
  await act(async () => {
    el.click();
  });
  await flush();
}

async function submit(form: Element | null) {
  if (!(form instanceof HTMLFormElement)) throw new Error("form not found");
  await act(async () => {
    form.dispatchEvent(
      new Event("submit", { bubbles: true, cancelable: true }),
    );
  });
  await flush();
}

describe("PayrollDialog (month-end salaries)", () => {
  const open = () =>
    render(
      createElement(PayrollDialog, {
        orgUuid: "dealer-1",
        open: true,
        staff: STAFF,
        onOpenChange: () => {},
      }),
    );

  it("previews only the active staff", async () => {
    await open();
    const paid = $$('[data-testid="payroll-row"]').map((el) =>
      el.getAttribute("data-staff"),
    );
    const skipped = $$('[data-testid="payroll-row-skipped"]').map((el) =>
      el.getAttribute("data-staff"),
    );
    expect(paid).toEqual(["ali", "veli"]);
    expect(skipped).toEqual(["yeni"]);
    expect($('[data-testid="payroll-preview"]')?.textContent).not.toContain(
      "Eski Personel",
    );
    expect($('[data-testid="payroll-preview"]')?.textContent).toContain(
      "50000 TRY",
    );
  });

  it("runs the payroll of the period and reports the result", async () => {
    http.platformRequest.mockResolvedValue({
      period: "2026-10",
      created: 2,
      skipped: 1,
      items: [],
    });
    await open();
    await click($('[data-testid="payroll-confirm"]'));
    expect(http.platformRequest).toHaveBeenCalledWith(
      "POST",
      "/v1/staff-payments/payroll",
      { query: { period: expect.stringMatching(/^\d{4}-\d{2}$/) } },
    );
    expect($('[data-testid="payroll-result"]')).not.toBeNull();
    expect($('[data-testid="payroll-already"]')).toBeNull();
  });

  it("a second run of the same period shows the already-ran message (409)", async () => {
    http.platformRequest.mockRejectedValue(
      new ApiError({
        status: 409,
        code: "STAFF_SALARY_EXISTS",
        message: "Salary already exists for this staff period",
      }),
    );
    await open();
    await click($('[data-testid="payroll-confirm"]'));
    const alert = $('[data-testid="payroll-already"]');
    expect(alert?.textContent).toContain("staff_reports.payroll.already_ran");
    expect($('[data-testid="payroll-result"]')).toBeNull();
    expect(
      ($('[data-testid="payroll-confirm"]') as HTMLButtonElement).disabled,
    ).toBe(true);
  });

  it("an idempotent re-run that created nothing shows the same message", async () => {
    http.platformRequest.mockResolvedValue({
      period: "2026-10",
      created: 0,
      skipped: 3,
      items: [],
    });
    await open();
    await click($('[data-testid="payroll-confirm"]'));
    expect($('[data-testid="payroll-already"]')?.textContent).toContain(
      "staff_reports.payroll.already_ran",
    );
    expect(toast.success).not.toHaveBeenCalled();
  });
});

describe("StaffPaymentForm", () => {
  it("a second salary of the same period shows the 409 error", async () => {
    const onSubmit = vi.fn().mockRejectedValue(
      new ApiError({
        status: 409,
        code: "STAFF_SALARY_EXISTS",
        message: "Salary already exists for this staff period",
      }),
    );
    await render(
      createElement(StaffPaymentForm, {
        staff: STAFF[0]!,
        accounts: [
          {
            uuid: "acc-1",
            name: "Kasa",
            currency: "TRY",
            active: true,
          } as never,
        ],
        onCancel: () => {},
        onSubmit,
      }),
    );
    await submit($('[data-testid="staff-payment-form"]'));
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ type: "salary", account_uuid: "acc-1" }),
    );
    expect($("#staff-payment-period-error")?.textContent).toBe(
      "staff_reports.payment.salary_exists",
    );
  });
});

describe("ReportsPage", () => {
  function respond(method: string, path: string) {
    if (method === "POST") {
      return Promise.resolve({
        uuid: "job-1",
        status: "queued",
        format: "csv",
        created_at: "2026-10-07T00:00:00Z",
      });
    }
    if (path.endsWith("/cari-aging")) return Promise.resolve(AGING);
    if (path.endsWith("/pnl")) {
      return Promise.resolve({
        organization: {},
        currency: "TRY",
        from: null,
        to: null,
        group: "month",
        lines: [],
        totals: { income: "0.00", expense: "0.00", net: "0.00" },
        generated_at: "2026-10-07T00:00:00Z",
      });
    }
    return Promise.reject(new Error(`unexpected ${path}`));
  }

  beforeEach(() => {
    http.platformRequest.mockImplementation((method: string, path: string) =>
      respond(method, path),
    );
  });

  it("the aging table has the four bucket headers in order", async () => {
    await render(createElement(ReportsPage, { slug: "d" }));
    await click($('[data-testid="report-tab-cari-aging"]'));
    expect(http.platformRequest).toHaveBeenCalledWith(
      "GET",
      "/v1/accounting/reports/cari-aging",
      { query: {} },
    );
    const headers = [
      ...(container
        .querySelector('[data-testid="aging-table"]')
        ?.querySelectorAll("th") ?? []),
    ].map((th) => th.textContent ?? "");
    const buckets = headers.filter((h) => h.includes(".aging.days_"));
    expect(buckets.map((h) => h.trim())).toEqual([
      "staff_reports.reports.aging.days_0_30",
      "staff_reports.reports.aging.days_31_60",
      "staff_reports.reports.aging.days_61_90",
      "staff_reports.reports.aging.days_90_plus",
    ]);
    expect(container.textContent).toContain("Müşteri A");
  });

  it("export buttons queue the open report's export endpoint", async () => {
    await render(createElement(ReportsPage, { slug: "d" }));
    await click($('[data-testid="report-export-csv"]'));
    expect(http.platformRequest).toHaveBeenCalledWith(
      "POST",
      "/v1/accounting/reports/pnl/export",
      { body: { format: "csv", group: "month", locale: "en" } },
    );

    await act(async () => root.unmount());
    root = createRoot(container);
    await render(createElement(ReportsPage, { slug: "d" }));
    await click($('[data-testid="report-tab-cari-aging"]'));
    await click($('[data-testid="report-export-xlsx"]'));
    expect(http.platformRequest).toHaveBeenCalledWith(
      "POST",
      "/v1/accounting/reports/cari-aging/export",
      { body: { format: "xlsx", locale: "en" } },
    );
  });
});

describe("StaffPage", () => {
  it("lists every staff card and opens the payroll preview", async () => {
    http.platformRequest.mockImplementation((method: string, path: string) =>
      path === "/v1/staff-profiles"
        ? Promise.resolve({ items: STAFF, total: 4, limit: 100, offset: 0 })
        : Promise.reject(new Error(`unexpected ${method} ${path}`)),
    );
    await render(createElement(StaffPage, { slug: "d" }));
    const list = container.querySelector('[data-testid="staff-list"]');
    for (const s of STAFF) expect(list?.textContent).toContain(s.name);
    await click($('[data-testid="payroll-open"]'));
    expect(
      $$('[data-testid="payroll-row"]').map((el) =>
        el.getAttribute("data-staff"),
      ),
    ).toEqual(["ali", "veli"]);
  });
});
