"use client";

import { useParams } from "next/navigation";

import { StatementPage } from "@/features/accounting";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <StatementPage
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
