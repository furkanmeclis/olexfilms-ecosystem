"use client";

import { useParams } from "next/navigation";

import { EinvoiceSettingsPage } from "@/features/einvoice";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <EinvoiceSettingsPage slug={String(params.slug ?? "")} />;
}
