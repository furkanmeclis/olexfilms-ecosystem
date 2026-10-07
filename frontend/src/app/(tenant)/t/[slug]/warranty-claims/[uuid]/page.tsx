"use client";

import { useParams } from "next/navigation";

import { WarrantyClaimDetailPage } from "@/features/warranty-claims";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <WarrantyClaimDetailPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
