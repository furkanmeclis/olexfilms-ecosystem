"use client";

import { useParams } from "next/navigation";

import { WarehouseSettingsPage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <WarehouseSettingsPage slug={String(params.slug ?? "")} />;
}
