"use client";

import { useParams } from "next/navigation";

import { CariPage } from "@/features/accounting";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <CariPage slug={String(params.slug ?? "")} />;
}
