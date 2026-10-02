"use client";

import { useParams } from "next/navigation";

import { TransferDetailPage } from "@/features/transfers";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <TransferDetailPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
