"use client";

import { Suspense } from "react";
import { useParams } from "next/navigation";

import { AnalyticsLab } from "./analytics-lab";

/**
 * TEC-1211 analytics kit showcase. Not in any menu; open to super admins
 * (and to everyone in development builds).
 */
export default function Page() {
  const params = useParams<{ slug: string }>();
  return (
    <Suspense fallback={null}>
      <AnalyticsLab slug={String(params.slug ?? "")} />
    </Suspense>
  );
}
