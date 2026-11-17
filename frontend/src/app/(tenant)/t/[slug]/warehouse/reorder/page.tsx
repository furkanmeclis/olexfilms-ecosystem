"use client";

import { useParams } from "next/navigation";

import { ReorderRulesPage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <ReorderRulesPage slug={String(params.slug ?? "")} />;
}
