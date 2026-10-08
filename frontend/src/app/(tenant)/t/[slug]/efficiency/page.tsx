"use client";

import { useParams } from "next/navigation";

import { EfficiencyPage } from "@/features/efficiency";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <EfficiencyPage slug={String(params.slug ?? "")} />;
}
