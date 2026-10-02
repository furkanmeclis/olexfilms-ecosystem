"use client";

import { useParams } from "next/navigation";

import { CustomerFormPage } from "@/features/customers";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <CustomerFormPage slug={String(params.slug ?? "")} />;
}
