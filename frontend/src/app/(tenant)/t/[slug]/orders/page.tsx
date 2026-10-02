"use client";

import { useParams } from "next/navigation";

import { OrdersListPage } from "@/features/orders";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <OrdersListPage slug={String(params.slug ?? "")} />;
}
