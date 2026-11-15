"use client";

import { useParams } from "next/navigation";

import { ReportsCenterPage } from "@/features/reports";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <ReportsCenterPage slug={String(params.slug ?? "")} />;
}
