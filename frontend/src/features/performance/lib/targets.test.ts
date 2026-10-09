import { describe, expect, it } from "vitest";

import {
  achievementWidth,
  bonusApproval,
  bonusRuleInput,
  canOpenTask,
  monthColumnId,
  monthOfColumn,
  shiftMonth,
  targetOrganizationOptions,
  teamMonths,
  teamRows,
} from "./targets";

describe("targetOrganizationOptions", () => {
  const orgs = [
    { uuid: "center", name: "Center", type: "center" },
    {
      uuid: "dist",
      name: "Dist",
      type: "distributor",
      parent: { uuid: "center" },
    },
    {
      uuid: "d1",
      name: "Own dealer",
      type: "dealer",
      parent: { uuid: "dist" },
    },
    { uuid: "d2", name: "Other dealer", type: "dealer", parent: { uuid: "x" } },
  ];

  it("lets the center pick any distributor or dealer", () => {
    expect(
      targetOrganizationOptions(orgs, "center", "center").map((o) => o.uuid),
    ).toEqual(["dist", "d1", "d2"]);
  });

  it("lets a distributor pick only its own dealers", () => {
    expect(
      targetOrganizationOptions(orgs, "distributor", "dist").map((o) => o.uuid),
    ).toEqual(["d1"]);
  });

  it("gives a dealer nothing", () => {
    expect(targetOrganizationOptions(orgs, "dealer", "d1")).toEqual([]);
  });
});

describe("bonusApproval", () => {
  it("approves the calculated amount without an override", () => {
    expect(bonusApproval("500.00", "500", "")).toEqual({ body: {} });
    expect(bonusApproval("500.00", "", " ok ")).toEqual({
      body: { note: "ok" },
    });
  });

  it("requires a note when the amount changes", () => {
    expect(bonusApproval("500.00", "450", "  ")).toEqual({
      error: "note_required",
    });
    expect(bonusApproval("500.00", "450", "late")).toEqual({
      body: { amount: "450", note: "late" },
    });
  });

  it("rejects a non-positive amount", () => {
    expect(bonusApproval("500.00", "0", "x")).toEqual({
      error: "amount_invalid",
    });
    expect(bonusApproval("500.00", "abc", "x")).toEqual({
      error: "amount_invalid",
    });
  });
});

describe("bonusRuleInput", () => {
  const base = {
    name: "Hedef",
    metric: "services_count" as const,
    threshold_pct: "100",
    kind: "fixed" as const,
    amount: "500",
    percent: "",
    active: true,
  };

  it("sends the amount of a fixed rule and the percent of a revenue rule", () => {
    expect(bonusRuleInput(base)).toEqual({
      name: "Hedef",
      metric: "services_count",
      threshold_pct: "100",
      kind: "fixed",
      amount: "500",
      active: true,
    });
    expect(
      bonusRuleInput({ ...base, kind: "percent_of_revenue", percent: "5" }),
    ).toMatchObject({ kind: "percent_of_revenue", percent: "5" });
  });

  it("rejects missing or out of range values", () => {
    expect(bonusRuleInput({ ...base, name: " " })).toBeNull();
    expect(bonusRuleInput({ ...base, amount: "" })).toBeNull();
    expect(bonusRuleInput({ ...base, threshold_pct: "1001" })).toBeNull();
    expect(
      bonusRuleInput({ ...base, kind: "percent_of_revenue", percent: "101" }),
    ).toBeNull();
  });
});

describe("team grid", () => {
  it("shifts months across years", () => {
    expect(shiftMonth("2026-01", -1)).toBe("2025-12");
    expect(shiftMonth("2026-11", 3)).toBe("2027-02");
    expect(teamMonths("2026-10")).toEqual([
      "2026-08",
      "2026-09",
      "2026-10",
      "2026-11",
      "2026-12",
      "2027-01",
    ]);
    expect(monthOfColumn(monthColumnId("2026-10"))).toBe("2026-10");
    expect(monthOfColumn("name")).toBeNull();
  });

  it("puts every member in a row with the targets of the metric", () => {
    const rows = teamRows(
      [
        { uuid: "u1", name: "Ayşe" },
        { uuid: "u2", name: "Mehmet" },
      ],
      [
        {
          uuid: "t1",
          user_uuid: "u1",
          period: "2026-10",
          metric: "services_count",
          value: "10",
        },
        {
          uuid: "t2",
          user_uuid: "u1",
          period: "2026-10",
          metric: "service_revenue",
          value: "5000",
        },
        {
          uuid: "t3",
          user_uuid: "u9",
          user_name: "Former",
          period: "2026-09",
          metric: "services_count",
          value: "4",
        },
      ],
      "services_count",
    );
    expect(rows.map((r) => r.uuid)).toEqual(["u1", "u2", "u9"]);
    expect(rows[0].months["2026-10"]?.uuid).toBe("t1");
    expect(rows[1].months).toEqual({});
    expect(rows[2].name).toBe("Former");
  });

  it("caps the achievement bar", () => {
    expect(achievementWidth("150")).toBe(100);
    expect(achievementWidth("42.5")).toBe(42.5);
    expect(achievementWidth(null)).toBe(0);
  });
});

describe("canOpenTask", () => {
  it("is center only", () => {
    expect(canOpenTask("center")).toBe(true);
    expect(canOpenTask("distributor")).toBe(false);
    expect(canOpenTask("dealer")).toBe(false);
  });
});
