"use client";

import { useParams } from "next/navigation";

import { ScanPage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <ScanPage slug={String(params.slug ?? "")} />;
}
