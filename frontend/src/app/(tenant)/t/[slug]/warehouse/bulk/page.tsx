"use client";

import { useParams } from "next/navigation";

import { StockBulkPage } from "@/features/stock-bulk";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <StockBulkPage slug={String(params.slug ?? "")} />;
}
