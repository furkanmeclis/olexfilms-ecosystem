"use client";

import { useParams } from "next/navigation";

import { MyStockPage } from "@/features/stock";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <MyStockPage slug={String(params.slug ?? "")} />;
}
