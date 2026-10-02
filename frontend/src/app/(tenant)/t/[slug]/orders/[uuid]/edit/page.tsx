"use client";

import { useParams } from "next/navigation";

import { OrderFormPage } from "@/features/orders";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <OrderFormPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
