// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

vi.mock("@/features/mcp/hooks/use-mcp", () => ({
  usePendingActionCount: () => ({ data: 4 }),
}));
vi.mock("@/features/conversations/hooks/use-conversations", () => ({
  useUnreadConversations: () => ({ data: 0 }),
}));
vi.mock("@/features/conversations/hooks/use-conversations-realtime", () => ({
  useConversationsRealtime: () => undefined,
}));
vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key }),
}));

import { platformNav, tenantNav } from "@/config/nav";
import { visibleNavItems } from "@/features/nav-engine/lib/access";
import type { NavAdornment, NavCatalog } from "@/features/nav-engine/types";
import { PendingActionsNavAdornment } from "@/features/mcp/nav";
import { safePortalNext } from "@/features/portal/components/portal-login";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const hrefs = (catalog: NavCatalog, perms: string[], features: string[]) =>
  catalog.groups.flatMap((group) =>
    visibleNavItems(group, {
      can: (p) => (Array.isArray(p) ? p : [p]).every((x) => perms.includes(x)),
      canAny: (ps) => ps.some((x) => perms.includes(x)),
      org: { type: "dealer", role: "owner", features },
    }).map((item) => item.href),
  );

describe("MCP menu entries (TEC-403)", () => {
  it("shows pending AI actions with ai.actions.confirm and the mcp module", () => {
    const href = "/t/acme/assistant/approvals";
    expect(hrefs(tenantNav("acme"), ["ai.actions.confirm"], ["mcp"])).toContain(
      href,
    );
    expect(hrefs(tenantNav("acme"), ["ai.actions.confirm"], [])).not.toContain(
      href,
    );
    expect(hrefs(tenantNav("acme"), [], ["mcp"])).not.toContain(href);
  });

  it("shows MCP clients in the platform panel behind mcp.clients.manage", () => {
    expect(hrefs(platformNav, ["mcp.clients.manage"], [])).toContain(
      "/platform/mcp-clients",
    );
    expect(hrefs(platformNav, [], [])).not.toContain("/platform/mcp-clients");
  });

  it("badges the pending count", async () => {
    let got: NavAdornment | null = null;
    const root = createRoot(document.createElement("div"));
    await act(async () => {
      root.render(
        <PendingActionsNavAdornment>
          {(a: NavAdornment) => {
            got = a;
            return null;
          }}
        </PendingActionsNavAdornment>,
      );
    });
    expect(got!.badges?.[0]).toMatchObject({
      kind: "count",
      value: 4,
      hiddenWhenZero: true,
    });
    act(() => root.unmount());
  });
});

describe("safePortalNext", () => {
  it("returns to the MCP consent screen after a portal sign-in", () => {
    expect(safePortalNext("/oauth/consent?request=abc")).toBe(
      "/oauth/consent?request=abc",
    );
    expect(safePortalNext("/oauth/consent?request=//evil.example")).toBe(
      "/portal",
    );
    expect(safePortalNext("/oauth/token")).toBe("/portal");
    expect(safePortalNext("//evil.example")).toBe("/portal");
  });
});
