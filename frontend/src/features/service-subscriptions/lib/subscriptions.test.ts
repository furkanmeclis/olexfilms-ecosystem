import { describe, expect, it } from "vitest";

import {
  assignFormReady,
  assignTargets,
  canRequestCancel,
  subscriptionStatusTone,
} from "@/features/service-subscriptions/lib/subscriptions";
import { SUBSCRIPTION_STATUSES } from "@/features/service-subscriptions/services/service-subscriptions.service";
import en from "@/locales/en/catalog.json";
import tr from "@/locales/tr/catalog.json";

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

describe("scheduled status (TEC-308)", () => {
  it("is a filterable status with a neutral badge and a label", () => {
    expect(SUBSCRIPTION_STATUSES).toContain("scheduled");
    expect(subscriptionStatusTone("scheduled")).toBe("default");
    expect(subscriptionStatusTone("active")).toBe("success");
    expect(tr["subscriptions.status.scheduled"]).toBe("Planlandı");
    expect(en["subscriptions.status.scheduled"]).toBe("Scheduled");
  });

  it("does not offer the cancel button before the start", () => {
    expect(
      canRequestCancel(
        { organization_uuid: "me", status: "scheduled" },
        "me",
        true,
      ),
    ).toBe(false);
  });
});
