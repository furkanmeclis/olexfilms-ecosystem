"use client";

import { useParams } from "next/navigation";

import { VehiclesListPage } from "@/features/vehicles";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <VehiclesListPage slug={String(params.slug ?? "")} />;
}
