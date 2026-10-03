"use client";

import { useParams } from "next/navigation";

import { StockEntriesPage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <StockEntriesPage slug={String(params.slug ?? "")} />;
}
