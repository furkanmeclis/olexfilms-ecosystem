"use client";

import type { ReactNode } from "react";

import {
  PeriodBar,
  usePeriodState,
  type PeriodState,
} from "@/components/analytics";
import { PageHeader } from "@/components/layout";
import type { ReportQuery } from "@/features/reports";

/** Default period of every overview: 30 days against the previous 30. */
export const OVERVIEW_PERIOD: PeriodState = {
  preset: "30d",
  compare: "previous",
};

/**
 * TEC-1212: header + the one PeriodBar of an overview. The period lives in
 * the URL; every panel gets the same API query.
 */
export function OverviewShell({
  title,
  description,
  icon,
  actions,
  testId,
  children,
}: {
  title: string;
  description?: string;
  icon?: ReactNode;
  actions?: ReactNode;
  testId: string;
  children: (query: ReportQuery) => ReactNode;
}) {
  const [period, setPeriod, query] = usePeriodState(OVERVIEW_PERIOD);
  return (
    <div className="flex flex-col gap-6" data-testid={testId}>
      <PageHeader
        title={title}
        description={description}
        icon={icon}
        actions={actions}
      />
      <div data-testid="overview-period">
        <PeriodBar value={period} onChange={setPeriod} />
      </div>
      {children(query)}
    </div>
  );
}

/** Section heading inside an overview. */
export function OverviewSection({
  title,
  children,
  testId,
}: {
  title: string;
  children: ReactNode;
  testId?: string;
}) {
  return (
    <section className="flex flex-col gap-3" data-testid={testId}>
      <h2 className="text-lg font-semibold">{title}</h2>
      {children}
    </section>
  );
}
