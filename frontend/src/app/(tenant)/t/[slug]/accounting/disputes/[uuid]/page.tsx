"use client";

import { useParams } from "next/navigation";

import { DisputeDetailPage } from "@/features/accounting";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <DisputeDetailPage
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
