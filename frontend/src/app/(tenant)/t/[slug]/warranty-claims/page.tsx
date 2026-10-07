"use client";

import { useParams } from "next/navigation";

import { WarrantyClaimsListPage } from "@/features/warranty-claims";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <WarrantyClaimsListPage slug={String(params.slug ?? "")} />;
}
