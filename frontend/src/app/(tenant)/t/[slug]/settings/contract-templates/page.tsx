"use client";

import { useParams } from "next/navigation";

import { ContractTemplatesPage } from "@/features/contracts";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <ContractTemplatesPage slug={String(params.slug ?? "")} />;
}
