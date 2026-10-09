"use client";

import { useParams } from "next/navigation";

import { PriceDisciplinePage } from "@/features/pricing";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <PriceDisciplinePage slug={String(params.slug ?? "")} />;
}
