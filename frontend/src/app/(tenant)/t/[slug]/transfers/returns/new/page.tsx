"use client";

import { useParams, useSearchParams } from "next/navigation";

import { TransferFormPage } from "@/features/transfers";

export default function Page() {
  const params = useParams<{ slug: string }>();
  // TEC-1085: ?from=<child org uuid>&order=<order uuid> preselect a return
  // opened for a child and the order it refers to (order page action).
  const search = useSearchParams();
  return (
    <TransferFormPage
      slug={String(params.slug ?? "")}
      kind="return"
      initialFromOrg={search.get("from") ?? ""}
      initialOrder={search.get("order") ?? ""}
    />
  );
}
