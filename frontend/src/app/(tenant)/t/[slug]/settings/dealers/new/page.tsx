"use client";

import { useParams } from "next/navigation";

import { DealerCreatePage } from "@/features/accounts";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <DealerCreatePage slug={String(params.slug ?? "")} />;
}
