"use client";

import { useParams } from "next/navigation";

import { LeadsListPage } from "@/features/leads";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <LeadsListPage slug={String(params.slug ?? "")} />;
}
