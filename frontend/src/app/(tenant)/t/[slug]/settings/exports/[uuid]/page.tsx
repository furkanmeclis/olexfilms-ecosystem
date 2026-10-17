"use client";

import { ExportDetailPage } from "@/features/io/components/export-detail-page";
import { useParams } from "next/navigation";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <ExportDetailPage
      uuid={String(params.uuid ?? "")}
      scope="tenant"
      slug={String(params.slug ?? "")}
    />
  );
}
