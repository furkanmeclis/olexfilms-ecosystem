"use client";

import { useParams } from "next/navigation";

import { AccountsPage } from "@/features/accounting";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <AccountsPage slug={String(params.slug ?? "")} />;
}
