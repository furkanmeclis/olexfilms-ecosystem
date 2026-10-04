// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  list: vi.fn(),
}));
const state = vi.hoisted(() => ({
  grants: new Set<string>(),
  featureEnabled: true,
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    t: (key: string) => key,
    format: {
      dateTime: (value: string) => `dt(${value})`,
    },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock("@/features/modules/hooks/use-features", () => ({
  useFeature: () => ({ enabled: state.featureEnabled }),
}));
vi.mock("@/features/announcements/services/announcements.service", () => ({
  announcementsService: api,
  announcementKeys: {
    lists: () => ["announcements", "list"],
    list: (params: unknown) => ["announcements", "list", params],
  },
}));

import { Permission } from "@/config/permissions";
import type { Announcement } from "@/features/announcements";

import { RecentAnnouncementsWidget } from "./recent-announcements-widget";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  state.grants = new Set([Permission.AnnouncementsRead]);
  state.featureEnabled = true;
  api.list.mockResolvedValue({
    items: [announcement()],
    total: 1,
    limit: 3,
    offset: 0,
  });
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
    body: "Hello network",
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

async function renderWidget() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(RecentAnnouncementsWidget, { slug: "acme" }),
      ),
    );
  });
  await flush();
}

describe("RecentAnnouncementsWidget", () => {
  it("does not fetch announcements without read permission", async () => {
    state.grants = new Set();
    await renderWidget();

    expect(api.list).not.toHaveBeenCalled();
    expect(container.textContent).toBe("");
  });

  it("does not fetch announcements when the feature is disabled", async () => {
    state.featureEnabled = false;
    await renderWidget();

    expect(api.list).not.toHaveBeenCalled();
    expect(container.textContent).toBe("");
  });

  it("shows recent announcements when the module is visible", async () => {
    await renderWidget();

    expect(api.list).toHaveBeenCalledWith({
      locale: "en",
      limit: 3,
      offset: 0,
    });
    expect(container.textContent).toContain("Network update");
  });
});
