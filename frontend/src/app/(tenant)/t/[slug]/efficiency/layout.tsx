"use client";

import type { ReactNode } from "react";
import { useParams } from "next/navigation";

import { FeatureGuard } from "@/features/modules";

/**
 * The efficiency pages need the efficiency add-on (TEC-516): a hidden menu
 * item must not stay reachable by URL once the module is off.
 */
export default function EfficiencyLayout({
  children,
}: {
  children: ReactNode;
}) {
  const params = useParams<{ slug: string }>();
  return (
    <FeatureGuard slug={String(params.slug ?? "")} feature="efficiency">
      {children}
    </FeatureGuard>
  );
}
