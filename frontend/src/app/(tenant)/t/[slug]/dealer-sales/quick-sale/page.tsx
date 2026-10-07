"use client";

import { useParams } from "next/navigation";

import { QuickSalePage } from "@/features/dealer-sales";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <QuickSalePage slug={String(params.slug ?? "")} />;
}
