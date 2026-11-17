"use client";

import { useParams } from "next/navigation";

import { StockMovementsPage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <StockMovementsPage slug={String(params.slug ?? "")} />;
}
