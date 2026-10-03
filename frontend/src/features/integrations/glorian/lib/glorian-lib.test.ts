import { describe, expect, it } from "vitest";

import {
  DRIFT_CATEGORIES,
  countEntries,
  driftRowEntries,
  parseDriftReport,
} from "./drift";
import { glorianFormSchema, toFormValues, toPutBody } from "./form";

const t = (key: string) => key;
const v = (key: string) => `integrations.glorian.validation.${key}`;

const valid = {
  base_url: "https://hub.example.com",
  api_key: "",
  default_warehouse_uuid: "",
  active: false,
};

function errors(apiKeySet: boolean, values: Partial<typeof valid>) {
  const res = glorianFormSchema(t, apiKeySet).safeParse({
    ...valid,
    ...values,
  });
  if (res.success) return {};
  return Object.fromEntries(
    res.error.issues.map((i) => [i.path.join("."), i.message]),
  );
}

describe("glorian connection form schema (TEC-274)", () => {
  it("accepts a valid inactive connection without a key", () => {
    expect(errors(false, {})).toEqual({});
  });

  it("requires an http(s) base_url", () => {
    expect(errors(false, { base_url: "  " })).toEqual({
      base_url: v("required"),
    });
    expect(errors(false, { base_url: "ftp://hub.example.com" })).toEqual({
      base_url: v("url"),
    });
    expect(errors(false, { base_url: "not a url" })).toEqual({
      base_url: v("url"),
    });
    expect(
      errors(false, { base_url: `https://h.example.com/${"a".repeat(500)}` }),
    ).toEqual({ base_url: v("too_long") });
  });

  it("checks the default warehouse UUID and the key length", () => {
    expect(errors(false, { default_warehouse_uuid: "WH-1" })).toEqual({
      default_warehouse_uuid: v("uuid"),
    });
    expect(
      errors(false, {
        default_warehouse_uuid: "0b9c4c1e-0000-4000-8000-000000000001",
      }),
    ).toEqual({});
    expect(errors(false, { api_key: "k".repeat(501) })).toEqual({
      api_key: v("too_long"),
    });
  });

  it("needs a key to activate unless one is stored", () => {
    expect(errors(false, { active: true })).toEqual({
      api_key: v("api_key_required"),
    });
    expect(errors(false, { active: true, api_key: "secret" })).toEqual({});
    expect(errors(true, { active: true })).toEqual({});
  });

  it("never pre-fills the key and leaves a blank key out of the body", () => {
    const values = toFormValues({
      configured: true,
      uuid: "0b9c4c1e-0000-4000-8000-0000000000aa",
      key: "glorian",
      base_url: "https://hub.example.com",
      active: true,
      api_version: "1",
      default_warehouse_uuid: null,
      api_key_set: true,
      api_key_masked: "********",
      created_at: null,
      updated_at: null,
    });
    expect(values.api_key).toBe("");
    expect(toPutBody(values)).toEqual({
      base_url: "https://hub.example.com",
      active: true,
      api_version: "1",
      default_warehouse_uuid: null,
    });
    expect(
      toPutBody({
        ...values,
        api_key: " new-key ",
        default_warehouse_uuid: "0b9c4c1e-0000-4000-8000-000000000001",
      }),
    ).toEqual({
      base_url: "https://hub.example.com",
      active: true,
      api_version: "1",
      default_warehouse_uuid: "0b9c4c1e-0000-4000-8000-000000000001",
      api_key: "new-key",
    });
  });
});

describe("drift report (glorian.ReconcileCounts)", () => {
  it("reads the five category counts and their rows", () => {
    const report = parseDriftReport({
      only_remote: 2,
      only_local: 1,
      status_drift: 0,
      product_drift: 3,
      owner_drift: 4,
      remote: 120,
      local: 119,
      pages: 3,
      skipped: 1,
      details: {
        only_remote: [{ barcode: "GL-1", remote_id: "r1" }],
        only_local: [{ barcode: "GL-2", unit_id: "u2" }],
        status_drift: [],
        product_drift: [],
        owner_drift: [],
      },
    });
    expect(report.counts).toEqual({
      only_remote: 2,
      only_local: 1,
      status_drift: 0,
      product_drift: 3,
      owner_drift: 4,
    });
    expect(report.total).toBe(10);
    expect(report.remote).toBe(120);
    expect(report.local).toBe(119);
    expect(report.skipped).toBe(1);
    expect(report.details.only_remote).toHaveLength(1);
    expect(report.details.owner_drift).toEqual([]);
  });

  it("tolerates a running run with empty counts", () => {
    const report = parseDriftReport({});
    expect(report.total).toBe(0);
    for (const cat of DRIFT_CATEGORIES) {
      expect(report.counts[cat]).toBe(0);
      expect(report.details[cat]).toEqual([]);
    }
    expect(parseDriftReport(null).total).toBe(0);
  });

  it("lists only the set fields of a row", () => {
    expect(
      driftRowEntries({
        barcode: "GL-1",
        unit_id: "u1",
        remote_status: "sold",
        local_external_status: "",
        local_owner_id: 0,
      }),
    ).toEqual([
      ["unit_id", "u1"],
      ["remote_status", "sold"],
    ]);
  });

  it("keeps the numeric totals of other runs", () => {
    expect(countEntries({ created: 3, updated: 1, note: "x" })).toEqual([
      ["created", 3],
      ["updated", 1],
    ]);
    expect(countEntries(null)).toEqual([]);
  });
});
