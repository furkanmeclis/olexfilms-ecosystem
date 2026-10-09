"use client";

import { useParams } from "next/navigation";

import { BillablePage } from "@/features/einvoice";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <BillablePage slug={String(params.slug ?? "")} />;
}
