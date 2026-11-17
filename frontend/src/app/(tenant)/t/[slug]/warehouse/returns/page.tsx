"use client";

import { useParams } from "next/navigation";

import { ReturnInspectionsPage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <ReturnInspectionsPage slug={String(params.slug ?? "")} />;
}
