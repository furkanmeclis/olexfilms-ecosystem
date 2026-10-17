"use client";

import { useParams } from "next/navigation";

import { LabelTemplatesPage } from "@/features/stock";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <LabelTemplatesPage slug={String(params.slug ?? "")} />;
}
