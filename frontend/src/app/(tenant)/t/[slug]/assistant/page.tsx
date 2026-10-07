"use client";

import { useParams } from "next/navigation";

import { PanelAssistantPage } from "@/features/ai-assistant";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <PanelAssistantPage slug={String(params.slug ?? "")} />;
}
