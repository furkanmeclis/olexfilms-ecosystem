"use client";

import { useParams } from "next/navigation";

import { DeliveryMethodsPage } from "@/features/orders";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <DeliveryMethodsPage slug={String(params.slug ?? "")} />;
}
