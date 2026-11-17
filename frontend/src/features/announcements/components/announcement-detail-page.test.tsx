// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({ get: vi.fn() }));
const state = vi.hoisted(() => ({ grants: new Set<string>() }));

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: { href: string; children: unknown } & Record<string, unknown>) =>
    createElement("a", { href, ...rest }, children as never),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    t: (key: string) => key,
    format: { dateTime: (v: string) => `dt(${v})` },
  }),
}));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: (p: string) => state.grants.has(p) }),
}));
vi.mock(
  "@/features/announcements/services/announcements.service",
  async (orig) => ({
    ...(await orig<object>()),
    announcementsService: api,
  }),
);

import { permissions } from "@/config/permissions";
import type { Announcement } from "@/features/announcements/services/announcements.service";

import { AnnouncementDetailPage } from "./announcement-detail-page";

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
  state.grants = new Set();
});

async function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client },
        createElement(AnnouncementDetailPage, { slug: "olex", uuid: "an-1" }),
      ),
    );
  });
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

const href = (text: string) =>
  [...container.querySelectorAll("a")]
    .find((a) => a.textContent === text)
    ?.getAttribute("href");

function announcement(): Announcement {
  return {
    uuid: "an-1",
    default_locale: "en",
    title: "Winter campaign",
    body: "Hello **network**",
    body_format: "markdown",
    status: "published",
    pinned: true,
    notify: false,
    publish_at: "2026-11-01T08:00:00Z",
    audiences: [{ target_type: "role", role_slug: "dealer_admin" }],
    author: { kind: "user", uuid: "us-1", name: "Jane Staff" },
    author_organization: {
      kind: "organization",
      uuid: "org-1",
      name: "Olex HQ",
    },
  };
}

describe("AnnouncementDetailPage (TEC-1218)", () => {
  it("renders the body and links the author when permitted", async () => {
    state.grants = new Set([
      permissions.announcements.read,
      permissions.members.read,
    ]);
    api.get.mockResolvedValue(announcement());
    await render();

    expect(api.get).toHaveBeenCalledWith("an-1", "en");
    expect(container.textContent).toContain("Winter campaign");
    expect(href("Jane Staff")).toBe("/t/olex/users/us-1");
    // No organizations read permission: plain text.
    expect(href("Olex HQ")).toBeUndefined();
    expect(container.textContent).toContain("Olex HQ");
    expect(
      container.querySelector('[data-testid="announcement-body"] strong')
        ?.textContent,
    ).toBe("network");
    expect(container.textContent).toContain(
      "announcements.audience.role · dealer_admin",
    );
  });

  it("shows the forbidden state without announcements.read", async () => {
    api.get.mockResolvedValue(announcement());
    await render();
    expect(api.get).not.toHaveBeenCalled();
    expect(container.textContent).toContain("announcements.forbidden");
  });
});
