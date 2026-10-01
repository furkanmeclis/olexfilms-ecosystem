"use client";

import { useParams } from "next/navigation";

import { ProductsPage } from "@/features/catalog";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <ProductsPage slug={String(params.slug ?? "")} />;
}
