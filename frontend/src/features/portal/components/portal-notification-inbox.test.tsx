// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const portal = vi.hoisted(() => ({
  listNotifications: vi.fn(),
  notificationsUnreadCount: vi.fn(),
  markNotificationRead: vi.fn(),
  markAllNotificationsRead: vi.fn(),
}));
const push = vi.hoisted(() => vi.fn());

vi.mock("next/navigation", () => ({
  usePathname: () => "/portal",
  useRouter: () => ({ replace: vi.fn(), push }),
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    locale: "en",
    dir: "ltr",
    t: (key: string) => key,
    format: { relative: (v: string) => `rel(${v})` },
  }),
}));
vi.mock("@/features/portal/lib/portal-client", async (orig) => ({
  ...(await orig<object>()),
  portalApi: portal,
}));

import type { PortalNotification } from "@/features/portal/lib/portal-client";

import { PortalNotificationInbox } from "./portal-notification-inbox";

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
});

const row = (over: Partial<PortalNotification>): PortalNotification =>
  ({
    uuid: "n-1",
    channel: "inapp",
    status: "sent",
    priority: "normal",
    title: "Appointment reminder",
    body: "Your appointment is coming up",
    payload: {},
    created_at: "2026-10-27T10:00:00Z",
    updated_at: "2026-10-27T10:00:00Z",
    ...over,
  }) as PortalNotification;

async function flush() {
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await Promise.resolve();
    });
  }
}

async function openInbox() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => {
    root.render(
      createElement(
        QueryClientProvider,
        { client: qc },
        createElement(PortalNotificationInbox),
      ),
    );
  });
  await flush();
  const trigger = document.querySelector<HTMLButtonElement>(
    "[data-testid=portal-notifications-trigger]",
  )!;
  await act(async () => {
    trigger.click();
  });
  await flush();
}

// TEC-1034: customer rows open their portal screen or the public quote link.
describe("PortalNotificationInbox", () => {
  it("lists the inbox, marks a row read and follows its link", async () => {
    portal.notificationsUnreadCount.mockResolvedValue({ count: 2 });
    portal.listNotifications.mockResolvedValue({
      items: [
        row({ uuid: "n-1", action_url: "/portal/appointments" }),
        row({
          uuid: "n-2",
          title: "Your quote is ready",
          action_url: "https://olexfilms.app/s/AbCdEfGhIj",
          read_at: "2026-10-27T11:00:00Z",
        }),
      ],
      total: 2,
      limit: 20,
      offset: 0,
    });
    portal.markNotificationRead.mockResolvedValue(row({}));
    const open = vi.spyOn(window, "open").mockReturnValue(null);

    await openInbox();
    expect(portal.listNotifications).toHaveBeenCalledWith({
      limit: 20,
      offset: 0,
    });
    const rows = document.querySelectorAll<HTMLButtonElement>(
      "[data-testid=portal-notification-row]",
    );
    expect(rows).toHaveLength(2);

    await act(async () => rows[0]!.click());
    expect(portal.markNotificationRead).toHaveBeenCalledWith("n-1");
    expect(push).toHaveBeenCalledWith("/portal/appointments");

    await openInbox();
    const again = document.querySelectorAll<HTMLButtonElement>(
      "[data-testid=portal-notification-row]",
    );
    await act(async () => again[1]!.click());
    expect(portal.markNotificationRead).toHaveBeenCalledTimes(1);
    expect(open).toHaveBeenCalledWith(
      "https://olexfilms.app/s/AbCdEfGhIj",
      "_blank",
      "noopener,noreferrer",
    );
  });
});
