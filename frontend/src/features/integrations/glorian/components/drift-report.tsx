"use client";

import {
  DRIFT_CATEGORIES,
  driftRowEntries,
  type DriftReport,
} from "@/features/integrations/glorian/lib/drift";
import { useLocale } from "@/providers/locale-provider";

/** The five drift categories of a reconcile run with their first rows. */
export function DriftReportView({ report }: { report: DriftReport }) {
  const { t } = useLocale();

  return (
    <div className="space-y-4" data-testid="drift-report">
      <div className="text-muted-foreground flex flex-wrap gap-x-6 gap-y-1 text-sm">
        <span>
          {t("integrations.glorian.drift.remote")}: {report.remote}
        </span>
        <span>
          {t("integrations.glorian.drift.local")}: {report.local}
        </span>
        <span>
          {t("integrations.glorian.drift.skipped")}: {report.skipped}
        </span>
        <span>
          {t("integrations.glorian.drift.total")}: {report.total}
        </span>
      </div>

      <dl className="grid gap-3 sm:grid-cols-3 lg:grid-cols-5">
        {DRIFT_CATEGORIES.map((cat) => (
          <div
            key={cat}
            className="rounded-lg border p-3"
            data-testid={`drift-count-${cat}`}
          >
            <dt className="text-muted-foreground text-xs">
              {t(`integrations.glorian.drift.category.${cat}`)}
            </dt>
            <dd
              className={
                report.counts[cat] > 0
                  ? "text-destructive text-2xl font-semibold"
                  : "text-2xl font-semibold"
              }
            >
              {report.counts[cat]}
            </dd>
          </div>
        ))}
      </dl>

      {report.total === 0 ? (
        <p className="text-sm">{t("integrations.glorian.drift.none")}</p>
      ) : (
        DRIFT_CATEGORIES.filter((cat) => report.details[cat].length > 0).map(
          (cat) => (
            <details key={cat} className="rounded-lg border p-3">
              <summary className="cursor-pointer text-sm font-medium">
                {t(`integrations.glorian.drift.category.${cat}`)} (
                {report.counts[cat]})
              </summary>
              <p className="text-muted-foreground mt-1 text-xs">
                {t(`integrations.glorian.drift.hint.${cat}`)}
              </p>
              <ul className="mt-2 space-y-1 text-sm">
                {report.details[cat].map((row, i) => (
                  <li
                    key={`${row.barcode}-${i}`}
                    className="flex flex-wrap gap-x-3 gap-y-0.5"
                  >
                    <span className="font-mono font-medium">{row.barcode}</span>
                    {driftRowEntries(row).map(([field, value]) => (
                      <span key={field} className="text-muted-foreground">
                        {t(`integrations.glorian.drift.field.${field}`)}:{" "}
                        <span className="font-mono">{value}</span>
                      </span>
                    ))}
                  </li>
                ))}
              </ul>
              {report.counts[cat] > report.details[cat].length ? (
                <p className="text-muted-foreground mt-2 text-xs">
                  {t("integrations.glorian.drift.truncated", {
                    shown: report.details[cat].length,
                    total: report.counts[cat],
                  })}
                </p>
              ) : null}
            </details>
          ),
        )
      )}
    </div>
  );
}
