"use client";

import { useParams } from "next/navigation";

import { WarehouseHomePage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <WarehouseHomePage slug={String(params.slug ?? "")} />;
}
