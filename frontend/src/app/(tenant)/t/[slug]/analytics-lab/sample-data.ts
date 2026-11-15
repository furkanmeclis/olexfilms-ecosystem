/**
 * Deterministic sample data for the analytics lab (TEC-1211). Shaped like
 * the report envelope v2 (TEC-1210) so the adapters are exercised too.
 */
import type {
  HeatmapCell,
  ReportEnvelopeV2,
  ReportRowV2,
} from "@/components/analytics";

function rng(seed: number) {
  let s = seed >>> 0;
  return () => {
    s = (s * 1664525 + 1013904223) >>> 0;
    return s / 0x100000000;
  };
}

const DAY = 86_400_000;
const END = Date.UTC(2026, 10, 14);

function isoDay(offset: number) {
  return new Date(END - offset * DAY).toISOString().slice(0, 10);
}

export function sampleEnvelope(days = 30): ReportEnvelopeV2 {
  const r = rng(42);
  const services = [] as {
    key: string;
    label: string;
    value: number;
    previous: number;
  }[];
  const warranties = [] as typeof services;
  for (let i = days - 1; i >= 0; i--) {
    const key = isoDay(i);
    const weekday = new Date(key).getUTCDay();
    const base = weekday === 0 ? 8 : weekday === 6 ? 18 : 30;
    services.push({
      key,
      label: key,
      value: Math.round(base + r() * 14 + (days - i) * 0.3),
      previous: Math.round(base + r() * 12),
    });
    warranties.push({
      key,
      label: key,
      value: Math.round(base * 0.6 + r() * 8),
      previous: Math.round(base * 0.55 + r() * 8),
    });
  }
  const sum = (xs: { value: number }[]) => xs.reduce((a, x) => a + x.value, 0);
  const psum = (xs: { previous: number }[]) =>
    xs.reduce((a, x) => a + x.previous, 0);
  const pct = (a: number, b: number) => (b === 0 ? null : ((a - b) / b) * 100);
  return {
    report: "services.summary",
    kind: "summary",
    title: "Hizmetler",
    period: {
      from: isoDay(days - 1),
      to: isoDay(0),
      granularity: "day",
      compare: "previous",
      previous_from: isoDay(days * 2 - 1),
      previous_to: isoDay(days),
    },
    kpis: [
      {
        key: "services",
        label: "Hizmet",
        unit: "count",
        value: sum(services),
        previous: psum(services),
        delta_pct: pct(sum(services), psum(services)),
        good: "up",
        spark: services.map((p) => p.value),
        link: null,
      },
      {
        key: "revenue",
        label: "Ciro",
        unit: "TRY",
        value: 1_284_550.75,
        previous: 1_402_120.1,
        delta_pct: pct(1_284_550.75, 1_402_120.1),
        good: "up",
        spark: services.map((p) => p.value * 1400),
      },
      {
        key: "prep_hours",
        label: "Ort. hazırlama süresi",
        unit: "hours",
        value: 5.4,
        previous: 6.9,
        delta_pct: pct(5.4, 6.9),
        good: "down",
        spark: services.map((p) => 4 + (p.value % 5)),
      },
      {
        key: "open_warranties",
        label: "Açık garanti",
        unit: "count",
        value: 37,
        previous: 37,
        delta_pct: 0,
        good: "down",
        spark: warranties.map((p) => p.value),
      },
    ],
    series: [
      {
        key: "services",
        label: "Hizmet",
        unit: "count",
        points: services,
        total: sum(services),
        previous_total: psum(services),
        delta_pct: pct(sum(services), psum(services)),
      },
      {
        key: "warranties",
        label: "Garanti",
        unit: "count",
        points: warranties,
        total: sum(warranties),
        previous_total: psum(warranties),
        delta_pct: pct(sum(warranties), psum(warranties)),
      },
    ],
    columns: [
      { key: "dealer", label: "Bayi", type: "org" },
      { key: "city", label: "Şehir", type: "text" },
      { key: "status", label: "Durum", type: "status" },
      { key: "services", label: "Hizmet", type: "number", aggregate: "sum" },
      {
        key: "revenue",
        label: "Ciro",
        type: "money",
        aggregate: "sum",
        currency: "TRY",
      },
      { key: "margin", label: "Marj", type: "percent", aggregate: "avg" },
      { key: "prep", label: "Hazırlama", type: "duration", aggregate: "avg" },
      { key: "last_service", label: "Son hizmet", type: "date" },
    ],
    rows: sampleRows(2400),
  };
}

const CITIES = [
  "İstanbul",
  "Ankara",
  "İzmir",
  "Bursa",
  "Antalya",
  "Sofya",
  "Bakü",
  "Atina",
];
const STATUSES = [
  { key: "active", label: "Aktif", tone: "done" },
  { key: "onboarding", label: "Kurulumda", tone: "progress" },
  { key: "paused", label: "Askıda", tone: "neutral" },
  { key: "overdue", label: "Gecikmiş", tone: "problem" },
] as const;

export function sampleRows(n: number): ReportRowV2[] {
  const r = rng(7);
  return Array.from({ length: n }, (_, i) => {
    const status = STATUSES[Math.floor(r() * STATUSES.length)];
    return {
      dealer: `Olex ${String(i + 1).padStart(4, "0")}`,
      city: CITIES[Math.floor(r() * CITIES.length)],
      status: status.key,
      services: Math.round(r() * 220),
      revenue: Math.round(r() * 950_000) / 1 + Math.round(r() * 100) / 100,
      margin: Math.round(r() * 4200) / 100,
      prep: Math.round(1800 + r() * 30_000),
      last_service: isoDay(Math.floor(r() * 90)),
      _link: {
        kind: "customer",
        uuid: `00000000-0000-4000-8000-${String(i).padStart(12, "0")}`,
      },
      _status: { status: { label: status.label, tone: status.tone } },
    } satisfies ReportRowV2;
  });
}

export const PRODUCT_SERIES = [
  "PPF Ultra",
  "PPF Matte",
  "Cam filmi",
  "Seramik",
  "Renk değişim",
  "Far filmi",
  "İç koruma",
  "Jant",
  "Detaylı yıkama",
  "Cila",
].map((label, i) => ({ key: `p${i + 1}`, label }));

export function productMonths() {
  const r = rng(99);
  return Array.from({ length: 12 }, (_, m) => {
    const key = new Date(Date.UTC(2025, 11 + m, 1)).toISOString().slice(0, 10);
    const row: Record<string, number | string> = { key };
    PRODUCT_SERIES.forEach((s, i) => {
      row[s.key] = Math.round((40 - i * 3) * (0.7 + r() * 0.6));
    });
    return row as { key: string } & Record<string, number | string>;
  });
}

export function sampleHeatmap(): HeatmapCell[] {
  const r = rng(3);
  const cells: HeatmapCell[] = [];
  for (let day = 1; day <= 7; day++) {
    for (let hour = 0; hour < 24; hour++) {
      const open = hour >= 9 && hour <= 19 && day !== 7;
      const peak = hour >= 11 && hour <= 16 ? 1.6 : 1;
      const weekend = day === 6 ? 0.7 : 1;
      cells.push({
        day,
        hour,
        value: open
          ? Math.round((6 + r() * 10) * peak * weekend)
          : Math.round(r() * 2),
      });
    }
  }
  return cells;
}
