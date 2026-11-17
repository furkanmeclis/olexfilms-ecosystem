"use client";

import { useParams } from "next/navigation";

import { WarehouseDetailPage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <WarehouseDetailPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
