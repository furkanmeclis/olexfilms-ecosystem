"use client";

import { useParams } from "next/navigation";

import { WarrantyDetailPage } from "@/features/warranty";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <WarrantyDetailPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
