"use client";

import { useParams } from "next/navigation";

import { UnitHistoryPage } from "@/features/stock";

export default function Page() {
  const params = useParams<{ slug: string; barcode: string }>();
  return (
    <UnitHistoryPage
      slug={String(params.slug ?? "")}
      barcode={decodeURIComponent(String(params.barcode ?? ""))}
    />
  );
}
