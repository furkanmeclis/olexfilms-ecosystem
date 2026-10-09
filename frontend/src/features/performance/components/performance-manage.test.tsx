// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  dashboard: vi.fn(),
  ranking: vi.fn(),
  benchmark: vi.fn(),
  reportCatalog: vi.fn(),
  reportLayout: vi.fn(),
  members: vi.fn(),
  rules: vi.fn(),
  createRule: vi.fn(),
  updateRule: vi.fn(),
  deleteRule: vi.fn(),
  bonuses: vi.fn(),
  approveBonus: vi.fn(),
  cancelBonus: vi.fn(),
  bonusRules: vi.fn(),
  bonusSettings: vi.fn(),
  staffTargets: vi.fn(),
  targets: vi.fn(),
  targetOrganizations: vi.fn(),
}));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  orgType: "dealer" as string,
  features: ["performance"] as string[] | null,
}));
const captured = vi.hoisted(() => ({
  tables: [] as Record<string, unknown>[],
  bulkMenus: [] as Record<string, unknown>[],
}));

vi.mock("next/link", () => ({
  default: ({ href, children }: { href: string; children: unknown }) =>
    createElement("a", { href }, children as never),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    locale: "en",
    format: {
      number: (v: number) => String(v),
      percent: (v: number) => `${v}`,
      currency: (v: number, c: string) => `${v} ${c}`,
      date: (v: string) => v,
      dateTime: (v: string) => v,
      dateParts: (v: string) => v.slice(0, 7),
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => state.grants.has(x)),
    canAny: (ps: string[]) => ps.some((x) => state.grants.has(x)),
    canScope: () => false,
  }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({
    slug: "acme",
    uuid: "org-1",
    type: state.orgType,
  }),
}));
vi.mock("@/hooks/use-debounce", () => ({
  useDebounce: <T,>(value: T) => value,
}));
vi.mock("@/features/modules/hooks/use-features", () => ({
  useEnabledFeatures: () => state.features,
}));
vi.mock("@/providers/toast-provider", () => ({
  appToast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock(
  "@/features/performance/services/performance.service",
  async (orig) => ({
    ...(await orig<object>()),
    performanceService: api,
  }),
);
vi.mock("@/features/bulk-engine", async (orig) => ({
  ...(await orig<object>()),
  BulkActionMenu: (props: Record<string, unknown>) => {
    captured.bulkMenus.push(props);
    return createElement("div", { "data-testid": "bulk-menu" });
  },
}));
vi.mock("@/components/charts", () => ({
  AppChart: () => createElement("div", { "data-testid": "chart" }),
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  EntityToolbar: () => null,
  EntityRowActions: () => null,
  EntityTable: (props: Record<string, unknown>) => {
    captured.tables.push(props);
    return createElement(
      "div",
      { "data-testid": "entity-table" },
      props.toolbarExtra as ReactNode,
    );
  },
}));

import { Permission } from "@/config/permissions";

import { BonusApproveDialog } from "./bonuses-tab";
import { managementTabs, PerformancePage } from "./performance-page";
import { RuleFormDialog, WeakRulesTab } from "./weak-rules-tab";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root | null;

beforeEach(() => {
  window.localStorage?.clear();
  captured.tables = [];
  captured.bulkMenus = [];
  state.grants = new Set([Permission.PerformanceRead]);
  state.orgType = "dealer";
  state.features = ["performance"];
  for (const fn of Object.values(api)) fn.mockReset();
  api.dashboard.mockResolvedValue({ metrics: {}, trend: [], targets: [] });
  api.benchmark.mockResolvedValue({
    own: { metrics: {} },
    network_average: {},
  });
  api.ranking.mockResolvedValue({ items: [], total: 0, limit: 20, offset: 0 });
  api.reportCatalog.mockResolvedValue({ items: [] });
  api.reportLayout.mockResolvedValue({ version: 1, widgets: [] });
  api.members.mockResolvedValue([{ uuid: "m1", name: "Merkez Uzmanı" }]);
  api.rules.mockResolvedValue([]);
  api.bonuses.mockResolvedValue({ items: [], total: 0, limit: 20, offset: 0 });
  api.bonusRules.mockResolvedValue([]);
  api.bonusSettings.mockResolvedValue({ payout_day: 5 });
  api.approveBonus.mockResolvedValue({});
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root?.unmount());
  root = null;
  container.remove();
  document.body.innerHTML = "";
});

async function flush() {
  for (let i = 0; i < 4; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function render(node: ReactNode) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  await act(async () => {
    root?.render(createElement(QueryClientProvider, { client: qc }, node));
  });
  await flush();
}

/** Dialogs render in a portal: search the whole document. */
const q = (sel: string) => document.querySelector(sel);

/** Radix tabs activate on a primary-button mousedown. */
async function pickTab(id: string) {
  const trigger = q(`[data-testid="performance-tab-${id}"]`)!;
  await act(async () => {
    trigger.dispatchEvent(
      new MouseEvent("mousedown", { bubbles: true, button: 0 }),
    );
  });
  await flush();
}

async function typeInto(el: Element, value: string) {
  const proto =
    el instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, "value")!.set!;
  await act(async () => {
    setter.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function click(el: Element) {
  await act(async () => {
    (el as HTMLElement).click();
  });
  await flush();
}

describe("weak dealer rule form", () => {
  it("has no open-task option in a distributor form", async () => {
    await render(
      createElement(RuleFormDialog, {
        orgType: "distributor",
        rule: null,
        onClose: () => {},
      }),
    );
    expect(q('[data-testid="performance-rule-form"]')).not.toBeNull();
    expect(q('[data-testid="performance-rule-create-task"]')).toBeNull();
    expect(document.body.textContent).not.toContain(
      "performance.rules.create_task",
    );
    expect(document.body.textContent).toContain(
      "performance.rules.distributor_hint",
    );
    expect(api.members).not.toHaveBeenCalled();
  });

  it("offers open task with an assignee in a center form", async () => {
    await render(
      createElement(RuleFormDialog, {
        orgType: "center",
        rule: null,
        onClose: () => {},
      }),
    );
    const toggle = q('[data-testid="performance-rule-create-task"]');
    expect(toggle).not.toBeNull();
    await click(toggle!);
    expect(document.body.textContent).toContain("performance.rules.assignee");
  });

  it("saves a distributor rule as notify only", async () => {
    api.createRule.mockResolvedValue({});
    await render(
      createElement(RuleFormDialog, {
        orgType: "distributor",
        rule: { uuid: "", name: "", create_task: true } as never,
        onClose: () => {},
      }),
    );
    await typeInto(q("#performance-rule-name")!, "Zayıf");
    await typeInto(q("#performance-rule-threshold")!, "50");
    await click(q('[data-testid="performance-rule-save"]')!);
    expect(api.createRule).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "Zayıf",
        threshold: "50",
        notify: true,
        create_task: false,
        assignee_user_uuid: null,
      }),
    );
  });

  it("lists distributor rules without task columns", async () => {
    await render(createElement(WeakRulesTab, { orgType: "distributor" }));
    const ids = (
      captured.tables.at(-1)!.columns as { id?: string; accessorKey?: string }[]
    ).map((c) => c.id ?? c.accessorKey);
    expect(ids).not.toContain("create_task");
    expect(ids).not.toContain("assignee");
    expect(ids).toContain("notify");
  });
});

describe("bonus approval", () => {
  const bonus = {
    uuid: "b1",
    user_name: "Ayşe",
    amount: "500.00",
    currency: "TRY",
    status: "calculated" as const,
  };

  it("requires a note when the amount changes", async () => {
    await render(
      createElement(BonusApproveDialog, { bonus, onClose: () => {} }),
    );
    const submit = q(
      '[data-testid="performance-bonus-approve-submit"]',
    ) as HTMLButtonElement;
    expect(submit.disabled).toBe(false);

    await typeInto(q('[data-testid="performance-bonus-amount"]')!, "450");
    expect(q('[data-testid="performance-bonus-error"]')?.textContent).toBe(
      "performance.bonuses.errors.note_required",
    );
    expect(submit.disabled).toBe(true);
    await click(submit);
    expect(api.approveBonus).not.toHaveBeenCalled();

    await typeInto(q('[data-testid="performance-bonus-note"]')!, "Geç teslim");
    expect(q('[data-testid="performance-bonus-error"]')).toBeNull();
    expect(submit.disabled).toBe(false);
    await click(submit);
    expect(api.approveBonus).toHaveBeenCalledWith("b1", {
      amount: "450",
      note: "Geç teslim",
    });
  });

  it("approves the calculated amount without a note", async () => {
    await render(
      createElement(BonusApproveDialog, { bonus, onClose: () => {} }),
    );
    await click(q('[data-testid="performance-bonus-approve-submit"]')!);
    expect(api.approveBonus).toHaveBeenCalledWith("b1", {});
  });
});

describe("bonus tabs and dealer_accounting", () => {
  beforeEach(() => {
    state.orgType = "dealer";
    state.grants.add(Permission.PerformanceBonusManage);
  });

  it("shows FeatureDisabled on the bonus tabs while dealer_accounting is off", async () => {
    state.features = ["performance"];
    await render(createElement(PerformancePage, { slug: "acme" }));
    await pickTab("bonuses");
    expect(q('[data-testid="feature-disabled"]')).not.toBeNull();
    expect(q('[data-testid="performance-bonuses"]')).toBeNull();
    expect(api.bonuses).not.toHaveBeenCalled();

    await pickTab("bonus_rules");
    expect(q('[data-testid="feature-disabled"]')).not.toBeNull();
    expect(q('[data-testid="performance-bonus-rules"]')).toBeNull();
    expect(api.bonusRules).not.toHaveBeenCalled();
  });

  it("opens the bonuses list with the bulk approve menu when dealer_accounting is on", async () => {
    state.features = ["performance", "dealer_accounting"];
    await render(createElement(PerformancePage, { slug: "acme" }));
    await pickTab("bonuses");
    expect(q('[data-testid="feature-disabled"]')).toBeNull();
    expect(q('[data-testid="performance-bonuses"]')).not.toBeNull();
    expect(api.bonuses).toHaveBeenCalledWith(
      expect.objectContaining({ period: expect.any(String) }),
    );
    const table = captured.tables.at(-1)!;
    expect((table.features as Record<string, unknown>).rowSelection).toBe(true);
    const menu = captured.bulkMenus.at(-1)!;
    expect(menu.resource).toBe("performance.bonuses");
    expect((menu.actions as { id: string }[]).map((a) => a.id)).toEqual([
      "approve",
    ]);
  });
});

describe("management tab screens", () => {
  it("lists center targets sorted by period with metric and period facets", async () => {
    state.orgType = "center";
    state.grants.add(Permission.PerformanceTargetsManage);
    api.targets.mockResolvedValue({
      items: [],
      total: 0,
      limit: 20,
      offset: 0,
    });
    await render(createElement(PerformancePage, { slug: "acme" }));
    await pickTab("targets");
    expect(q('[data-testid="performance-targets"]')).not.toBeNull();
    expect(api.targets).toHaveBeenCalledWith(
      expect.objectContaining({ sort: "-period_start" }),
    );
    const ids = (
      captured.tables.at(-1)!.columns as { id?: string; accessorKey?: string }[]
    ).map((c) => c.id ?? c.accessorKey);
    expect(ids).toEqual(
      expect.arrayContaining([
        "target_name",
        "metric",
        "period_kind",
        "achievement_pct",
        "actions",
      ]),
    );
  });

  it("builds the staff x month grid with inline edit", async () => {
    state.grants.add(Permission.PerformanceStaffTargetsManage);
    api.members.mockResolvedValue([
      { uuid: "u1", name: "Ayşe" },
      { uuid: "u2", name: "Mehmet" },
    ]);
    api.staffTargets.mockResolvedValue([
      {
        uuid: "t1",
        user_uuid: "u1",
        period: "2026-10",
        metric: "services_count",
        value: "10",
        achievement_pct: "50.00",
      },
    ]);
    await render(createElement(PerformancePage, { slug: "acme" }));
    await pickTab("team");
    expect(q('[data-testid="performance-team-targets"]')).not.toBeNull();
    const table = captured.tables.at(-1)!;
    expect((table.data as { uuid: string }[]).map((r) => r.uuid)).toEqual([
      "u1",
      "u2",
    ]);
    expect((table.features as Record<string, unknown>).inlineEdit).toBe(true);
    expect(api.staffTargets).toHaveBeenCalledWith(
      expect.objectContaining({ period_from: expect.any(String) }),
    );
  });

  it("shows the payout day and bonus rules with dealer_accounting on", async () => {
    state.grants.add(Permission.PerformanceBonusManage);
    state.features = ["performance", "dealer_accounting"];
    await render(createElement(PerformancePage, { slug: "acme" }));
    await pickTab("bonus_rules");
    expect(q('[data-testid="performance-bonus-rules"]')).not.toBeNull();
    expect((q("#performance-payout-day") as HTMLInputElement).value).toBe("5");
    expect(api.bonusRules).toHaveBeenCalled();
  });
});

describe("managementTabs", () => {
  const can = (granted: string[]) => (p: string) => granted.includes(p);

  it("gives center and distributor targets, rules behind rules.manage", () => {
    expect(managementTabs("center", can([]))).toEqual(["targets"]);
    expect(
      managementTabs("distributor", can([Permission.PerformanceRulesManage])),
    ).toEqual(["targets", "rules"]);
  });

  it("gives a dealer team targets and the bonus tabs by permission", () => {
    expect(managementTabs("dealer", can([]))).toEqual([]);
    expect(
      managementTabs(
        "dealer",
        can([
          Permission.PerformanceStaffTargetsManage,
          Permission.PerformanceBonusManage,
        ]),
      ),
    ).toEqual(["team", "bonus_rules", "bonuses"]);
  });
});
