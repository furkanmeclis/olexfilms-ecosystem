"use client";

import { useParams } from "next/navigation";

import { AIUsagePage } from "@/features/ai-admin";

export default function Page() {
  return (
    <AIUsagePage slug={String(useParams<{ slug: string }>().slug ?? "")} />
  );
}
