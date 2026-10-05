"use client";

import { useParams } from "next/navigation";

import { WarrantyClaimReportsPage } from "@/features/warranty-claims";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <WarrantyClaimReportsPage slug={String(params.slug ?? "")} />;
}
