import { describe, expect, it, vi } from "vitest";

vi.mock("@/features/conversations/hooks/use-conversations", () => ({
  useUnreadConversations: () => ({ data: 0 }),
}));
vi.mock("@/features/conversations/hooks/use-conversations-realtime", () => ({
  useConversationsRealtime: () => undefined,
}));

import { cmsNav, platformNav, tenantNav } from "@/config/nav";
import { visibleNavItems } from "@/features/nav-engine/lib/access";
import type { NavCatalog } from "@/features/nav-engine/types";

const hrefs = (catalog: NavCatalog, perms: string[]) =>
  catalog.groups.flatMap((group) =>
    visibleNavItems(group, {
      can: (p) => (Array.isArray(p) ? p : [p]).every((x) => perms.includes(x)),
      canAny: (ps) => ps.some((x) => perms.includes(x)),
      org: { type: "center", role: "owner", features: ["whatsapp_gateway"] },
    }).map((item) => item.href),
  );

describe("conversations menu (S2: platform admin only)", () => {
  it("is in the platform panel behind conversations.read", () => {
    expect(hrefs(platformNav, ["conversations.read"])).toContain(
      "/platform/conversations",
    );
    expect(hrefs(platformNav, [])).not.toContain("/platform/conversations");
  });

  it("is never listed in tenant (center, distributor, dealer) or CMS menus", () => {
    const all = (catalog: NavCatalog) =>
      catalog.groups.flatMap((group) => group.items.map((item) => item.href));
    for (const href of [...all(tenantNav("acme")), ...all(cmsNav)]) {
      expect(href).not.toContain("conversations");
    }
  });
});
