"use client";

import { useParams } from "next/navigation";

import { OrderFormPage } from "@/features/orders";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <OrderFormPage slug={String(params.slug ?? "")} />;
}
