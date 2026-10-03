import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

function visibleIds(granted: string[], orgType = "center") {
  const access = {
    can: (p: string | string[]) =>
      (Array.isArray(p) ? p : [p]).every((x) => granted.includes(x)),
    canAny: (ps: string[]) => ps.some((x) => granted.includes(x)),
    org: { type: orgType, role: "owner", features: [] as string[] },
  };
  return tenantNav("acme").groups.flatMap((group) =>
    visibleNavItems(group, access).map((item) => item.id),
  );
}

describe("tenant nav: center tasks (TEC-221)", () => {
  it("links the task pages", () => {
    const group = tenantNav("acme").groups.find((g) => g.id === "tasks");
    expect(group?.items.map((i) => i.href)).toEqual([
      routes.tenant.tasks.list("acme"),
      routes.tenant.tasks.create("acme"),
    ]);
  });

  it("shows the list to a center role with tasks.read", () => {
    const ids = visibleIds([Permission.TasksRead]);
    expect(ids).toContain("tasks-list");
    expect(ids).not.toContain("tasks-new");
  });

  it("shows new with tasks.write", () => {
    expect(visibleIds([Permission.TasksRead, Permission.TasksWrite])).toContain(
      "tasks-new",
    );
  });

  it("is hidden without tasks.read or outside the center", () => {
    expect(visibleIds([Permission.CatalogRead])).not.toContain("tasks-list");
    expect(visibleIds([Permission.TasksRead], "dealer")).not.toContain(
      "tasks-list",
    );
    expect(visibleIds([Permission.TasksRead], "distributor")).not.toContain(
      "tasks-list",
    );
  });
});
