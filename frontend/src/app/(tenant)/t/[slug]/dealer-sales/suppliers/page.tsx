"use client";

import { useParams } from "next/navigation";

import { SuppliersPage } from "@/features/dealer-sales";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <SuppliersPage slug={String(params.slug ?? "")} />;
}
