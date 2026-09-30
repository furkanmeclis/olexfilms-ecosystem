"use client";

import { ImportDetailPage } from "@/features/io/components/import-detail-page";
import { useParams } from "next/navigation";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <ImportDetailPage
      uuid={String(params.uuid ?? "")}
      scope="tenant"
      slug={String(params.slug ?? "")}
    />
  );
}
