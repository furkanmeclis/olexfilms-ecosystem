"use client";

import { useParams, useSearchParams } from "next/navigation";

import { MyStockPage } from "@/features/stock";

export default function Page() {
  const params = useParams<{ slug: string }>();
  // TEC-213: the command palette opens a unit (?barcode=) or an
  // organization's stock (?organization=); TEC-1243: ?remnant=true lists
  // the remnants ("ragle").
  const search = useSearchParams();
  return (
    <MyStockPage
      slug={String(params.slug ?? "")}
      initialBarcode={search?.get("barcode") ?? undefined}
      initialRemnant={search?.get("remnant") === "true"}
      initialOrganization={search?.get("organization") ?? undefined}
    />
  );
}
