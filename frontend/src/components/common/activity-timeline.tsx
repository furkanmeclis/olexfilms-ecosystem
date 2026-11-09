"use client";

import type { ReactNode } from "react";

import { useLocale } from "@/providers/locale-provider";

/** One step of a record: who did what, and when (TEC-1060). */
export type ActivityStep = {
  key: string;
  at: string;
  /** The user's name; null for a system or migrated step (label only). */
  user: string | null;
  label: string;
  detail?: ReactNode;
};

/**
 * TEC-1060 (RV-5.4.1): a compact "user · action · time" list, newest
 * first. Order, transfer and stock entry details share it.
 */
export function ActivityTimeline({
  steps,
  testId = "activity-timeline",
  stepTestId = "activity-step",
}: {
  steps: ActivityStep[];
  testId?: string;
  stepTestId?: string;
}) {
  const { format } = useLocale();
  const rows = [...steps].reverse();
  if (rows.length === 0) {
    return <p className="text-muted-foreground text-sm">—</p>;
  }
  return (
    <ol className="space-y-3" data-testid={testId}>
      {rows.map((s) => (
        <li key={s.key} className="border-s-2 ps-3" data-testid={stepTestId}>
          <div className="text-sm">
            {s.user ? (
              <>
                <span className="font-medium">{s.user}</span>
                <span className="text-muted-foreground"> · </span>
              </>
            ) : null}
            <span>{s.label}</span>
          </div>
          {s.detail ? <div className="text-sm">{s.detail}</div> : null}
          <div className="text-muted-foreground text-xs">
            {format.dateTime(s.at)}
          </div>
        </li>
      ))}
    </ol>
  );
}
