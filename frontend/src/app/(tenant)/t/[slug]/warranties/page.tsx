"use client";

import { useParams } from "next/navigation";

import { WarrantiesListPage } from "@/features/warranty";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <WarrantiesListPage slug={String(params.slug ?? "")} />;
}
