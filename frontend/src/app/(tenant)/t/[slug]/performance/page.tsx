"use client";

import { useParams } from "next/navigation";

import { PerformancePage } from "@/features/performance";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <PerformancePage slug={String(params.slug ?? "")} />;
}
