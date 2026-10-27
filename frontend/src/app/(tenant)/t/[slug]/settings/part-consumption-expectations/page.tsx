"use client";

import { useParams } from "next/navigation";

import { PartConsumptionExpectationsPage } from "@/features/efficiency";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <PartConsumptionExpectationsPage slug={String(params.slug ?? "")} />;
}
