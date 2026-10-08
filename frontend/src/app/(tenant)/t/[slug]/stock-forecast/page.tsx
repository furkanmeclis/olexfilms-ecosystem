"use client";

import { useParams } from "next/navigation";

import { StockForecastPage } from "@/features/stock-forecast";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <StockForecastPage slug={String(params.slug ?? "")} />;
}
