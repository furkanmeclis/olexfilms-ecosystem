"use client";

import { useParams } from "next/navigation";

import { RecipientPoolPage } from "@/features/accounts";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <RecipientPoolPage slug={String(params.slug ?? "")} />;
}
