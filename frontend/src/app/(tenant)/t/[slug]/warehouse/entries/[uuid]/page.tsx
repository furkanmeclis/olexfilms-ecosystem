"use client";

import { useParams } from "next/navigation";

import { StockEntryDetailPage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <StockEntryDetailPage
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
