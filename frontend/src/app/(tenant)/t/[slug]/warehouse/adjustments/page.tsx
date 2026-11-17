"use client";

import { useParams } from "next/navigation";

import { AdjustmentsPage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <AdjustmentsPage slug={String(params.slug ?? "")} />;
}
