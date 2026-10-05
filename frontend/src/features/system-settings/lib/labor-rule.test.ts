import { describe, expect, it } from "vitest";

import {
  laborChanges,
  splitLaborSettings,
  toLaborDraft,
  validateLaborDraft,
  type LaborSettings,
} from "@/features/system-settings/lib/labor-rule";
import type { SystemSetting } from "@/features/system-settings/services/system-settings.service";

function setting(patch: Partial<SystemSetting>): SystemSetting {
  return {
    key: "x",
    group: "general",
    kind: "string",
    default: "",
    description: "",
    value: "",
    is_default: true,
    schema_version: 1,
    ...patch,
  };
}

const rule = setting({
  key: "warranty_claims.labor_rule",
  value: "dealer",
  default: "dealer",
});
const amount = setting({
  key: "warranty_claims.labor_amount",
  kind: "int",
  value: 0,
  default: 0,
});
const percent = setting({
  key: "warranty_claims.labor_share_percent",
  kind: "int",
  value: 50,
  default: 50,
  min: 0,
  max: 100,
});
const labor: LaborSettings = { rule, amount, percent };

describe("splitLaborSettings", () => {
  it("stays out of the way until the catalog ships the rule key", () => {
    const other = setting({ key: "contract_grace_days" });
    expect(splitLaborSettings([other])).toEqual({
      labor: null,
      rest: [other],
    });
  });

  it("moves the labor keys to the card", () => {
    const other = setting({ key: "contract_grace_days" });
    const { labor: l, rest } = splitLaborSettings([
      other,
      rule,
      amount,
      percent,
    ]);
    expect(l).toEqual({ rule, amount, percent });
    expect(rest).toEqual([other]);
  });
});

describe("validateLaborDraft", () => {
  it("checks the percentage (0–100) only for the shared rule", () => {
    const base = toLaborDraft(labor);
    expect(validateLaborDraft({ ...base, percent: "150" }, labor)).toEqual({});
    expect(
      validateLaborDraft({ ...base, rule: "shared", percent: "150" }, labor),
    ).toEqual({ percent: "range" });
    expect(
      validateLaborDraft({ ...base, rule: "shared", percent: "-1" }, labor),
    ).toEqual({ percent: "range" });
    expect(
      validateLaborDraft({ ...base, rule: "shared", percent: "12.5" }, labor),
    ).toEqual({ percent: "integer" });
    expect(
      validateLaborDraft({ ...base, rule: "shared", percent: "100" }, labor),
    ).toEqual({});
    expect(
      validateLaborDraft({ ...base, rule: "shared", percent: "0" }, labor),
    ).toEqual({});
  });

  it("checks the amount", () => {
    const base = toLaborDraft(labor);
    expect(validateLaborDraft({ ...base, amount: "-5" }, labor)).toEqual({
      amount: "min",
    });
    expect(validateLaborDraft({ ...base, amount: "12.5" }, labor)).toEqual({
      amount: "number",
    });
    const decimal = {
      ...labor,
      amount: { ...amount, kind: "string" as const },
    };
    expect(validateLaborDraft({ ...base, amount: "12,50" }, decimal)).toEqual(
      {},
    );
  });
});

describe("laborChanges", () => {
  it("sends only changed keys, the percentage only when shared", () => {
    const base = toLaborDraft(labor);
    expect(laborChanges(base, labor)).toEqual([]);
    expect(
      laborChanges({ ...base, rule: "center", percent: "70" }, labor),
    ).toEqual([{ key: "warranty_claims.labor_rule", value: "center" }]);
    expect(
      laborChanges({ rule: "shared", amount: "300", percent: "40" }, labor),
    ).toEqual([
      { key: "warranty_claims.labor_rule", value: "shared" },
      { key: "warranty_claims.labor_amount", value: 300 },
      { key: "warranty_claims.labor_share_percent", value: 40 },
    ]);
  });
});
