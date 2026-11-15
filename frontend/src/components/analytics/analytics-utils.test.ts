import { describe, expect, it } from "vitest";

import { createFormatter } from "@/lib/i18n/format";

import {
  categoriesFromSeries,
  envelopeGranularity,
  gridColumnsFromReport,
  kpisToTiles,
  normalizeColumnType,
  reportExportPath,
  reportLinkHref,
  timeSeriesFromReport,
} from "./adapters";
import {
  deltaPct,
  deltaTone,
  formatDeltaPct,
  formatDuration,
  formatValue,
  unitFormat,
} from "./format";
import {
  OTHER_KEY,
  assignColorSlots,
  categoricalColor,
  divergingColor,
  foldCategories,
  foldSeries,
  sequentialStep,
} from "./palette";
import {
  parsePeriodParams,
  periodToQuery,
  suggestedGranularity,
  writePeriodParams,
} from "./period";
import { sparklinePath } from "./sparkline";
import { previousKey } from "./types";

const tr = createFormatter({ locale: "tr", timeZone: "Europe/Istanbul" });
const en = createFormatter({ locale: "en", timeZone: "Europe/Istanbul" });
const durations = { day: "g", hour: "sa", minute: "dk", second: "sn" };
const norm = (s: string) => s.replace(/ | /g, " ");

describe("format", () => {
  it("formats money with the column currency", () => {
    expect(
      norm(formatValue(1234.5, { kind: "money", currency: "EUR" }, en)),
    ).toBe("€1,234.50");
  });
  it("treats percent values as points", () => {
    expect(norm(formatValue(12.5, { kind: "percent" }, tr))).toBe("%12,5");
    expect(norm(formatValue(12.5, { kind: "percent" }, en))).toBe("12.5%");
  });
  it("formats durations with the two largest units", () => {
    expect(formatDuration(8100, tr, durations)).toBe("2 sa 15 dk");
    expect(formatDuration(90061, tr, durations)).toBe("1 g 1 sa");
    expect(formatDuration(0, tr, durations)).toBe("0 sn");
    expect(
      formatValue(
        1.5,
        { kind: "duration", durationScale: 3600 },
        tr,
        durations,
      ),
    ).toBe("1 sa 30 dk");
  });
  it("shows a dash for missing values", () => {
    expect(formatValue(null, { kind: "money" }, tr)).toBe("—");
    expect(formatValue(Number.NaN, undefined, tr)).toBe("—");
  });
  it("signs deltas and reads them by the good direction", () => {
    expect(norm(formatDeltaPct(12.34, en))).toBe("+12.3%");
    expect(norm(formatDeltaPct(-4, en))).toBe("-4%");
    expect(deltaPct(110, 100)).toBeCloseTo(10);
    expect(deltaPct(5, 0)).toBeNull();
    expect(deltaTone(10, "up")).toBe("good");
    expect(deltaTone(10, "down")).toBe("bad");
    expect(deltaTone(-3, "down")).toBe("good");
    expect(deltaTone(0, "up")).toBe("neutral");
    expect(deltaTone(null)).toBe("neutral");
  });
  it("maps envelope units to formats", () => {
    expect(unitFormat("count")).toEqual({
      kind: "number",
      maximumFractionDigits: 0,
    });
    expect(unitFormat("TRY")).toEqual({ kind: "money", currency: "TRY" });
    expect(unitFormat("hours")).toEqual({
      kind: "duration",
      durationScale: 3600,
    });
    expect(unitFormat("percent")).toEqual({ kind: "percent" });
  });
});

describe("palette", () => {
  it("never cycles: slot 9 is Other", () => {
    expect(categoricalColor(0)).toBe("var(--viz-cat-1)");
    expect(categoricalColor(7)).toBe("var(--viz-cat-8)");
    expect(categoricalColor(8)).toBe("var(--viz-other)");
  });
  it("keeps colours with identity when series are filtered", () => {
    const first = assignColorSlots(["a", "b", "c"]);
    expect(first).toEqual({ a: 0, b: 1, c: 2 });
    const filtered = assignColorSlots(["a", "c"], first);
    expect(filtered.c).toBe(2);
    const withNew = assignColorSlots(["c", "d"], filtered);
    expect(withNew.d).toBe(3);
  });
  it("folds the 9th and later series into Other", () => {
    const series = Array.from({ length: 10 }, (_, i) => ({
      key: `s${i}`,
      label: `S${i}`,
    }));
    const row = Object.fromEntries(series.map((s, i) => [s.key, i + 1]));
    const { rows, series: out } = foldSeries([row], series, "Other");
    expect(out).toHaveLength(9);
    expect(out[8]).toEqual({ key: OTHER_KEY, label: "Other" });
    expect(rows[0][OTHER_KEY]).toBe(9 + 10);
    expect(rows[0].s8).toBeUndefined();
    const cats = foldCategories(
      Array.from({ length: 9 }, (_, i) => ({ label: `c${i}`, value: i })),
      "Other",
    );
    expect(cats).toHaveLength(9);
    expect(cats[8]).toMatchObject({ key: OTHER_KEY, value: 0 });
  });
  it("quantises sequential and diverging scales", () => {
    expect(sequentialStep(0)).toBe(0);
    expect(sequentialStep(1)).toBe(12);
    expect(sequentialStep(0.5)).toBe(6);
    expect(divergingColor(-1)).toBe("var(--viz-div-1)");
    expect(divergingColor(0)).toBe("var(--viz-div-6)");
    expect(divergingColor(1)).toBe("var(--viz-div-11)");
  });
});

describe("period", () => {
  it("round-trips the URL with API parameter names", () => {
    const state = parsePeriodParams(
      new URLSearchParams("period=90d&granularity=week&compare=previous&tab=x"),
    );
    expect(state).toMatchObject({
      preset: "90d",
      granularity: "week",
      compare: "previous",
    });
    expect(periodToQuery(state)).toEqual({
      period: "90d",
      granularity: "week",
      compare: "previous",
    });
    const written = writePeriodParams("tab=x&period=7d", state);
    expect(written.get("tab")).toBe("x");
    expect(written.get("period")).toBe("90d");
  });
  it("parses and orders custom ranges", () => {
    const state = parsePeriodParams(
      new URLSearchParams("range_from=2026-03-10&range_to=2026-01-01"),
    );
    expect(state).toMatchObject({
      preset: "custom",
      from: "2026-01-01",
      to: "2026-03-10",
    });
    expect(periodToQuery(state)).toEqual({
      range_from: "2026-01-01",
      range_to: "2026-03-10",
    });
    expect(suggestedGranularity(state)).toBe("day");
    expect(suggestedGranularity({ preset: "12m", compare: "none" })).toBe(
      "month",
    );
  });
  it("drops defaults and ignores junk", () => {
    const state = parsePeriodParams(
      new URLSearchParams("period=bogus&compare=nope"),
    );
    expect(state).toMatchObject({ preset: "30d", compare: "none" });
    expect(writePeriodParams("", state).toString()).toBe("");
  });
});

describe("adapters", () => {
  const series = [
    {
      key: "services",
      label: "Services",
      unit: "count",
      points: [
        { key: "2026-11-02", label: "2", value: 5, previous: 4 },
        { key: "2026-11-01", label: "1", value: 3, previous: null },
      ],
    },
    {
      key: "warranties",
      label: "Warranties",
      unit: "count",
      points: [{ key: "2026-11-01", label: "1", value: 1 }],
    },
  ];
  it("merges series into sorted chart rows with previous values", () => {
    const out = timeSeriesFromReport(series);
    expect(out.data.map((d) => d.key)).toEqual(["2026-11-01", "2026-11-02"]);
    expect(out.data[0]).toMatchObject({ services: 3, warranties: 1 });
    expect(out.data[1][previousKey("services")]).toBe(4);
    expect(out.hasPrevious).toBe(true);
    expect(categoriesFromSeries(series[0])).toHaveLength(2);
  });
  it("maps KPIs, links and columns", () => {
    const [tile] = kpisToTiles(
      [
        {
          key: "rev",
          label: "Revenue",
          unit: "EUR",
          value: 10,
          previous: 8,
          delta_pct: 25,
          good: "up",
          spark: [1, 2],
          link: { kind: "order", uuid: "u1" },
        },
      ],
      "olex",
    );
    expect(tile).toMatchObject({
      id: "rev",
      deltaPct: 25,
      format: { kind: "money", currency: "EUR" },
      href: "/t/olex/orders/u1",
    });
    expect(
      reportLinkHref({ kind: "organization", uuid: "x" }, "olex"),
    ).toBeNull();
    expect(
      gridColumnsFromReport([
        { key: "a", label: "A", type: "money", aggregate: "sum" },
      ])[0],
    ).toMatchObject({ type: "money", aggregate: "sum" });
    expect(normalizeColumnType("string")).toBe("text");
    expect(
      envelopeGranularity({
        period: { from: "", to: "", granularity: "week" },
      }),
    ).toBe("week");
    expect(envelopeGranularity({ period: "30d", granularity: "day" })).toBe(
      "day",
    );
  });
  it("builds the backend export path", () => {
    expect(
      reportExportPath(
        "services.summary",
        { period: "30d", compare: "", granularity: null },
        "xlsx",
      ),
    ).toBe("/api/v1/reports/services.summary/export?period=30d&format=xlsx");
  });
});

describe("sparkline", () => {
  it("breaks the line on gaps", () => {
    const { line, last } = sparklinePath([1, 2, null, 3], 30, 10);
    expect(line.match(/M/g)).toHaveLength(2);
    expect(last).not.toBeNull();
    expect(sparklinePath([], 30, 10).line).toBe("");
  });
});
