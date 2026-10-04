"use client";

import { useParams } from "next/navigation";

import { LeadFormPage } from "@/features/leads";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <LeadFormPage slug={String(params.slug ?? "")} />;
}
