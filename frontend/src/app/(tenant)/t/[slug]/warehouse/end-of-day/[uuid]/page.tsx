"use client";

import { useParams } from "next/navigation";

import { EodReportDetailPage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <EodReportDetailPage
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
