"use client";

import type { ReactNode } from "react";
import { useParams } from "next/navigation";

import { FeatureGuard } from "@/features/modules";

/** The accounting pages need the accounting module (TEC-176). */
export default function AccountingLayout({
  children,
}: {
  children: ReactNode;
}) {
  const params = useParams<{ slug: string }>();
  return (
    <FeatureGuard slug={String(params.slug ?? "")} feature="accounting">
      {children}
    </FeatureGuard>
  );
}
