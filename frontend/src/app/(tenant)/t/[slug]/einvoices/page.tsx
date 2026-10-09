"use client";

import { useParams } from "next/navigation";

import { InvoicesPage } from "@/features/einvoice";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <InvoicesPage slug={String(params.slug ?? "")} />;
}
