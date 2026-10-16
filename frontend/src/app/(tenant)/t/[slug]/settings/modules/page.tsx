"use client";

import { useParams } from "next/navigation";

import { FeaturesPage } from "@/features/modules";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <FeaturesPage slug={String(params.slug ?? "")} />;
}
