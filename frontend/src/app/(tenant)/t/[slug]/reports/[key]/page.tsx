"use client";

import { useParams } from "next/navigation";

import { ReportDetailPage } from "@/features/reports";

export default function Page() {
  const params = useParams<{ slug: string; key: string }>();
  return (
    <ReportDetailPage
      slug={String(params.slug ?? "")}
      reportKey={decodeURIComponent(String(params.key ?? ""))}
    />
  );
}
