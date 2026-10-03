"use client";

import { useParams } from "next/navigation";

import { LocationTreePage } from "@/features/warehouse";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <LocationTreePage slug={String(params.slug ?? "")} />;
}
