"use client";

import { useParams } from "next/navigation";

import { CountDetailPage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <CountDetailPage
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
