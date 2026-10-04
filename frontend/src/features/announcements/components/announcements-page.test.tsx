// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  create: vi.fn(),
  putLocale: vi.fn(),
  publish: vi.fn(),
  reads: vi.fn(),
  unreadCount: vi.fn(),
}));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  orgType: "center" as "center" | "distributor" | "dealer",
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    format: {
      dateTime: (value: string) => `dt(${value})`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({
    uuid: "org-1",
    slug: "acme",
    name: "Acme",
    role: "owner",
    status: "active",
    type: state.orgType,
  }),
}));
vi.mock("@/features/announcements/services/announcements.service", () => ({
  announcementsService: api,
  announcementKeys: {
    all: ["announcements"],
    lists: () => ["announcements", "list"],
    list: (params: unknown) => ["announcements", "list", params],
    details: () => ["announcements", "detail"],
    detail: (uuid: string, locale?: string) => [
      "announcements",
      "detail",
      uuid,
      locale ?? "",
    ],
    unreadCount: (locale?: string) => [
      "announcements",
      "unread-count",
      locale ?? "",
    ],
    reads: (uuid: string) => ["announcements", "reads", uuid],
  },
}));

import { Permission } from "@/config/permissions";
import type { Announcement } from "@/features/announcements";

import { AnnouncementsPage } from "./announcements-page";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
(
  globalThis as typeof globalThis & { ResizeObserver?: typeof ResizeObserver }
).ResizeObserver = class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as typeof ResizeObserver;

let container: HTMLDivElement;
let root: Root;
let invalidateSpy: unknown;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  state.grants = new Set([
    Permission.AnnouncementsRead,
    Permission.AnnouncementsWrite,
  ]);
  state.orgType = "center";
  api.list.mockResolvedValue({
    items: [announcement()],
    total: 1,
    limit: 20,
    offset: 0,
  });
  api.get.mockResolvedValue(announcement({ read_at: "2026-10-04T09:00:00Z" }));
  api.reads.mockResolvedValue({
    target_total: 2,
    read_total: 1,
    read_rate: 0.5,
    items: [],
  });
  api.create.mockResolvedValue(announcement());
  api.putLocale.mockResolvedValue(announcement());
  api.publish.mockResolvedValue(announcement());
  invalidateSpy = vi.spyOn(QueryClient.prototype, "invalidateQueries");
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.clearAllMocks();
});

function announcement(patch: Partial<Announcement> = {}): Announcement {
  return {
    uuid: "a-1",
    default_locale: "en",
    locale: "en",
    title: "Network update",
    body: "Hello **network**",
    body_format: "markdown",
    status: "published",
    pinned: false,
    notify: false,
    read_at: null,
    publish_at: "2026-10-04T08:00:00Z",
    expires_at: null,
    created_at: "2026-10-04T07:00:00Z",
    updated_at: "2026-10-04T07:00:00Z",
    audiences: [{ target_type: "all_network" }],
    ...patch,
  };
}

async function flush() {
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

async function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(AnnouncementsPage, { slug: "acme" }),
      ),
    );
  });
  await flush();
}

async function type(sel: string, value: string) {
  const el = container.querySelector(sel) as
    HTMLInputElement | HTMLTextAreaElement;
  const proto =
    el instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, "value")!.set!;
  await act(async () => {
    setter.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await flush();
}

async function click(sel: string) {
  await act(async () => {
    (container.querySelector(sel) as HTMLElement).click();
  });
  await flush();
}

describe("AnnouncementsPage", () => {
  it("limits distributor audience to sub-dealers", async () => {
    state.orgType = "distributor";
    await renderPage();

    const options = Array.from(
      container.querySelectorAll<HTMLSelectElement>(
        "#announcement-audience option",
      ),
    );
    expect(options.map((o) => o.value)).toEqual(["subtree"]);
  });

  it("invalidates list and unread badge when a detail is opened", async () => {
    await renderPage();
    await click("[data-testid=announcement-row]");

    expect(api.get).toHaveBeenCalledWith("a-1", "en");
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["announcements", "list"],
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["announcements", "unread-count", "en"],
    });
  });

  it("rejects an end date before the publish date", async () => {
    await renderPage();
    await type("#announcement-title", "Plan");
    await type("#announcement-body", "Body");
    await type("#announcement-publish-at", "2026-10-05T10:00");
    await type("#announcement-expires-at", "2026-10-04T10:00");
    await click("[data-testid=announcement-form] button[type=submit]");

    expect(container.textContent).toContain(
      "announcements.errors.expires_before_publish",
    );
    expect(api.create).not.toHaveBeenCalled();
  });

  it("does not render script tags in the Markdown preview", async () => {
    await renderPage();
    await type("#announcement-body", "<script>alert(1)</script> **ok**");

    const preview = container.querySelector(
      "[data-testid=announcement-preview]",
    );
    expect(preview?.querySelector("script")).toBeNull();
    expect(preview?.innerHTML).not.toContain("<script>");
    expect(preview?.textContent).toContain("<script>alert(1)</script>");
  });
});
