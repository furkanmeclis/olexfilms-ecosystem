"use client";

import { useParams } from "next/navigation";

import { CustomersListPage } from "@/features/customers";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <CustomersListPage slug={String(params.slug ?? "")} />;
}
