"use client";

import { useParams } from "next/navigation";

import { TransfersPage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <TransfersPage slug={String(params.slug ?? "")} />;
}
