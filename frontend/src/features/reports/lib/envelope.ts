import {
  gridColumnsFromReport,
  kpisToTiles,
  reportLinkHref,
  type GridColumn,
  type GridRow,
  type KpiTileProps,
  type ReportEnvelopeV2,
  type ReportSeriesV2,
} from "@/components/analytics";
import { routes } from "@/config/routes";

/**
 * TEC-1212: screen-side reading of the report envelope v2 (TEC-1210) on
 * top of the kit adapters (TEC-1211):
 * - series split: the legacy "cards" series is never drawn (its numbers
 *   are in kpis[]); date-keyed series are trends, the rest categories;
 * - unit bridge: the backend sends percent as a ratio (0..1) and duration
 *   in hours, the kit formats percent points and seconds;
 * - links: organization rows open the filtered services / orders list,
 *   "report" links the report detail page.
 */

/** Series key of the legacy cards (overview, *.summary). */
export const CARDS_SERIES = "cards";

const DATE_KEY = /^\d{4}-\d{2}-\d{2}/;

function isTimeSeries(s: ReportSeriesV2): boolean {
  return s.points.length > 0 && s.points.every((p) => DATE_KEY.test(p.key));
}

export type SplitSeries = {
  /** Date-keyed series sharing the buckets (one chart). */
  trends: ReportSeriesV2[];
  /** Category series (status distribution, rankings): one chart each. */
  categories: ReportSeriesV2[];
};

export function splitSeries(
  series: readonly ReportSeriesV2[] | null | undefined,
): SplitSeries {
  const trends: ReportSeriesV2[] = [];
  const categories: ReportSeriesV2[] = [];
  for (const s of series ?? []) {
    if (s.key === CARDS_SERIES) continue;
    if (isTimeSeries(s)) trends.push(s);
    else if (s.points.length > 0) categories.push(s);
  }
  return { trends, categories };
}

/** The grid columns of an envelope with the unit bridge applied. */
export function reportGridColumns(
  env: Pick<ReportEnvelopeV2, "columns">,
): GridColumn[] {
  return gridColumnsFromReport(env.columns).map((c) => {
    if (c.type === "percent") {
      return {
        ...c,
        accessor: (row: GridRow) => scaled(row[c.key], 100),
      };
    }
    if (c.type === "duration") {
      return {
        ...c,
        accessor: (row: GridRow) => scaled(row[c.key], 3600),
      };
    }
    return c;
  });
}

function scaled(v: unknown, factor: number): unknown {
  if (v === null || v === undefined || v === "") return v;
  const n = typeof v === "number" ? v : Number(v);
  return Number.isFinite(n) ? n * factor : v;
}

type AnyLink = {
  kind: string;
  uuid?: string | null;
  report?: string | null;
};

/**
 * Detail href of a row / KPI / ranking link. Organizations have no tenant
 * detail page: an orders report opens the incoming orders of that buyer,
 * every other report the services of that organization (TEC-1212
 * decision).
 */
export function reportHref(
  link: AnyLink | null | undefined,
  slug: string,
  reportKey: string,
): string | null {
  if (!link || !slug) return null;
  if (link.kind === "report") {
    return link.report ? routes.tenant.reports.detail(slug, link.report) : null;
  }
  if (link.kind === "organization") {
    if (!link.uuid) return null;
    const id = encodeURIComponent(link.uuid);
    return reportKey.startsWith("orders.")
      ? `${routes.tenant.orders.list(slug)}?buyer_org_uuid=${id}`
      : `${routes.tenant.services.list(slug)}?organization_uuid=${id}`;
  }
  return reportLinkHref(link as Parameters<typeof reportLinkHref>[0], slug);
}

/** KPI tiles with the TEC-1212 link rules. */
export function reportKpiTiles(
  env: Pick<ReportEnvelopeV2, "kpis" | "report">,
  slug: string,
): KpiTileProps[] {
  const tiles = kpisToTiles(env.kpis, slug);
  return tiles.map((tile, i) => {
    const link = env.kpis?.[i]?.link as AnyLink | null | undefined;
    const href = reportHref(link, slug, env.report);
    return href ? { ...tile, href } : tile;
  });
}

/**
 * Row href of a ranking point: ranking keys are organization / product /
 * buyer uuids depending on the report.
 */
export function rankingPointHref(
  reportKey: string,
  pointKey: string,
  slug: string,
): string | undefined {
  if (!/^[0-9a-f-]{36}$/i.test(pointKey)) return undefined;
  if (
    reportKey === "orders.by_buyer" ||
    reportKey === "services.by_org" ||
    reportKey === "dealers.top_by_warranty"
  ) {
    return (
      reportHref({ kind: "organization", uuid: pointKey }, slug, reportKey) ??
      undefined
    );
  }
  if (reportKey === "services.top_products") {
    return (
      reportHref({ kind: "product", uuid: pointKey }, slug, reportKey) ??
      undefined
    );
  }
  return undefined;
}

/** Envelope as the kit adapters read it (period_info → period). */
export function asV2(env: unknown): ReportEnvelopeV2 {
  const e = env as ReportEnvelopeV2 & { period_info?: unknown };
  return {
    ...e,
    period:
      (e.period_info as ReportEnvelopeV2["period"] | undefined) ??
      (typeof e.period === "object" ? e.period : null),
  };
}
