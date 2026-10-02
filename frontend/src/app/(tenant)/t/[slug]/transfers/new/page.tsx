"use client";

import { useParams } from "next/navigation";

import { TransferFormPage } from "@/features/transfers";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <TransferFormPage slug={String(params.slug ?? "")} />;
}
