"use client";

import { useParams } from "next/navigation";

import { SalePricesPage } from "@/features/dealer-sales";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <SalePricesPage slug={String(params.slug ?? "")} />;
}
