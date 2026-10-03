"use client";

import { useParams } from "next/navigation";

import { CountsPage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <CountsPage slug={String(params.slug ?? "")} />;
}
