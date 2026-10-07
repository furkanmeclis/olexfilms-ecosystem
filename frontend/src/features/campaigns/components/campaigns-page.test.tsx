// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";

import type {
  Campaign,
  CampaignPreview,
} from "@/features/campaigns/services/campaigns.service";
import type { AuthUser } from "@/lib/auth/types";

const service = vi.hoisted(() => ({
  list: vi.fn(),
  approvals: vi.fn(),
  get: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  putContent: vi.fn(),
  addMedia: vi.fn(),
  deleteMedia: vi.fn(),
  preview: vi.fn(),
  submit: vi.fn(),
  schedule: vi.fn(),
  sendNow: vi.fn(),
  cancel: vi.fn(),
  approve: vi.fn(),
  reject: vi.fn(),
  requestChanges: vi.fn(),
  recipients: vi.fn(),
}));

const router = vi.hoisted(() => ({ push: vi.fn() }));
const auth = vi.hoisted(() => ({ user: null as AuthUser | null }));

vi.mock("next/navigation", () => ({ useRouter: () => router }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/providers/auth-provider", () => ({ useAuth: () => auth }));
vi.mock("@/hooks/use-mobile", () => ({
  useIsMobile: () => false,
  useIsXl: () => true,
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "tr",
    dir: "ltr",
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      dateTime: (value: string) => `dt(${value})`,
      date: (value: string) => `d(${value})`,
      number: (value: number) => String(value),
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({
    can: () => true,
    canAny: () => true,
    canAll: () => true,
    hasPermission: () => true,
    hasRole: () => true,
    scopeFor: () => "all",
    canScope: () => true,
  }),
}));
vi.mock("@/features/campaigns/services/campaigns.service", async (orig) => ({
  ...(await orig<object>()),
  campaignsService: service,
}));

import { CampaignApprovalsPage } from "./campaign-approvals-page";
import { CampaignWizardPage } from "./campaign-wizard-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
(globalThis as { ResizeObserver?: typeof ResizeObserver }).ResizeObserver =
  class ResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as typeof ResizeObserver;
Object.defineProperty(window, "matchMedia", {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
});

let root: Root;
let container: HTMLDivElement;

function user(type: "center" | "distributor" | "dealer"): AuthUser {
  return {
    uuid: "u1",
    email: "u@example.com",
    name: "Ada",
    surname: "Lovelace",
    fullName: "Ada Lovelace",
    status: "active",
    locale: "tr",
    ownLocale: null,
    timeZone: "Europe/Istanbul",
    ownTimeZone: null,
    isSuperAdmin: false,
    emailVerified: true,
    permissions: ["campaigns.read", "campaigns.write"],
    grants: {},
    roles: [],
    organizationRoles: [],
    organizations: [
      {
        uuid: "org1",
        slug: "olex",
        name: "Olex",
        role: "member",
        status: "active",
        type,
      },
    ],
  };
}

function campaign(over: Partial<Campaign> = {}): Campaign {
  return {
    uuid: "c1",
    organization_uuid: "org1",
    organization_name: "Bayi",
    timezone: "Europe/Istanbul",
    approver_organization_uuid: "center",
    name: "Yeni kampanya",
    channels: ["push"],
    audience_filter: { audience_type: "customers" },
    status: "pending_approval",
    scheduled_at: null,
    started_at: null,
    finished_at: null,
    recipients_total: 12,
    recipients_sent: 0,
    recipients_failed: 0,
    recipients_skipped: 0,
    created_at: "2026-10-07T10:00:00Z",
    updated_at: "2026-10-07T10:00:00Z",
    contents: [
      {
        locale: "tr",
        title: "Başlık",
        body: "Gövde",
        deeplink: null,
        media: [],
        updated_at: "2026-10-07T10:00:00Z",
      },
    ],
    events: [],
    ...over,
  };
}

async function flush() {
  for (let i = 0; i < 5; i++) {
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

async function click(el: Element | null) {
  if (!(el instanceof HTMLElement)) throw new Error("element not found");
  await act(async () => {
    el.click();
  });
  await flush();
}

async function input(selector: string, value: string) {
  const el = container.querySelector(selector);
  if (!(el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement)) {
    throw new Error(`input not found: ${selector}`);
  }
  await act(async () => {
    el.value = value;
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await flush();
}

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  auth.user = user("dealer");
  service.approvals.mockResolvedValue({
    items: [campaign()],
    total: 1,
    limit: 20,
    offset: 0,
  });
  service.preview.mockResolvedValue({
    total: 12,
    locales: [{ locale: "tr", count: 12 }],
    channels: [{ channel: "push", reachable: 10 }],
    unreachable: 2,
    excluded: { total: 1, no_consent: 1, opted_out: 0 },
    missing_locales: [],
    sample: [],
  } satisfies CampaignPreview);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
  vi.clearAllMocks();
});

describe("CampaignWizardPage", () => {
  it("keeps approval submit disabled while languages are missing and never offers SMS", async () => {
    await render(createElement(CampaignWizardPage, { slug: "olex" }));

    expect(
      container.querySelector('[data-testid="campaign-channel-sms"]'),
    ).toBeNull();
    const submit = container.querySelector('[data-testid="campaign-submit"]');
    expect(submit?.textContent).toBe("campaigns.actions.submit_approval");
    expect(submit).toHaveProperty("disabled", true);
    expect(
      container.querySelector('[data-testid="campaign-missing-locales"]')
        ?.textContent,
    ).toContain("tr");
  });

  it("shows Schedule for center users and errors when push title exceeds 65 characters", async () => {
    auth.user = user("center");
    await render(createElement(CampaignWizardPage, { slug: "olex" }));

    expect(
      container.querySelector('[data-testid="campaign-submit"]')?.textContent,
    ).toBe("campaigns.actions.schedule");
    await input("#campaign-title-tr", "a".repeat(66));
    expect(container.textContent).toContain("65");
  });
});

describe("CampaignApprovalsPage", () => {
  it("does not submit a rejection without a reason", async () => {
    await render(createElement(CampaignApprovalsPage, { slug: "olex" }));

    await click(
      Array.from(container.querySelectorAll("div")).find(
        (el) => el.textContent === "Yeni kampanya",
      ) ?? null,
    );
    await click(
      Array.from(document.querySelectorAll("button")).find(
        (el) => el.textContent === "campaigns.actions.reject",
      ) ?? null,
    );

    const submit = document.querySelector(
      '[data-testid="campaign-decision-submit"]',
    );
    expect(submit).toHaveProperty("disabled", true);
    expect(
      document.querySelector('[data-testid="campaign-reason-required"]'),
    ).not.toBeNull();
    expect(service.reject).not.toHaveBeenCalled();
  });
});
