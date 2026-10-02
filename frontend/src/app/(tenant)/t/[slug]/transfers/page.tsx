"use client";

import { useParams } from "next/navigation";

import { TransfersListPage } from "@/features/transfers";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <TransfersListPage slug={String(params.slug ?? "")} />;
}
