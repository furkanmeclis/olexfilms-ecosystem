"use client";

import { useParams } from "next/navigation";

import { ContractDetailPage } from "@/features/contracts";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <ContractDetailPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
