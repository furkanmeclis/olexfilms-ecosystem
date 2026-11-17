"use client";

import { useParams, useSearchParams } from "next/navigation";

import { OrderFormPage } from "@/features/orders";

export default function Page() {
  const params = useParams<{ slug: string }>();
  // TEC-1222: ?supplement_of=<uuid> opens a supplementary order.
  const search = useSearchParams();
  return (
    <OrderFormPage
      slug={String(params.slug ?? "")}
      supplementOf={search.get("supplement_of") ?? undefined}
    />
  );
}
