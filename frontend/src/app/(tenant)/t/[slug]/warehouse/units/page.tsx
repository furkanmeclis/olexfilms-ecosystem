"use client";

import { useParams } from "next/navigation";

import { WarehouseUnitsPage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <WarehouseUnitsPage slug={String(params.slug ?? "")} />;
}
