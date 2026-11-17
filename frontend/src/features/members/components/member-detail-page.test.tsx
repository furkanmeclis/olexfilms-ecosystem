// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({
  granted: new Set<string>(),
  request: vi.fn(),
  related: [] as { kind: string; filter: Record<string, unknown> }[],
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string) => key,
    format: {
      number: (v: number) => String(v),
      date: (v: string) => `d(${v})`,
      dateTime: (v: string) => `dt(${v})`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (slug: string) => state.granted.has(slug) }),
}));
vi.mock("@/features/organizations/providers/tenant-provider", () => ({
  useOptionalTenant: () => ({ slug: "olex" }),
}));
vi.mock("next/link", async () => {
  const { createElement: h } = await import("react");
  return {
    default: ({ href, children, ...rest }: Record<string, unknown>) =>
      h("a", { href, ...rest }, children as ReactNode),
  };
});
vi.mock("@/lib/api", async (orig) => ({
  ...(await orig<object>()),
  platformRequest: state.request,
}));
vi.mock("@/components/entity", async (orig) => ({
  ...(await orig<object>()),
  RelatedList: ({
    kind,
    filter,
  }: {
    kind: string;
    filter: Record<string, unknown>;
  }) => {
    state.related.push({ kind, filter });
    return createElement("div", { "data-related": kind });
  },
}));
vi.mock("@/features/users/components/user-activity-section", () => ({
  UserActivitySection: () => createElement("div", { "data-activity": "1" }),
}));

import { Permission, permissions } from "@/config/permissions";

import { MemberDetailPage, memberTabs } from "./member-detail-page";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  state.granted.clear();
  state.related = [];
  state.request.mockReset();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

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

const summary = {
  user: {
    uuid: "u-1",
    name: "Jane",
    surname: "Staff",
    email: "jane@example.com",
    phone_e164: "+905551112233",
    status: "active",
    last_login_at: null,
    created_at: null,
  },
  ref: { kind: "user", uuid: "u-1", name: "Jane Staff" },
  memberships: [
    {
      organization: { kind: "organization", uuid: "o-1", name: "Acme Bayi" },
      role: "staff",
      role_slugs: ["dealer_staff"],
      warehouses: [{ kind: "warehouse", uuid: "w-1", name: "Ana Depo" }],
      created_at: null,
    },
  ],
  counts: {
    services_created: 4,
    services_performed: null,
    orders_created: 2,
    open_tasks: 1,
    leads_assigned: null,
  },
};

describe("memberTabs (TEC-1218)", () => {
  it("follows the module permissions; payroll needs staff.manage", () => {
    const can = (set: string[]) => (p: string) => set.includes(p);
    expect(memberTabs(can([]))).toEqual([]);
    expect(
      memberTabs(can([Permission.ServicesRead, Permission.TasksRead])),
    ).toEqual(["services_created", "services_performed", "tasks"]);
    expect(memberTabs(can([Permission.StaffManage]))).toEqual(["payroll"]);
  });
});

describe("MemberDetailPage (TEC-1218)", () => {
  it("shows forbidden without members.read and makes no request", async () => {
    await render(
      createElement(MemberDetailPage, { slug: "olex", uuid: "u-1" }),
    );
    expect(container.textContent).toContain("common.error_forbidden");
    expect(state.request).not.toHaveBeenCalled();
  });

  it("links memberships and lists what the user did", async () => {
    state.granted = new Set([
      Permission.MembersRead,
      Permission.ServicesRead,
      permissions.organizations.tenantRead,
    ]);
    state.request.mockResolvedValue(summary);
    await render(
      createElement(MemberDetailPage, { slug: "olex", uuid: "u-1" }),
    );
    expect(state.request).toHaveBeenCalledWith(
      "GET",
      "/v1/tenant/users/u-1/summary",
    );
    const org = [...container.querySelectorAll("a")].find(
      (a) => a.textContent === "Acme Bayi",
    );
    expect(org?.getAttribute("href")).toBe("/t/olex/organizations/o-1");
    // No warehouse.read: the warehouse stays plain text.
    expect(container.textContent).toContain("Ana Depo");
    // Null counters (no permission) are hidden.
    const counters = container.querySelector("[data-testid=member-counters]");
    expect(counters?.textContent).toContain(
      "users.member.counts.services_created",
    );
    expect(counters?.textContent).not.toContain("services_performed");
    expect(state.related[0]).toEqual({
      kind: "service",
      filter: { created_by_uuid: "u-1" },
    });
  });
});
