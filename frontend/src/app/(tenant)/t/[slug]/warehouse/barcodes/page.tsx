"use client";

import { useParams } from "next/navigation";

import { BarcodesPage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <BarcodesPage slug={String(params.slug ?? "")} />;
}
