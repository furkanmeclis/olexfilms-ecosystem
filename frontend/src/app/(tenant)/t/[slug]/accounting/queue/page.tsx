"use client";

import { useParams } from "next/navigation";

import { AccountingQueuePage } from "@/features/accounting/components/accounting-queue-page";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <AccountingQueuePage slug={String(params.slug ?? "")} />;
}
