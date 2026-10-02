"use client";

import { useParams } from "next/navigation";

import { VehicleDetailPage } from "@/features/vehicles";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <VehicleDetailPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
