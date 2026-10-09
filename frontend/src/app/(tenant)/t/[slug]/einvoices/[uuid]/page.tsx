"use client";

import { useParams } from "next/navigation";

import { InvoiceDetailPage } from "@/features/einvoice";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <InvoiceDetailPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
