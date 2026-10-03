"use client";

import { useParams } from "next/navigation";

import { EodReportsPage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <EodReportsPage slug={String(params.slug ?? "")} />;
}
