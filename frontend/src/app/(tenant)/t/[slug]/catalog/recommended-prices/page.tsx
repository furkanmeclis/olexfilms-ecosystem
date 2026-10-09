"use client";

import { useParams } from "next/navigation";

import { RecommendedPricesPage } from "@/features/pricing";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <RecommendedPricesPage slug={String(params.slug ?? "")} />;
}
