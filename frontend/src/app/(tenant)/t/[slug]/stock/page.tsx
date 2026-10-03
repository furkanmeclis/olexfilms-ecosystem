"use client";

import { useParams, useSearchParams } from "next/navigation";

import { MyStockPage } from "@/features/stock";

export default function Page() {
  const params = useParams<{ slug: string }>();
  // TEC-213: the command palette opens a unit (?barcode=) or an
  // organization's stock (?organization=).
  const search = useSearchParams();
  return (
    <MyStockPage
      slug={String(params.slug ?? "")}
      initialBarcode={search?.get("barcode") ?? undefined}
      initialOrganization={search?.get("organization") ?? undefined}
    />
  );
}
