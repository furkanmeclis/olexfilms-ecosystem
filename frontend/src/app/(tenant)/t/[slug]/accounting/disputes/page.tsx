"use client";

import { useParams } from "next/navigation";

import { DisputesPage } from "@/features/accounting";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <DisputesPage slug={String(params.slug ?? "")} />;
}
