"use client";

import { useParams } from "next/navigation";

import { AdjustmentDetailPage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <AdjustmentDetailPage
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
