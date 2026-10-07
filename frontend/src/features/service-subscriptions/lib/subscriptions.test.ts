import { describe, expect, it } from "vitest";

import {
  assignFormReady,
  assignTargets,
  canRequestCancel,
} from "@/features/service-subscriptions/lib/subscriptions";

const filled = {
  organizationUuid: "org",
  itemUuid: "item",
  startsOn: "2026-10-07",
  endsOn: "2027-10-07",
};

describe("subscription rules (TEC-311)", () => {
  it("needs every field and an end after the start", () => {
    expect(assignFormReady(filled)).toBe(true);
    expect(assignFormReady({ ...filled, endsOn: "" })).toBe(false);
    expect(assignFormReady({ ...filled, startsOn: "" })).toBe(false);
    expect(assignFormReady({ ...filled, organizationUuid: "" })).toBe(false);
    expect(assignFormReady({ ...filled, endsOn: "2026-10-07" })).toBe(false);
  });

  it("limits targets by the assigning organization", () => {
    const orgs = [
      { uuid: "c", name: "C", type: "center" },
      { uuid: "d", name: "D", type: "distributor" },
      { uuid: "b", name: "B", type: "dealer" },
    ];
    expect(assignTargets(orgs, "center").map((o) => o.uuid)).toEqual([
      "d",
      "b",
    ]);
    expect(assignTargets(orgs, "distributor").map((o) => o.uuid)).toEqual([
      "b",
    ]);
    expect(assignTargets(orgs, "dealer")).toEqual([]);
  });

  it("offers early cancellation only on own active subscriptions", () => {
    const own = { organization_uuid: "me", status: "active" as const };
    expect(canRequestCancel(own, "me", true)).toBe(true);
    expect(canRequestCancel(own, "me", false)).toBe(false);
    expect(canRequestCancel(own, "other", true)).toBe(false);
    expect(
      canRequestCancel({ ...own, status: "cancel_requested" }, "me", true),
    ).toBe(false);
  });
});
