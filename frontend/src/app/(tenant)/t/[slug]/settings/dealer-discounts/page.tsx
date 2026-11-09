"use client";

import { useParams } from "next/navigation";

import { DealerDiscountsPage } from "@/features/orders";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <DealerDiscountsPage slug={String(params.slug ?? "")} />;
}
