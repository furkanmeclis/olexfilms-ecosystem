"use client";

import type { ReactNode } from "react";
import { useParams } from "next/navigation";

import { FeatureGuard } from "@/features/modules";

/** The dealer sales pages need the dealer_accounting module (TEC-348). */
export default function DealerSalesLayout({
  children,
}: {
  children: ReactNode;
}) {
  const params = useParams<{ slug: string }>();
  return (
    <FeatureGuard slug={String(params.slug ?? "")} feature="dealer_accounting">
      {children}
    </FeatureGuard>
  );
}
